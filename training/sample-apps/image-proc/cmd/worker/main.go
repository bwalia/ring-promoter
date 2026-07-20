// Command worker is the async consumer half of image-proc. It blocks on the
// Redis list the API pushes to (BRPOP), "processes" each job (a short sleep
// standing in for real image work), and marks it done. It has NO user-facing
// ingress — only a side HTTP port exposing /healthz and /metrics so Kubernetes
// probes and Prometheus can reach it.
//
// In the Ring Promoter academy this is the piece that scales: an HPA (CPU as a
// stand-in in training; queue depth in production) adds worker replicas as the
// backlog grows. See the chart's worker-hpa.yaml and architecture.md.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/bwalia/ring-promoter/training/image-proc/internal/queue"
	"github.com/bwalia/ring-promoter/training/image-proc/internal/redis"
)

var version = "dev"

func resolveVersion() string {
	if v := os.Getenv("RP_VERSION"); v != "" {
		return v
	}
	return version
}

func main() {
	metricsAddr := ":" + envOr("METRICS_PORT", "9090")
	redisAddr := envOr("REDIS_ADDR", "localhost:6379")
	procDelay := envDuration("PROCESS_DELAY", 500*time.Millisecond)
	ver := resolveVersion()

	rdb := redis.New(redisAddr)

	var processed atomic.Int64

	// Side HTTP server: liveness + metrics only, no ingress.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-App-Version", ver)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "{\"status\":\"ok\",\"version\":%q}\n", ver)
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		depth := int64(-1)
		if n, err := rdb.LLen(queue.Key); err == nil {
			depth = n
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP imageproc_jobs_processed_total Jobs processed by this worker since start.\n")
		fmt.Fprintf(w, "# TYPE imageproc_jobs_processed_total counter\n")
		fmt.Fprintf(w, "imageproc_jobs_processed_total %d\n", processed.Load())
		fmt.Fprintf(w, "# HELP imageproc_queue_depth Jobs currently waiting in the Redis queue (-1 if Redis unreachable).\n")
		fmt.Fprintf(w, "# TYPE imageproc_queue_depth gauge\n")
		fmt.Fprintf(w, "imageproc_queue_depth %d\n", depth)
		fmt.Fprintf(w, "# HELP imageproc_build_info Build info as labels.\n")
		fmt.Fprintf(w, "# TYPE imageproc_build_info gauge\n")
		fmt.Fprintf(w, "imageproc_build_info{version=%q,component=%q} 1\n", ver, "worker")
	})

	go func() {
		srv := &http.Server{Addr: metricsAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		log.Printf("image-proc worker %s metrics on %s", ver, metricsAddr)
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	// Consume loop: block for a job, process it, mark it done, repeat. Errors
	// (including Redis being down) are logged and retried after a short backoff
	// so the worker self-heals when Redis returns.
	log.Printf("image-proc worker %s consuming %s from redis %s", ver, queue.Key, redisAddr)
	for {
		id, err := rdb.BRPop(queue.Key, 5*time.Second)
		if err == redis.Nil {
			continue // timed out with no work; loop again
		}
		if err != nil {
			log.Printf("brpop error: %v (retrying)", err)
			time.Sleep(1 * time.Second)
			continue
		}

		_ = rdb.Set(queue.StatusKey(id), queue.StatusProcessing)
		time.Sleep(procDelay) // stand-in for real image processing
		if err := rdb.Set(queue.StatusKey(id), queue.StatusDone); err != nil {
			log.Printf("failed to mark job %s done: %v", id, err)
			continue
		}
		processed.Add(1)
		log.Printf("processed job %s", id)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
