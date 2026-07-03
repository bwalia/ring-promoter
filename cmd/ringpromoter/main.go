// Command ringpromoter runs the Ring Promoter control-plane service: an
// HTTP API + embedded web UI that promotes application versions through the
// shared deployment rings.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
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

// Build metadata, injected at build time via -ldflags "-X main.version=..."
// (see Dockerfile). Defaults keep local `go run` working without flags.
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

func main() {
	configPath := flag.String("config", envOr("RP_CONFIG_FILE", "config.yaml"), "path to the config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("ring-promoter %s (commit %s, built %s)\n", version, commit, buildTime)
		return
	}

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

	deployers, defaultDeployer, err := buildDeployers(cfg, logger)
	if err != nil {
		return err
	}
	prom := promoter.New(cfg, st, deployers, defaultDeployer, buildChecker(cfg), logger)
	srv := api.NewServer(prom, cfg.APIToken, web.Handler(), cfg.OperationTimeout.Std(), logger,
		api.BuildInfo{Version: version, Commit: commit, BuildTime: buildTime})

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

// buildDeployers constructs one deployer per application (honoring an app's
// `deployer` override) plus a default deployer for anything unlisted. Shared
// mechanisms (kubectl, log) are instantiated once; the github deployer is
// per-app because each app dispatches its own workflow.
func buildDeployers(cfg *config.Config, logger *slog.Logger) (map[string]deployer.Deployer, deployer.Deployer, error) {
	var kubectlDep, logDep deployer.Deployer
	shared := func(kind string) deployer.Deployer {
		switch kind {
		case config.DeployerKubectl:
			if kubectlDep == nil {
				kubectlDep = deployer.NewKubectlDeployer(logger, 2*time.Minute)
			}
			return kubectlDep
		default:
			if logDep == nil {
				logDep = deployer.NewLogDeployer(logger)
			}
			return logDep
		}
	}

	perApp := make(map[string]deployer.Deployer, len(cfg.Apps))
	for _, app := range cfg.Apps {
		switch cfg.DeployerFor(app) {
		case config.DeployerGitHub:
			d, err := buildGitHubDeployer(app, logger)
			if err != nil {
				return nil, nil, err
			}
			perApp[app.Name] = d
		case config.DeployerKubectl:
			perApp[app.Name] = shared(config.DeployerKubectl)
		default:
			perApp[app.Name] = shared(config.DeployerLog)
		}
	}

	// Default for any app not explicitly mapped (defensive; every app is mapped
	// above). A global "github" default has no per-app config, so fall back to
	// the log deployer for the default slot.
	def := shared(cfg.Deployer)
	return perApp, def, nil
}

func buildGitHubDeployer(app config.AppConfig, logger *slog.Logger) (deployer.Deployer, error) {
	g := app.GitHub // guaranteed non-nil by config validation
	token := os.Getenv(g.TokenEnvName())
	if token == "" {
		return nil, fmt.Errorf("app %q uses the github deployer but env %s is empty", app.Name, g.TokenEnvName())
	}
	cfg := deployer.GitHubActionsConfig{
		Owner:        g.Owner,
		Repo:         g.Repo,
		Workflow:     g.Workflow,
		Ref:          g.Ref,
		DeployMode:   g.DeployMode,
		EnvInput:     g.EnvInput,
		VersionInput: g.VersionInput,
		ModeInput:    g.ModeInput,
		ExtraInputs:  g.ExtraInputs,
		Token:        token,
		APIBaseURL:   g.APIBaseURL,
	}
	if g.PollInterval != nil {
		cfg.PollInterval = g.PollInterval.Std()
	}
	if g.RunLookupTimeout != nil {
		cfg.RunLookupTimeout = g.RunLookupTimeout.Std()
	}
	return deployer.NewGitHubActionsDeployer(logger, cfg, nil), nil
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
