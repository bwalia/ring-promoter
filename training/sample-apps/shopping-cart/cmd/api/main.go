// Command api is the shopping-cart backend: a dependency-free (standard library
// only) HTTP service that stores cart items in Redis, degrading gracefully to an
// in-memory store when REDIS_ADDR is unset and to an empty cart when Redis is
// down. It never crashes because of Redis.
//
// It exposes the exact endpoints Ring Promoter's health checks understand:
//
//	GET  /api/cart   -> {"items":[...],"backend":"redis|memory"}
//	POST /api/cart   -> {"item":"..."} appends an item
//	GET  /healthz    -> {"status":"ok","version":"..."}  (liveness + version)
//	GET  /readyz     -> 200 when Redis reachable, 503 otherwise (never crashes)
//	GET  /metrics    -> Prometheus text exposition
//
// The version comes from RP_VERSION (set by the Helm chart / Ring Promoter to
// the deployed tag) or the -X main.version ldflag, falling back to "dev". Because
// /healthz echoes the version, a ring configured with `health_version_field:
// version` only passes once the endpoint serves the promoted build.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bwalia/ring-promoter/training/shopping-cart/internal/cart"
	"github.com/bwalia/ring-promoter/training/shopping-cart/internal/redisclient"
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

	// Choose a store: Redis when REDIS_ADDR is set, otherwise an in-memory
	// fallback so the app is fully usable locally.
	var store cart.Store
	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		store = cart.NewRedisStore(redisclient.New(redisAddr))
		log.Printf("shopping-cart api using redis at %s", redisAddr)
	} else {
		store = cart.NewMemoryStore()
		log.Printf("shopping-cart api using in-memory store (REDIS_ADDR unset)")
	}

	var requests, itemsAdded atomic.Int64

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/cart", func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		items, err := store.List()
		if err != nil {
			// Redis down: degrade to an empty cart rather than erroring.
			log.Printf("cart list failed, returning empty: %v", err)
			items = []string{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items":   items,
			"backend": store.Backend(),
		})
	})

	mux.HandleFunc("POST /api/cart", func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var body struct {
			Item string `json:"item"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		item := strings.TrimSpace(body.Item)
		if item == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "item must not be empty"})
			return
		}
		if err := store.Add(item); err != nil {
			log.Printf("cart add failed: %v", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cart storage unavailable"})
			return
		}
		itemsAdded.Add(1)
		writeJSON(w, http.StatusCreated, map[string]string{"item": item})
	})

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": ver})
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		// Not ready if the store is unreachable — but never crash.
		if err := store.Ping(); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "not-ready",
				"reason": "store unreachable",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		writeMetric(w, "cart_requests_total", "Total cart API requests served.", "counter", requests.Load(), "")
		writeMetric(w, "cart_items_added_total", "Total items added to carts.", "counter", itemsAdded.Load(), "")
		writeMetric(w, "cart_build_info", "Build info as labels.", "gauge", 1, `version="`+ver+`",backend="`+store.Backend()+`"`)
	})

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("shopping-cart api %s listening on %s", ver, addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeMetric(w http.ResponseWriter, name, help, typ string, value int64, labels string) {
	w.Write([]byte("# HELP " + name + " " + help + "\n"))
	w.Write([]byte("# TYPE " + name + " " + typ + "\n"))
	if labels != "" {
		w.Write([]byte(name + "{" + labels + "} "))
	} else {
		w.Write([]byte(name + " "))
	}
	w.Write([]byte(strconv.FormatInt(value, 10) + "\n"))
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
