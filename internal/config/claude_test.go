package config

import "testing"

func TestClaude_OffByDefault(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	cfg, err := Load(writeConfig(t, baseApps))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Claude.Enabled() {
		t.Fatal("expected claude off without an api key")
	}
	if cfg.Claude.Model != "claude-opus-5-5" {
		t.Fatalf("model = %q, want default claude-opus-5-5", cfg.Claude.Model)
	}
}

func TestClaude_FromFileAndEnv(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	cfg, err := Load(writeConfig(t, "claude:\n  model: claude-sonnet-5-5\n"+baseApps))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Claude.Enabled() || cfg.Claude.Model != "claude-sonnet-5-5" {
		t.Fatalf("file config: %+v", cfg.Claude)
	}

	t.Setenv("RP_CLAUDE_API_KEY", "sk-test")
	t.Setenv("RP_CLAUDE_MODEL", "claude-haiku-4-5")
	cfg, err = Load(writeConfig(t, "claude:\n  model: claude-sonnet-5-5\n"+baseApps))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Claude.Enabled() || cfg.Claude.APIKey != "sk-test" || cfg.Claude.Model != "claude-haiku-4-5" {
		t.Fatalf("env should override file: %+v", cfg.Claude)
	}
}
