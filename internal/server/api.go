package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/estul26/Contextarium/internal/engine"
)

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	requestID, err := engine.NewID("req")
	if err != nil {
		apiError(w, 500, "INTERNAL_ERROR", "Operation failed.")
		return
	}
	w.Header().Set("X-Request-ID", requestID)
	// Loopback binding alone does not prevent browser-origin/DNS rebinding access.
	if !localHost(r.Host) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		apiError(w, 403, "ORIGIN_REJECTED", "Browser origin is not allowed.")
		return
	}
	if origins := r.Header.Values("Origin"); len(origins) > 0 {
		origin, err := url.Parse(origins[0])
		if len(origins) != 1 || err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host != r.Host || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
			apiError(w, 403, "ORIGIN_REJECTED", "Browser origin is not allowed.")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	ctx = engine.WithAttribution(ctx, engine.Actor{Kind: "development_test", ID: "local-http-adapter"}, requestID)
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	parts := strings.Split(path, "/")
	collection := (path == "subjects" || path == "records" || path == "schemas")
	item := (len(parts) == 2 && (parts[0] == "subjects" || parts[0] == "records") && parts[1] != "")
	schema := (len(parts) == 3 && parts[0] == "schemas" && parts[1] != "" && parts[2] != "")
	revisions := len(parts) == 3 && parts[0] == "records" && parts[1] != "" && parts[2] == "revisions"
	revision := len(parts) == 4 && parts[0] == "records" && parts[1] != "" && parts[2] == "revisions" && parts[3] != ""
	restore := len(parts) == 4 && parts[0] == "records" && parts[1] != "" && parts[2] == "restore" && parts[3] != ""
	if !collection && !item && !schema && !revisions && !revision && !restore {
		apiError(w, 404, "NOT_FOUND", "Resource not found.")
		return
	}
	allow := "GET"
	if collection {
		if path == "schemas" {
			allow = "POST"
		} else {
			allow = "GET, POST"
		}
	} else if item && parts[0] == "records" {
		allow = "GET, PATCH"
	}
	if restore {
		allow = "POST"
	}
	if !containsMethod(allow, r.Method) {
		w.Header().Set("Allow", allow)
		apiError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed.")
		return
	}
	opts, err := queryOptions(r, (collection || revisions) && r.Method == "GET", path == "records")
	if err != nil {
		serviceError(w, err)
		return
	}
	var body []byte
	key := ""
	if r.Method == "POST" || r.Method == "PATCH" {
		if parts[0] == "records" && len(r.Header.Values("If-Match")) != 0 {
			serviceError(w, engine.ErrInvalid)
			return
		}
		media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 || r.Header.Get("Content-Encoding") != "" {
			apiError(w, 415, "UNSUPPORTED_MEDIA_TYPE", "Use unencoded application/json.")
			return
		}
		for name, value := range params {
			if name != "charset" || !strings.EqualFold(value, "utf-8") {
				apiError(w, 415, "UNSUPPORTED_MEDIA_TYPE", "Use unencoded application/json.")
				return
			}
		}
		keys := r.Header.Values("Idempotency-Key")
		if len(keys) != 1 {
			serviceError(w, engine.ErrInvalid)
			return
		}
		key = keys[0]
		reader := http.MaxBytesReader(w, r.Body, engine.MaxBody)
		defer reader.Close()
		body, err = io.ReadAll(reader)
		if err != nil {
			var size *http.MaxBytesError
			if errors.As(err, &size) {
				apiError(w, 413, "PAYLOAD_TOO_LARGE", "Request body is too large.")
			} else {
				serviceError(w, engine.ErrInvalid)
			}
			return
		}
	}
	var data any
	var next *string
	status := 200
	switch {
	case revisions:
		var page engine.Page[engine.Revision]
		page, err = s.engine.ListRevisions(ctx, parts[1], opts)
		data = page.Items
		next = &page.NextCursor
	case revision || restore:
		var n int64
		n, err = engine.ParseRevision(parts[3])
		if err == nil {
			if restore {
				data, err = s.engine.RestoreRecord(ctx, parts[1], n, key, body)
			} else {
				data, err = s.engine.GetRevision(ctx, parts[1], n)
			}
		}
	case path == "subjects" && r.Method == "POST":
		data, err = s.engine.CreateSubject(ctx, key, body)
		status = 201
	case path == "subjects":
		var page engine.Page[engine.Subject]
		page, err = s.engine.ListSubjects(ctx, opts)
		data = page.Items
		next = &page.NextCursor
	case path == "schemas":
		data, err = s.engine.PublishSchema(ctx, key, body)
		status = 201
	case path == "records" && r.Method == "POST":
		data, err = s.engine.CreateRecord(ctx, key, body)
		status = 201
	case path == "records":
		var page engine.Page[engine.Record]
		page, err = s.engine.ListRecords(ctx, opts)
		data = page.Items
		next = &page.NextCursor
	case schema:
		version, parseErr := strconv.Atoi(parts[2])
		if parseErr != nil || strconv.Itoa(version) != parts[2] {
			err = engine.ErrInvalid
		} else {
			data, err = s.engine.GetSchema(ctx, parts[1], version)
		}
	case parts[0] == "subjects":
		data, err = s.engine.GetSubject(ctx, parts[1])
	case r.Method == "PATCH":
		data, err = s.engine.UpdateRecord(ctx, parts[1], key, body)
	default:
		data, err = s.engine.GetRecord(ctx, parts[1])
	}
	if err != nil {
		serviceError(w, err)
		return
	}
	meta := map[string]any{"api_version": "v1", "request_id": requestID}
	if result, ok := data.(engine.MutationResult); ok {
		meta["result_contract"] = result.Contract
		data = result.Data
	}
	if next != nil {
		meta["next_cursor"] = *next
	}
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(map[string]any{"data": data, "meta": meta})
}
func containsMethod(allow, method string) bool {
	for _, v := range strings.Split(allow, ", ") {
		if v == method {
			return true
		}
	}
	return false
}
func localHost(hostport string) bool {
	host := hostport
	if h, port, err := net.SplitHostPort(hostport); err == nil {
		if port == "" || strings.IndexFunc(port, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return false
		}
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return false
		}
		host = h
	} else if strings.HasPrefix(hostport, "[") && strings.HasSuffix(hostport, "]") {
		// An IPv6 Host header retains brackets when the default port is omitted.
		ip, err := netip.ParseAddr(hostport[1 : len(hostport)-1])
		return err == nil && ip.Is6() && ip.IsLoopback() && ip.Zone() == ""
	}
	if host == hostport && strings.Contains(host, ":") {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback() && ip.Zone() == ""
}
func queryOptions(r *http.Request, list, records bool) (engine.ListOptions, error) {
	var o engine.ListOptions
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return o, engine.ErrInvalid
	}
	for key, v := range values {
		if !list || len(v) != 1 || v[0] == "" {
			return o, engine.ErrInvalid
		}
		switch key {
		case "limit":
			o.Limit, err = strconv.Atoi(v[0])
			if err != nil || o.Limit < 1 || o.Limit > 100 {
				return o, engine.ErrInvalid
			}
		case "cursor":
			o.Cursor = v[0]
		case "namespace":
			if !records {
				return o, engine.ErrInvalid
			}
			o.Namespace = v[0]
		case "subject_id":
			if !records {
				return o, engine.ErrInvalid
			}
			o.SubjectID = v[0]
		case "status":
			if !records {
				return o, engine.ErrInvalid
			}
			o.Status = v[0]
		default:
			return o, engine.ErrInvalid
		}
	}
	return o, nil
}
func serviceError(w http.ResponseWriter, err error) {
	var conflict *engine.RevisionConflict
	switch {
	case errors.As(err, &conflict):
		w.WriteHeader(409)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "REVISION_CONFLICT", "message": "Base revision does not match current state.", "details": map[string]int64{"expected_revision": conflict.Expected, "current_revision": conflict.Current}}})
	case errors.Is(err, engine.ErrBaseRequired):
		apiError(w, 400, "BASE_REVISION_REQUIRED", "A base revision is required.")
	case errors.Is(err, engine.ErrRevisionLimit):
		apiError(w, 409, "REVISION_LIMIT_REACHED", "Revision limit reached.")
	case errors.Is(err, engine.ErrSchemaPair):
		apiError(w, 409, "SCHEMA_PAIR_CONFLICT", "Historical schema pair differs.")
	case errors.Is(err, engine.ErrInvalid):
		apiError(w, 400, "INVALID_ARGUMENT", "Invalid request.")
	case errors.Is(err, engine.ErrNotFound):
		apiError(w, 404, "NOT_FOUND", "Resource not found.")
	case errors.Is(err, engine.ErrConflict):
		apiError(w, 409, "CONFLICT", "Request conflicts with stored state.")
	case errors.Is(err, engine.ErrValidation):
		apiError(w, 422, "SCHEMA_VALIDATION_FAILED", "Schema validation failed.")
	case errors.Is(err, engine.ErrUnavailable), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		apiError(w, 503, "UNAVAILABLE", "Storage is temporarily unavailable.")
	default:
		apiError(w, 500, "INTERNAL_ERROR", "Operation failed.")
	}
}
func apiError(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
