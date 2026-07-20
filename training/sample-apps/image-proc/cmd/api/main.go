// Command api is the front door of the image-proc training app: an async job
// API backed by a Redis list. It demonstrates a queue-based workload for the
// Ring Promoter academy — the companion `worker` binary drains the same list.
//
//	POST /jobs        -> enqueue a job, returns {"id":"...","status":"queued"}
//	                     (503 if Redis is down — the queue, not the API, is sick)
//	GET  /jobs/{id}   -> {"id":"...","status":"queued|processing|done"}
//	GET  /healthz     -> {"status":"ok","version":"..."} (liveness; ok even if
//	                     Redis is down — the API process itself is healthy)
//	GET  /readyz      -> 200 once warmed up
//	GET  /metrics     -> Prometheus text (queue_depth gauge, jobs_submitted_total)
//
// VERSION-AWARE HEALTH — HEADER VARIANT: unlike hello-world (which exposes the
// version as a JSON field), this API sets the "X-App-Version" response header on
// EVERY response, including /healthz. A Ring Promoter ring configured with
// `health_version_header: X-App-Version` reads the header to confirm the running
// build matches the promoted version.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/bwalia/ring-promoter/training/image-proc/internal/queue"
	"github.com/bwalia/ring-promoter/training/image-proc/internal/redis"
)

// version is overridable at build time: go build -ldflags "-X main.version=v1.2.3".
// At runtime RP_VERSION wins so the same image can report the tag it was
// deployed as.
var version = "dev"

func resolveVersion() string {
	if v := os.Getenv("RP_VERSION"); v != "" {
		return v
	}
	return version
}

func main() {
	addr := ":" + envOr("PORT", "8080")
	redisAddr := envOr("REDIS_ADDR", "localhost:6379")
	ver := resolveVersion()

	rdb := redis.New(redisAddr)

	var submitted atomic.Int64
	ready := &atomic.Bool{}
	go func() {
		time.Sleep(2 * time.Second)
		ready.Store(true)
	}()

	mux := http.NewServeMux()

	// POST /jobs — enqueue a job onto the Redis list. If Redis is unreachable we
	// return 503: the request cannot be accepted, but /healthz stays ok because
	// the API process itself is fine.
	mux.HandleFunc("POST /jobs", func(w http.ResponseWriter, r *http.Request) {
		id := newID()
		if err := rdb.Set(queue.StatusKey(id), queue.StatusQueued); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "queue-unavailable", "error": err.Error(),
			})
			return
		}
		if _, err := rdb.LPush(queue.Key, id); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "queue-unavailable", "error": err.Error(),
			})
			return
		}
		submitted.Add(1)
		writeJSON(w, http.StatusAccepted, map[string]string{
			"id": id, "status": queue.StatusQueued,
		})
	})

	// GET /jobs/{id} — report a job's status from Redis.
	mux.HandleFunc("GET /jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		status, err := rdb.Get(queue.StatusKey(id))
		if err == redis.Nil {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"id": id, "status": "unknown",
			})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"id": id, "status": "queue-unavailable", "error": err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": status})
	})

	// GET /healthz — liveness. Stays ok even when Redis is down; the version is
	// carried in the X-App-Version header (set for every response below) AND in
	// the JSON body for convenience.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": ver})
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "warming-up"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	// GET /metrics — queue_depth is read live from Redis (LLEN); if Redis is down
	// we report -1 so the gauge is obviously anomalous rather than a silent 0.
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		depth := int64(-1)
		if n, err := rdb.LLen(queue.Key); err == nil {
			depth = n
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP imageproc_queue_depth Jobs currently waiting in the Redis queue (-1 if Redis unreachable).\n")
		fmt.Fprintf(w, "# TYPE imageproc_queue_depth gauge\n")
		fmt.Fprintf(w, "imageproc_queue_depth %d\n", depth)
		fmt.Fprintf(w, "# HELP imageproc_jobs_submitted_total Jobs accepted onto the queue since start.\n")
		fmt.Fprintf(w, "# TYPE imageproc_jobs_submitted_total counter\n")
		fmt.Fprintf(w, "imageproc_jobs_submitted_total %d\n", submitted.Load())
		fmt.Fprintf(w, "# HELP imageproc_build_info Build info as labels.\n")
		fmt.Fprintf(w, "# TYPE imageproc_build_info gauge\n")
		fmt.Fprintf(w, "imageproc_build_info{version=%q,component=%q} 1\n", ver, "api")
	})

	// Wrap the mux so the X-App-Version header is present on EVERY response —
	// this is the header variant of Ring Promoter's version-aware health check.
	handler := versionHeader(ver, mux)

	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("image-proc api %s listening on %s (redis %s)", ver, addr, redisAddr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// versionHeader sets X-App-Version on every response before delegating to next.
func versionHeader(ver string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-App-Version", ver)
		next.ServeHTTP(w, r)
	})
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
