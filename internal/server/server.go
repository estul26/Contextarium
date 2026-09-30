// Package server adapts M1 application services and infrastructure probes to HTTP.
package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/estul26/Contextarium/internal/engine"
	"github.com/estul26/Contextarium/internal/storage"
)

type Server struct {
	db              *sql.DB
	engine          *engine.Service
	http            *http.Server
	ready           atomic.Bool
	shutdownTimeout time.Duration
}

// New requires a database that has completed storage.Open successfully.
func New(db *sql.DB) *Server {
	s := &Server{db: db, engine: engine.New(db), shutdownTimeout: 5 * time.Second}
	s.http = &http.Server{
		Handler:           http.HandlerFunc(s.handle),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    8 << 10,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	return s
}

// Serve owns the listener. Cancellation withdraws readiness before draining HTTP.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	if ctx.Err() != nil {
		return listener.Close()
	}
	s.ready.Store(true)
	done := make(chan error, 1)
	go func() { done <- s.http.Serve(listener) }()
	select {
	case err := <-done:
		s.ready.Store(false)
		s.http.Close()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		s.ready.Store(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		err := s.http.Shutdown(shutdownCtx)
		if err != nil {
			s.http.Close()
		}
		serveErr := <-done
		if err != nil {
			return fmt.Errorf("drain HTTP: %w", err)
		}
		if !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		return nil
	}
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if strings.HasPrefix(r.URL.Path, "/api/v1/") {
		s.handleAPI(w, r)
		return
	}
	code, status := http.StatusOK, "ok"
	switch r.URL.Path {
	case "/healthz", "/readyz":
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			code, status = http.StatusMethodNotAllowed, "method_not_allowed"
		} else if r.URL.Path == "/readyz" {
			code, status = http.StatusServiceUnavailable, "not_ready"
			if s.ready.Load() {
				ctx, cancel := context.WithTimeout(r.Context(), 250*time.Millisecond)
				err := storage.CheckReady(ctx, s.db)
				cancel()
				if err == nil && s.ready.Load() {
					code, status = http.StatusOK, "ready"
				}
			}
		}
	default:
		code, status = http.StatusNotFound, "not_found"
	}
	w.WriteHeader(code)
	if r.Method != http.MethodHead {
		// status is a fixed internal value, not request content.
		fmt.Fprintf(w, "{\"status\":\"%s\"}\n", status)
	}
}
