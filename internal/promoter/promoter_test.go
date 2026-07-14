package promoter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/deployer"
	"github.com/example/ring-promoter/internal/health"
	"github.com/example/ring-promoter/internal/ring"
	"github.com/example/ring-promoter/internal/store"
)

// ---- test doubles ----

// fakeDeployer records deploys and tracks the "live" version per (app, ring).
// It can be told to fail deploying specific versions.
type fakeDeployer struct {
	mu      sync.Mutex
	live    map[string]string // app/ring -> version
	failVer map[string]bool   // version -> Deploy returns error
	deploys []string          // "app/ring=version" in order
}

func newFakeDeployer() *fakeDeployer {
	return &fakeDeployer{live: map[string]string{}, failVer: map[string]bool{}}
}

func key(app, r string) string { return app + "/" + r }

func (f *fakeDeployer) Deploy(_ context.Context, t deployer.Target, version string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deploys = append(f.deploys, fmt.Sprintf("%s=%s", key(t.App, t.Ring), version))
	if f.failVer[version] {
		return fmt.Errorf("deploy failed for version %s", version)
	}
	f.live[key(t.App, t.Ring)] = version
	return nil
}

func (f *fakeDeployer) LiveVersion(_ context.Context, t deployer.Target) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.live[key(t.App, t.Ring)], nil
}

func (f *fakeDeployer) version(k string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.live[k]
}

func (f *fakeDeployer) deployCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.deploys)
}

func (f *fakeDeployer) failVersion(v string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failVer[v] = true
}

// scriptedChecker reports a (ring, version) combination unhealthy when marked.
// Health URLs are of the form "health://<app>/<ring>"; the version consulted is
// whatever the fakeDeployer currently has live in that ring. When a probe
// carries a version expectation, the endpoint "reports" the ring's live
// version — unless reportVersion pinned a stale one (simulating an old build
// still serving after a deploy).
type scriptedChecker struct {
	dep       *fakeDeployer
	mu        sync.Mutex
	unhealthy map[string]bool   // "app/ring|version" -> unhealthy
	checks    map[string]int    // "app/ring" -> number of checks
	reports   map[string]string // "app/ring" -> version the endpoint claims
}

func newScriptedChecker(dep *fakeDeployer) *scriptedChecker {
	return &scriptedChecker{dep: dep, unhealthy: map[string]bool{}, checks: map[string]int{}, reports: map[string]string{}}
}

func (c *scriptedChecker) Check(_ context.Context, pr health.Probe) error {
	k := strings.TrimPrefix(pr.URL, "health://")
	ver := c.dep.version(k)
	c.mu.Lock()
	c.checks[k]++
	bad := c.unhealthy[k+"|"+ver]
	reported, pinned := c.reports[k]
	c.mu.Unlock()
	if bad {
		return fmt.Errorf("unhealthy: %s running %s", k, ver)
	}
	if pr.WantVersion != "" && (pr.VersionField != "" || pr.VersionHeader != "") {
		if !pinned {
			reported = ver
		}
		if reported != pr.WantVersion {
			return fmt.Errorf("wrong version live: endpoint reports %q, want %q", reported, pr.WantVersion)
		}
	}
	return nil
}

// ReportedVersion implements health.VersionReporter: the pinned report if set,
// else whatever the fakeDeployer has live in the ring.
func (c *scriptedChecker) ReportedVersion(_ context.Context, pr health.Probe) (string, error) {
	k := strings.TrimPrefix(pr.URL, "health://")
	c.mu.Lock()
	reported, pinned := c.reports[k]
	c.mu.Unlock()
	if pinned {
		return reported, nil
	}
	return c.dep.version(k), nil
}

func (c *scriptedChecker) markUnhealthy(app, r, version string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unhealthy[key(app, r)+"|"+version] = true
}

// reportVersion pins the version the ring's health endpoint claims to run,
// regardless of what was deployed.
func (c *scriptedChecker) reportVersion(app, r, version string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reports[key(app, r)] = version
}

func (c *scriptedChecker) checkCount(app, r string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.checks[key(app, r)]
}

// ---- harness ----

const testApp = "web"

