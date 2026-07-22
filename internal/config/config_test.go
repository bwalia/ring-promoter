package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const baseApps = `
apps:
  - name: web
    rings:
      int: { namespace: ring0, deployment: web, container: web, image: repo/web, health_url: "http://x/health" }
`

// An explicit retry.count: 0 must be honored (one check, no retries), not
// silently replaced by the default.
func TestRetryCountZeroHonored(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	cfg, err := Load(writeConfig(t, "retry:\n  count: 0\n"+baseApps))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Retry.RetryCount(); got != 0 {
		t.Fatalf("explicit count:0 -> RetryCount()=%d, want 0", got)
	}
}

func TestRetryCountDefaultWhenUnset(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	cfg, err := Load(writeConfig(t, baseApps))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Retry.RetryCount(); got != defaultRetryCount {
		t.Fatalf("unset count -> RetryCount()=%d, want %d", got, defaultRetryCount)
	}
}

// RP_RETRY_COUNT=0 must override a non-zero file value with a real zero.
func TestRetryCountEnvZeroOverrides(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	t.Setenv("RP_RETRY_COUNT", "0")
	cfg, err := Load(writeConfig(t, "retry:\n  count: 5\n"+baseApps))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Retry.RetryCount(); got != 0 {
		t.Fatalf("RP_RETRY_COUNT=0 -> RetryCount()=%d, want 0", got)
	}
}

func TestOperationTimeoutDefaultAndOverride(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")

	cfg, err := Load(writeConfig(t, baseApps))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.OperationTimeout.Std(); got != defaultOpTimeout {
		t.Fatalf("default operation timeout = %v, want %v", got, defaultOpTimeout)
	}

	t.Setenv("RP_OP_TIMEOUT", "90s")
	cfg, err = Load(writeConfig(t, baseApps))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.OperationTimeout.Std().Seconds(); got != 90 {
		t.Fatalf("RP_OP_TIMEOUT override = %vs, want 90s", got)
	}
}

func TestNegativeRetryCountRejected(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	if _, err := Load(writeConfig(t, "retry:\n  count: -1\n"+baseApps)); err == nil {
		t.Fatal("expected error for negative retry count")
	}
}

// A ring may pin an exact healthy status code (e.g. spectoncr's 401 on /v2/).
func TestHealthExpectStatusValid(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	body := `
apps:
  - name: spectoncr
    rings:
      int: { target_env: int, health_url: "http://x/v2/", health_expect_status: 401 }
`
	cfg, err := Load(writeConfig(t, body))
	if err != nil {
		t.Fatalf("valid health_expect_status should load: %v", err)
	}
	if got := cfg.Apps[0].Rings["int"].HealthExpectStatus; got != 401 {
		t.Fatalf("HealthExpectStatus = %d, want 401", got)
	}
}

// An out-of-range health_expect_status is a config error (a typo like 4010
// would otherwise silently never match and keep the ring "unhealthy" forever).
func TestHealthExpectStatusOutOfRangeRejected(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	body := `
apps:
  - name: spectoncr
    rings:
      int: { target_env: int, health_url: "http://x/v2/", health_expect_status: 4010 }
`
	if _, err := Load(writeConfig(t, body)); err == nil {
		t.Fatal("expected error for out-of-range health_expect_status")
	}
}

// A ring may report its version via a JSON field OR a header, not both.
func TestHealthVersionSources(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")

	body := `
apps:
  - name: web
    rings:
      int: { health_url: "http://x/healthz", health_version_field: "build.version" }
      test: { health_url: "http://y/healthz", health_version_header: "X-App-Version" }
`
	cfg, err := Load(writeConfig(t, body))
	if err != nil {
		t.Fatalf("valid version sources should load: %v", err)
	}
	if got := cfg.Apps[0].Rings["int"].HealthVersionField; got != "build.version" {
		t.Fatalf("HealthVersionField = %q, want build.version", got)
	}
	if got := cfg.Apps[0].Rings["test"].HealthVersionHeader; got != "X-App-Version" {
		t.Fatalf("HealthVersionHeader = %q, want X-App-Version", got)
	}

	both := `
apps:
  - name: web
    rings:
      int: { health_url: "http://x/healthz", health_version_field: "version", health_version_header: "X-App-Version" }
`
	if _, err := Load(writeConfig(t, both)); err == nil {
		t.Fatal("expected error when both health_version_field and health_version_header are set")
	}
}

