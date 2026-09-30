package server

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/estul26/Contextarium/internal/engine"
)

const apiSchema = `{"schema_id":"example.task","schema_version":1,"migration_policy":"explicit","definition":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"title":{"type":"string"},"done":{"type":"boolean"}},"required":["title","done"],"additionalProperties":false}}`

func request(s *Server, method, path, key, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.handle(w, r)
	return w
}
func responseData[T any](t *testing.T, w *httptest.ResponseRecorder, status int) T {
	t.Helper()
	if w.Code != status {
		t.Fatalf("HTTP %d, want %d: %s", w.Code, status, w.Body.String())
	}
	var envelope struct {
		Data T `json:"data"`
		Meta struct {
			APIVersion string `json:"api_version"`
			RequestID  string `json:"request_id"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Meta.APIVersion != "v1" || envelope.Meta.RequestID != w.Header().Get("X-Request-ID") || !strings.HasPrefix(envelope.Meta.RequestID, "req_") {
		t.Fatal("missing response metadata")
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Content-Type") != "application/json" {
		t.Fatal("missing safety headers")
	}
	return envelope.Data
}
func TestHTTPRecordLifecycle(t *testing.T) {
	s, _ := newTestServer(t)
	sub := responseData[engine.Subject](t, request(s, "POST", "/api/v1/subjects", "subject", `{"kind":"project","display_name":"Synthetic project"}`, nil), 201)
	responseData[engine.Schema](t, request(s, "POST", "/api/v1/schemas", "schema", apiSchema, nil), 201)
	responseData[engine.Schema](t, request(s, "GET", "/api/v1/schemas/example.task/1", "", "", nil), 200)
	body := fmt.Sprintf(`{"subject_id":%q,"namespace":"tasks.example","schema_id":"example.task","schema_version":1,"data":{"title":"Example","done":false}}`, sub.ID)
	record := responseData[engine.Record](t, request(s, "POST", "/api/v1/records", "create", body, nil), 201)
	replay := responseData[engine.Record](t, request(s, "POST", "/api/v1/records", "create", body, nil), 201)
	if replay.ID != record.ID {
		t.Fatal("HTTP replay duplicated record")
	}
	patched := responseData[engine.Record](t, request(s, "PATCH", "/api/v1/records/"+record.ID, "update", `{"data":{"title":"Example","done":true},"status":"archived"}`, nil), 200)
	if patched.Status != "archived" {
		t.Fatal("patch status failed")
	}
	got := responseData[engine.Record](t, request(s, "GET", "/api/v1/records/"+record.ID, "", "", nil), 200)
	if string(got.Data) != string(patched.Data) {
		t.Fatal("read differs from update")
	}
	records := responseData[[]engine.Record](t, request(s, "GET", "/api/v1/records?namespace=tasks.example&status=archived&limit=1", "", "", nil), 200)
	if len(records) != 1 {
		t.Fatal("filter failed")
	}
	subjects := responseData[[]engine.Subject](t, request(s, "GET", "/api/v1/subjects?limit=1", "", "", nil), 200)
	if len(subjects) != 1 {
		t.Fatal("subject list failed")
	}
	responseData[engine.Subject](t, request(s, "GET", "/api/v1/subjects/"+sub.ID, "", "", nil), 200)
	unknownSchema := strings.Replace(body, `"schema_version":1`, `"schema_version":2`, 1)
	if w := request(s, "POST", "/api/v1/records", "unknown", unknownSchema, nil); w.Code != 404 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, patch := range []string{`{"namespace":"other"}`, `{"subject_id":"other"}`, `{"schema_id":"other"}`, `{"data":{"title":"Example","done":"true"}}`} {
		w := request(s, "PATCH", "/api/v1/records/"+record.ID, "invalid", patch, nil)
		if w.Code != 400 && w.Code != 422 {
			t.Fatal("bad patch accepted", w.Code, w.Body.String())
		}
	}
}
func TestAPIRejectsMalformedAndUnboundedRequests(t *testing.T) {
	s, _ := newTestServer(t)
	cases := []struct {
		method, path, key, body string
		headers                 map[string]string
		status                  int
	}{
		{"POST", "/api/v1/subjects", "", `{"kind":"person","display_name":"Example"}`, nil, 400},
		{"POST", "/api/v1/subjects", "key", "{} {}", nil, 400},
		{"POST", "/api/v1/subjects", "key", `{"kind":"person","kind":"project","display_name":"Example"}`, nil, 400},
		{"POST", "/api/v1/subjects", "key", `{"Kind":"person","display_name":"Example"}`, nil, 400},
		{"POST", "/api/v1/subjects", "key", `null`, nil, 400},
		{"POST", "/api/v1/subjects", "key", `{}`, map[string]string{"Content-Type": "text/plain"}, 415},
		{"POST", "/api/v1/subjects", "key", `{}`, map[string]string{"Content-Encoding": "gzip"}, 415},
		{"POST", "/api/v1/subjects", "key", `{}`, map[string]string{"Content-Type": "application/json; charset=latin1"}, 415},
		{"POST", "/api/v1/subjects", "key", strings.Repeat(" ", engine.MaxBody+1), nil, 413},
		{"POST", "/api/v1/subjects?unexpected=1", "key", `{}`, nil, 400},
		{"GET", "/api/v1/records?limit=101", "", "", nil, 400},
		{"GET", "/api/v1/records?limit=0", "", "", nil, 400},
		{"GET", "/api/v1/records?limit=1&limit=2", "", "", nil, 400},
		{"GET", "/api/v1/records?namespace=Tasks", "", "", nil, 400},
		{"GET", "/api/v1/records?cursor=bad", "", "", nil, 400},
		{"GET", "/api/v1/records?limit=%zz", "", "", nil, 400},
		{"GET", "/api/v1/records?namespace=", "", "", nil, 400},
		{"GET", "/api/v1/records/not-an-id", "", "", nil, 400},
		{"GET", "/api/v1/records/rec_00000000000000000000000000000000", "", "", nil, 404},
		{"GET", "/api/v1/schemas/example.task/01", "", "", nil, 400},
		{"DELETE", "/api/v1/records/rec_00000000000000000000000000000000", "", "", nil, 405},
		{"GET", "/api/v1/proposals", "", "", nil, 404},
		{"GET", "/api/v1/search", "", "", nil, 404},
		{"GET", "/api/v1/records/rec_00000000000000000000000000000000/revisions", "", "", nil, 404},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			w := request(s, tc.method, tc.path, tc.key, tc.body, tc.headers)
			if w.Code != tc.status {
				t.Fatalf("%s %s: %d want %d: %s", tc.method, tc.path, w.Code, tc.status, w.Body.String())
			}
			if tc.status == 405 && w.Header().Get("Allow") != "GET, PATCH" {
				t.Fatal("missing Allow")
			}
			if !strings.Contains(w.Body.String(), `"error"`) {
				t.Fatal("missing error envelope")
			}
		})
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1/api/v1/subjects", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Add("Idempotency-Key", "a")
	r.Header.Add("Idempotency-Key", "b")
	w := httptest.NewRecorder()
	s.handle(w, r)
	if w.Code != 400 {
		t.Fatal("duplicate key headers accepted")
	}
}
func TestAPIBrowserBoundaryAndErrorPrivacy(t *testing.T) {
	s, db := newTestServer(t)
	for _, headers := range []map[string]string{{"Origin": "https://example.com"}, {"Origin": "null"}, {"Sec-Fetch-Site": "cross-site"}} {
		if w := request(s, "GET", "/api/v1/records", "", "", headers); w.Code != 403 {
			t.Fatal("cross origin accepted", w.Code)
		}
	}
	r := httptest.NewRequest("GET", "http://attacker.example/api/v1/records", nil)
	w := httptest.NewRecorder()
	s.handle(w, r)
	if w.Code != 403 {
		t.Fatal("rebinding Host accepted")
	}
	responseData[[]engine.Record](t, request(s, "GET", "/api/v1/records", "", "", map[string]string{"Origin": "http://127.0.0.1:8080"}), 200)
	db.Close()
	w = request(s, "GET", "/api/v1/records", "", "", map[string]string{"Authorization": "Bearer SYNTHETIC_SECRET", "X-Request-ID": "SYNTHETIC_SECRET"})
	if w.Code != 503 || strings.Contains(w.Body.String(), "SYNTHETIC_SECRET") || strings.Contains(w.Body.String(), "database") || w.Header().Get("X-Request-ID") == "SYNTHETIC_SECRET" {
		t.Fatal("unsafe error", w.Code, w.Body.String())
	}
}