func testConfig(retryCount int) *config.Config {
	rings := map[string]config.RingConfig{}
	for _, r := range ring.Names() {
		rings[r] = config.RingConfig{
			Namespace:  r,
			Deployment: testApp,
			Container:  testApp,
			Image:      "repo/" + testApp,
			HealthURL:  "health://" + key(testApp, r),
		}
	}
	delay := config.Duration(time.Millisecond)
	return &config.Config{
		APIToken: "test-token",
		Deployer: config.DeployerLog,
		Health:   config.HealthAlways,
		Retry:    config.RetryConfig{Count: &retryCount, Delay: &delay},
		Database: config.DatabaseConfig{Driver: config.StoreMemory},
		Apps:     []config.AppConfig{{Name: testApp, Rings: rings}},
	}
}

func newHarness(t *testing.T, retryCount int) (*Promoter, *fakeDeployer, *scriptedChecker, store.Store) {
	t.Helper()
	dep := newFakeDeployer()
	chk := newScriptedChecker(dep)
	st := store.NewMemory()
	// nil per-app map: every app falls back to the single default deployer.
	p := New(testConfig(retryCount), st, nil, dep, chk, nil)
	return p, dep, chk, st
}

func mustState(t *testing.T, st store.Store, app, r string) store.RingState {
	t.Helper()
	s, err := st.GetRingState(context.Background(), app, r)
	if err != nil {
		t.Fatalf("get state %s/%s: %v", app, r, err)
	}
	return s
}

// ---- tests ----

func TestSeed_Healthy(t *testing.T) {
	p, dep, _, st := newHarness(t, 2)
	res, err := p.Seed(context.Background(), testApp, "int", "v1")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
	s := mustState(t, st, testApp, "int")
	if s.CurrentVersion != "v1" || s.PreviousVersion != "" || !s.Healthy {
		t.Fatalf("bad state: %+v", s)
	}
	if dep.deployCount() != 1 {
		t.Fatalf("expected 1 deploy, got %d", dep.deployCount())
	}
}

func TestSeed_UnhealthyDoesNotRollBack(t *testing.T) {
	p, dep, chk, st := newHarness(t, 1)
	chk.markUnhealthy(testApp, "int", "v1")
	res, err := p.Seed(context.Background(), testApp, "int", "v1")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if res.Success {
		t.Fatalf("expected failure, got success")
	}
	// State should still record the seeded version (nothing to roll back to).
	s := mustState(t, st, testApp, "int")
	if s.CurrentVersion != "v1" || s.Healthy {
		t.Fatalf("bad state: %+v", s)
	}
	if dep.deployCount() != 1 {
		t.Fatalf("expected exactly 1 deploy (no rollback), got %d", dep.deployCount())
	}
}

func TestSeed_EmptyVersion(t *testing.T) {
	p, _, _, _ := newHarness(t, 1)
	if _, err := p.Seed(context.Background(), testApp, "int", ""); !errors.Is(err, ErrEmptyVersion) {
		t.Fatalf("expected ErrEmptyVersion, got %v", err)
	}
}

func TestPromote_HappyPath_OneRingAtATime(t *testing.T) {
	p, _, _, st := newHarness(t, 2)
	ctx := context.Background()
	mustSeed(t, p, "int", "v1")

	res, err := p.Promote(ctx, testApp, "int")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if !res.Success || res.Ring != "test" || res.Version != "v1" {
		t.Fatalf("unexpected result: %+v", res)
	}
	// ring1 got v1; ring2 untouched (no skipping).
	if s := mustState(t, st, testApp, "test"); s.CurrentVersion != "v1" {
		t.Fatalf("ring1 current = %q, want v1", s.CurrentVersion)
	}
	if _, err := st.GetRingState(ctx, testApp, "acc"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("ring2 should be untouched, got %v", err)
	}
}

func TestPromote_LastRingHasNoNext(t *testing.T) {
	p, _, _, _ := newHarness(t, 1)
	last := ring.Names()[len(ring.Names())-1]
	mustSeed(t, p, last, "v1")
	if _, err := p.Promote(context.Background(), testApp, last); !errors.Is(err, ErrNoNextRing) {
		t.Fatalf("expected ErrNoNextRing, got %v", err)
	}
}

func TestPromote_NothingToPromote(t *testing.T) {
	p, _, _, _ := newHarness(t, 1)
	if _, err := p.Promote(context.Background(), testApp, "int"); !errors.Is(err, ErrNothingToPromote) {
		t.Fatalf("expected ErrNothingToPromote, got %v", err)
	}
}