// A valid per-app github deployer must load and resolve to DeployerGitHub.
const githubApp = `
apps:
  - name: wslproxy
    deployer: github
    github:
      owner: bwalia
      repo: wslproxy
      workflow: deploy-wslproxy-delivery-pipeline.yml
    rings:
      int: { target_env: int,  health_url: "http://int/health" }
      test: { target_env: test, health_url: "http://test/health" }
      acc: { target_env: acc,  health_url: "http://acc/health" }
      prod: { target_env: prod, health_url: "http://prod/health" }
`

func TestGitHubDeployer_Valid(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	cfg, err := Load(writeConfig(t, githubApp))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	app, ok := cfg.App("wslproxy")
	if !ok {
		t.Fatal("wslproxy app missing")
	}
	if cfg.DeployerFor(app) != DeployerGitHub {
		t.Fatalf("DeployerFor = %q, want github", cfg.DeployerFor(app))
	}
	if got := app.GitHub.TokenEnvName(); got != "RP_GITHUB_TOKEN" {
		t.Fatalf("default token env = %q", got)
	}
}

func TestGitHubDeployer_RequiresGitHubBlock(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	body := `
apps:
  - name: wslproxy
    deployer: github
    rings:
      int: { target_env: int, health_url: "http://int/health" }
`
	if _, err := Load(writeConfig(t, body)); err == nil {
		t.Fatal("expected error: github deployer without github block")
	}
}

func TestGitHubDeployer_RequiresTargetEnvPerRing(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	body := `
apps:
  - name: wslproxy
    deployer: github
    github: { owner: bwalia, repo: wslproxy, workflow: wf.yml }
    rings:
      int: { health_url: "http://int/health" }
`
	if _, err := Load(writeConfig(t, body)); err == nil {
		t.Fatal("expected error: ring missing target_env for github deployer")
	}
}

func TestUnknownPerAppDeployerRejected(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	body := `
apps:
  - name: web
    deployer: banana
    rings:
      int: { namespace: ring0, deployment: web, container: web, image: repo/web, health_url: "http://x/health" }
`
	if _, err := Load(writeConfig(t, body)); err == nil {
		t.Fatal("expected error for unknown per-app deployer")
	}
}

// ---- k8sjob deployer ----

const k8sjobApp = `
apps:
  - name: myapp
    deployer: k8sjob
    k8sjob:
      image: ghcr.io/bwalia/deploy-runner:v1
      command: ["/scripts/deploy.sh"]
      env: { DEPLOY_FLAVOR: full }
      env_from_secrets: [myapp-deploy-credentials]
      service_account: ring-deploy-job
      resources: { cpu_request: 250m, memory_limit: 512Mi }
      timeout: 45m
      retries: 2
      ttl_after_finished: 2h
    rings:
      int:  { target_env: int,  health_url: "http://int/health" }
      test: { target_env: test, health_url: "http://test/health" }
`

func TestK8sJobDeployer_ValidAndDefaults(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	cfg, err := Load(writeConfig(t, k8sjobApp))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	app, ok := cfg.App("myapp")
	if !ok {
		t.Fatal("myapp missing")
	}
	if cfg.DeployerFor(app) != DeployerK8sJob {
		t.Fatalf("DeployerFor = %q, want k8sjob", cfg.DeployerFor(app))
	}
	j := app.K8sJob
	if j.ResolvedNamespace() != "ring-exec" {
		t.Fatalf("default namespace = %q", j.ResolvedNamespace())
	}
	if j.ResolvedTimeout().Minutes() != 45 {
		t.Fatalf("timeout = %s", j.ResolvedTimeout())
	}
	if j.ResolvedRetries() != 2 {
		t.Fatalf("retries = %d", j.ResolvedRetries())
	}
	if j.ResolvedTTL().Hours() != 2 {
		t.Fatalf("ttl = %s", j.ResolvedTTL())
	}
	if j.ResolvedPollInterval().Seconds() != 3 {
		t.Fatalf("default poll = %s", j.ResolvedPollInterval())
	}
	if j.Env["DEPLOY_FLAVOR"] != "full" || j.EnvFromSecrets[0] != "myapp-deploy-credentials" {
		t.Fatalf("k8sjob block parsed wrong: %+v", j)
	}
}

func TestK8sJobDeployer_RequiresBlock(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	body := `
apps:
  - name: myapp
    deployer: k8sjob
    rings:
      int: { health_url: "http://int/health" }
`
	if _, err := Load(writeConfig(t, body)); err == nil {
		t.Fatal("expected error: k8sjob deployer without k8sjob block")
	}
}

