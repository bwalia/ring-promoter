package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/example/ring-promoter/internal/store"
)

var (
	errBadBeforeID = errors.New("before_id must be a non-negative integer")
	errBadLimit    = errors.New("limit must be a non-negative integer")
)

// handleAudit serves the audit ledger, newest first, with keyset paging:
//
//	GET /api/audit?app=&ring=&category=&actor=&before_id=&limit=
//
// The response includes next_before_id when another (possibly empty) page may
// follow; pass it back as before_id to continue. Zero means the end.
func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.AuditFilter{
		App:      q.Get("app"),
		Ring:     q.Get("ring"),
		Category: q.Get("category"),
		Actor:    q.Get("actor"),
	}
	if v := q.Get("before_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id < 0 {
			writeError(w, http.StatusBadRequest, errBadBeforeID)
			return
		}
		f.BeforeID = id
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, errBadLimit)
			return
		}
		f.Limit = n
	}

	events, err := s.prom.Audit(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if events == nil {
		events = []store.AuditEvent{}
	}
	// A full page means more may follow; a short page is the end.
	var nextBefore int64
	if len(events) == clampLimit(f.Limit) {
		nextBefore = events[len(events)-1].ID
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"audit":          events,
		"next_before_id": nextBefore,
	})
}

// clampLimit mirrors the store's paging clamp so the handler can tell a full
// page (more may follow) from a short final one.
func clampLimit(n int) int {
	switch {
	case n <= 0:
		return store.AuditDefaultLimit
	case n > store.AuditMaxLimit:
		return store.AuditMaxLimit
	}
	return n
}