func TestPromote_SourceMustBeHealthy(t *testing.T) {
	p, dep, chk, st := newHarness(t, 1)
	ctx := context.Background()
	mustSeed(t, p, "int", "v1")
	chk.markUnhealthy(testApp, "int", "v1") // source becomes unhealthy
	before := dep.deployCount()

	res, err := p.Promote(ctx, testApp, "int")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if res.Success {
		t.Fatalf("expected failure due to unhealthy source")
	}
	if dep.deployCount() != before {
		t.Fatalf("target should not have been deployed when source unhealthy")
	}
	if _, err := st.GetRingState(ctx, testApp, "test"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("ring1 should be untouched")
	}
}

func TestPromote_RetryThenAutoRollback(t *testing.T) {
	const retry = 3
	p, dep, chk, st := newHarness(t, retry)
	ctx := context.Background()

	// Establish a good baseline in ring1 (v1) via seed+promote.
	mustSeed(t, p, "int", "v1")
	if res, err := p.Promote(ctx, testApp, "int"); err != nil || !res.Success {
		t.Fatalf("baseline promote failed: %+v %v", res, err)
	}

	// New version v2 is healthy in ring0 but unhealthy in ring1.
	mustSeed(t, p, "int", "v2")
	chk.markUnhealthy(testApp, "test", "v2")

	deploysBefore := dep.deployCount()
	checksBefore := chk.checkCount(testApp, "test")

	res, err := p.Promote(ctx, testApp, "int")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if res.Success {
		t.Fatalf("expected promote to fail")
	}
	if !res.RolledBack {
		t.Fatalf("expected rollback to have happened: %+v", res)
	}

	// ring1 must be back on v1 and healthy, with v2 remembered as previous.
	s := mustState(t, st, testApp, "test")
	if s.CurrentVersion != "v1" || s.PreviousVersion != "v2" || !s.Healthy {
		t.Fatalf("bad ring1 state after rollback: %+v", s)
	}

	// The health checker should have retried on ring1: retry+1 failed attempts
	// for v2, plus at least one for the rolled-back v1.
	targetChecks := chk.checkCount(testApp, "test") - checksBefore
	if targetChecks < retry+1+1 {
		t.Fatalf("expected >= %d ring1 checks, got %d", retry+2, targetChecks)
	}

	// Two deploys on the target: v2 (failed health) then v1 (rollback).
	if got := dep.deployCount() - deploysBefore; got != 2 {
		t.Fatalf("expected 2 target deploys (promote + rollback), got %d", got)
	}

	// History records both a failed promote and a successful rollback.
	hist, _ := st.ListHistory(ctx, testApp)
	if !containsHistory(hist, store.ActionPromote, store.ResultFailure) {
		t.Fatalf("missing failed promote in history")
	}
	if !containsHistory(hist, store.ActionRollback, store.ResultSuccess) {
		t.Fatalf("missing successful rollback in history")
	}
}

// With retry count 0 there must be exactly ONE health check on the failing
// version before the auto-rollback (proving count:0 is honored, not defaulted).
func TestPromote_ZeroRetries_SingleCheckThenRollback(t *testing.T) {
	p, _, chk, st := newHarness(t, 0)
	ctx := context.Background()

	mustSeed(t, p, "int", "v1")
	if res, err := p.Promote(ctx, testApp, "int"); err != nil || !res.Success {
		t.Fatalf("baseline promote failed: %+v %v", res, err)
	}
	mustSeed(t, p, "int", "v2")
	chk.markUnhealthy(testApp, "test", "v2")

	checksBefore := chk.checkCount(testApp, "test")
	res, err := p.Promote(ctx, testApp, "int")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if res.Success || !res.RolledBack {
		t.Fatalf("expected failed promote with rollback: %+v", res)
	}
	// Exactly one check for the bad v2 + one for the rolled-back v1.
	if got := chk.checkCount(testApp, "test") - checksBefore; got != 2 {
		t.Fatalf("expected exactly 2 checks (v2 once, then v1), got %d", got)
	}
	if s := mustState(t, st, testApp, "test"); s.CurrentVersion != "v1" {
		t.Fatalf("expected rollback to v1, got %q", s.CurrentVersion)
	}
}

