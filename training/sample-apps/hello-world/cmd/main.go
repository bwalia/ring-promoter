// Command hello-world is the smallest realistic service in the Ring Promoter
// training academy: an HTTP server that reports which version it is running.
//
// It is deliberately dependency-free (standard library only) so it builds and
// runs anywhere, and it exposes the exact endpoints Ring Promoter's health
// checks understand:
//
//	GET /            -> greeting + version (human)
//	GET /healthz     -> {"status":"ok","version":"..."}  (liveness + version)
//	GET /readyz      -> 200 once warmed up                (readiness)
//	GET /metrics     -> Prometheus text exposition        (observability)
//
// The version comes from the RP_VERSION environment variable (Ring Promoter and
// the Helm chart set it to the image tag being deployed) or the -X ldflag,
// falling back to "dev". Because /healthz echoes the version, a ring configured
// with `health_version_field: version` will only pass once the endpoint is
// actually serving the promoted build — the core "did the new version really go
// live?" guarantee.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"
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
	ver := resolveVersion()

	var requests atomic.Int64
	ready := &atomic.Bool{}
	// Simulate a short warm-up so readiness is meaningful in demos.
	go func() {
		time.Sleep(2 * time.Second)
		ready.Store(true)
	}()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprintf(w, "Hello from ring-promoter training 👋\nversion: %s\nhost: %s\n", ver, hostname())
	})

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

	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP helloworld_requests_total Total requests served.\n")
		fmt.Fprintf(w, "# TYPE helloworld_requests_total counter\n")
		fmt.Fprintf(w, "helloworld_requests_total %d\n", requests.Load())
		fmt.Fprintf(w, "# HELP helloworld_build_info Build info as labels.\n")
		fmt.Fprintf(w, "# TYPE helloworld_build_info gauge\n")
		fmt.Fprintf(w, "helloworld_build_info{version=%q} 1\n", ver)
	})

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("hello-world %s listening on %s", ver, addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
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

func hostname() string {
	h, _ := os.Hostname()
	return h
}
