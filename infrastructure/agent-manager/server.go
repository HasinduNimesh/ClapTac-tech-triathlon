package main

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/agenttrace"
)

//go:embed web
var webFiles embed.FS

const maxIngestBytes = 256 << 10

// Server exposes trace ingest for the agents and a read-only viewer for people.
// Ingest and viewing use separate tokens so an agent can write but not read.
// An empty token leaves that side open, which main only allows locally.
type Server struct {
	Store       *Store
	IngestToken string
	ViewToken   string
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "live"})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("POST /api/v1/traces", s.guard(s.IngestToken, s.ingest))
	mux.HandleFunc("GET /api/v1/traces", s.guard(s.ViewToken, s.list))
	mux.HandleFunc("GET /api/v1/traces/{id}", s.guard(s.ViewToken, s.get))
	// The page itself holds no data; the API calls it makes carry the view token.
	site, _ := fs.Sub(webFiles, "web")
	mux.Handle("GET /", http.FileServerFS(site))
	return secure(mux)
}

func secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func (s Server) guard(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func (s Server) ingest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxIngestBytes)
	dec := json.NewDecoder(r.Body)
	var t agenttrace.Trace
	if err := dec.Decode(&t); err != nil {
		http.Error(w, "invalid trace", http.StatusBadRequest)
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "invalid trace", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(t.Agent) == "" || strings.TrimSpace(t.Operation) == "" {
		http.Error(w, "agent and operation are required", http.StatusBadRequest)
		return
	}
	// Clients always send times; default them so a sloppy sender does not show up as year 0001.
	if t.StartedAt.IsZero() {
		t.StartedAt = time.Now().UTC()
	}
	if t.EndedAt.IsZero() || t.EndedAt.Before(t.StartedAt) {
		t.EndedAt = t.StartedAt
	}
	if err := s.Store.Add(t); err != nil {
		http.Error(w, "unable to store trace", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s Server) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  s.Store.List(Filter{Agent: q.Get("agent"), Status: q.Get("status"), Query: q.Get("q"), Limit: limit}),
		"agents": s.Store.Agents(),
	})
}

func (s Server) get(w http.ResponseWriter, r *http.Request) {
	t, ok := s.Store.Get(r.PathValue("id"))
	if !ok {
		http.Error(w, "trace not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