// When the promote health check fails AND the auto-rollback deploy also fails,
// the cluster is stuck on the bad version — the stored state must reflect that,
// not the old healthy version.
func TestPromote_HealthFailsAndRollbackFails_StoreMatchesCluster(t *testing.T) {
	p, dep, chk, st := newHarness(t, 0)
	ctx := context.Background()

	mustSeed(t, p, "int", "v1")
	if res, err := p.Promote(ctx, testApp, "int"); err != nil || !res.Success {
		t.Fatalf("baseline promote: %+v %v", res, err)
	}
	mustSeed(t, p, "int", "v2")
	chk.markUnhealthy(testApp, "test", "v2") // v2 unhealthy in ring1
	dep.failVersion("v1")                    // rolling back to v1 will fail to deploy

	res, err := p.Promote(ctx, testApp, "int")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if res.Success || res.RolledBack {
		t.Fatalf("expected failed promote with failed rollback: %+v", res)
	}
	// Cluster is stuck on v2; the store must agree (not report the old good v1).
	if cluster := dep.version(key(testApp, "test")); cluster != "v2" {
		t.Fatalf("cluster should still run v2, got %q", cluster)
	}
	s := mustState(t, st, testApp, "test")
	if s.CurrentVersion != "v2" || s.Healthy {
		t.Fatalf("store must reflect cluster (current=v2 unhealthy), got current=%q healthy=%v",
			s.CurrentVersion, s.Healthy)
	}
}

func TestRollback_Manual(t *testing.T) {
	p, _, _, st := newHarness(t, 1)
	ctx := context.Background()
	mustSeed(t, p, "int", "v1")
	mustSeed(t, p, "int", "v2")

	res, err := p.Rollback(ctx, testApp, "int")
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if !res.Success || res.Version != "v1" {
		t.Fatalf("unexpected rollback result: %+v", res)
	}
	s := mustState(t, st, testApp, "int")
	if s.CurrentVersion != "v1" || s.PreviousVersion != "v2" {
		t.Fatalf("bad state after rollback: %+v", s)
	}
}

func TestRollback_NothingToRollBack(t *testing.T) {
	p, _, _, _ := newHarness(t, 1)
	mustSeed(t, p, "int", "v1") // only one version -> no previous
	if _, err := p.Rollback(context.Background(), testApp, "int"); !errors.Is(err, ErrNothingToRollback) {
		t.Fatalf("expected ErrNothingToRollback, got %v", err)
	}
}

func TestErrors_UnknownAppAndRing(t *testing.T) {
	p, _, _, _ := newHarness(t, 1)
	ctx := context.Background()
	if _, err := p.Seed(ctx, "nope", "int", "v1"); !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("expected ErrAppNotFound, got %v", err)
	}
	if _, err := p.Seed(ctx, testApp, "ring99", "v1"); !errors.Is(err, ErrRingNotConfigured) {
		t.Fatalf("expected ErrRingNotConfigured, got %v", err)
	}
}

func TestConcurrent_SameAppSerialized(t *testing.T) {
	// Exercises the per-app lock; run with -race to detect data races.
	p, _, _, _ := newHarness(t, 0)
	ctx := context.Background()
	mustSeed(t, p, "int", "v1")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = p.Seed(ctx, testApp, "int", fmt.Sprintf("v%d", i))
			_, _ = p.Rings(ctx, testApp)
			_, _ = p.Promote(ctx, testApp, "int")
		}(i)
	}
	wg.Wait()
}

// ---- helpers ----

func mustSeed(t *testing.T, p *Promoter, r, version string) {
	t.Helper()
	res, err := p.Seed(context.Background(), testApp, r, version)
	if err != nil {
		t.Fatalf("seed %s %s: %v", r, version, err)
	}
	if !res.Success {
		t.Fatalf("seed %s %s not healthy: %s", r, version, res.Message)
	}
}

func containsHistory(hist []store.HistoryEntry, action, result string) bool {
	for _, h := range hist {
		if h.Action == action && h.Result == result {
			return true
		}
	}
	return false
}

