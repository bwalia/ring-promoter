// Package controller holds the operator's reconcile loop and its standard-
// library Kubernetes REST client. It is kept out of package main so the binary
// entrypoint stays a thin shell and the logic is independently testable.
package controller

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	// The Greeting CRD coordinates. Kept in one place so the REST paths and the
	// CRD manifest (api/crd.yaml) stay in lock-step.
	crdGroup   = "training.ringpromoter.io"
	crdVersion = "v1"
	crdPlural  = "greetings"

	managedByKey = "app.kubernetes.io/managed-by"
	managedByVal = "ring-promoter-training-operator"
)

func resolveVersion(buildVersion string) string {
	if v := os.Getenv("RP_VERSION"); v != "" {
		return v
	}
	return buildVersion
}

// metrics are process-wide counters exported on /metrics.
type metrics struct {
	reconcileTotal   atomic.Int64
	reconcileErrors  atomic.Int64
	configMapsMade   atomic.Int64
	configMapsPatch  atomic.Int64
	greetingsSeen    atomic.Int64 // last observed count
	lastReconcileUTC atomic.Int64 // unix seconds
}

// Run is the operator entrypoint: it starts the health/metrics server and, when
// running in a cluster, the reconcile poll loop. It blocks until SIGINT/SIGTERM.
func Run(buildVersion string) {
	ver := resolveVersion(buildVersion)
	addr := ":" + envOr("PORT", "8080")
	interval := envDuration("RECONCILE_INTERVAL", 15*time.Second)
	// Empty = all namespaces (needs cluster-scoped list); set to scope to one.
	watchNS := os.Getenv("WATCH_NAMESPACE")

	m := &metrics{}
	srv := newHealthServer(addr, ver, m)
	go func() {
		log.Printf("operator %s: health/metrics listening on %s", ver, addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("health server: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client, err := newInClusterClient()
	if err != nil {
		// Running outside a cluster (e.g. `go run ./cmd` on a laptop): keep the
		// health/metrics server up so the container is still probeable, but skip
		// reconciliation since there is no API server to talk to.
		log.Printf("operator %s: not running in a cluster (%v) — reconcile loop disabled, health/metrics only", ver, err)
		<-ctx.Done()
		shutdown(srv)
		return
	}
	log.Printf("operator %s: reconciling Greetings every %s (namespace=%q, all=%t)",
		ver, interval, watchNS, watchNS == "")

	tick := time.NewTicker(interval)
	defer tick.Stop()
	reconcileOnce(ctx, client, m, watchNS) // reconcile immediately at startup
	for {
		select {
		case <-ctx.Done():
			log.Printf("operator %s: shutting down", ver)
			shutdown(srv)
			return
		case <-tick.C:
			reconcileOnce(ctx, client, m, watchNS)
		}
	}
}

func newHealthServer(addr, ver string, m *metrics) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": ver})
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP operator_reconcile_total Reconcile loop iterations.\n")
		fmt.Fprintf(w, "# TYPE operator_reconcile_total counter\n")
		fmt.Fprintf(w, "operator_reconcile_total %d\n", m.reconcileTotal.Load())
		fmt.Fprintf(w, "# HELP operator_reconcile_errors_total Reconcile iterations that hit an error.\n")
		fmt.Fprintf(w, "# TYPE operator_reconcile_errors_total counter\n")
		fmt.Fprintf(w, "operator_reconcile_errors_total %d\n", m.reconcileErrors.Load())
		fmt.Fprintf(w, "# HELP operator_configmaps_created_total ConfigMaps created for Greetings.\n")
		fmt.Fprintf(w, "# TYPE operator_configmaps_created_total counter\n")
		fmt.Fprintf(w, "operator_configmaps_created_total %d\n", m.configMapsMade.Load())
		fmt.Fprintf(w, "# HELP operator_configmaps_updated_total ConfigMaps updated to fix drift.\n")
		fmt.Fprintf(w, "# TYPE operator_configmaps_updated_total counter\n")
		fmt.Fprintf(w, "operator_configmaps_updated_total %d\n", m.configMapsPatch.Load())
		fmt.Fprintf(w, "# HELP operator_greetings Observed Greeting custom resources.\n")
		fmt.Fprintf(w, "# TYPE operator_greetings gauge\n")
		fmt.Fprintf(w, "operator_greetings %d\n", m.greetingsSeen.Load())
		fmt.Fprintf(w, "# HELP operator_last_reconcile_timestamp_seconds Unix time of the last reconcile.\n")
		fmt.Fprintf(w, "# TYPE operator_last_reconcile_timestamp_seconds gauge\n")
		fmt.Fprintf(w, "operator_last_reconcile_timestamp_seconds %d\n", m.lastReconcileUTC.Load())
		fmt.Fprintf(w, "# HELP operator_build_info Build info as labels.\n")
		fmt.Fprintf(w, "# TYPE operator_build_info gauge\n")
		fmt.Fprintf(w, "operator_build_info{version=%q} 1\n", ver)
	})
	return &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
}

func shutdown(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

// reconcileOnce lists Greetings and drives each one's ConfigMap to the desired
// state. It is level-triggered: it is safe to run repeatedly and only writes
// when something is actually wrong.
func reconcileOnce(ctx context.Context, c *k8sClient, m *metrics, watchNS string) {
	m.reconcileTotal.Add(1)
	m.lastReconcileUTC.Store(time.Now().Unix())

	greetings, err := c.listGreetings(ctx, watchNS)
	if err != nil {
		m.reconcileErrors.Add(1)
		log.Printf("reconcile: list greetings: %v", err)
		return
	}
	m.greetingsSeen.Store(int64(len(greetings)))

	for _, g := range greetings {
		if err := reconcileGreeting(ctx, c, m, g); err != nil {
			m.reconcileErrors.Add(1)
			log.Printf("reconcile %s/%s: %v", g.Metadata.Namespace, g.Metadata.Name, err)
		}
	}
}

func reconcileGreeting(ctx context.Context, c *k8sClient, m *metrics, g greeting) error {
	ns, name := g.Metadata.Namespace, g.Metadata.Name
	desired := g.Spec.Message

	existing, found, err := c.getConfigMap(ctx, ns, name)
	if err != nil {
		return fmt.Errorf("get configmap: %w", err)
	}

	if !found {
		cm := configMap{}
		cm.Metadata.Name = name
		cm.Metadata.Namespace = ns
		cm.Metadata.Labels = map[string]string{managedByKey: managedByVal}
		cm.Data = map[string]string{"message": desired}
		if err := c.createConfigMap(ctx, ns, cm); err != nil {
			return fmt.Errorf("create configmap: %w", err)
		}
		m.configMapsMade.Add(1)
		log.Printf("reconcile %s/%s: created ConfigMap (message=%q)", ns, name, desired)
		return nil
	}

	if existing.Data["message"] == desired {
		return nil // already correct — nothing to do
	}

	if existing.Data == nil {
		existing.Data = map[string]string{}
	}
	existing.Data["message"] = desired
	if existing.Metadata.Labels == nil {
		existing.Metadata.Labels = map[string]string{}
	}
	existing.Metadata.Labels[managedByKey] = managedByVal
	if err := c.updateConfigMap(ctx, ns, existing); err != nil {
		return fmt.Errorf("update configmap: %w", err)
	}
	m.configMapsPatch.Add(1)
	log.Printf("reconcile %s/%s: updated ConfigMap (message=%q)", ns, name, desired)
	return nil
}
