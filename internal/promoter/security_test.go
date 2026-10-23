package promoter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/store"
)

func securityHarness(t *testing.T, now time.Time, pol *config.SecurityPolicy, appLevel bool) (*Promoter, *fakeDeployer) {
	t.Helper()
	dep := newFakeDeployer()
	chk := newScriptedChecker(dep)
	clock := func() time.Time { return now }
	st := store.NewMemoryWithClock(clock)
	cfg := testConfig(1)
	if appLevel {
		cfg.Apps[0].PromotionPolicy = &config.PromotionPolicy{Security: pol}
	} else {
		cfg.SecurityPolicy = pol
	}
	p := New(cfg, st, nil, dep, chk, nil)
	p.now = clock
	return p, dep
}

func scan(t *testing.T, p *Promoter, version, kind string, findings ...store.SecurityFinding) {
	t.Helper()
	if findings == nil {
		findings = []store.SecurityFinding{}
	}
	if _, err := p.RecordSecurityScan(context.Background(), SecurityScanInput{
		App: testApp, Version: version, Kind: kind, Scanner: kind + "-scanner", Findings: findings,
	}); err != nil {
		t.Fatal(err)
	}
}

// The top-level security_policy guards an app with no promotion_policy:
// missing scans, then a high finding block; a waiver lets it through.
func TestSecurityGate_DefaultPolicy(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, dep := securityHarness(t, now, &config.SecurityPolicy{Rings: []string{"test"}, Require: []string{"sast", "sca"}}, false)
	ctx := context.Background()
	mustSeed(t, p, "int", "v1") // int is not guarded

	before := dep.deployCount()
	if _, err := p.Promote(ctx, testApp, "int"); !errors.Is(err, ErrSecurityScanMissing) {
		t.Fatalf("want ErrSecurityScanMissing, got %v", err)
	}
	scan(t, p, "v1", "sast")
	scan(t, p, "v1", "sca", store.SecurityFinding{ID: "CVE-1", Severity: "high"}, store.SecurityFinding{ID: "CVE-2", Severity: "medium"})
	if _, err := p.Promote(ctx, testApp, "int"); !errors.Is(err, ErrSecurityNoGo) {
		t.Fatalf("want ErrSecurityNoGo, got %v", err)
	}
	if dep.deployCount() != before {
		t.Fatal("deployed despite security no-go")
	}
	ev, _ := p.EvaluateSecurity(ctx, testApp, "test", "v1")
	if ev.Verdict != SecurityNoGo || ev.Counts.High != 1 || ev.Checks[1].Status != CheckFail || !ev.Guarded {
		t.Fatalf("eval = %+v", ev)
	}

	// A waiver for another ring does not help; one for test (or all rings) does.
	if _, err := p.CreateSecurityWaiver(ctx, testApp, "CVE-1", "acc", "no fix", "sec", time.Time{}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Promote(ctx, testApp, "int"); !errors.Is(err, ErrSecurityNoGo) {
		t.Fatalf("acc waiver must not open test: %v", err)
	}
	if _, err := p.CreateSecurityWaiver(ctx, testApp, "CVE-1", "", "no fix upstream", "sec", time.Time{}, 7); err != nil {
		t.Fatal(err)
	}
	res, err := p.Promote(ctx, testApp, "int")
	if err != nil || !res.Success {
		t.Fatalf("promote after waiver: %+v %v", res, err)
	}
}

func TestSecurityEvaluation_StaleAndExpiredWaiver(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, _ := securityHarness(t, now, &config.SecurityPolicy{Require: []string{"sast"}, MaxAge: config.Duration(time.Hour)}, true)
	ctx := context.Background()
	if _, err := p.RecordSecurityScan(ctx, SecurityScanInput{App: testApp, Version: "v1", Kind: "sast", Scanner: "semgrep",
		ScannedAt: now.Add(-2 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	ev, _ := p.EvaluateSecurity(ctx, testApp, "prod", "v1")
	if ev.Verdict != SecurityNoGo || ev.Checks[0].Status != CheckStale {
		t.Fatalf("eval = %+v", ev)
	}

	// Counts-only report (no findings) is taken as given.
	if _, err := p.RecordSecurityScan(ctx, SecurityScanInput{App: testApp, Version: "v2", Kind: "sast", Scanner: "semgrep",
		Counts: SeverityCounts{Critical: 1}}); err != nil {
		t.Fatal(err)
	}
	ev, _ = p.EvaluateSecurity(ctx, testApp, "prod", "v2")
	if ev.Verdict != SecurityNoGo || ev.Counts.Critical != 1 {
		t.Fatalf("eval = %+v", ev)
	}
	// Nothing deployed -> none; nothing scanned -> pending.
	if ev, _ := p.EvaluateSecurity(ctx, testApp, "prod", ""); ev.Verdict != SecurityNone {
		t.Fatalf("none = %+v", ev)
	}
	if ev, _ := p.EvaluateSecurity(ctx, testApp, "prod", "v9"); ev.Verdict != SecurityPending {
		t.Fatalf("pending = %+v", ev)
	}
}

func TestSecurityWaiver_Validation(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, _ := securityHarness(t, now, nil, false)
	ctx := context.Background()
	cases := []struct {
		finding, ring, reason string
		exp                   time.Time
		days                  int
		want                  error
	}{
		{"", "", "r", time.Time{}, 0, ErrInvalidWaiver},
		{"CVE", "", "", time.Time{}, 0, ErrInvalidWaiver},
		{"CVE", "", "r", time.Time{}, 91, ErrInvalidWaiver},
		{"CVE", "", "r", now.Add(-time.Hour), 0, ErrInvalidWaiver},
		{"CVE", "moon", "r", time.Time{}, 0, ErrRingNotConfigured},
	}
	for i, c := range cases {
		if _, err := p.CreateSecurityWaiver(ctx, testApp, c.finding, c.ring, c.reason, "x", c.exp, c.days); !errors.Is(err, c.want) {
			t.Fatalf("case %d: err = %v, want %v", i, err, c.want)
		}
	}
	w, err := p.CreateSecurityWaiver(ctx, testApp, "CVE", "", "r", "x", time.Time{}, 0)
	if err != nil || !w.ExpiresAt.Equal(now.Add(30*24*time.Hour)) {
		t.Fatalf("default expiry: %+v %v", w, err)
	}
	if err := p.DeleteSecurityWaiver(ctx, testApp, w.ID); err != nil {
		t.Fatal(err)
	}
	if err := p.DeleteSecurityWaiver(ctx, testApp, w.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("second delete = %v", err)
	}
}

// With no policy configured the promotion gate is off, but posture and the
// pipeline gate use the built-in default.
func TestAppSecurity_BuiltinPostureAndPipelineVersion(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, _ := securityHarness(t, now, nil, false)
	ctx := context.Background()
	mustSeed(t, p, "int", "v1")
	if _, err := p.Promote(ctx, testApp, "int"); err != nil {
		t.Fatalf("unguarded promote: %v", err)
	}
	for _, k := range []string{"sast", "sca", "secrets"} {
		scan(t, p, "v1", k)
	}
	sec, err := p.AppSecurity(ctx, testApp)
	if err != nil {
		t.Fatal(err)
	}
	if sec.Policy.Source != "builtin" || len(sec.Scans) != 3 {
		t.Fatalf("security = %+v", sec)
	}
	for _, ev := range sec.Posture {
		if ev.Guarded {
			t.Fatalf("builtin must not guard: %+v", ev)
		}
		if ev.Ring == "int" && ev.Verdict != SecurityGo {
			t.Fatalf("int posture = %+v", ev)
		}
	}
	// Pipeline into acc with no version checks test's current version (v1).
	ev, err := p.SecurityVerdict(ctx, testApp, "acc", "")
	if err != nil || ev.Version != "v1" || ev.Verdict != SecurityGo {
		t.Fatalf("pipeline eval = %+v %v", ev, err)
	}
}

func TestRecordSecurityScan_Validation(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, _ := securityHarness(t, now, nil, false)
	ctx := context.Background()
	bad := []SecurityScanInput{
		{App: "nope", Version: "v", Kind: "sast", Scanner: "s"},
		{App: testApp, Kind: "sast", Scanner: "s"},
		{App: testApp, Version: "v", Kind: "fuzz", Scanner: "s"},
		{App: testApp, Version: "v", Kind: "sast"},
		{App: testApp, Version: "v", Kind: "sast", Scanner: "s", Findings: []store.SecurityFinding{{ID: "x", Severity: "meh"}}},
		{App: testApp, Version: "v", Kind: "sast", Scanner: "s", Counts: SeverityCounts{High: -1}},
	}
	for i, in := range bad {
		if _, err := p.RecordSecurityScan(ctx, in); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
	s, err := p.RecordSecurityScan(ctx, SecurityScanInput{App: testApp, Version: "v", Kind: "SCA", SARIF: []byte(trivySARIF)})
	if err != nil || s.Scanner != "Trivy" || s.Critical != 2 || s.High != 1 || s.Medium != 1 || len(s.Findings) != 4 {
		t.Fatalf("sarif scan = %+v %v", s, err)
	}
}