// newHarnessWithRings builds a harness where per-ring config can be customised
// (e.g. pinning a deploy ref on a ring).
func newHarnessWithRings(t *testing.T, retryCount int, mutate func(r string, rc *config.RingConfig)) (*Promoter, *fakeDeployer, *scriptedChecker, store.Store) {
	t.Helper()
	dep := newFakeDeployer()
	chk := newScriptedChecker(dep)
	st := store.NewMemory()
	delay := config.Duration(time.Millisecond)
	rings := map[string]config.RingConfig{}
	for _, r := range ring.Names() {
		rc := config.RingConfig{
			Namespace: r, Deployment: testApp, Container: testApp,
			Image: "repo/" + testApp, HealthURL: "health://" + key(testApp, r), TargetEnv: r,
		}
		if mutate != nil {
			mutate(r, &rc)
		}
		rings[r] = rc
	}
	cfg := &config.Config{
		APIToken: "test-token", Deployer: config.DeployerLog, Health: config.HealthAlways,
		Retry:    config.RetryConfig{Count: &retryCount, Delay: &delay},
		Database: config.DatabaseConfig{Driver: config.StoreMemory},
		Apps:     []config.AppConfig{{Name: testApp, Rings: rings}},
	}
	return New(cfg, st, nil, dep, chk, nil), dep, chk, st
}

// TestPinnedRef_OverridesPromotedAndSeededVersion verifies that a ring pinning
// `ref` (e.g. acc -> release) always deploys and records that ref — so
// "promote to acc" ships release even though int/test carry main.
func TestPinnedRef_OverridesPromotedAndSeededVersion(t *testing.T) {
	p, dep, _, st := newHarnessWithRings(t, 2, func(r string, rc *config.RingConfig) {
		if r == "acc" {
			rc.Ref = "release"
		}
	})
	ctx := context.Background()

	if _, err := p.Seed(ctx, testApp, "int", "main"); err != nil {
		t.Fatalf("seed int: %v", err)
	}
	if _, err := p.Promote(ctx, testApp, "int"); err != nil { // int -> test
		t.Fatalf("promote int->test: %v", err)
	}
	res, err := p.Promote(ctx, testApp, "test") // test -> acc (pinned to release)
	if err != nil {
		t.Fatalf("promote test->acc: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected acc promote to succeed, got %+v", res)
	}

	// acc ran release, not the promoted main.
	if got := dep.version(key(testApp, "acc")); got != "release" {
		t.Fatalf("acc live version = %q, want release", got)
	}
	if res.Version != "release" {
		t.Fatalf("result version = %q, want release", res.Version)
	}
	if s := mustState(t, st, testApp, "acc"); s.CurrentVersion != "release" {
		t.Fatalf("acc recorded version = %q, want release", s.CurrentVersion)
	}
	// int/test still carry main.
	if got := dep.version(key(testApp, "test")); got != "main" {
		t.Fatalf("test live version = %q, want main", got)
	}

	// Seeding acc directly with something else is also pinned to release.
	res2, err := p.Seed(ctx, testApp, "acc", "hotfix-branch")
	if err != nil {
		t.Fatalf("seed acc: %v", err)
	}
	if res2.Version != "release" || dep.version(key(testApp, "acc")) != "release" {
		t.Fatalf("seed acc not pinned: res=%q live=%q", res2.Version, dep.version(key(testApp, "acc")))
	}
}

// ---- version validation (deployer.VersionSource) ----

// validatingDeployer wraps fakeDeployer with a fixed set of versions that
// exist in the "source repository".
type validatingDeployer struct {
	*fakeDeployer
	known map[string]bool
}

func (v *validatingDeployer) ListVersions(context.Context) ([]deployer.Version, error) {
	out := make([]deployer.Version, 0, len(v.known))
	for name := range v.known {
		out = append(out, deployer.Version{Name: name, Type: "branch"})
	}
	return out, nil
}

func (v *validatingDeployer) ValidateVersion(_ context.Context, version string) error {
	if v.known[version] {
		return nil
	}
	return deployer.ErrVersionNotFound
}

// ---- pinned rings record the endpoint-reported version ----

