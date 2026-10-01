package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/estul26/Contextarium/internal/engine"
)

func TestM2HTTPContractAndSafety(t *testing.T) {
	s, db := newTestServer(t)
	sub := responseData[engine.Subject](t, request(s, "POST", "/api/v1/subjects", "subject", `{"kind":"project","display_name":"Synthetic M2"}`, nil), 201)
	responseData[engine.Schema](t, request(s, "POST", "/api/v1/schemas", "schema", apiSchema, nil), 201)
	body := fmt.Sprintf(`{"subject_id":%q,"namespace":"example.task","schema_id":"example.task","schema_version":1,"data":{"title":"Example","done":false}}`, sub.ID)
	headers := map[string]string{"X-Actor-ID": "forged", "X-Client-ID": "forged", "X-Request-ID": "forged", "Authorization": "Bearer synthetic-not-a-credential"}
	created := request(s, "POST", "/api/v1/records", "create", body, headers)
	r := responseData[engine.Record](t, created, 201)
	if !strings.Contains(created.Body.String(), `"result_contract":"m2"`) || r.Revision != 1 {
		t.Fatal("missing M2 metadata")
	}
	path := "/api/v1/records/" + r.ID
	v := responseData[engine.Revision](t, request(s, "GET", path+"/revisions/1", "", "", nil), 200)
	if v.Actor.ID != "local-http-adapter" || v.Actor.Kind != "development_test" || v.RequestID != created.Header().Get("X-Request-ID") {
		t.Fatal("forged attribution")
	}
	for _, tc := range []struct {
		body, code string
		status     int
	}{{`{"status":"active"}`, "BASE_REVISION_REQUIRED", 400}, {`{"base_revision":1.0,"status":"active"}`, "INVALID_ARGUMENT", 400}, {`{"base_revision":2,"status":"active"}`, "REVISION_CONFLICT", 409}} {
		w := request(s, "PATCH", path, "precondition", tc.body, nil)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := request(s, "PATCH", path, "if-match", `{"base_revision":1,"status":"active"}`, map[string]string{"If-Match": "1"}); w.Code != 400 {
		t.Fatal(w.Code)
	}
	updated := responseData[engine.Record](t, request(s, "PATCH", path, "update", `{"base_revision":1,"status":"archived"}`, nil), 200)
	restored := responseData[engine.Record](t, request(s, "POST", path+"/restore/1", "restore", `{"base_revision":2}`, nil), 200)
	if updated.Revision != 2 || restored.Revision != 3 || restored.Status != "active" {
		t.Fatal("HTTP restore")
	}
	replay := request(s, "POST", "/api/v1/records", "create", body, nil)
	old := responseData[engine.Record](t, replay, 201)
	if old.Revision != 1 || replay.Header().Get("X-Request-ID") == created.Header().Get("X-Request-ID") {
		t.Fatal("replay request correlation")
	}
	vs := responseData[[]engine.Revision](t, request(s, "GET", path+"/revisions?limit=2", "", "", nil), 200)
	if len(vs) != 2 || vs[0].Number != 1 || vs[1].Number != 2 {
		t.Fatal("history list")
	}
	for _, p := range []string{path + "/revisions", path + "/revisions/1", path + "/restore/1"} {
		method := "GET"
		b := ""
		if strings.Contains(p, "restore") {
			method = "POST"
			b = `{"base_revision":3}`
		}
		for _, h := range []map[string]string{{"Origin": "https://hostile.example"}, {"Sec-Fetch-Site": "cross-site"}} {
			if w := request(s, method, p, "security", b, h); w.Code != 403 {
				t.Fatal("origin bypass", p, w.Code)
			}
		}
		req := httptest.NewRequest(method, p, strings.NewReader(b))
		req.Host = "hostile.example"
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatal("Host bypass")
		}
	}
	for _, p := range []string{path + "/revisions", path + "/revisions/1"} {
		for _, method := range []string{"PATCH", "DELETE", "POST"} {
			w := request(s, method, p, "immutable", `{}`, nil)
			if w.Code != 405 || w.Header().Get("Allow") != "GET" {
				t.Fatal("mutable history route", w.Code)
			}
		}
	}
	for _, p := range []string{"/api/v1/audit", "/api/v1/permissions", "/api/v1/proposals", "/api/v1/search", "/mcp"} {
		if w := request(s, "GET", p, "", "", nil); w.Code != 404 {
			t.Fatal("out-of-scope API", p, w.Code)
		}
	}
	for _, suffix := range []string{"?limit=101", "?limit=0", "?limit=2&limit=3", "?status=archived", "?cursor=bad"} {
		if w := request(s, "GET", path+"/revisions"+suffix, "", "", nil); w.Code != 400 {
			t.Fatal("bad revision query", w.Code)
		}
	}
	for _, target := range []string{"0", "01", "1.0", "1e0", "9007199254740992"} {
		if w := request(s, "GET", path+"/revisions/"+target, "", "", nil); w.Code != 400 {
			t.Fatal("bad number", w.Code)
		}
	}
	if w := request(s, "POST", path+"/restore/1", "large", strings.Repeat(" ", engine.MaxBody+1), nil); w.Code != 413 {
		t.Fatal(w.Code)
	}
	if w := request(s, "POST", path+"/restore/1", "media", `{}`, map[string]string{"Content-Type": "text/plain"}); w.Code != 415 {
		t.Fatal(w.Code)
	}
	var actor, kind string
	var count int
	if err := db.QueryRow("SELECT actor_id,attribution_kind,count(*) FROM mutation_audit").Scan(&actor, &kind, &count); err != nil || actor != "local-http-adapter" || kind != "development_test" || count != 3 {
		t.Fatal("audit attribution/count", err)
	}
	var envelope map[string]any
	json.Unmarshal(replay.Body.Bytes(), &envelope)
	if envelope["meta"].(map[string]any)["result_contract"] != "m2" {
		t.Fatal("replay contract")
	}
}

