package promoter

import (
	"context"
	"errors"
	"testing"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/store"
)

// configuredHarness builds a promoter whose named rings carry a config-declared
// auto_promote value. Rings absent from decl keep the field unset, i.e. the
// historical runtime-only behaviour.
func configuredHarness(t *testing.T, decl map[string]bool) (*Promoter, *fakeDeployer, store.Store) {
	t.Helper()
	dep := newFakeDeployer()
	cfg := testConfig(0)
	for rname, want := range decl {
		rc := cfg.Apps[0].Rings[rname]
		v := want
		rc.AutoPromote = &v
		cfg.Apps[0].Rings[rname] = rc
	}
	st := store.NewMemory()
	p := New(cfg, st, nil, dep, newScriptedChecker(dep), nil)
	return p, dep, st
}

// autoPromoteOf reads a ring's stored switch, treating "no row yet" as off —
// GetRingState reports ErrNotFound for a ring that has never been deployed.
func autoPromoteOf(t *testing.T, st store.Store, app, ringName string) bool {
	t.Helper()
	s, err := st.GetRingState(context.Background(), app, ringName)
	if errors.Is(err, store.ErrNotFound) {
		return false
	}
	if err != nil {
		t.Fatalf("get state %s/%s: %v", app, ringName, err)
	}
	return s.AutoPromote
}

// A ring whose config declares auto_promote gets that value applied at start-up,
// and a ring that declares nothing is left alone.
func TestReconcileAutoPromote_AppliesDeclaredValues(t *testing.T) {
	p, _, st := configuredHarness(t, map[string]bool{"test": true})
	ctx := context.Background()

	if err := p.ReconcileAutoPromote(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !autoPromoteOf(t, st, testApp, "test") {
		t.Fatal("config declared auto_promote: true, store says off")
	}
	// int declares nothing: untouched, so still the default off.
	if autoPromoteOf(t, st, testApp, "int") {
		t.Fatal("a ring that declares nothing must not be touched")
	}
}

// The reconcile is a correction, not a seed: a ring switched on behind its back
// is switched off again on the next run. This is the drift the feature exists
// to remove, so it is the central test.
func TestReconcileAutoPromote_CorrectsOutOfBandToggle(t *testing.T) {
	p, _, st := configuredHarness(t, map[string]bool{"test": false})
	ctx := context.Background()

	// Someone flips it on directly in the store, bypassing config.
	if err := st.SetAutoPromote(ctx, testApp, "test", true); err != nil {
		t.Fatalf("seed drift: %v", err)
	}
	if err := p.ReconcileAutoPromote(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if autoPromoteOf(t, st, testApp, "test") {
		t.Fatal("reconcile must switch an out-of-band toggle back off")
	}
}

// Re-running with unchanged config writes nothing — the store's UpdatedAt is
// the observable proxy for "no write happened".
func TestReconcileAutoPromote_Idempotent(t *testing.T) {
	p, _, st := configuredHarness(t, map[string]bool{"test": true})
	ctx := context.Background()

	if err := p.ReconcileAutoPromote(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	first := mustState(t, st, testApp, "test").UpdatedAt
	if err := p.ReconcileAutoPromote(ctx); err != nil {
		t.Fatalf("reconcile again: %v", err)
	}
	if got := mustState(t, st, testApp, "test").UpdatedAt; !got.Equal(first) {
		t.Fatalf("second reconcile wrote: %v -> %v", first, got)
	}
}

// Config ownership is reported so the API and UI can react to it.
func TestAutoPromoteOwnedByConfig(t *testing.T) {
	p, _, _ := configuredHarness(t, map[string]bool{"test": false})
	if !p.AutoPromoteOwnedByConfig(testApp, "test") {
		t.Fatal("declared ring must report as config-owned")
	}
	if p.AutoPromoteOwnedByConfig(testApp, "int") {
		t.Fatal("undeclared ring must not report as config-owned")
	}
	if p.AutoPromoteOwnedByConfig("nope", "test") {
		t.Fatal("unknown app must not report as config-owned")
	}
}

// The API toggle is refused for a config-owned ring, and still works for one
// config does not declare.
func TestSetAutoPromote_RefusesConfigOwnedRing(t *testing.T) {
	p, _, st := configuredHarness(t, map[string]bool{"test": false})
	ctx := context.Background()

	err := p.SetAutoPromote(ctx, testApp, "test", true)
	if !errors.Is(err, ErrAutoPromoteConfigOwned) {
		t.Fatalf("want ErrAutoPromoteConfigOwned, got %v", err)
	}
	if autoPromoteOf(t, st, testApp, "test") {
		t.Fatal("refused call must not have written")
	}
	// A ring config says nothing about stays operator-controlled.
	if err := p.SetAutoPromote(ctx, testApp, "int", true); err != nil {
		t.Fatalf("undeclared ring must stay toggleable: %v", err)
	}
}

// What actually matters: a config-declared value drives the promotion chain.
func TestReconciledAutoPromote_DrivesPromotionChain(t *testing.T) {
	p, dep, _ := configuredHarness(t, map[string]bool{"test": true})
	ctx := context.Background()

	if err := p.ReconcileAutoPromote(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if _, err := p.Seed(ctx, testApp, "int", "v1"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	res, err := p.Promote(ctx, testApp, "int")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	// test's config-declared switch carried the chain on to acc.
	if !res.Success || res.Ring != "acc" {
		t.Fatalf("chain should have reached acc, got %+v", res)
	}
	if got := dep.version(key(testApp, "acc")); got != "v1" {
		t.Fatalf("acc should run v1, got %q", got)
	}
}

// Rings a config never mentions behave exactly as before the field existed.
func TestReconcileAutoPromote_NoDeclarationsIsNoOp(t *testing.T) {
	p, _, st := configuredHarness(t, nil)
	ctx := context.Background()

	if err := st.SetAutoPromote(ctx, testApp, "test", true); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := p.ReconcileAutoPromote(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !autoPromoteOf(t, st, testApp, "test") {
		t.Fatal("reconcile must not touch rings config does not declare")
	}
}

// Guard against the pointer being flattened to a plain bool later: absent and
// explicitly-false must stay distinguishable, since that distinction is what
// keeps existing configs behaving unchanged.
func TestRingConfig_AbsentAndFalseDiffer(t *testing.T) {
	no := config.RingConfig{}
	f := false
	explicit := config.RingConfig{AutoPromote: &f}
	if no.AutoPromoteOwnedByConfig() {
		t.Fatal("absent must not claim ownership")
	}
	if !explicit.AutoPromoteOwnedByConfig() {
		t.Fatal("explicit false must claim ownership")
	}
}
