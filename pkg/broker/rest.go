package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"agent-memory/pkg/api"
)

type RESTBroker struct {
	svc api.MemoryService
	mux *http.ServeMux
	srv *http.Server
}

func NewRESTBroker(svc api.MemoryService) *RESTBroker {
	b := &RESTBroker{svc: svc, mux: http.NewServeMux()}
	b.registerRoutes()
	return b
}

func (b *RESTBroker) registerRoutes() {
	b.mux.HandleFunc("POST /api/sessions/{sessionID}/log", b.handleLog)
	b.mux.HandleFunc("GET /api/sessions/{sessionID}/search", b.handleSearch)
	b.mux.HandleFunc("POST /api/sessions/{sessionID}/lock", b.handleAcquireLock)
	b.mux.HandleFunc("DELETE /api/sessions/{sessionID}/lock", b.handleReleaseLock)
	b.mux.HandleFunc("GET /api/sessions/{sessionID}/lock/status", b.handleLockStatus)
	b.mux.HandleFunc("POST /api/sessions/{sessionID}/compact", b.handleCompact)
	b.mux.HandleFunc("GET /api/sessions/{sessionID}/export", b.handleExport)
}

func (b *RESTBroker) Serve(addr string) error {
	b.srv = &http.Server{Addr: addr, Handler: b.mux}
	log.Printf("REST API server listening on %s", addr)
	return b.srv.ListenAndServe()
}

func (b *RESTBroker) Shutdown(ctx context.Context) error {
	return b.srv.Shutdown(ctx)
}

func (b *RESTBroker) session(r *http.Request) (api.Session, error) {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		return nil, fmt.Errorf("sessionID is required")
	}
	return b.svc.Connect(r.Context(), sessionID)
}

type logRequest struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (b *RESTBroker) handleLog(w http.ResponseWriter, r *http.Request) {
	sess, err := b.session(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req logRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := sess.Log(r.Context(), req.Role, req.Content); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (b *RESTBroker) handleSearch(w http.ResponseWriter, r *http.Request) {
	sess, err := b.session(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter is required")
		return
	}
	entries, err := sess.Ask(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entries == nil {
		entries = []api.LogEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

type lockRequest struct {
	Path       string `json:"path"`
	Owner      string `json:"owner"`
	TTLSeconds int    `json:"ttl_seconds"`
}

func (b *RESTBroker) handleAcquireLock(w http.ResponseWriter, r *http.Request) {
	sess, err := b.session(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req lockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 1 * time.Hour
	}
	_, err = sess.Lock(r.Context(), req.Path, req.Owner, ttl)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "locked"})
}

func (b *RESTBroker) handleReleaseLock(w http.ResponseWriter, r *http.Request) {
	sess, err := b.session(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path query parameter is required")
		return
	}
	if err := sess.ReleaseLock(r.Context(), path); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "released"})
}

func (b *RESTBroker) handleLockStatus(w http.ResponseWriter, r *http.Request) {
	sess, err := b.session(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path query parameter is required")
		return
	}
	locked, owner, err := sess.GetLockStatus(r.Context(), path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":   path,
		"locked": locked,
		"owner":  owner,
	})
}

func (b *RESTBroker) handleCompact(w http.ResponseWriter, r *http.Request) {
	sess, err := b.session(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	count, err := sess.Compact(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"archived": count})
}

func (b *RESTBroker) handleExport(w http.ResponseWriter, r *http.Request) {
	sess, err := b.session(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	sess.Export(r.Context(), w)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
