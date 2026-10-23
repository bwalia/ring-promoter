package config

import (
	"strings"
	"testing"
	"time"
)

func TestSecurityPolicy_Load(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	cfg, err := Load(writeConfig(t, `
security_policy:
  require: [sast, secrets]
  max: { critical: 0, high: 2 }
  max_age: 168h
apps:
  - name: db
    rings:
      int: { namespace: ring0, deployment: db, container: db, image: repo/db, health_url: "http://x/health" }
  - name: web
    promotion_policy:
      security: { rings: [prod], require: [] }
    rings:
      int: { namespace: ring0, deployment: web, container: web, image: repo/web, health_url: "http://x/health" }
pipelines:
  - name: release
    stages:
      - { name: deploy, apps: [db] }
      - { name: appsec, type: gate, gate: security, needs: [deploy] }
`))
	if err != nil {
		t.Fatal(err)
	}
	pol, src, ok := cfg.SecurityPolicyFor("db")
	if !ok || src != "default" || pol.MaxAgeStd() != 168*time.Hour || pol.Limits()["high"] != 2 {
		t.Fatalf("db policy = %+v %q %v", pol, src, ok)
	}
	if !pol.Guards("acc") || pol.Guards("int") {
		t.Fatal("default rings should be acc, prod")
	}
	pol, src, _ = cfg.SecurityPolicyFor("web")
	if src != "app" || len(pol.RequiredKinds()) != 0 || !pol.Guards("prod") || pol.Guards("acc") {
		t.Fatalf("web policy = %+v %q", pol, src)
	}
	if got := pol.Limits(); got["critical"] != 0 || got["high"] != 0 {
		t.Fatalf("default limits = %v", got)
	}
	p, _ := cfg.Pipeline("release")
	if p.Stages[1].ResolvedGate() != GateSecurity {
		t.Fatal("gate kind not kept")
	}
}

func TestSecurityPolicy_Invalid(t *testing.T) {
	cases := []struct{ name, block, want string }{
		{"kind", `security_policy: { require: [fuzz] }`, "unknown scan kind"},
		{"dup kind", `security_policy: { require: [sast, sast] }`, "twice"},
		{"severity", `security_policy: { max: { severe: 1 } }`, "unknown severity"},
		{"negative", `security_policy: { max: { high: -1 } }`, "must not be negative"},
		{"ring", `security_policy: { rings: [moon] }`, "unknown ring"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RP_API_TOKEN", "tok")
			_, err := Load(writeConfig(t, tc.block+"\n"+pipelineApps))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
	t.Run("app policy", func(t *testing.T) {
		t.Setenv("RP_API_TOKEN", "tok")
		_, err := Load(writeConfig(t, `
apps:
  - name: web
    promotion_policy:
      security: { require: [nope] }
    rings:
      int: { namespace: ring0, deployment: web, container: web, image: repo/web, health_url: "http://x/health" }
`))
		if err == nil || !strings.Contains(err.Error(), "promotion_policy.security") {
			t.Fatalf("err = %v", err)
		}
	})
}
