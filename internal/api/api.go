// Package api exposes the app-scoped REST API and mounts the embedded web UI.
// All /api routes are protected by a bearer token; /healthz and the UI are not.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/example/ring-promoter/internal/deployer"
	"github.com/example/ring-promoter/internal/promoter"
	"github.com/example/ring-promoter/internal/ring"
	"github.com/example/ring-promoter/internal/store"
)

// BuildInfo carries version metadata baked into the binary at build time.
type BuildInfo struct {
	Version   string
	Commit    string
	BuildTime string
}

// Server wires the promoter, auth token and UI into an http.Handler.
type Server struct {
	prom      *promoter.Promoter
	token     string
	prodPass  string
	log       *slog.Logger
	ui        http.Handler
	opTimeout time.Duration
	jobs      *JobManager
	build     BuildInfo
	startedAt time.Time
	diag      Diagnoser
	histDiag  historyDiagnoses
}

// NewServer constructs an API server. ui serves the embedded web assets and
// opTimeout bounds each mutating operation. build carries version metadata
// surfaced on /version. prodPass, when non-empty, is additionally required to
// deploy anything to the last (production) ring. diag, when non-nil, enables
// AI diagnosis of failed jobs.
func NewServer(prom *promoter.Promoter, token, prodPass string, ui http.Handler, opTimeout time.Duration, log *slog.Logger, build BuildInfo, diag Diagnoser) *Server {
	if log == nil {
		log = slog.Default()
	}
	if opTimeout <= 0 {
		opTimeout = 10 * time.Minute
	}
	return &Server{prom: prom, token: token, prodPass: prodPass, ui: ui, opTimeout: opTimeout, log: log, jobs: NewJobManager(), build: build, startedAt: time.Now(), diag: diag,
		histDiag: historyDiagnoses{state: make(map[int64]historyDiagState)}}
}

// prodRing is the pipeline's last ring — the one the production password
// protects.
func prodRing() string {
	all := ring.All()
	return all[len(all)-1].Name
}

// checkProdPassword authorizes an operation that deploys to the production
// ring. It returns false (after writing a 403) when a production password is
// configured and the request's password does not match.
func (s *Server) checkProdPassword(w http.ResponseWriter, password string) bool {
	if s.prodPass == "" {
		return true
	}
	if subtle.ConstantTimeCompare([]byte(password), []byte(s.prodPass)) == 1 {
		return true
	}
	if password == "" {
		writeError(w, http.StatusForbidden, errors.New("production password required"))
	} else {
		writeError(w, http.StatusForbidden, errors.New("incorrect production password"))
	}
	return false
}

// opContext returns a context for a mutating operation that is DETACHED from the
// request lifecycle: a client disconnect or load-balancer idle-timeout must not
// abort an in-flight deploy or — critically — its auto-rollback. It keeps the
// request's values but drops its cancellation, and bounds the work with an
// explicit server-side timeout.
func (s *Server) opContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.Context()), s.opTimeout)
}

// Handler returns the fully-assembled HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Service liveness + build/version info — unauthenticated.
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /version", s.handleVersion)

	// App-scoped REST API — authenticated.
	api := http.NewServeMux()
	api.HandleFunc("GET /api/apps", s.handleListApps)
	api.HandleFunc("GET /api/apps/{app}/rings", s.handleRings)
	api.HandleFunc("GET /api/apps/{app}/history", s.handleHistory)
	api.HandleFunc("GET /api/apps/{app}/versions", s.handleVersions)
	api.HandleFunc("GET /api/apps/{app}/jobs/{id}", s.handleGetJob)
	api.HandleFunc("POST /api/apps/{app}/jobs/{id}/diagnose", s.handleDiagnoseJob)
	api.HandleFunc("POST /api/apps/{app}/history/{id}/diagnose", s.handleDiagnoseHistory)
	api.HandleFunc("GET /api/apps/{app}/history/{id}/diagnose", s.handleGetHistoryDiagnosis)
	// Newest job per app — shared by every user, so one person's promotion is
	// visible on everyone's screen.
	api.HandleFunc("GET /api/jobs", s.handleListJobs)
	api.HandleFunc("POST /api/apps/{app}/seed", s.handleSeed)
	api.HandleFunc("POST /api/apps/{app}/promote", s.handlePromote)
	api.HandleFunc("POST /api/apps/{app}/rollback", s.handleRollback)
	api.HandleFunc("PUT /api/apps/{app}/rings/{ring}/auto-promote", s.handleAutoPromote)
	// Application groups — stored server-side, shared by all users.
	api.HandleFunc("GET /api/groups", s.handleListGroups)
	api.HandleFunc("POST /api/groups", s.handleCreateGroup)
	api.HandleFunc("PUT /api/groups/{id}", s.handleUpdateGroup)
	api.HandleFunc("DELETE /api/groups/{id}", s.handleDeleteGroup)
	mux.Handle("/api/", s.authenticate(api))

	// Web UI (single-page app) — served at the root.
	mux.Handle("/", s.ui)

	return s.logRequests(mux)
}

