package config

import "testing"

func TestQAAgent_AbsentIsOff(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	cfg, err := Load(writeConfig(t, baseApps))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.QAAgent.Enabled() {
		t.Fatal("expected qa agent off when block absent")
	}
}

func TestQAAgent_EmptyBlockIsOff(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	cfg, err := Load(writeConfig(t, "qa_agent:\n"+baseApps))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.QAAgent != nil {
		t.Fatalf("empty qa_agent block should be nilled, got %+v", cfg.QAAgent)
	}
}

func TestQAAgent_RequiresName(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	if _, err := Load(writeConfig(t, "qa_agent:\n  url: https://qa.example.com\n"+baseApps)); err == nil {
		t.Fatal("expected error when name missing")
	}
}

func TestQAAgent_Valid(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	cfg, err := Load(writeConfig(t, `
qa_agent:
  name: qa-bot
  url: https://qa.example.com
  apps: [web]
`+baseApps))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.QAAgent.Enabled() {
		t.Fatal("expected enabled")
	}
	if !cfg.QAAgent.Watches("web") {
		t.Fatal("expected watches web")
	}
	if cfg.QAAgent.Watches("other") {
		t.Fatal("other is out of scope")
	}
}

func TestQAAgent_UnknownApp(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	if _, err := Load(writeConfig(t, `
qa_agent:
  name: qa-bot
  apps: [missing]
`+baseApps)); err == nil {
		t.Fatal("expected error for unknown app")
	}
}

func TestQAAgent_AllAppsWhenEmpty(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	cfg, err := Load(writeConfig(t, `
qa_agent:
  name: qa-bot
`+baseApps))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.QAAgent.Watches("web") {
		t.Fatal("empty apps list should watch every app")
	}
}
