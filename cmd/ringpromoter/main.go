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
	"github.com/example/ring-promoter/internal/changerequest"
	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/deployer"
	"github.com/example/ring-promoter/internal/diagnose"
	"github.com/example/ring-promoter/internal/executor"
	"github.com/example/ring-promoter/internal/executor/k8sjob"
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

	// Change-request validators for apps whose promotion policy gates rings
	// behind a valid CR code (validated against JIRA or another business
	// system). Built up-front so a missing token fails fast at start-up.
	crValidators, err := buildChangeRequestValidators(cfg, logger)
	if err != nil {
		return err
	}
	prom.SetChangeRequestValidators(crValidators)

	// AI failure diagnosis (optional): enabled only when both the Ollama URL
	// and the JWT secret are configured.
	var diag api.Diagnoser
	if cfg.Ollama.Enabled() {
		diag = diagnose.New(cfg.Ollama.URL, cfg.Ollama.Model, cfg.Ollama.JWTSecret, logger)
		logger.Info("ai diagnosis enabled", "url", cfg.Ollama.URL, "model", cfg.Ollama.Model)
	} else {
		logger.Info("ai diagnosis disabled (set ollama.url and RP_OLLAMA_JWT_SECRET to enable)")
	}

	srv := api.NewServer(prom, cfg.APIToken, cfg.ProdPassword, web.Handler(), cfg.OperationTimeout.Std(), logger,
		api.BuildInfo{Version: version, Commit: commit, BuildTime: buildTime}, diag)

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
// mechanisms (kubectl, log, the k8sjob executor) are instantiated once; the
// github and k8sjob deployers are per-app because each app carries its own
// workflow/Job configuration.
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

	// The Kubernetes Job executor is stateless and shared by every k8sjob app;
	// each app maps its own config onto the Spec.
	var k8sExec *k8sjob.Executor
	sharedK8sExec := func() *k8sjob.Executor {
		if k8sExec == nil {
			k8sExec = k8sjob.New(logger, k8sjob.Options{})
		}
		return k8sExec
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
		case config.DeployerK8sJob:
			perApp[app.Name] = buildK8sJobDeployer(app, logger, sharedK8sExec())
		case config.DeployerKubectl:
			perApp[app.Name] = shared(config.DeployerKubectl)
		default:
			perApp[app.Name] = shared(config.DeployerLog)
		}
	}

	// Default for any app not explicitly mapped (defensive; every app is mapped
	// above). A global "github"/"k8sjob" default has no per-app config, so fall
	// back to the log deployer for the default slot.
	def := shared(cfg.Deployer)
	return perApp, def, nil
}

// buildK8sJobDeployer maps one app's k8sjob config onto the execution
// abstraction: the spec function translates each Deploy(target, version) into
// the Job to run, and the ExecDeployer adapter drives it (status polling, log
// streaming, cancellation) behind the ordinary Deployer contract.
func buildK8sJobDeployer(app config.AppConfig, logger *slog.Logger, ex *k8sjob.Executor) deployer.Deployer {
	j := app.K8sJob // guaranteed non-nil by config validation

	specFor := func(t deployer.Target, version string) (executor.Spec, error) {
		env := make(map[string]string, len(j.Env)+5)
		for k, v := range j.Env {
			env[k] = v
		}
		// The runner contract. TargetEnv falls back to the ring name so
		// scripts always receive a concrete environment.
		env[executor.EnvApp] = t.App
		env[executor.EnvRing] = t.Ring
		env[executor.EnvVersion] = version
		targetEnv := t.TargetEnv
		if targetEnv == "" {
			targetEnv = t.Ring
		}
		env[executor.EnvTargetEnv] = targetEnv

		tolerations := make([]executor.Toleration, 0, len(j.Tolerations))
		for _, tol := range j.Tolerations {
			tolerations = append(tolerations, executor.Toleration(tol))
		}

		return executor.Spec{
			App:               t.App,
			Ring:              t.Ring,
			Image:             j.Image,
			Command:           j.Command,
			Args:              j.Args,
			Env:               env,
			EnvFromSecrets:    j.EnvFromSecrets,
			EnvFromConfigMaps: j.EnvFromConfigMaps,
			ImagePullSecrets:  j.ImagePullSecrets,
			Namespace:         j.ResolvedNamespace(),
			ServiceAccount:    j.ServiceAccount,
			Resources:         executor.Resources(j.Resources),
			NodeSelector:      j.NodeSelector,
			Tolerations:       tolerations,
			Affinity:          j.Affinity,
			Timeout:           j.ResolvedTimeout(),
			Retries:           j.ResolvedRetries(),
			TTLAfterFinish:    j.ResolvedTTL(),
			Labels:            j.Labels,
			Annotations:       j.Annotations,
		}, nil
	}

	return deployer.FromExecutor(logger, ex, specFor, j.ResolvedPollInterval())
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
		VersionAsRef: g.VersionAsRef,
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

// buildChangeRequestValidators constructs one change-request validator per app
// whose promotion policy enables the change-request gate. The "test" provider
// (or an app with no CR system) uses the demo-only validator; the "jira"
// provider reads its API token from the env var named by the config.
func buildChangeRequestValidators(cfg *config.Config, logger *slog.Logger) (map[string]changerequest.Validator, error) {
	out := make(map[string]changerequest.Validator)
	for _, app := range cfg.Apps {
		pol := app.PromotionPolicy
		if pol == nil || pol.ChangeRequest == nil {
			continue
		}
		switch pol.ChangeRequest.ProviderKind() {
		case config.CRProviderJIRA:
			j := pol.ChangeRequest.JIRA // guaranteed non-nil by config validation
			token := os.Getenv(j.TokenEnvName())
			if token == "" {
				return nil, fmt.Errorf("app %q change_request uses the jira provider but env %s is empty", app.Name, j.TokenEnvName())
			}
			out[app.Name] = changerequest.NewJIRA(changerequest.JIRAParams{
				BaseURL:         j.BaseURL,
				Email:           j.Email,
				Token:           token,
				AllowedStatuses: j.AllowedStatuses,
				ProjectKeys:     j.ProjectKeys,
				Log:             logger,
			})
			logger.Info("change-request gate enabled", "app", app.Name, "provider", "jira", "base_url", j.BaseURL)
		default:
			out[app.Name] = changerequest.Test{}
			logger.Info("change-request gate enabled", "app", app.Name, "provider", "test (demo code only)")
		}
	}
	return out, nil
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
