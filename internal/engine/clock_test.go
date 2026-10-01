package engine

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestM2ControlledActionClock(t *testing.T) {
	for _, mode := range []string{"equal", "regressing", "advancing"} {
		t.Run(mode, func(t *testing.T) {
			s, db, _ := fixture(t)
			sub := subject(t, s)
			publish(t, s, "example.clock", 1, `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`)
			start := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
			step := time.Duration(0)
			if mode == "regressing" {
				step = -time.Hour
			} else if mode == "advancing" {
				step = time.Hour
			}
			times := []time.Time{start, start.Add(step), start.Add(2 * step), start.Add(3 * step)}
			calls := 0
			s.clock = func() time.Time {
				if calls >= len(times) {
					t.Fatal("clock read by replay or rejected request")
				}
				v := times[calls]
				calls++
				return v
			}
			actor := func(id string, n int) context.Context {
				return WithAttribution(context.Background(), Actor{Kind: "development_test", ID: id}, fmt.Sprintf("req_%032d", n))
			}
			body := recordBody(sub.ID, "example.clock", "example.clock", `{"n":9007199254740993,"d":0.30}`)
			createdRaw, err := s.CreateRecord(actor("creator", 1), "create", body)
			created := decode[Record](t, createdRaw, err)
			patch := []byte(`{"base_revision":1,"data":{"changed":true},"status":"archived"}`)
			updatedRaw, err := s.UpdateRecord(actor("editor", 2), created.ID, "patch", patch)
			updated := decode[Record](t, updatedRaw, err)
			// A fresh no-op must append even when the action clock is equal or regresses.
			noop := []byte(`{"base_revision":2,"status":"archived"}`)
			noopRaw, err := s.UpdateRecord(actor("editor", 3), created.ID, "noop", noop)
			third := decode[Record](t, noopRaw, err)
			restoreBody := []byte(`{"base_revision":3}`)
			restoredRaw, err := s.RestoreRecord(actor("restorer", 4), created.ID, 1, "restore", restoreBody)
			restored := decode[Record](t, restoredRaw, err)
			records := []Record{created, updated, third, restored}
			for i, r := range records {
				timestamp := times[i].Format(time.RFC3339Nano)
				if r.Revision != int64(i+1) || r.CreatedAt != start.Format(time.RFC3339Nano) || r.UpdatedAt != timestamp {
					t.Fatal("clock affected revision identity/order", r)
				}
				v, err := s.GetRevision(ctx, r.ID, r.Revision)
				if err != nil {
					t.Fatal(err)
				}
				if v.RecordedAt != timestamp || !reflect.DeepEqual(v.Snapshot, r.Snapshot) {
					t.Fatal("snapshot action time mismatch")
				}
				var eventTime, action, actorID, requestID string
				if err := db.QueryRow("SELECT timestamp,action,actor_id,request_id FROM mutation_audit WHERE event_id=?", v.AuditEventID).Scan(&eventTime, &action, &actorID, &requestID); err != nil {
					t.Fatal(err)
				}
				expectedAction := []string{"record.created", "record.archived", "record.updated", "revision.restored"}[i]
				if eventTime != timestamp || action != expectedAction || actorID != v.Actor.ID || requestID != v.RequestID {
					t.Fatal("audit action/time/linkage mismatch")
				}
			}
			historical, err := s.GetRevision(ctx, created.ID, 1)
			if err != nil {
				t.Fatal(err)
			}
			latest, err := s.GetRevision(ctx, created.ID, 4)
			if err != nil {
				t.Fatal(err)
			}
			content := latest.Snapshot
			content.UpdatedAt = historical.Snapshot.UpdatedAt
			if !reflect.DeepEqual(content, historical.Snapshot) || latest.Actor.ID != "restorer" || latest.RequestID == historical.RequestID || latest.AuditEventID == historical.AuditEventID || latest.Base == nil || *latest.Base != 3 || latest.Source == nil || *latest.Source != 1 {
				t.Fatal("restore copied historical attribution or lost content")
			}
			page, err := s.ListRevisions(ctx, created.ID, ListOptions{})
			if err != nil || len(page.Items) != 4 {
				t.Fatal(err)
			}
			for i, v := range page.Items {
				if v.Number != int64(i+1) || v.RecordedAt != times[i].Format(time.RFC3339Nano) {
					t.Fatal("history sorted by clock instead of revision")
				}
			}
			before := observe(t, db)
			for _, base := range []int{1, 3, 5} {
				_, err := s.UpdateRecord(ctx, created.ID, "stale", []byte(fmt.Sprintf(`{"base_revision":%d,"status":"active"}`, base)))
				var conflict *RevisionConflict
				if !errors.As(err, &conflict) || conflict.Expected != int64(base) || conflict.Current != 4 {
					t.Fatal("clock affected base check", err)
				}
			}
			_, err = s.RestoreRecord(ctx, created.ID, 1, "stale-restore", []byte(`{"base_revision":3}`))
			var restoreConflict *RevisionConflict
			if !errors.As(err, &restoreConflict) || restoreConflict.Current != 4 {
				t.Fatal("clock affected restore conflict", err)
			}
			// Every original result replays after the latest head with its own time and
			// attribution, without consulting today's clock or allocating another event.
			again, err := s.CreateRecord(actor("retry", 5), "create", body)
			if err != nil || string(again.Data) != string(createdRaw.Data) {
				t.Fatal("create replay", err)
			}
			again, err = s.UpdateRecord(actor("retry", 6), created.ID, "patch", patch)
			if err != nil || string(again.Data) != string(updatedRaw.Data) {
				t.Fatal("patch replay", err)
			}
			again, err = s.UpdateRecord(actor("retry", 7), created.ID, "noop", noop)
			if err != nil || string(again.Data) != string(noopRaw.Data) {
				t.Fatal("no-op replay", err)
			}
			again, err = s.RestoreRecord(actor("retry", 8), created.ID, 1, "restore", restoreBody)
			if err != nil || string(again.Data) != string(restoredRaw.Data) {
				t.Fatal("restore replay", err)
			}
			if calls != 4 || observe(t, db) != before {
				t.Fatal("clock/replay allocated extra effects")
			}
			healthy(t, db)
		})
	}
}
