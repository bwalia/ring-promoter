package promoter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/grafana"
)

// stubGrafana answers Query from a fixed value/error and counts calls, so tests
// can assert both the verdict and that verdicts are cached.
type stubGrafana struct {
	value float64
	// at dates the run behind the value; zero means "the query selected no
	// timestamp", which the staleness rule treats as unknown.
	at    time.Time
	err   error
	calls int
	last  grafana.Query
}

func (s *stubGrafana) Query(_ context.Context, q grafana.Query) (grafana.Sample, error) {
	s.calls++
	s.last = q
	if s.err != nil {
		return grafana.Sample{}, s.err
	}
	return grafana.Sample{Value: s.value, At: s.at}, nil
}

func float64Ptr(v float64) *float64 { return &v }

// grafanaPolicy builds a live (non-demo) Grafana gate on the "test" ring, with
// the diytaxreturn Go/No-Go panel's thresholds: 2 = go, 0 = no-go.
func grafanaPolicy() *config.PromotionPolicy {
	return &config.PromotionPolicy{
		Grafana: &config.GrafanaPolicy{
			Rings:         []string{"test"},
			URL:           "https://grafana.example.com",
			DashboardUID:  "sre-build-release-status",
			DashboardName: "Build & Release Status",
			DatasourceUID: "QA-PostgreSQL",
			GoMin:         float64Ptr(2),
			NoGoMax:       float64Ptr(0),
			Checks: []config.GrafanaCheck{
				{Name: "Register & Login", Query: "SELECT ok FROM runs WHERE environment ~ '${env:raw}'"},
			},
		},
	}
}

// A dashboard reporting no-go blocks the promotion before anything is deployed;
// a go verdict lets the same promotion through.
func TestGrafanaGate_BlocksAndAllows(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, dep, _ := gatedHarness(t, now, grafanaPolicy())
	stub := &stubGrafana{value: 0} // 0 = NO GO
	p.SetGrafanaClient(testApp, stub)
	ctx := context.Background()
	mustSeed(t, p, "int", "v1")

	before := dep.deployCount()
	if _, err := p.Promote(ctx, testApp, "int"); !errors.Is(err, ErrGrafanaNoGo) {
		t.Fatalf("want ErrGrafanaNoGo, got %v", err)
	}
	if dep.deployCount() != before {
		t.Fatal("target deployed despite a no-go verdict")
	}

	// A go verdict releases the gate. The cache is keyed per app/ring, so it
	// must be cleared for the new value to be seen.
	stub.value = 2
	p.SetGrafanaClient(testApp, stub)
	if _, err := p.Promote(ctx, testApp, "int"); err != nil {
		t.Fatalf("promote with a go verdict: %v", err)
	}
}

// A "check" verdict (the dashboard's PENDING) is advisory: it is shown, but it
// does not block. Neither does a Grafana that cannot be reached — an
// observability outage must not become a release outage.
func TestGrafanaGate_AdvisoryVerdictsDoNotBlock(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	for name, stub := range map[string]*stubGrafana{
		"pending":     {value: 1},
		"unreachable": {err: fmt.Errorf("dial tcp: connection refused")},
	} {
		t.Run(name, func(t *testing.T) {
			p, _, _ := gatedHarness(t, now, grafanaPolicy())
			p.SetGrafanaClient(testApp, stub)
			mustSeed(t, p, "int", "v1")
			if _, err := p.Promote(context.Background(), testApp, "int"); err != nil {
				t.Fatalf("promote should not be blocked by a %s verdict: %v", name, err)
			}
		})
	}
}

// A no-go may be overruled, but only with a stated reason.
func TestGrafanaGate_Override(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, dep, _ := gatedHarness(t, now, grafanaPolicy())
	p.SetGrafanaClient(testApp, &stubGrafana{value: 0})
	mustSeed(t, p, "int", "v1")

	// Override without a reason is refused, and nothing is deployed.
	before := dep.deployCount()
	ctx := WithGateInputs(context.Background(), GateInputs{OverrideGrafana: true})
	if _, err := p.Promote(ctx, testApp, "int"); !errors.Is(err, ErrGrafanaOverrideReason) {
		t.Fatalf("want ErrGrafanaOverrideReason, got %v", err)
	}
	if dep.deployCount() != before {
		t.Fatal("target deployed on a reasonless override")
	}

	// With a reason, the same promotion goes through.
	ctx = WithGateInputs(context.Background(), GateInputs{
		OverrideGrafana: true,
		OverrideReason:  "known flaky e2e, re-run passed",
	})
	if _, err := p.Promote(ctx, testApp, "int"); err != nil {
		t.Fatalf("override with a reason: %v", err)
	}
}