type failedResponse struct {
	header    http.Header
	remaining int
}

func (w *failedResponse) Header() http.Header { return w.header }
func (w *failedResponse) WriteHeader(int)     {}
func (w *failedResponse) Write(p []byte) (int, error) {
	n := len(p)
	if n > w.remaining {
		n = w.remaining
	}
	w.remaining -= n
	return n, io.ErrClosedPipe
}
func TestM2CommittedResponseLossReplay(t *testing.T) {
	s, db := newTestServer(t)
	sub := responseData[engine.Subject](t, request(s, "POST", "/api/v1/subjects", "subject", `{"kind":"project","display_name":"Synthetic"}`, nil), 201)
	responseData[engine.Schema](t, request(s, "POST", "/api/v1/schemas", "schema", apiSchema, nil), 201)
	body := fmt.Sprintf(`{"subject_id":%q,"namespace":"example.task","schema_id":"example.task","schema_version":1,"data":{"title":"Example","done":false}}`, sub.ID)
	for i, cut := range []int{0, 31} {
		req := httptest.NewRequest("POST", "/api/v1/records", strings.NewReader(body))
		req.Host = "localhost"
		req.Header.Set("Content-Type", "application/json")
		key := fmt.Sprintf("lost-%d", i)
		req.Header.Set("Idempotency-Key", key)
		s.http.Handler.ServeHTTP(&failedResponse{header: make(http.Header), remaining: cut}, req)
		replay := request(s, "POST", "/api/v1/records", key, body, nil)
		r := responseData[engine.Record](t, replay, 201)
		if r.Revision != 1 {
			t.Fatal("response loss duplicated mutation")
		}
	}
	for _, table := range []string{"records", "record_revisions", "mutation_audit"} {
		var count int
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 2 {
			t.Fatal("duplicate effect", table, count, err)
		}
	}
}

type commitBarrierWriter struct {
	http.ResponseWriter
	ctx              context.Context
	entered, release chan struct{}
}

func (w *commitBarrierWriter) WriteHeader(status int) {
	if status == 200 {
		close(w.entered)
		select {
		case <-w.release:
		case <-w.ctx.Done():
		}
	}
	w.ResponseWriter.WriteHeader(status)
}
func TestM2ShutdownWithCommittedMutation(t *testing.T) {
	for _, forced := range []bool{false, true} {
		t.Run(fmt.Sprint(forced), func(t *testing.T) {
			s, db := newTestServer(t)
			sub := responseData[engine.Subject](t, request(s, "POST", "/api/v1/subjects", "subject", `{"kind":"project","display_name":"Synthetic"}`, nil), 201)
			responseData[engine.Schema](t, request(s, "POST", "/api/v1/schemas", "schema", apiSchema, nil), 201)
			body := fmt.Sprintf(`{"subject_id":%q,"namespace":"example.task","schema_id":"example.task","schema_version":1,"data":{"title":"Example","done":false}}`, sub.ID)
			record := responseData[engine.Record](t, request(s, "POST", "/api/v1/records", "create", body, nil), 201)
			path := "/api/v1/records/" + record.ID
			patch := `{"base_revision":1,"status":"archived"}`
			entered, release := make(chan struct{}), make(chan struct{})
			original := s.http.Handler
			s.http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				original.ServeHTTP(&commitBarrierWriter{w, r.Context(), entered, release}, r)
			})
			if forced {
				s.shutdownTimeout = 20 * time.Millisecond
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- s.Serve(ctx, listener) }()
			client := &http.Client{Timeout: 3 * time.Second}
			defer client.CloseIdleConnections()
			response := make(chan error, 1)
			go func() {
				req, _ := http.NewRequest("PATCH", "http://"+listener.Addr().String()+path, strings.NewReader(patch))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", "shutdown")
				res, err := client.Do(req)
				if res != nil {
					io.Copy(io.Discard, res.Body)
					res.Body.Close()
				}
				response <- err
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("mutation did not commit")
			}
			cancel()
			waitFor(t, func() bool { return !s.ready.Load() })
			if !forced {
				close(release)
			}
			select {
			case err := <-done:
				if forced && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal(err)
				}
				if !forced && err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("drain timed out")
			}
			select {
			case err := <-response:
				if !forced && err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("response did not terminate")
			}
			retry := New(db)
			got := responseData[engine.Record](t, request(retry, "PATCH", path, "shutdown", patch, nil), 200)
			if got.Revision != 2 || got.Status != "archived" {
				t.Fatal("committed shutdown result missing")
			}
			for _, table := range []string{"record_revisions", "mutation_audit"} {
				var n int
				db.QueryRow("SELECT count(*) FROM " + table).Scan(&n)
				if n != 2 {
					t.Fatal("shutdown retry duplicate")
				}
			}
		})
	}
}