// ---- middleware ----

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		h := r.Header.Get("Authorization")
		if len(h) <= len(prefix) || h[:len(prefix)] != prefix ||
			subtle.ConstantTimeCompare([]byte(h[len(prefix):]), []byte(s.token)) != 1 {
			writeError(w, http.StatusUnauthorized, errors.New("missing or invalid bearer token"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.log.Info("http",
			"method", r.Method, "path", r.URL.Path,
			"status", rec.status, "duration_ms", time.Since(start).Milliseconds())
	})
}

// ---- handlers ----

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleVersion reports build metadata and when this instance started, which
// (with the immutable image) reflects when it was last deployed. The UI footer
// consumes this.
func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":    s.build.Version,
		"commit":     s.build.Commit,
		"built_at":   s.build.BuildTime,
		"started_at": s.startedAt.UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleListApps(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"apps": s.prom.Apps(),
		// Display titles per app (config display_name, falling back to the
		// name). Purely cosmetic: every API path still uses the app name.
		"titles": s.prom.AppTitles(),
		"rings":  ring.All(),
		// Tells the UI to ask for the production password where needed.
		"prod_protected": s.prodPass != "",
		// Tells the UI to offer "Diagnose with AI" on failed jobs.
		"ai_enabled": s.diag != nil,
	})
}

func (s *Server) handleRings(w http.ResponseWriter, r *http.Request) {
	views, err := s.prom.Rings(r.Context(), r.PathValue("app"))
	if err != nil {
		writeError(w, statusForErr(err), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rings": views})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	hist, err := s.prom.History(r.Context(), r.PathValue("app"))
	if err != nil {
		writeError(w, statusForErr(err), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": hist})
}

// handleVersions lists the versions that exist in the app's source repository
// (branches/tags for github-deployed apps). supported=false tells the UI the
// deployer cannot enumerate versions, so it falls back to free-form input.
func (s *Server) handleVersions(w http.ResponseWriter, r *http.Request) {
	supported, versions, err := s.prom.Versions(r.Context(), r.PathValue("app"))
	if err != nil {
		writeError(w, statusForErr(err), err)
		return
	}
	if versions == nil {
		versions = []deployer.Version{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"supported": supported,
		"versions":  versions,
	})
}

func (s *Server) handleSeed(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ring     string `json:"ring"`
		Version  string `json:"version"`
		Password string `json:"password,omitempty"`
	}
	if !decode(w, r, &body) {
		return
	}
	app := r.PathValue("app")
	// Seeding straight into production needs the production password.
	if body.Ring == prodRing() && !s.checkProdPassword(w, body.Password) {
		return
	}
	if wantsAsync(r) {
		// Reject precondition failures (unknown ring, version missing from the
		// source repo) on the request itself instead of spawning a doomed job —
		// the UI keeps its dialog open and shows the reason.
		if err := s.prom.ValidateSeed(r.Context(), app, body.Ring, body.Version); err != nil {
			writeError(w, statusForErr(err), err)
			return
		}
		job := s.jobs.run(r.Context(), s.opTimeout, app, "seed", func(ctx context.Context) (promoter.Result, error) {
			return s.prom.Seed(ctx, app, body.Ring, body.Version)
		})
		writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.id()})
		return
	}
	ctx, cancel := s.opContext(r)
	defer cancel()
	res, err := s.prom.Seed(ctx, app, body.Ring, body.Version)
	writeResult(w, res, err)
}

func (s *Server) handlePromote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FromRing string `json:"from_ring"`
		Password string `json:"password,omitempty"`
	}
	if !decode(w, r, &body) {
		return
	}
	app := r.PathValue("app")
	// Promoting INTO production needs the production password.
	if next, ok := ring.Next(body.FromRing); ok && next.Name == prodRing() &&
		!s.checkProdPassword(w, body.Password) {
		return
	}
	if wantsAsync(r) {
		job := s.jobs.run(r.Context(), s.opTimeout, app, "promote", func(ctx context.Context) (promoter.Result, error) {
			return s.prom.Promote(ctx, app, body.FromRing)
		})
		writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.id()})
		return
	}
	ctx, cancel := s.opContext(r)
	defer cancel()
	res, err := s.prom.Promote(ctx, app, body.FromRing)
	writeResult(w, res, err)
}

