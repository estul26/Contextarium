package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/estul26/Contextarium/internal/storage"
)

type coldInput struct {
	Path, Operation, RecordID, Key, Before, Expected string
	Body                                             []byte
	Target                                           int64
	Want                                             Snapshot
}
type coldBarrier struct {
	Point    string
	PID      int
	Expected string
}

func coldMutation(s *Service, a context.Context, input coldInput) (MutationResult, error) {
	switch input.Operation {
	case "create":
		return s.CreateRecord(a, input.Key, input.Body)
	case "patch":
		return s.UpdateRecord(a, input.RecordID, input.Key, input.Body)
	case "restore":
		return s.RestoreRecord(a, input.RecordID, input.Target, input.Key, input.Body)
	default:
		return MutationResult{}, ErrInvalid
	}
}
func coldCommand(inputFile, role, point string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestM2ColdRestartProcess$", "-test.timeout=30s")
	cmd.Env = append(os.Environ(), "CONTEXTARIUM_TEST_COLD_ROLE="+role, "CONTEXTARIUM_TEST_COLD_INPUT="+inputFile, "CONTEXTARIUM_TEST_COLD_POINT="+point)
	return cmd
}
func writeColdInput(t *testing.T, path string, input coldInput) {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

// Only this test executable reads these controls. The application has no cold
// test environment variables, clock controls or fault endpoints.
func TestM2ColdRestartProcess(t *testing.T) {
	role := os.Getenv("CONTEXTARIUM_TEST_COLD_ROLE")
	if role == "" {
		return
	}
	raw, err := os.ReadFile(os.Getenv("CONTEXTARIUM_TEST_COLD_INPUT"))
	if err != nil {
		t.Fatal(err)
	}
	var input coldInput
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(ctx, input.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() // The mutation child is killed while blocked; this never runs there.
	s := New(db)
	actionTime := time.Date(2031, 2, 3, 4, 5, 6, 0, time.UTC)
	s.clock = func() time.Time { return actionTime }
	a := WithAttribution(context.Background(), Actor{Kind: "development_test", ID: "cold-mutator"}, "req_00000000000000000000000000000011")
	if role == "mutate" {
		// Deterministically spill uncommitted pages in the F6 case; keep committed
		// pages in WAL in the F8 case. These PRAGMAs apply only to this test-owned
		// connection. WAL/FULL/FK and the application's mutation code are unchanged.
		if _, err := db.Exec("PRAGMA cache_size=1; PRAGMA cache_spill=ON; PRAGMA wal_autocheckpoint=0"); err != nil {
			t.Fatal(err)
		}
		point := os.Getenv("CONTEXTARIUM_TEST_COLD_POINT")
		s.fault = func(p string) error {
			if p != point {
				return nil
			}
			expected := input.Before
			if p == "F8" {
				expected = observe(t, db)
			} // Same child owns SQLite; never a parent observer.
			if err := json.NewEncoder(os.Stdout).Encode(coldBarrier{p, os.Getpid(), expected}); err != nil {
				t.Fatal(err)
			}
			var b [1]byte
			if _, err := os.Stdin.Read(b[:]); err != nil {
				t.Fatal("barrier released without test kill", err)
			}
			t.Fatal("mutation child must be killed at its acknowledged barrier")
			return ErrUnavailable
		}
		if _, err := coldMutation(s, a, input); err != nil {
			t.Fatal(err)
		}
		t.Fatal("mutation returned without reaching the requested barrier")
	}
	if role != "recover" {
		t.Fatal("unknown child role")
	}
	// This is the FIRST SQLite connection after the crash, in a fresh process.
	healthy(t, db)
	if got := observe(t, db); got != input.Expected {
		t.Fatal("cold recovery changed the complete expected four-store state")
	}
	postcommit := input.Expected != input.Before
	if postcommit {
		s.clock = func() time.Time { t.Fatal("committed replay consulted the action clock"); return time.Time{} }
	}
	// A new request actor must not relabel an already committed action.
	a = WithAttribution(context.Background(), Actor{Kind: "development_test", ID: "cold-recovery"}, "req_00000000000000000000000000000012")
	result, err := coldMutation(s, a, input)
	r := decode[Record](t, result, err)
	want := input.Want
	if input.Operation == "create" {
		want.ID = r.ID
		want.CreatedAt = actionTime.Format(time.RFC3339Nano)
	}
	want.UpdatedAt = actionTime.Format(time.RFC3339Nano)
	if !reflect.DeepEqual(r.Snapshot, want) {
		t.Fatal("recovered/retried content differs from intended mutation")
	}
	expectedRevision := int64(4)
	if input.Operation == "create" {
		expectedRevision = 1
	}
	if r.Revision != expectedRevision || result.Contract != "m2" {
		t.Fatal("duplicate or missing revision")
	}
	v, err := s.GetRevision(ctx, r.ID, r.Revision)
	if err != nil {
		t.Fatal(err)
	}
	wantActor := "cold-recovery"
	if postcommit {
		wantActor = "cold-mutator"
	}
	if v.Actor.ID != wantActor || v.RecordedAt != r.UpdatedAt || !reflect.DeepEqual(v.Snapshot, r.Snapshot) {
		t.Fatal("recovery attribution/snapshot mismatch")
	}
	var auditTime, action string
	if err := db.QueryRow("SELECT timestamp,action FROM mutation_audit WHERE event_id=?", v.AuditEventID).Scan(&auditTime, &action); err != nil {
		t.Fatal(err)
	}
	wantAction := map[string]string{"create": "record.created", "patch": "record.archived", "restore": "revision.restored"}[input.Operation]
	if auditTime != r.UpdatedAt || action != wantAction {
		t.Fatal("recovered required audit mismatch")
	}
	after := observe(t, db)
	if postcommit && after != input.Expected {
		t.Fatal("postcommit retry changed any store")
	}
	assertColdMutationDelta(t, input.Before, after, input.Operation, input.RecordID)
	s.clock = func() time.Time { t.Fatal("same-key replay consulted clock"); return time.Time{} }
	replay, err := coldMutation(s, a, input)
	if err != nil || string(replay.Data) != string(result.Data) || observe(t, db) != after {
		t.Fatal("same-key replay changed result or stores", err)
	}
	healthy(t, db)
	fmt.Println("COLD_RECOVERY_OK")
}

// Verify one accepted delta while retaining every earlier history/audit/replay
// row byte-for-byte; only the intended current record can change on PATCH/restore.
func assertColdMutationDelta(t *testing.T, before, after, operation, id string) {
	t.Helper()
	var old, new [][][]json.RawMessage
	if json.Unmarshal([]byte(before), &old) != nil || json.Unmarshal([]byte(after), &new) != nil || len(old) != 4 || len(new) != 4 {
		t.Fatal("invalid oracle snapshot")
	}
	for table := range old {
		added := 1
		if table == 0 && operation != "create" {
			added = 0
		}
		if len(new[table]) != len(old[table])+added {
			t.Fatal("wrong committed row delta", table)
		}
		existing := map[string]bool{}
		for _, row := range new[table] {
			raw, _ := json.Marshal(row)
			existing[string(raw)] = true
		}
		for _, row := range old[table] {
			if table == 0 && operation != "create" {
				var recordID string
				json.Unmarshal(row[0], &recordID)
				if recordID == id {
					continue
				}
			}
			raw, _ := json.Marshal(row)
			if !existing[string(raw)] {
				t.Fatal("recovery changed an unrelated or immutable row", table)
			}
		}
	}
}

// Inspect WAL bytes without opening SQLite. Validate the complete frame layout,
// header salts and commit markers. This is process-crash evidence, not simulated
// power failure or a substitute for SQLite's recovery/integrity checks.
func inspectColdWAL(t *testing.T, wal []byte, committed bool) {
	t.Helper()
	if len(wal) < 32 {
		t.Fatal("crash WAL header absent")
	}
	magic := binary.BigEndian.Uint32(wal[:4])
	if magic != 0x377f0682 && magic != 0x377f0683 {
		t.Fatal("invalid WAL magic")
	}
	if binary.BigEndian.Uint32(wal[4:8]) != 3007000 {
		t.Fatal("unexpected WAL format")
	}
	page := int(binary.BigEndian.Uint32(wal[8:12]))
	if page < 512 || page > 65536 || page&(page-1) != 0 {
		t.Fatal("invalid WAL page size")
	}
	frameSize := 24 + page
	if (len(wal)-32)%frameSize != 0 {
		t.Fatal("incomplete WAL frame at acknowledged barrier")
	}
	frames := (len(wal) - 32) / frameSize
	if frames == 0 {
		t.Fatal("no spilled mutation frames in crash WAL")
	}
	// Match bundled SQLite walChecksumBytes/walEncodeFrame. Frames rewritten
	// during spilling may defer salts/checksums (16 zero bytes) until commit.
	var order binary.ByteOrder = binary.LittleEndian
	if magic&1 != 0 {
		order = binary.BigEndian
	}
	checksum := func(raw []byte, sum [2]uint32) [2]uint32 {
		for pos := 0; pos < len(raw); pos += 8 {
			sum[0] += order.Uint32(raw[pos:pos+4]) + sum[1]
			sum[1] += order.Uint32(raw[pos+4:pos+8]) + sum[0]
		}
		return sum
	}
	sum := checksum(wal[:24], [2]uint32{})
	if sum[0] != binary.BigEndian.Uint32(wal[24:28]) || sum[1] != binary.BigEndian.Uint32(wal[28:32]) {
		t.Fatal("invalid WAL header checksum")
	}
	commits, pending := 0, 0
	for pos := 32; pos < len(wal); pos += frameSize {
		frame := wal[pos : pos+24]
		if binary.BigEndian.Uint32(frame[:4]) == 0 {
			t.Fatal("invalid WAL page number")
		}
		if !bytes.Equal(frame[8:16], wal[16:24]) {
			if committed || !bytes.Equal(frame[8:24], make([]byte, 16)) {
				t.Fatal("unexpected WAL salt/checksum state")
			}
			pending++
		}
		if committed {
			sum = checksum(frame[:8], sum)
			sum = checksum(wal[pos+24:pos+frameSize], sum)
			if sum[0] != binary.BigEndian.Uint32(frame[16:20]) || sum[1] != binary.BigEndian.Uint32(frame[20:24]) {
				t.Fatal("invalid committed WAL checksum")
			}
		}
		if binary.BigEndian.Uint32(frame[4:8]) != 0 {
			commits++
		}
	}
	if committed {
		if commits < 1 || binary.BigEndian.Uint32(wal[len(wal)-frameSize+4:len(wal)-frameSize+8]) == 0 {
			t.Fatal("postcommit WAL lacks its final commit marker")
		}
	} else if commits != 0 {
		t.Fatal("precommit WAL contains a commit marker")
	}
	t.Logf("R2 cold WAL before recovery: frames=%d commit_markers=%d pending_checksum_headers=%d page_size=%d", frames, commits, pending, page)
}

func TestM2ColdRestartRecovery(t *testing.T) {
	for _, operation := range []string{"create", "patch", "restore"} {
		for _, point := range []string{"F6", "F8"} {
			t.Run(operation+"/"+point, func(t *testing.T) {
				s, db, path, r := revisionFixture(t)
				data := func(fill string) string {
					return `{"exact":9007199254740993,"decimal":0.30,"payload":"` + strings.Repeat(fill, 50<<10) + `"}`
				}
				r = update(t, s, r, "large-history", `"data":`+data("a"))
				target := r
				r = update(t, s, r, "large-current", `"data":`+data("b"))
				input := coldInput{Path: path, Operation: operation, RecordID: r.ID, Key: "cold-crash", Target: 2, Before: observe(t, db), Want: r.Snapshot}
				switch operation {
				case "create":
					input.Body = recordBody(r.SubjectID, r.Namespace, r.SchemaID, string(r.Data))
				case "patch":
					input.Body = []byte(`{"base_revision":3,"data":` + data("c") + `,"status":"archived"}`)
					input.Want.Data = json.RawMessage(data("c"))
					input.Want.Status = "archived"
				case "restore":
					input.Body = []byte(`{"base_revision":3}`)
					input.Want = target.Snapshot
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				} // No setup/parent connection survives.
				if info, err := os.Stat(path + "-wal"); err == nil && info.Size() != 0 {
					t.Fatal("setup left WAL content")
				} else if err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				originalDB, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				inputFile := filepath.Join(t.TempDir(), "synthetic-input.json")
				writeColdInput(t, inputFile, input)
				cmd := coldCommand(inputFile, "mutate", point)
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				out, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				in, err := cmd.StdinPipe()
				if err != nil {
					t.Fatal(err)
				}
				defer in.Close()
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if cmd.ProcessState == nil {
						cmd.Process.Kill()
						cmd.Wait()
					}
				}()
				type reply struct {
					barrier coldBarrier
					err     error
				}
				ready := make(chan reply, 1)
				go func() { var b coldBarrier; err := json.NewDecoder(out).Decode(&b); ready <- reply{b, err} }()
				var barrier coldBarrier
				select {
				case response := <-ready:
					if response.err != nil {
						t.Fatal("child did not acknowledge barrier", response.err)
					}
					barrier = response.barrier
				case <-time.After(15 * time.Second):
					t.Fatal("cold mutation barrier timed out")
				}
				if barrier.Point != point || barrier.PID != cmd.Process.Pid {
					t.Fatal("wrong child/barrier acknowledgement")
				}
				if (point == "F6") != (barrier.Expected == input.Before) {
					t.Fatal("wrong expected commit outcome")
				}
				beforeKill, err := os.ReadFile(path + "-wal")
				if err != nil {
					t.Fatal(err)
				}
				inspectColdWAL(t, beforeKill, point == "F8")
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				exitErr, ok := cmd.Wait().(*exec.ExitError)
				if !ok {
					t.Fatal("child was not abruptly terminated")
				}
				status, ok := exitErr.Sys().(syscall.WaitStatus)
				if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
					t.Fatal("test-owned child did not exit from SIGKILL")
				}
				// Between this kill and the recovery child, ONLY raw filesystem reads occur.
				// No SQLite open, clean close, checkpoint, diagnostic connection or WAL copy.
				crashWAL, err := os.ReadFile(path + "-wal")
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(crashWAL, beforeKill) {
					t.Fatal("crash-left WAL changed before recovery")
				}
				crashDB, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(crashDB, originalDB) {
					t.Fatal("mutation checkpointed into main DB before cold recovery")
				}
				if _, err := os.Stat(path + "-shm"); err != nil {
					t.Fatal("crash-left shared-memory sidecar absent", err)
				}
				inspectColdWAL(t, crashWAL, point == "F8")
				input.Expected = barrier.Expected
				writeColdInput(t, inputFile, input)
				recovered, err := coldCommand(inputFile, "recover", point).CombinedOutput()
				if err != nil || !strings.Contains(string(recovered), "COLD_RECOVERY_OK") {
					t.Fatalf("fresh-process recovery failed: %v\n%s", err, recovered)
				}
			})
		}
	}
}
