// Package bankapi is the high-governance sample service in the Ring Promoter
// training academy: a small REST API backed by PostgreSQL that issues JWTs and
// reads account balances.
//
// It exists to demonstrate everything the smallest app (hello-world) does NOT
// exercise: a real dependency (Postgres) that may be down, database migrations,
// Kubernetes Secrets, and — through its Ring Promoter registration — all three
// promotion gates (maintenance_window, qa_signoff, change_request) plus the
// production password. See README.md and architecture.md.
//
// Endpoints:
//
//	POST /login                    -> validates the demo user, returns a JWT
//	GET  /accounts/{id}/balance    -> JWT-protected balance (Postgres, or demo)
//	GET  /healthz                  -> {"status":"ok","version":"..."} (liveness)
//	GET  /readyz                   -> 200 only when the DB is reachable
//	GET  /metrics                  -> Prometheus text exposition
//
// The service NEVER crashes because the database is unreachable: it opens a lazy
// connection pool at boot, serves /healthz immediately, reports /readyz
// not-ready until the DB answers, and falls back to a demo balance on reads.
package bankapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	_ "github.com/lib/pq"
)

// version is overridable at build time: go build -ldflags "-X main.version=...".
// The cmd wrapper passes its linked value in via Run. At runtime RP_VERSION wins
// so the same image reports the tag it was deployed as — the field Ring
// Promoter's health_version_field: version compares against.
func resolveVersion(linked string) string {
	if v := os.Getenv("RP_VERSION"); v != "" {
		return v
	}
	if linked != "" {
		return linked
	}
	return "dev"
}

// demoBalance is returned by the balance endpoint when Postgres is unreachable,
// so the API stays useful in demos even with no database wired up.
const demoBalance = "1000.00"

// server holds the shared runtime state for the handlers.
type server struct {
	version   string
	db        *sql.DB // never nil; may point at an unreachable database
	jwtSecret []byte

	requests   atomic.Int64
	logins     atomic.Int64
	dbUp       atomic.Bool
	demoServed atomic.Int64 // balance reads served from the demo fallback
}

// Run builds the server and blocks serving HTTP. linkedVersion is the value the
// cmd wrapper linked in via -ldflags. It returns only on a fatal listen error.
func Run(linkedVersion string) error {
	ver := resolveVersion(linkedVersion)
	addr := ":" + envOr("PORT", "8080")

	// JWT secret comes from a Kubernetes Secret via env. A missing secret is a
	// misconfiguration, but we do NOT crash — we log loudly and use an obvious
	// insecure default so /healthz still comes up and the pod can be inspected.
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Printf("WARNING: JWT_SECRET is empty; using an insecure development default. Set it from a Secret.")
		secret = "insecure-dev-secret-change-me"
	}

	srv := &server{
		version:   ver,
		jwtSecret: []byte(secret),
	}

	// Open a lazy pool. sql.Open never dials, so a down/absent database cannot
	// crash boot. Connectivity is probed asynchronously and by /readyz.
	dsn := os.Getenv("DATABASE_URL")
	db, err := sql.Open("postgres", dsnOrPlaceholder(dsn))
	if err != nil {
		// Only malformed DSNs reach here; still don't crash — serve degraded.
		log.Printf("WARNING: could not initialise DB pool: %v (serving without DB)", err)
		db, _ = sql.Open("postgres", "postgres://invalid")
	}
	db.SetMaxOpenConns(5)
	db.SetConnMaxIdleTime(2 * time.Minute)
	srv.db = db

	// Optionally run migrations on boot. Failures are logged, not fatal: the
	// canonical path is the Kubernetes migrate Job, and the DB may not be up yet.
	if truthy(os.Getenv("RUN_MIGRATIONS")) {
		if err := srv.tryMigrate(); err != nil {
			log.Printf("WARNING: boot migrations failed (continuing; the migrate Job is the canonical path): %v", err)
		}
	}

	// Probe DB readiness in the background so we never block boot on it.
	go srv.watchDB()

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("bank-api %s listening on %s", ver, addr)
	return httpSrv.ListenAndServe()
}

// tryMigrate pings the DB then applies embedded migrations.
func (s *server) tryMigrate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		return err
	}
	log.Printf("running embedded migrations (RUN_MIGRATIONS=true)")
	return runMigrations(s.db)
}

// watchDB keeps dbUp in sync with the database's reachability so /readyz and the
// balance fallback reflect reality without a probe on every request.
func (s *server) watchDB() {
	check := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		up := s.db.PingContext(ctx) == nil
		if up != s.dbUp.Swap(up) {
			if up {
				log.Printf("database is reachable")
			} else {
				log.Printf("database is UNREACHABLE — /readyz not-ready, balances fall back to demo")
			}
		}
	}
	check()
	for range time.Tick(5 * time.Second) {
		check()
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("GET /accounts/{id}/balance", s.requireJWT(s.handleBalance))
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	return mux
}

