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
// The version comes from the -X ldflag baked in at build time (CI sets it to
// the image tag), then the RP_VERSION environment variable for images built
// without one, falling back to "dev". Because /healthz echoes the version, a ring configured
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

// version is set at build time: go build -ldflags "-X main.version=v1.2.3".
// A baked-in version always wins: the image is immutable, so it is the only
// honest answer to "which build is this?". Ring Promoter's kubectl deployer
// changes just the image, so an RP_VERSION env var left on the Deployment by an
// earlier Helm install would otherwise make every new build claim to be the
// old one and fail the version-aware health check. RP_VERSION is the fallback
// for images built without a version ("dev"), e.g. the original v1 image.
var version = "dev"

func resolveVersion() string {
	if version != "" && version != "dev" {
		return version
	}
	if v := os.Getenv("RP_VERSION"); v != "" {
		return v
	}
	return "dev"
}

func main() {
	addr := ":" + envOr("PORT", "8080")
	ver := resolveVersion()

	ready := &atomic.Bool{}
	// Simulate a short warm-up so readiness is meaningful in demos.
	go func() {
		time.Sleep(2 * time.Second)
		ready.Store(true)
	}()

	srv := &http.Server{Addr: addr, Handler: newMux(ver, ready), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("hello-world %s listening on %s", ver, addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// newMux wires the four endpoints. It is separate from main so the unit tests
// exercise exactly the handlers the server runs, without opening a socket.
func newMux(ver string, ready *atomic.Bool) *http.ServeMux {
	var requests atomic.Int64
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

	return mux
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