// The override applies ONLY to the Grafana gate: it must not wave through a
// closed maintenance window or a missing sign-off.
func TestGrafanaGate_OverrideDoesNotBypassOtherGates(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	pol := grafanaPolicy()
	pol.QASignoff = &config.GatePolicy{Rings: []string{"test"}}
	p, _, _ := gatedHarness(t, now, pol)
	p.SetGrafanaClient(testApp, &stubGrafana{value: 2}) // grafana says go
	mustSeed(t, p, "int", "v1")

	ctx := WithGateInputs(context.Background(), GateInputs{
		OverrideGrafana: true,
		OverrideReason:  "trying to sneak past QA",
	})
	if _, err := p.Promote(ctx, testApp, "int"); !errors.Is(err, ErrSignoffRequired) {
		t.Fatalf("want ErrSignoffRequired despite the grafana override, got %v", err)
	}
}

// Verdicts are cached: the rings endpoint is polled every few seconds and must
// not turn into the same rate of queries against Grafana.
func TestGrafanaGate_VerdictIsCached(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, _, _ := gatedHarness(t, now, grafanaPolicy())
	stub := &stubGrafana{value: 2}
	p.SetGrafanaClient(testApp, stub)
	ctx := context.Background()

	for range 5 {
		if _, err := p.Rings(ctx, testApp); err != nil {
			t.Fatal(err)
		}
	}
	if stub.calls != 1 {
		t.Fatalf("want 1 grafana query for 5 polls, got %d", stub.calls)
	}

	// Past the TTL the verdict is refetched.
	p.now = func() time.Time { return now.Add(grafanaCacheTTL + time.Second) }
	if _, err := p.Rings(ctx, testApp); err != nil {
		t.Fatal(err)
	}
	if stub.calls != 2 {
		t.Fatalf("want a refetch after the TTL, got %d queries", stub.calls)
	}
}

// The ring view carries the verdict, so the UI can draw the gate between cards.
func TestGrafanaGate_RingViewCarriesVerdict(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, _, _ := gatedHarness(t, now, grafanaPolicy())
	p.SetGrafanaClient(testApp, &stubGrafana{value: 0})

	views, err := p.Rings(context.Background(), testApp)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		switch v.Ring.Name {
		case "test":
			if !v.Gates.Grafana || v.Gates.GrafanaStatus == nil {
				t.Fatal("gated ring reports no grafana gate")
			}
			if got := v.Gates.GrafanaStatus.Verdict; got != grafana.VerdictNoGo {
				t.Fatalf("want no_go verdict, got %q", got)
			}
			if v.Gates.GrafanaStatus.Dashboard != "Build & Release Status" {
				t.Fatalf("dashboard name missing: %+v", v.Gates.GrafanaStatus)
			}
		default:
			if v.Gates.Grafana {
				t.Fatalf("ring %q is not gated but reports a grafana gate", v.Ring.Name)
			}
		}
	}
}

// perCheckGrafana answers each check from a per-query-name map, so a test can
// make one suite red while the rest pass.
type perCheckGrafana struct {
	byQuery map[string]grafana.Sample
}

func (s *perCheckGrafana) Query(_ context.Context, q grafana.Query) (grafana.Sample, error) {
	sample, ok := s.byQuery[q.Expr]
	if !ok {
		return grafana.Sample{}, fmt.Errorf("no stub for %q", q.Expr)
	}
	return sample, nil
}

// e2ePolicy mirrors the real diytaxreturn shape: several nightly suites behind
// one gate, each its own check.
func e2ePolicy(maxAge time.Duration) *config.PromotionPolicy {
	return &config.PromotionPolicy{
		Grafana: &config.GrafanaPolicy{
			Rings:         []string{"test"},
			URL:           "https://grafana.example.com",
			DashboardUID:  "sre-e2e-test-suite",
			DashboardName: "DIY Tax Return — E2E Test Suite",
			DatasourceUID: "QA-PostgreSQL",
			GoMin:         float64Ptr(2),
			NoGoMax:       float64Ptr(0),
			MaxAge:        maxAge,
			Checks: []config.GrafanaCheck{
				{Name: "Register & Login", Query: "q-register"},
				{Name: "Full Tax Return Flow", Query: "q-fulltax"},
				{Name: "RBAC & Admin Access", Query: "q-rbac"},
			},
		},
	}
}

