package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/deployer"
	"github.com/example/ring-promoter/internal/health"
	"github.com/example/ring-promoter/internal/pipeline"
	"github.com/example/ring-promoter/internal/promoter"
	"github.com/example/ring-promoter/internal/ring"
	"github.com/example/ring-promoter/internal/store"
)

const apiTrivySARIF = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"Trivy","rules":[
 {"id":"CVE-2024-9","properties":{"security-severity":"8.1"}}]}},
 "results":[{"ruleId":"CVE-2024-9","message":{"text":"x/net"}}]}]}`

// The top-level security_policy blocks a promotion into test until the
// required scans are clean (or the finding is waived).
func TestAPI_SecurityGate(t *testing.T) {
	policy := &config.SecurityPolicy{Rings: []string{"test"}, Require: []string{"sca", "secrets"}}
	h := newSecurityServer(t, policy)

	if rec := doJSON(t, h, "POST", "/api/apps/web/seed", `{"ring":"int","version":"v1"}`); rec.Code != http.StatusOK {
		t.Fatalf("seed int: %d %s", rec.Code, rec.Body)
	}
	rec := doJSON(t, h, "POST", "/api/apps/web/promote", `{"from_ring":"int"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "security scan missing") {
		t.Fatalf("promote without scans: want 409, got %d %s", rec.Code, rec.Body)
	}

	// SARIF upload: one high CVE.
	body := `{"app":"web","version":"v1","kind":"sca","url":"https://ci/run/1","sarif":` + apiTrivySARIF + `}`
	rec = doJSON(t, h, "POST", "/api/security/scans", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("record scan: %d %s", rec.Code, rec.Body)
	}
	var scan store.SecurityScan
	_ = json.Unmarshal(rec.Body.Bytes(), &scan)
	if scan.Scanner != "Trivy" || scan.High != 1 || len(scan.Findings) != 1 || scan.ID == "" {
		t.Fatalf("scan = %+v", scan)
	}
	// Counts-only report.
	if rec := doJSON(t, h, "POST", "/api/security/scans", `{"app":"web","version":"v1","kind":"secrets","scanner":"gitleaks"}`); rec.Code != http.StatusCreated {
		t.Fatalf("record counts scan: %d %s", rec.Code, rec.Body)
	}
	for _, bad := range []string{
		`{"app":"web","version":"v1","kind":"fuzz","scanner":"x"}`,
		`{"app":"web","kind":"sca","scanner":"x"}`,
		`{"app":"web","version":"v1","kind":"sca","scanner":"x","bogus":1}`,
	} {
		if rec := doJSON(t, h, "POST", "/api/security/scans", bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad scan %s: want 400, got %d", bad, rec.Code)
		}
	}
	if rec := doJSON(t, h, "POST", "/api/security/scans", `{"app":"nope","version":"v1","kind":"sca","scanner":"x"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown app: want 404, got %d", rec.Code)
	}

	rec = doJSON(t, h, "POST", "/api/apps/web/promote", `{"from_ring":"int"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "security policy is no-go") {
		t.Fatalf("promote with a high CVE: want 409, got %d %s", rec.Code, rec.Body)
	}

	// Evaluate and overview.
	rec = doJSON(t, h, "GET", "/api/apps/web/security/evaluate?version=v1&ring=test", "")
	var ev promoter.SecurityEvaluation
	_ = json.Unmarshal(rec.Body.Bytes(), &ev)
	if rec.Code != http.StatusOK || ev.Verdict != "no_go" || !ev.Guarded || len(ev.Checks) != 2 || ev.Counts.High != 1 {
		t.Fatalf("evaluate: %d %+v", rec.Code, ev)
	}
	if rec := doJSON(t, h, "GET", "/api/apps/web/security/evaluate", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("evaluate without version: %d", rec.Code)
	}

	// Waive the CVE; the promotion goes through.
	rec = doJSON(t, h, "POST", "/api/apps/web/security/waivers", `{"finding":"CVE-2024-9","reason":"not reachable","days":14}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create waiver: %d %s", rec.Code, rec.Body)
	}
	var wv store.SecurityWaiver
	_ = json.Unmarshal(rec.Body.Bytes(), &wv)
	if rec := doJSON(t, h, "POST", "/api/apps/web/security/waivers", `{"finding":"CVE-2024-9"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("waiver without reason: want 400, got %d", rec.Code)
	}
	if rec := doJSON(t, h, "POST", "/api/apps/web/promote", `{"from_ring":"int"}`); rec.Code != http.StatusOK {
		t.Fatalf("promote after waiver: want 200, got %d %s", rec.Code, rec.Body)
	}

	rec = doJSON(t, h, "GET", "/api/apps/web/security", "")
	var sec promoter.AppSecurity
	if err := json.Unmarshal(rec.Body.Bytes(), &sec); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("security: %d %s", rec.Code, rec.Body)
	}
	if sec.Policy.Source != "default" || len(sec.Scans) != 2 || len(sec.Waivers) != 1 || len(sec.Posture) != len(ring.Names()) {
		t.Fatalf("security = %+v", sec)
	}
	for _, p := range sec.Posture {
		if p.Ring == "test" && (p.Version != "v1" || p.Verdict != "go" || p.Waived != 1 || !p.Guarded) {
			t.Fatalf("test posture = %+v", p)
		}
	}
	waivedMarked := false
	for _, s := range sec.Scans {
		for _, f := range s.Findings {
			waivedMarked = waivedMarked || f.Waived
		}
	}
	if !waivedMarked {
		t.Fatal("waived finding not marked")
	}

	if rec := doJSON(t, h, "DELETE", "/api/apps/web/security/waivers/"+wv.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete waiver: %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, "DELETE", "/api/apps/web/security/waivers/"+wv.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("delete again: want 404, got %d", rec.Code)
	}
}