func TestK8sJobDeployer_RequiresImage(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	body := `
apps:
  - name: myapp
    deployer: k8sjob
    k8sjob: { namespace: ring-exec }
    rings:
      int: { health_url: "http://int/health" }
`
	if _, err := Load(writeConfig(t, body)); err == nil {
		t.Fatal("expected error: k8sjob block without image")
	}
}

// An explicit retries: 0 must be honored (no retries), not defaulted away.
func TestK8sJobDeployer_ZeroRetriesHonored(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	body := `
apps:
  - name: myapp
    deployer: k8sjob
    k8sjob:
      image: ghcr.io/x/runner:v1
      retries: 0
    rings:
      int: { health_url: "http://int/health" }
`
	cfg, err := Load(writeConfig(t, body))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	app, _ := cfg.App("myapp")
	if got := app.K8sJob.ResolvedRetries(); got != 0 {
		t.Fatalf("retries = %d, want 0", got)
	}
}

// ---- declarative auto-promote ----

// A ring's auto_promote parses into three distinguishable states: absent (config
// does not own it), explicit true, explicit false. The absent/false distinction
// is what keeps configs written before this field behaving unchanged.
func TestAutoPromote_AbsentTrueFalse(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	cfg, err := Load(writeConfig(t, `
apps:
  - name: web
    rings:
      int:  { namespace: r0, deployment: web, container: web, image: repo/web, health_url: "http://x/h" }
      test: { namespace: r1, deployment: web, container: web, image: repo/web, health_url: "http://x/h", auto_promote: true }
      acc:  { namespace: r2, deployment: web, container: web, image: repo/web, health_url: "http://x/h", auto_promote: false }
`))
	if err != nil {
		t.Fatal(err)
	}
	rings := cfg.Apps[0].Rings
	if rings["int"].AutoPromote != nil {
		t.Fatal("int omits auto_promote: want nil (config does not own it)")
	}
	if rings["int"].AutoPromoteOwnedByConfig() {
		t.Fatal("int must not report as config-owned")
	}
	if v := rings["test"].AutoPromote; v == nil || !*v {
		t.Fatalf("test: want explicit true, got %v", v)
	}
	if v := rings["acc"].AutoPromote; v == nil || *v {
		t.Fatalf("acc: want explicit false, got %v", v)
	}
	if !rings["acc"].AutoPromoteOwnedByConfig() {
		t.Fatal("explicit false must still be config-owned")
	}
}

// The production guard: config must not be able to open the hands-free path
// into the prod ring, because that path is password-protected at the API and a
// config-declared value never passes through that check.
func TestAutoPromote_RejectsAutoPromoteIntoProd(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	_, err := Load(writeConfig(t, `
apps:
  - name: web
    rings:
      acc:  { namespace: r2, deployment: web, container: web, image: repo/web, health_url: "http://x/h", auto_promote: true }
      prod: { namespace: r3, deployment: web, container: web, image: repo/web, health_url: "http://x/h" }
`))
	if err == nil {
		t.Fatal("config enabling auto-promote into prod must be rejected")
	}
	if !strings.Contains(err.Error(), "production password") {
		t.Fatalf("error should explain the bypass, got: %v", err)
	}
	// Explicit false into prod is fine — it only ever makes things safer.
	if _, err := Load(writeConfig(t, `
apps:
  - name: web
    rings:
      acc:  { namespace: r2, deployment: web, container: web, image: repo/web, health_url: "http://x/h", auto_promote: false }
      prod: { namespace: r3, deployment: web, container: web, image: repo/web, health_url: "http://x/h" }
`)); err != nil {
		t.Fatalf("auto_promote: false into prod must be allowed: %v", err)
	}
}

// auto_promote: true needs somewhere to go: not the last ring, and not a next
// ring this app does not configure.
func TestAutoPromote_RequiresConfiguredNextRing(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	// prod is last: nothing to promote into.
	if _, err := Load(writeConfig(t, `
apps:
  - name: web
    rings:
      prod: { namespace: r3, deployment: web, container: web, image: repo/web, health_url: "http://x/h", auto_promote: true }
`)); err == nil {
		t.Fatal("auto_promote on the last ring must be rejected")
	}
	// int's next ring (test) is not configured for this app.
	if _, err := Load(writeConfig(t, `
apps:
  - name: web
    rings:
      int: { namespace: r0, deployment: web, container: web, image: repo/web, health_url: "http://x/h", auto_promote: true }
      acc: { namespace: r2, deployment: web, container: web, image: repo/web, health_url: "http://x/h" }
`)); err == nil {
		t.Fatal("auto_promote with an unconfigured next ring must be rejected")
	}
}
