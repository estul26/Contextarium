package server

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/estul26/Contextarium/internal/storage"
)

func newTestServer(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return New(db), db
}

func TestProbeContractAndFutureRoutesAbsent(t *testing.T) {
	s, _ := newTestServer(t)
	s.ready.Store(true)
	for _, tc := range []struct {
		method, path, status string
		code                 int
	}{
		{"GET", "/healthz", "ok", 200},
		{"GET", "/readyz", "ready", 200},
		{"HEAD", "/healthz", "", 200},
		{"HEAD", "/readyz", "", 200},
		{"POST", "/healthz", "method_not_allowed", 405},
		{"DELETE", "/readyz", "method_not_allowed", 405},
		{"GET", "/healthz/extra", "not_found", 404},
		{"GET", "/", "not_found", 404},
		{"GET", "/mcp", "not_found", 404},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path+"?private=SYNTHETIC_SECRET", strings.NewReader("SYNTHETIC_BODY"))
			r.Host = "127.0.0.1"
			r.Header.Set("Authorization", "Bearer SYNTHETIC_TOKEN")
			w := httptest.NewRecorder()
			s.handle(w, r)
			if w.Code != tc.code {
				t.Fatalf("status = %d, want %d", w.Code, tc.code)
			}
			want := ""
			if tc.method != "HEAD" {
				want = "{\"status\":\"" + tc.status + "\"}\n"
			}
			if w.Body.String() != want {
				t.Fatalf("body = %q, want %q", w.Body.String(), want)
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("missing probe safety headers")
			}
			if tc.code == 405 && w.Header().Get("Allow") != "GET, HEAD" {
				t.Fatal("missing Allow header")
			}
		})
	}
}

func TestReadinessBeforeStartupAndAfterDatabaseClose(t *testing.T) {
	s, db := newTestServer(t)
	probe := func(path string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		s.handle(w, httptest.NewRequest("GET", path, nil))
		if w.Code != want {
			t.Fatalf("%s returned %d, want %d", path, w.Code, want)
		}
	}
	probe("/healthz", 200)
	probe("/readyz", 503)
	s.ready.Store(true)
	probe("/readyz", 200)
	db.Close()
	probe("/readyz", 503)
	probe("/healthz", 200)
}

func TestReadinessDetectsMissingInfrastructure(t *testing.T) {
	s, db := newTestServer(t)
	s.ready.Store(true)
	if _, err := db.Exec("DROP TABLE schema_migrations"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.handle(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 503 || w.Body.String() != "{\"status\":\"not_ready\"}\n" {
		t.Fatalf("unavailable metadata leaked or appeared ready: %d %s", w.Code, w.Body)
	}
}

func TestReadinessWaitIsBounded(t *testing.T) {
	s, db := newTestServer(t)
	s.ready.Store(true)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	start := time.Now()
	w := httptest.NewRecorder()
	s.handle(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 503 || time.Since(start) > 2*time.Second {
		t.Fatalf("readiness did not fail within its bounded connection wait: %d", w.Code)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for lifecycle transition")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestGracefulShutdownDrainsActiveProbe(t *testing.T) {
	s, db := newTestServer(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, ln) }()
	waitFor(t, s.ready.Load)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	response := make(chan *http.Response, 1)
	requestErr := make(chan error, 1)
	client := &http.Client{Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	go func() {
		r, err := client.Get("http://" + ln.Addr().String() + "/readyz")
		if err != nil {
			requestErr <- err
			return
		}
		response <- r
	}()
	waitFor(t, func() bool { return db.Stats().WaitCount > 0 })
	cancel()
	waitFor(t, func() bool { return !s.ready.Load() })
	select {
	case err := <-done:
		t.Fatalf("shutdown did not drain the active probe: %v", err)
	default:
	}
	conn.Close()
	select {
	case r := <-response:
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		if r.StatusCode != 503 {
			t.Fatalf("readiness was not withdrawn: %d", r.StatusCode)
		}
	case err := <-requestErr:
		t.Fatal(err)
	case <-time.After(4 * time.Second):
		t.Fatal("probe did not finish")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	if err := storage.CheckReady(context.Background(), db); err != nil {
		t.Fatal("HTTP shutdown unexpectedly closed SQLite before its owner")
	}
	if _, err := client.Get("http://" + ln.Addr().String() + "/healthz"); err == nil {
		t.Fatal("listener remained open after shutdown")
	}
}

func TestShutdownDeadlineForceClosesConnections(t *testing.T) {
	s, _ := newTestServer(t)
	s.shutdownTimeout = 20 * time.Millisecond
	entered, exited := make(chan struct{}), make(chan struct{})
	s.http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(exited)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, ln) }()
	client := &http.Client{Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		resp, _ := client.Get("http://" + ln.Addr().String() + "/healthz")
		if resp != nil {
			resp.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not enter handler")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected drain deadline, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("forced shutdown hung")
	}
	for _, ch := range []chan struct{}{exited, requestDone} {
		select {
		case <-ch:
		case <-time.After(3 * time.Second):
			t.Fatal("force close did not cancel the active connection")
		}
	}
}