// TestPinnedRef_RecordsReportedVersion: a ref-pinned ring still deploys the
// literal pin (its pipeline may require it — diytaxreturn's guard refuses acc
// unless DEPLOY_BRANCH=release), but afterwards the ring records the version
// the health endpoint says it is running, not the word "release".
func TestPinnedRef_RecordsReportedVersion(t *testing.T) {
	p, dep, chk, st := newHarnessWithRings(t, 0, func(r string, rc *config.RingConfig) {
		if r == "acc" {
			rc.Ref = "release"
			rc.HealthVersionField = "version"
		}
	})
	ctx := context.Background()
	chk.reportVersion(testApp, "acc", "v1.0.36") // what /health answers after the deploy

	mustSeed(t, p, "int", "main")
	if _, err := p.Promote(ctx, testApp, "int"); err != nil {
		t.Fatalf("promote int->test: %v", err)
	}
	res, err := p.Promote(ctx, testApp, "test") // test -> acc (pinned)
	if err != nil {
		t.Fatalf("promote test->acc: %v", err)
	}
	if !res.Success || res.Version != "v1.0.36" {
		t.Fatalf("acc promote should record the reported version, got %+v", res)
	}
	// The pipeline was still dispatched with the pin, not the version.
	if got := dep.version(key(testApp, "acc")); got != "release" {
		t.Fatalf("acc deploy dispatched %q, want release", got)
	}
	if s := mustState(t, st, testApp, "acc"); s.CurrentVersion != "v1.0.36" || !s.Healthy {
		t.Fatalf("acc state = %+v, want healthy v1.0.36", s)
	}
}

// TestPinnedRef_RollbackChecksStatusOnly: rolling a pinned ring back to old
// state whose previous version is the literal pin ("release") must not demand
// the endpoint report that string — the rollback is healthy on status alone,
// and the recorded version is what the endpoint says is running.
func TestPinnedRef_RollbackChecksStatusOnly(t *testing.T) {
	p, _, chk, st := newHarnessWithRings(t, 0, func(r string, rc *config.RingConfig) {
		if r == "acc" {
			rc.Ref = "release"
			rc.HealthVersionField = "version"
		}
	})
	ctx := context.Background()

	// Pre-upgrade state: acc's previous version is the ref name itself.
	if err := st.UpsertRingState(ctx, store.RingState{
		App: testApp, Ring: "acc", CurrentVersion: "v-broken", PreviousVersion: "release",
	}); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	chk.reportVersion(testApp, "acc", "v1.0.36") // what /health answers after the rollback

	res, err := p.Rollback(ctx, testApp, "acc")
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if !res.Success || !res.RolledBack {
		t.Fatalf("rollback to the pin must be healthy on status alone, got %+v", res)
	}
	s := mustState(t, st, testApp, "acc")
	if s.CurrentVersion != "v1.0.36" || !s.Healthy {
		t.Fatalf("acc state = %+v, want healthy v1.0.36 (the reported version)", s)
	}
}

func TestSeed_RejectsVersionMissingFromSource(t *testing.T) {
	dep := &validatingDeployer{fakeDeployer: newFakeDeployer(), known: map[string]bool{"v1": true}}
	st := store.NewMemory()
	p := New(testConfig(0), st, nil, dep, newScriptedChecker(dep.fakeDeployer), nil)

	_, err := p.Seed(context.Background(), testApp, "int", "does-not-exist")
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("expected ErrVersionNotFound, got %v", err)
	}
	if dep.deployCount() != 0 {
		t.Fatalf("nothing must be deployed for an unknown version, got %d deploys", dep.deployCount())
	}
	if _, err := st.GetRingState(context.Background(), testApp, "int"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("no state must be recorded for a rejected seed, got %v", err)
	}

	// A version that exists proceeds normally.
	res, err := p.Seed(context.Background(), testApp, "int", "v1")
	if err != nil || !res.Success {
		t.Fatalf("seed of existing version: err=%v res=%+v", err, res)
	}
}

func TestVersions_SupportedOnlyWithVersionSource(t *testing.T) {
	// Plain deployer: not supported.
	p, _, _, _ := newHarness(t, 0)
	supported, _, err := p.Versions(context.Background(), testApp)
	if err != nil || supported {
		t.Fatalf("plain deployer: supported=%v err=%v, want false/nil", supported, err)
	}

	// VersionSource deployer: supported with the known list.
	dep := &validatingDeployer{fakeDeployer: newFakeDeployer(), known: map[string]bool{"main": true}}
	p2 := New(testConfig(0), store.NewMemory(), nil, dep, newScriptedChecker(dep.fakeDeployer), nil)
	supported, versions, err := p2.Versions(context.Background(), testApp)
	if err != nil || !supported || len(versions) != 1 || versions[0].Name != "main" {
		t.Fatalf("version-source deployer: supported=%v versions=%+v err=%v", supported, versions, err)
	}

	// Unknown app.
	if _, _, err := p2.Versions(context.Background(), "nope"); !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("unknown app: %v, want ErrAppNotFound", err)
	}
}

