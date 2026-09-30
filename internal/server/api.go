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
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	parts := strings.Split(path, "/")
	collection := (path == "subjects" || path == "records" || path == "schemas")
	item := (len(parts) == 2 && (parts[0] == "subjects" || parts[0] == "records") && parts[1] != "")
	schema := (len(parts) == 3 && parts[0] == "schemas" && parts[1] != "" && parts[2] != "")
	if !collection && !item && !schema {
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
	if !containsMethod(allow, r.Method) {
		w.Header().Set("Allow", allow)
		apiError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed.")
		return
	}
	opts, err := queryOptions(r, collection && r.Method == "GET", path == "records")
	if err != nil {
		serviceError(w, err)
		return
	}
	var body []byte
	key := ""
	if r.Method == "POST" || r.Method == "PATCH" {
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
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
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
	switch {
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