// ONE red suite blocks the whole gate, and the error names it — "which suite is
// red?" is the first question anyone asks.
func TestGrafanaGate_OneRedSuiteBlocksAndIsNamed(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	ranAt := now.Add(-4 * time.Hour) // last night's run
	p, _, _ := gatedHarness(t, now, e2ePolicy(36*time.Hour))
	p.SetGrafanaClient(testApp, &perCheckGrafana{byQuery: map[string]grafana.Sample{
		"q-register": {Value: 2, At: ranAt},
		"q-fulltax":  {Value: 0, At: ranAt}, // the one failure
		"q-rbac":     {Value: 2, At: ranAt},
	}})
	mustSeed(t, p, "int", "v1")

	_, err := p.Promote(context.Background(), testApp, "int")
	if !errors.Is(err, ErrGrafanaNoGo) {
		t.Fatalf("want ErrGrafanaNoGo, got %v", err)
	}
	if !strings.Contains(err.Error(), "Full Tax Return Flow") {
		t.Fatalf("error should name the failing suite, got: %v", err)
	}
	// ...and not the passing ones, or the message stops being a shortcut.
	if strings.Contains(err.Error(), "RBAC") {
		t.Fatalf("error should not list passing suites, got: %v", err)
	}
}

// All suites green -> the gate is green and lists every check for the UI.
func TestGrafanaGate_AllSuitesGreen(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	ranAt := now.Add(-4 * time.Hour)
	p, _, _ := gatedHarness(t, now, e2ePolicy(36*time.Hour))
	p.SetGrafanaClient(testApp, &perCheckGrafana{byQuery: map[string]grafana.Sample{
		"q-register": {Value: 2, At: ranAt},
		"q-fulltax":  {Value: 2, At: ranAt},
		"q-rbac":     {Value: 2, At: ranAt},
	}})
	mustSeed(t, p, "int", "v1")

	if _, err := p.Promote(context.Background(), testApp, "int"); err != nil {
		t.Fatalf("all-green gate should not block: %v", err)
	}

	views, _ := p.Rings(context.Background(), testApp)
	for _, v := range views {
		if v.Ring.Name != "test" {
			continue
		}
		st := v.Gates.GrafanaStatus
		if st.Verdict != grafana.VerdictGo {
			t.Fatalf("want go, got %q", st.Verdict)
		}
		if len(st.Checks) != 3 {
			t.Fatalf("want all 3 checks listed for the UI, got %d", len(st.Checks))
		}
	}
}

// The staleness rule is the point of a NIGHTLY gate: a suite that stopped
// running keeps returning its last conclusion forever, and without max_age that
// stale pass would read as a fresh GO.
func TestGrafanaGate_StaleRunIsNotAGo(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, _, _ := gatedHarness(t, now, e2ePolicy(36*time.Hour))
	// Every suite last PASSED — but three days ago.
	old := now.Add(-72 * time.Hour)
	p.SetGrafanaClient(testApp, &perCheckGrafana{byQuery: map[string]grafana.Sample{
		"q-register": {Value: 2, At: old},
		"q-fulltax":  {Value: 2, At: old},
		"q-rbac":     {Value: 2, At: old},
	}})

	views, err := p.Rings(context.Background(), testApp)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		if v.Ring.Name != "test" {
			continue
		}
		st := v.Gates.GrafanaStatus
		if st.Verdict != grafana.VerdictUnknown {
			t.Fatalf("a gate whose every run is stale should read unknown, got %q", st.Verdict)
		}
		for _, c := range st.Checks {
			if !c.Stale {
				t.Fatalf("check %q should be marked stale", c.Name)
			}
			if c.Verdict != grafana.VerdictUnknown {
				t.Fatalf("stale check %q should not report %q", c.Name, c.Verdict)
			}
		}
	}

	// Stale is advisory, not blocking — the same reasoning as an unreachable
	// Grafana. A promotion still goes through.
	mustSeed(t, p, "int", "v1")
	if _, err := p.Promote(context.Background(), testApp, "int"); err != nil {
		t.Fatalf("a stale gate should not block: %v", err)
	}
}