// ---- auto-promote chaining ----

func TestPromote_AutoChainsThroughEnabledRings(t *testing.T) {
	p, dep, _, st := newHarness(t, 0)
	ctx := context.Background()

	if _, err := p.Seed(ctx, testApp, "int", "v1"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Auto-promote on for test only: int→test should carry on to acc and stop.
	if err := p.SetAutoPromote(ctx, testApp, "test", true); err != nil {
		t.Fatalf("set auto promote: %v", err)
	}

	res, err := p.Promote(ctx, testApp, "int")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if !res.Success || res.Ring != "acc" {
		t.Fatalf("expected chain to end successfully at acc, got %+v", res)
	}
	if got := dep.version(key(testApp, "test")); got != "v1" {
		t.Fatalf("test should run v1, got %q", got)
	}
	if got := dep.version(key(testApp, "acc")); got != "v1" {
		t.Fatalf("acc should run v1, got %q", got)
	}
	if got := dep.version(key(testApp, "prod")); got != "" {
		t.Fatalf("prod must NOT be auto-promoted (switch off), got %q", got)
	}
	if s := mustState(t, st, testApp, "acc"); s.CurrentVersion != "v1" || !s.Healthy {
		t.Fatalf("bad acc state: %+v", s)
	}
	// The upserts along the chain must not have cleared the setting.
	if s := mustState(t, st, testApp, "test"); !s.AutoPromote {
		t.Fatal("test's auto-promote setting was lost by state upserts")
	}
}

func TestSeed_AutoChainsFromSeededRing(t *testing.T) {
	p, dep, _, _ := newHarness(t, 0)
	ctx := context.Background()

	if err := p.SetAutoPromote(ctx, testApp, "test", true); err != nil {
		t.Fatalf("set auto promote: %v", err)
	}
	res, err := p.Seed(ctx, testApp, "test", "v2")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if !res.Success || res.Ring != "acc" {
		t.Fatalf("seed into test should chain to acc, got %+v", res)
	}
	if got := dep.version(key(testApp, "acc")); got != "v2" {
		t.Fatalf("acc should run v2, got %q", got)
	}
	if got := dep.version(key(testApp, "prod")); got != "" {
		t.Fatalf("prod must stay untouched, got %q", got)
	}
}

func TestAutoChain_StopsWhenHopFails(t *testing.T) {
	p, dep, chk, _ := newHarness(t, 0)
	ctx := context.Background()

	if _, err := p.Seed(ctx, testApp, "int", "v1"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = p.SetAutoPromote(ctx, testApp, "test", true)
	_ = p.SetAutoPromote(ctx, testApp, "acc", true)
	chk.markUnhealthy(testApp, "acc", "v1") // the auto test→acc hop will fail

	res, err := p.Promote(ctx, testApp, "int")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if res.Success || res.Ring != "acc" {
		t.Fatalf("chain should end unsuccessfully at acc, got %+v", res)
	}
	if got := dep.version(key(testApp, "prod")); got != "" {
		t.Fatalf("prod must not deploy after a failed hop, got %q", got)
	}
}

func TestSetAutoPromote_Validation(t *testing.T) {
	p, _, _, _ := newHarness(t, 0)
	ctx := context.Background()

	if err := p.SetAutoPromote(ctx, testApp, "prod", true); !errors.Is(err, ErrNoNextRing) {
		t.Fatalf("enabling on the last ring: %v, want ErrNoNextRing", err)
	}
	if err := p.SetAutoPromote(ctx, "nope", "int", true); !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("unknown app: %v, want ErrAppNotFound", err)
	}
	if err := p.SetAutoPromote(ctx, testApp, "test", true); err != nil {
		t.Fatalf("valid enable: %v", err)
	}
	if err := p.SetAutoPromote(ctx, testApp, "test", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
}

// ---- version-verified health checks ----

// versionHarness builds a harness where every ring verifies the reported
// version (health_version_field is set), so a healthy status alone is not
// enough after a deploy.
func versionHarness(t *testing.T, retryCount int) (*Promoter, *fakeDeployer, *scriptedChecker, store.Store) {
	t.Helper()
	return newHarnessWithRings(t, retryCount, func(_ string, rc *config.RingConfig) {
		rc.HealthVersionField = "version"
	})
}

// TestPromote_StaleVersionServing_RollsBack is the scenario version checking
// exists for: the deploy "succeeds" but the endpoint keeps reporting the OLD
// version. A status-only check would pass; the version check must fail the
// promotion and roll the ring back.
func TestPromote_StaleVersionServing_RollsBack(t *testing.T) {
	p, _, chk, st := versionHarness(t, 1)
	ctx := context.Background()

	mustSeed(t, p, "test", "v0") // baseline in the target ring
	mustSeed(t, p, "int", "v1")

	// After the promote deploys v1 to test, the endpoint still claims v0.
	chk.reportVersion(testApp, "test", "v0")

	res, err := p.Promote(ctx, testApp, "int")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if res.Success {
		t.Fatalf("promotion must fail when the endpoint reports the old version, got %+v", res)
	}
	if !res.RolledBack {
		t.Fatalf("expected auto-rollback, got %+v", res)
	}
	if !strings.Contains(res.Message, "wrong version") && !strings.Contains(res.Message, "health check") {
		t.Fatalf("message should explain the failure, got %q", res.Message)
	}
	// The ring is back on v0, and the rollback health check passed because the
	// endpoint (still reporting v0) now matches the rolled-back version.
	if s := mustState(t, st, testApp, "test"); s.CurrentVersion != "v0" || !s.Healthy {
		t.Fatalf("test ring should be back on healthy v0, got %+v", s)
	}
}

// TestPromote_VersionVerified_Succeeds: with a version source configured and
// the endpoint reporting what was deployed, promotion works end to end.
func TestPromote_VersionVerified_Succeeds(t *testing.T) {
	p, dep, _, st := versionHarness(t, 0)
	ctx := context.Background()

	mustSeed(t, p, "int", "v1")
	res, err := p.Promote(ctx, testApp, "int")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if !res.Success {
		t.Fatalf("promote should succeed: %+v", res)
	}
	if got := dep.version(key(testApp, "test")); got != "v1" {
		t.Fatalf("test live version = %q, want v1", got)
	}
	if s := mustState(t, st, testApp, "test"); s.CurrentVersion != "v1" || !s.Healthy {
		t.Fatalf("test ring state = %+v, want healthy v1", s)
	}
}

// TestSeed_StaleVersionServing_Fails: seeding also verifies the reported
// version; a stale endpoint marks the ring unhealthy (no rollback on seed).
func TestSeed_StaleVersionServing_Fails(t *testing.T) {
	p, _, chk, st := versionHarness(t, 0)
	ctx := context.Background()

	chk.reportVersion(testApp, "int", "v0")
	res, err := p.Seed(ctx, testApp, "int", "v1")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if res.Success {
		t.Fatalf("seed must fail when the endpoint reports a different version, got %+v", res)
	}
	if !strings.Contains(res.Message, "wrong version") {
		t.Fatalf("message should mention the version mismatch, got %q", res.Message)
	}
	if s := mustState(t, st, testApp, "int"); s.Healthy {
		t.Fatalf("int ring must be recorded unhealthy, got %+v", s)
	}
}

// TestPromote_SourceWrongVersion_Aborts: the pre-promotion source check also
// verifies the source actually runs the version about to be promoted, so a
// drifted source aborts the promotion before anything deploys.
func TestPromote_SourceWrongVersion_Aborts(t *testing.T) {
	p, dep, chk, _ := versionHarness(t, 0)
	ctx := context.Background()

	mustSeed(t, p, "int", "v1")
	deploysBefore := dep.deployCount()

	chk.reportVersion(testApp, "int", "v9") // source drifted
	res, err := p.Promote(ctx, testApp, "int")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if res.Success {
		t.Fatalf("promotion must abort on a drifted source, got %+v", res)
	}
	if !strings.Contains(res.Message, "source ring") {
		t.Fatalf("message should blame the source ring, got %q", res.Message)
	}
	if dep.deployCount() != deploysBefore {
		t.Fatalf("nothing must deploy when the source check fails")
	}
}
