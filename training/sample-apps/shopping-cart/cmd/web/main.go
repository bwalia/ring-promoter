// Command web is the shopping-cart frontend: a tiny standard-library-only static
// server that serves a single embedded index.html and exposes the health
// endpoints Ring Promoter understands. The page fetches the backend's /api/cart.
//
//	GET /          -> the embedded single-page UI
//	GET /healthz   -> {"status":"ok","version":"..."}  (liveness + version)
//	GET /readyz    -> 200 (the static server has no external dependency)
//	GET /metrics   -> Prometheus text exposition
//
// The backend API base URL is injected into the page from the API_BASE env var
// so the frontend and backend can live on separate hosts (shop.<domain> and
// shopping-cart-api.<domain>). Version resolution matches every other training
// app: RP_VERSION, then the -X main.version ldflag, then "dev".
package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

//go:embed index.html
var indexHTML string

// version is overridable at build time: go build -ldflags "-X main.version=v1.2.3".
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

	// Inject the backend base URL once at startup.
	page := strings.ReplaceAll(indexHTML, "__API_BASE__", os.Getenv("API_BASE"))

	var requests atomic.Int64

	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		requests.Add(1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	})

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": ver})
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte("# HELP web_requests_total Total page requests served.\n"))
		w.Write([]byte("# TYPE web_requests_total counter\n"))
		w.Write([]byte("web_requests_total " + strconv.FormatInt(requests.Load(), 10) + "\n"))
		w.Write([]byte("# HELP web_build_info Build info as labels.\n"))
		w.Write([]byte("# TYPE web_build_info gauge\n"))
		w.Write([]byte(`web_build_info{version="` + ver + `"} 1` + "\n"))
	})

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("shopping-cart web %s listening on %s", ver, addr)
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