// A fresh run inside max_age is judged normally.
func TestGrafanaGate_FreshRunIsJudged(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, _, _ := gatedHarness(t, now, e2ePolicy(36*time.Hour))
	p.SetGrafanaClient(testApp, &perCheckGrafana{byQuery: map[string]grafana.Sample{
		"q-register": {Value: 2, At: now.Add(-30 * time.Hour)}, // inside the window
		"q-fulltax":  {Value: 2, At: now.Add(-30 * time.Hour)},
		"q-rbac":     {Value: 2, At: now.Add(-30 * time.Hour)},
	}})
	mustSeed(t, p, "int", "v1")
	if _, err := p.Promote(context.Background(), testApp, "int"); err != nil {
		t.Fatalf("a fresh gate should not block: %v", err)
	}
}

// With max_age set, a query that forgot to select the run timestamp cannot be
// aged — that must read as unknown, never as a pass.
func TestGrafanaGate_MissingTimestampIsUnknown(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, _, _ := gatedHarness(t, now, e2ePolicy(36*time.Hour))
	p.SetGrafanaClient(testApp, &perCheckGrafana{byQuery: map[string]grafana.Sample{
		"q-register": {Value: 2}, // no At
		"q-fulltax":  {Value: 2},
		"q-rbac":     {Value: 2},
	}})

	views, _ := p.Rings(context.Background(), testApp)
	for _, v := range views {
		if v.Ring.Name != "test" {
			continue
		}
		if got := v.Gates.GrafanaStatus.Verdict; got != grafana.VerdictUnknown {
			t.Fatalf("want unknown without a run time, got %q", got)
		}
	}
}

// Rollup: a mix of green and unreadable is "check" — partial information must
// not be presented as a clean pass.
func TestGrafanaGate_PartialDataIsCheck(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	ranAt := now.Add(-2 * time.Hour)
	p, _, _ := gatedHarness(t, now, e2ePolicy(0)) // staleness off
	p.SetGrafanaClient(testApp, &perCheckGrafana{byQuery: map[string]grafana.Sample{
		"q-register": {Value: 2, At: ranAt},
		"q-fulltax":  {Value: 2, At: ranAt},
		// q-rbac is missing from the map -> the stub errors -> unknown
	}})

	views, _ := p.Rings(context.Background(), testApp)
	for _, v := range views {
		if v.Ring.Name != "test" {
			continue
		}
		if got := v.Gates.GrafanaStatus.Verdict; got != grafana.VerdictCheck {
			t.Fatalf("want check for partial data, got %q", got)
		}
	}
}

// A query copied out of a dashboard keeps its $env/$workflow variables; they are
// substituted before it is sent, while Grafana's own macros are left alone.
func TestExpandDashboardVars(t *testing.T) {
	got := expandDashboardVars(
		"SELECT 1 WHERE environment ~ '${env:raw}' AND workflow_file ~ '${workflow:raw}' AND at BETWEEN $__timeFrom() AND $__timeTo()",
		"acc", ".*")
	want := "SELECT 1 WHERE environment ~ 'acc' AND workflow_file ~ '.*' AND at BETWEEN $__timeFrom() AND $__timeTo()"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

// Demo mode answers without any Grafana at all, per target ring, so the gate can
// be shown offline.
func TestGrafanaGate_DemoMode(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	pol := &config.PromotionPolicy{
		Grafana: &config.GrafanaPolicy{
			Rings:        []string{"test", "acc"},
			DemoVerdict:  config.GrafanaVerdictGo,
			DemoVerdicts: map[string]string{"acc": config.GrafanaVerdictNoGo},
		},
	}
	p, _, _ := gatedHarness(t, now, pol)
	mustSeed(t, p, "int", "v1")

	// test is a demo GO, so the promotion runs without any client configured.
	if _, err := p.Promote(context.Background(), testApp, "int"); err != nil {
		t.Fatalf("demo go verdict should not block: %v", err)
	}
	// acc is a demo NO-GO.
	if _, err := p.Promote(context.Background(), testApp, "test"); !errors.Is(err, ErrGrafanaNoGo) {
		t.Fatalf("want ErrGrafanaNoGo for the demo no-go ring, got %v", err)
	}
}
