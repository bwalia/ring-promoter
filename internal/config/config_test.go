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
      ring0: { namespace: ring0, deployment: web, container: web, image: repo/web, health_url: "http://x/health" }
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
