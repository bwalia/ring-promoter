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
// whatever the fakeDeployer currently has live in that ring.
type scriptedChecker struct {
	dep       *fakeDeployer
	mu        sync.Mutex
	unhealthy map[string]bool // "app/ring|version" -> unhealthy
	checks    map[string]int  // "app/ring" -> number of checks
}

func newScriptedChecker(dep *fakeDeployer) *scriptedChecker {
	return &scriptedChecker{dep: dep, unhealthy: map[string]bool{}, checks: map[string]int{}}
}

func (c *scriptedChecker) Check(_ context.Context, url string) error {
	k := strings.TrimPrefix(url, "health://")
	ver := c.dep.version(k)
	c.mu.Lock()
	c.checks[k]++
	bad := c.unhealthy[k+"|"+ver]
	c.mu.Unlock()
	if bad {
		return fmt.Errorf("unhealthy: %s running %s", k, ver)
	}
	return nil
}

func (c *scriptedChecker) markUnhealthy(app, r, version string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unhealthy[key(app, r)+"|"+version] = true
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
