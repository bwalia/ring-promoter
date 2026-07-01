// Command ringpromoter runs the Ring Promoter control-plane service: an
// HTTP API + embedded web UI that promotes application versions through the
// shared deployment rings.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/ring-promoter/internal/api"
	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/deployer"
	"github.com/example/ring-promoter/internal/health"
	"github.com/example/ring-promoter/internal/promoter"
	"github.com/example/ring-promoter/internal/ring"
	"github.com/example/ring-promoter/internal/store"
	"github.com/example/ring-promoter/internal/web"
)

func main() {
	configPath := flag.String("config", envOr("RP_CONFIG_FILE", "config.yaml"), "path to the config file")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(*configPath, logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(configPath string, logger *slog.Logger) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := buildStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	prom := promoter.New(cfg, st, buildDeployer(cfg, logger), buildChecker(cfg), logger)
	srv := api.NewServer(prom, cfg.APIToken, web.Handler(), logger)

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	go func() {
		<-ctx.Done()
		logger.Info("shutdown signal received, draining")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "err", err)
		}
	}()

	logger.Info("ring promoter started",
		"addr", cfg.ListenAddr, "deployer", cfg.Deployer, "health", cfg.Health,
		"store", cfg.Database.Driver, "apps", len(cfg.Apps), "rings", ring.Names())

	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	logger.Info("stopped")
	return nil
}

func buildStore(ctx context.Context, cfg *config.Config) (store.Store, error) {
	switch cfg.Database.Driver {
	case config.StorePostgres:
		return store.NewPostgres(ctx, cfg.Database.DSN)
	default:
		return store.NewMemory(), nil
	}
}

func buildDeployer(cfg *config.Config, logger *slog.Logger) deployer.Deployer {
	switch cfg.Deployer {
	case config.DeployerKubectl:
		return deployer.NewKubectlDeployer(logger, 2*time.Minute)
	default:
		return deployer.NewLogDeployer(logger)
	}
}

func buildChecker(cfg *config.Config) health.Checker {
	switch cfg.Health {
	case config.HealthHTTP:
		return health.NewHTTPChecker(5 * time.Second)
	default:
		return health.AlwaysHealthy{}
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