func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ring string `json:"ring"`
	}
	if !decode(w, r, &body) {
		return
	}
	app := r.PathValue("app")
	if wantsAsync(r) {
		job := s.jobs.run(r.Context(), s.opTimeout, app, "rollback", func(ctx context.Context) (promoter.Result, error) {
			return s.prom.Rollback(ctx, app, body.Ring)
		})
		writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.id()})
		return
	}
	ctx, cancel := s.opContext(r)
	defer cancel()
	res, err := s.prom.Rollback(ctx, app, body.Ring)
	writeResult(w, res, err)
}

// handleAutoPromote flips a ring's auto-promote setting: when a version lands
// healthy in that ring, it is promoted onward automatically (the chain runs
// inside the seed/promote operation itself).
func (s *Server) handleAutoPromote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled  bool   `json:"enabled"`
		Password string `json:"password,omitempty"`
	}
	if !decode(w, r, &body) {
		return
	}
	app, ringName := r.PathValue("app"), r.PathValue("ring")
	// Enabling the hands-free path INTO production needs the password too —
	// otherwise auto-promote would be a way around it. Disabling is always
	// allowed (it only makes things safer).
	if body.Enabled {
		if next, ok := ring.Next(ringName); ok && next.Name == prodRing() &&
			!s.checkProdPassword(w, body.Password) {
			return
		}
	}
	if err := s.prom.SetAutoPromote(r.Context(), app, ringName, body.Enabled); err != nil {
		writeError(w, statusForErr(err), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app": app, "ring": ringName, "auto_promote": body.Enabled,
	})
}

// ---- application groups ----

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.prom.Groups(r.Context())
	if err != nil {
		writeError(w, statusForErr(err), err)
		return
	}
	if groups == nil {
		groups = []store.Group{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string   `json:"name"`
		Apps []string `json:"apps"`
	}
	if !decode(w, r, &body) {
		return
	}
	g, err := s.prom.CreateGroup(r.Context(), body.Name, body.Apps)
	if err != nil {
		writeError(w, statusForErr(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string   `json:"name"`
		Apps []string `json:"apps"`
	}
	if !decode(w, r, &body) {
		return
	}
	g, err := s.prom.UpdateGroup(r.Context(), r.PathValue("id"), body.Name, body.Apps)
	if err != nil {
		writeError(w, statusForErr(err), err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	if err := s.prom.DeleteGroup(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, statusForErr(err), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleListJobs returns the newest job of every application. Every browser
// polls this, so a promotion started on one screen shows on all of them.
func (s *Server) handleListJobs(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"jobs": s.jobs.latestPerApp()})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	job, ok := s.jobs.get(r.PathValue("id"))
	if !ok || job.snapshot().App != r.PathValue("app") {
		writeError(w, http.StatusNotFound, errors.New("job not found"))
		return
	}
	writeJSON(w, http.StatusOK, job.snapshot())
}

// wantsAsync reports whether the caller requested async (job-based) execution.
func wantsAsync(r *http.Request) bool {
	v := r.URL.Query().Get("async")
	return v == "1" || v == "true"
}

// ---- helpers ----

// writeResult maps a promoter outcome to an HTTP response: 4xx for precondition
// errors, 422 when the operation ran but did not succeed, 200 on success.
func writeResult(w http.ResponseWriter, res promoter.Result, err error) {
	if err != nil {
		writeError(w, statusForErr(err), err)
		return
	}
	if !res.Success {
		writeJSON(w, http.StatusUnprocessableEntity, res)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func statusForErr(err error) int {
	switch {
	case errors.Is(err, promoter.ErrAppNotFound), errors.Is(err, promoter.ErrRingNotConfigured):
		return http.StatusNotFound
	case errors.Is(err, promoter.ErrGroupNotFound), errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, promoter.ErrNoNextRing), errors.Is(err, promoter.ErrEmptyVersion),
		errors.Is(err, promoter.ErrVersionNotFound), errors.Is(err, promoter.ErrEmptyGroupName),
		errors.Is(err, promoter.ErrUnknownApp):
		return http.StatusBadRequest
	case errors.Is(err, promoter.ErrNothingToPromote), errors.Is(err, promoter.ErrNothingToRollback):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON body: "+err.Error()))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