func newSecurityServer(t *testing.T, policy *config.SecurityPolicy) http.Handler {
	t.Helper()
	rings := map[string]config.RingConfig{}
	for _, r := range ring.Names() {
		rings[r] = config.RingConfig{Namespace: r, Deployment: "web", Container: "web", Image: "repo/web", HealthURL: "health://web/" + r}
	}
	zero := 0
	delay := config.Duration(time.Millisecond)
	cfg := &config.Config{
		APIToken: "tok", Retry: config.RetryConfig{Count: &zero, Delay: &delay},
		Apps:           []config.AppConfig{{Name: "web", Rings: rings}},
		SecurityPolicy: policy,
		Pipelines: []config.PipelineConfig{{Name: "secure", Stages: []config.PipelineStage{
			{Name: "appsec", Type: config.StageGate, Gate: config.GateSecurity, Apps: []string{"web"}, Timeout: config.Duration(time.Minute)},
			{Name: "deploy", Apps: []string{"web"}, Needs: []string{"appsec"}},
		}}},
	}
	prom := promoter.New(cfg, store.NewMemory(), nil, deployer.NewLogDeployer(nil), health.AlwaysHealthy{}, nil)
	srv := NewServer(prom, "tok", "", http.NotFoundHandler(), time.Minute, nil, BuildInfo{}, nil)
	srv.pipes.Poll = 5 * time.Millisecond
	return srv.Handler()
}

// A pipeline security gate waits for the scans of the requested version, then
// lets the deploy stage run; its item carries the per-kind checks.
func TestAPI_PipelineSecurityGate(t *testing.T) {
	h := newSecurityServer(t, nil) // built-in default: sast, sca, secrets
	rec := doJSON(t, h, "POST", "/api/pipelines/secure/runs", `{"ring":"int","versions":{"web":"v2"}}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	run := decodeRun(t, rec.Body.Bytes())
	if run.Stages[0].Gate != "security" {
		t.Fatalf("stage gate = %q", run.Stages[0].Gate)
	}
	r := pollRun(t, h, run.ID, func(r pipeline.Run) bool { return r.Stages[0].Status == pipeline.StatusWaiting })
	if it := r.Stages[0].Items[0]; it.Version != "v2" || !strings.Contains(it.Detail, "sast missing") {
		t.Fatalf("waiting item = %+v", it)
	}
	for _, k := range []string{"sast", "sca", "secrets"} {
		if rec := doJSON(t, h, "POST", "/api/security/scans", `{"app":"web","version":"v2","kind":"`+k+`","scanner":"s"}`); rec.Code != http.StatusCreated {
			t.Fatalf("scan %s: %d", k, rec.Code)
		}
	}
	r = pollRun(t, h, run.ID, func(r pipeline.Run) bool { return r.FinishedAt != nil })
	if r.Status != pipeline.StatusSuccess {
		t.Fatalf("run = %+v", r)
	}
	it := r.Stages[0].Items[0]
	var checks []promoter.SecurityCheck
	if err := json.Unmarshal(it.Checks, &checks); err != nil || len(checks) != 3 || checks[0].Status != "pass" || it.Verdict != "go" {
		t.Fatalf("gate item = %+v (%v)", it, err)
	}
}