// ---- Handlers --------------------------------------------------------------

// loginRequest is the POST /login body.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleLogin validates the demo user and issues a short-lived HS256 JWT signed
// with JWT_SECRET. Credentials default to demo/demo and can be overridden with
// DEMO_USER / DEMO_PASSWORD (from a Secret) for lab exercises.
func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	s.requests.Add(1)
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	wantUser := envOr("DEMO_USER", "demo")
	wantPass := envOr("DEMO_PASSWORD", "demo")
	if req.Username != wantUser || req.Password != wantPass {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   req.Username,
		Issuer:    "bank-api",
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not sign token"})
		return
	}
	s.logins.Add(1)
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "token_type": "Bearer", "expires_in": 3600})
}

// balanceResponse is the GET /accounts/{id}/balance body.
type balanceResponse struct {
	AccountID int64  `json:"account_id"`
	Balance   string `json:"balance"`
	Currency  string `json:"currency"`
	Source    string `json:"source"` // "database" or "demo-fallback"
}

// handleBalance returns an account's balance from Postgres, falling back to a
// fixed demo balance when the database is unreachable so the endpoint stays
// useful without a DB.
func (s *server) handleBalance(w http.ResponseWriter, r *http.Request) {
	s.requests.Add(1)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "account id must be an integer"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	var balance, currency string
	row := s.db.QueryRowContext(ctx, "SELECT balance::text, currency FROM accounts WHERE id = $1", id)
	switch err := row.Scan(&balance, &currency); {
	case err == nil:
		writeJSON(w, http.StatusOK, balanceResponse{AccountID: id, Balance: balance, Currency: currency, Source: "database"})
	case errors.Is(err, sql.ErrNoRows):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "account not found"})
	default:
		// DB unreachable / query error: degrade gracefully with a demo balance.
		s.demoServed.Add(1)
		log.Printf("balance query failed for account %d (%v); serving demo fallback", id, err)
		writeJSON(w, http.StatusOK, balanceResponse{AccountID: id, Balance: demoBalance, Currency: "GBP", Source: "demo-fallback"})
	}
}

func (s *server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": s.version})
}

func (s *server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if !s.dbUp.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not-ready", "reason": "database unreachable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	dbUp := 0
	if s.dbUp.Load() {
		dbUp = 1
	}
	writeMetric(w, "bankapi_requests_total", "Total application requests served.", "counter", s.requests.Load(), "")
	writeMetric(w, "bankapi_logins_total", "Successful logins (JWTs issued).", "counter", s.logins.Load(), "")
	writeMetric(w, "bankapi_demo_fallback_total", "Balance reads served from the demo fallback (DB down).", "counter", s.demoServed.Load(), "")
	writeMetric(w, "bankapi_database_up", "1 if the database is reachable, else 0.", "gauge", int64(dbUp), "")
	writeMetric(w, "bankapi_build_info", "Build info as labels.", "gauge", 1, `version="`+s.version+`"`)
}

// ---- JWT middleware --------------------------------------------------------

// requireJWT wraps a handler, rejecting requests without a valid Bearer token
// signed with JWT_SECRET.
func (s *server) requireJWT(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing Bearer token"})
			return
		}
		tokenStr := auth[len(prefix):]
		_, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return s.jwtSecret, nil
		}, jwt.WithValidMethods([]string{"HS256"}))
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
			return
		}
		next(w, r)
	}
}

// ---- helpers ---------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeMetric(w http.ResponseWriter, name, help, typ string, value int64, labels string) {
	if _, err := w.Write([]byte("# HELP " + name + " " + help + "\n# TYPE " + name + " " + typ + "\n" + name)); err != nil {
		return
	}
	if labels != "" {
		_, _ = w.Write([]byte("{" + labels + "}"))
	}
	_, _ = w.Write([]byte(" " + strconv.FormatInt(value, 10) + "\n"))
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func truthy(s string) bool {
	switch s {
	case "1", "true", "TRUE", "True", "yes", "on":
		return true
	}
	return false
}

// dsnOrPlaceholder returns dsn, or an obviously-unreachable placeholder when it
// is empty, so sql.Open always succeeds and the server can start degraded.
func dsnOrPlaceholder(dsn string) string {
	if dsn == "" {
		log.Printf("WARNING: DATABASE_URL is empty; starting without a database (/readyz will be not-ready). Set it from a Secret.")
		return "postgres://localhost:5432/nonexistent?sslmode=disable&connect_timeout=1"
	}
	return dsn
}
