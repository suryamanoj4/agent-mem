package broker

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"agent-memory/pkg/decision"
)

type RESTBroker struct {
	store   decision.DecisionStore
	lockMgr decision.LockManager
	mux     *http.ServeMux
	srv     *http.Server
}

func NewRESTBroker(store decision.DecisionStore, lockMgr decision.LockManager) *RESTBroker {
	b := &RESTBroker{store: store, lockMgr: lockMgr, mux: http.NewServeMux()}
	b.registerRoutes()
	return b
}

func (b *RESTBroker) registerRoutes() {
	b.mux.HandleFunc("POST /api/sessions/{sessionID}/decide", b.handleDecide)
	b.mux.HandleFunc("GET /api/sessions/{sessionID}/context", b.handleGetContext)
	b.mux.HandleFunc("POST /api/sessions/{sessionID}/prefer", b.handlePrefer)
	b.mux.HandleFunc("GET /api/sessions/{sessionID}/search", b.handleSearch)
	b.mux.HandleFunc("POST /api/sessions/{sessionID}/lock", b.handleAcquireLock)
	b.mux.HandleFunc("DELETE /api/sessions/{sessionID}/lock", b.handleReleaseLock)
	b.mux.HandleFunc("GET /api/sessions/{sessionID}/lock/status", b.handleLockStatus)
	b.mux.HandleFunc("POST /api/sessions/{sessionID}/compact", b.handleCompact)
	b.mux.HandleFunc("GET /api/sessions/{sessionID}/export", b.handleExport)
}

func (b *RESTBroker) session(r *http.Request) *decision.DecisionSession {
	sessionID := r.PathValue("sessionID")
	if sessionID == "" {
		return nil
	}
	return decision.NewDecisionSession(sessionID, b.store, b.lockMgr)
}

func (b *RESTBroker) Serve(addr string) error {
	b.srv = &http.Server{Addr: addr, Handler: b.mux}
	log.Printf("REST API server listening on %s", addr)
	return b.srv.ListenAndServe()
}

func (b *RESTBroker) Shutdown(ctx context.Context) error {
	return b.srv.Shutdown(ctx)
}

type decideRequest struct {
	AgentID string `json:"agent_id"`
	Type    string `json:"type"`
	Summary string `json:"summary"`
	Diff    string `json:"diff,omitempty"`
}

func (b *RESTBroker) handleDecide(w http.ResponseWriter, r *http.Request) {
	sess := b.session(r)
	if sess == nil {
		writeError(w, http.StatusBadRequest, "sessionID is required")
		return
	}
	var req decideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ctx := decision.WithCaller(r.Context(), req.AgentID)
	var opts []decision.DecideOption
	if req.Diff != "" {
		opts = append(opts, decision.WithDiff(req.Diff))
	}
	if err := sess.Decide(ctx, decision.DecisionType(req.Type), req.Summary, opts...); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (b *RESTBroker) handleGetContext(w http.ResponseWriter, r *http.Request) {
	sess := b.session(r)
	if sess == nil {
		writeError(w, http.StatusBadRequest, "sessionID is required")
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	ctx := decision.WithCaller(r.Context(), agentID)

	decisions, err := sess.GetContext(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if decisions == nil {
		decisions = []decision.Decision{}
	}
	writeJSON(w, http.StatusOK, decisions)
}

type preferRequest struct {
	Summary string `json:"summary"`
}

func (b *RESTBroker) handlePrefer(w http.ResponseWriter, r *http.Request) {
	sess := b.session(r)
	if sess == nil {
		writeError(w, http.StatusBadRequest, "sessionID is required")
		return
	}
	var req preferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ctx := decision.WithUser(r.Context())
	if err := sess.Prefer(ctx, req.Summary); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (b *RESTBroker) handleSearch(w http.ResponseWriter, r *http.Request) {
	sess := b.session(r)
	if sess == nil {
		writeError(w, http.StatusBadRequest, "sessionID is required")
		return
	}
	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter is required")
		return
	}

	filters := decision.SearchFilters{Limit: 20}
	if dt := r.URL.Query().Get("type"); dt != "" {
		filters = filters.WithTypes(decision.DecisionType(dt))
	}

	decisions, err := sess.Search(r.Context(), query, filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if decisions == nil {
		decisions = []decision.Decision{}
	}
	writeJSON(w, http.StatusOK, decisions)
}

type lockRequest struct {
	Path       string `json:"path"`
	Owner      string `json:"owner"`
	TTLSeconds int    `json:"ttl_seconds"`
}

func (b *RESTBroker) handleAcquireLock(w http.ResponseWriter, r *http.Request) {
	sess := b.session(r)
	if sess == nil {
		writeError(w, http.StatusBadRequest, "sessionID is required")
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
	_, err := sess.Lock(r.Context(), req.Path, req.Owner, ttl)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "locked"})
}

func (b *RESTBroker) handleReleaseLock(w http.ResponseWriter, r *http.Request) {
	sess := b.session(r)
	if sess == nil {
		writeError(w, http.StatusBadRequest, "sessionID is required")
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
	sess := b.session(r)
	if sess == nil {
		writeError(w, http.StatusBadRequest, "sessionID is required")
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
	sess := b.session(r)
	if sess == nil {
		writeError(w, http.StatusBadRequest, "sessionID is required")
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
	sess := b.session(r)
	if sess == nil {
		writeError(w, http.StatusBadRequest, "sessionID is required")
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
