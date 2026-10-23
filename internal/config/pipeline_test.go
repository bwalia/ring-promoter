package config

import (
	"strings"
	"testing"
	"time"
)

const pipelineApps = `
apps:
  - name: db
    rings:
      int: { namespace: ring0, deployment: db, container: db, image: repo/db, health_url: "http://x/health" }
  - name: web
    rings:
      int: { namespace: ring0, deployment: web, container: web, image: repo/web, health_url: "http://x/health" }
`

func TestPipelines_Valid(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	cfg, err := Load(writeConfig(t, pipelineApps+`
pipelines:
  - name: release
    display_name: Release
    stages:
      - { name: database, apps: [db] }
      - { name: qa, type: gate, needs: [database] }
      - { name: approve, type: approval, needs: [qa], timeout: 1h }
      - { name: frontend, apps: [web], needs: [approve] }
`))
	if err != nil {
		t.Fatal(err)
	}
	p, ok := cfg.Pipeline("release")
	if !ok || p.Title() != "Release" || len(p.Stages) != 4 {
		t.Fatalf("pipeline = %+v", p)
	}
	if got := p.Stages[0].ResolvedType(); got != StageDeploy {
		t.Fatalf("default type = %q", got)
	}
	if got := p.Stages[1].ResolvedGate(); got != GateQA {
		t.Fatalf("default gate = %q", got)
	}
	if got := p.Stages[1].ResolvedTimeout(); got != DefaultGateTimeout {
		t.Fatalf("gate timeout = %s", got)
	}
	if got := p.Stages[2].ResolvedTimeout(); got != time.Hour {
		t.Fatalf("approval timeout = %s", got)
	}
}

func TestPipelines_Invalid(t *testing.T) {
	cases := []struct {
		name, yaml, want string
	}{
		{"duplicate pipeline", `
  - { name: p, stages: [{ name: a, apps: [db] }] }
  - { name: p, stages: [{ name: a, apps: [db] }] }`, "duplicate pipeline"},
		{"no stages", `
  - { name: p }`, "has no stages"},
		{"duplicate stage", `
  - { name: p, stages: [{ name: a, apps: [db] }, { name: a, apps: [web] }] }`, "duplicate stage"},
		{"unknown type", `
  - { name: p, stages: [{ name: a, type: build, apps: [db] }] }`, "unknown type"},
		{"deploy without apps", `
  - { name: p, stages: [{ name: a }] }`, "at least one app"},
		{"approval with apps", `
  - { name: p, stages: [{ name: a, type: approval, apps: [db] }] }`, "takes no apps"},
		{"unknown gate", `
  - { name: p, stages: [{ name: a, type: gate, gate: grafana }] }`, "unknown gate"},
		{"unknown app", `
  - { name: p, stages: [{ name: a, apps: [nope] }] }`, "unknown application"},
		{"unknown need", `
  - { name: p, stages: [{ name: a, apps: [db], needs: [zzz] }] }`, "needs unknown stage"},
		{"self need", `
  - { name: p, stages: [{ name: a, apps: [db], needs: [a] }] }`, "must not need itself"},
		{"cycle", `
  - { name: p, stages: [{ name: a, apps: [db], needs: [b] }, { name: b, apps: [web], needs: [a] }] }`, "cycle"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("RP_API_TOKEN", "tok")
			_, err := Load(writeConfig(t, pipelineApps+"\npipelines:"+c.yaml+"\n"))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want containing %q", err, c.want)
			}
		})
	}
}
