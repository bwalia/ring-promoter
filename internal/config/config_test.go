package config

import (
	"os"
	"path/filepath"
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
