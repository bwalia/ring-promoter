package promoter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/changerequest"
	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/store"
)

// gatedHarness builds a promoter whose testApp has a promotion policy, with a
// clock fixed at `now` so maintenance-window gates are deterministic.
func gatedHarness(t *testing.T, now time.Time, policy *config.PromotionPolicy) (*Promoter, *fakeDeployer, store.Store) {
	t.Helper()
	dep := newFakeDeployer()
	chk := newScriptedChecker(dep)
	st := store.NewMemory()
	cfg := testConfig(1)
	cfg.Apps[0].PromotionPolicy = policy
	p := New(cfg, st, nil, dep, chk, nil)
	p.now = func() time.Time { return now }
	return p, dep, st
}

// A GO sign-off recorded for the exact version lets a gated promotion through;
// its absence blocks it.
func TestGate_QASignoff(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	pol := &config.PromotionPolicy{QASignoff: &config.GatePolicy{Rings: []string{"test"}}}
	p, dep, st := gatedHarness(t, now, pol)
	ctx := context.Background()
	mustSeed(t, p, "int", "v1")

	before := dep.deployCount()
	// No sign-off yet: promotion into the gated ring is refused with a 4xx-class
	// precondition error, and nothing is deployed.
	if _, err := p.Promote(ctx, testApp, "int"); !errors.Is(err, ErrSignoffRequired) {
		t.Fatalf("want ErrSignoffRequired, got %v", err)
	}
	if dep.deployCount() != before {
		t.Fatal("target deployed despite missing sign-off")
	}

	// A NO-GO sign-off still blocks.
	if err := st.UpsertSignoff(ctx, store.Signoff{App: testApp, Ring: "test", Version: "v1", Decision: store.DecisionNoGo, Engineer: "jp"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Promote(ctx, testApp, "int"); !errors.Is(err, ErrSignoffNoGo) {
		t.Fatalf("want ErrSignoffNoGo, got %v", err)
	}

	// A GO sign-off for v1 lets it through.
	_ = st.UpsertSignoff(ctx, store.Signoff{App: testApp, Ring: "test", Version: "v1", Decision: store.DecisionGo, Engineer: "jp", QAStatus: "passed"})
	res, err := p.Promote(ctx, testApp, "int")
	if err != nil || !res.Success {
		t.Fatalf("promote after GO should succeed: res=%+v err=%v", res, err)
	}
}

// A sign-off is version-specific: a GO for v1 does not authorize v2.
func TestGate_QASignoffIsVersionSpecific(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	pol := &config.PromotionPolicy{QASignoff: &config.GatePolicy{Rings: []string{"test"}}}
	p, _, st := gatedHarness(t, now, pol)
	ctx := context.Background()
	_ = st.UpsertSignoff(ctx, store.Signoff{App: testApp, Ring: "test", Version: "v1", Decision: store.DecisionGo, Engineer: "jp"})

	mustSeed(t, p, "int", "v2") // different version
	if _, err := p.Promote(ctx, testApp, "int"); !errors.Is(err, ErrSignoffRequired) {
		t.Fatalf("GO for v1 must not authorize v2: got %v", err)
	}
}

// Maintenance window: a recurring config window and an ad-hoc window are a
// union — either open lets the promotion through; neither blocks it.
func TestGate_MaintenanceWindow(t *testing.T) {
	// Recurring window: Sat 02:00-04:00 UTC. Our clock is a Wednesday noon.
	wed := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	pol := &config.PromotionPolicy{
		MaintenanceWindow: &config.MaintenanceWindowPolicy{
			Rings:     []string{"test"},
			Recurring: []config.RecurringWindow{{Days: []string{"Sat"}, Start: "02:00", End: "04:00", Timezone: "UTC"}},
		},
	}
	p, _, st := gatedHarness(t, wed, pol)
	ctx := context.Background()
	mustSeed(t, p, "int", "v1")

	// Outside any window → blocked.
	if _, err := p.Promote(ctx, testApp, "int"); !errors.Is(err, ErrMaintenanceWindowClosed) {
		t.Fatalf("want ErrMaintenanceWindowClosed, got %v", err)
	}

	// Open an ad-hoc window covering "test" around now → allowed.
	_ = st.CreateMaintenanceWindow(ctx, store.MaintenanceWindow{
		ID: "w1", App: testApp, Ring: "test",
		StartsAt: wed.Add(-time.Hour), EndsAt: wed.Add(time.Hour),
	})
	res, err := p.Promote(ctx, testApp, "int")
	if err != nil || !res.Success {
		t.Fatalf("promote inside ad-hoc window should succeed: res=%+v err=%v", res, err)
	}
}

// The recurring config window alone opens the gate at the right time.
func TestGate_RecurringWindowOpens(t *testing.T) {
	sat := time.Date(2026, 7, 18, 3, 0, 0, 0, time.UTC) // Sat 03:00
	pol := &config.PromotionPolicy{
		MaintenanceWindow: &config.MaintenanceWindowPolicy{
			Rings:     []string{"test"},
			Recurring: []config.RecurringWindow{{Days: []string{"Sat"}, Start: "02:00", End: "04:00", Timezone: "UTC"}},
		},
	}
	p, _, _ := gatedHarness(t, sat, pol)
	ctx := context.Background()
	mustSeed(t, p, "int", "v1")
	res, err := p.Promote(ctx, testApp, "int")
	if err != nil || !res.Success {
		t.Fatalf("promote inside recurring window should succeed: res=%+v err=%v", res, err)
	}
}

// Change-request gate: a code is required; the demo code "test" always passes;
// a real code is validated by the app's validator.
func TestGate_ChangeRequest(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	pol := &config.PromotionPolicy{ChangeRequest: &config.ChangeRequestPolicy{Rings: []string{"test"}, Provider: "test"}}
	p, _, _ := gatedHarness(t, now, pol)
	ctx := context.Background()
	mustSeed(t, p, "int", "v1")

	// No code → required error.
	if _, err := p.Promote(ctx, testApp, "int"); !errors.Is(err, ErrChangeRequestRequired) {
		t.Fatalf("want ErrChangeRequestRequired, got %v", err)
	}

	// A non-demo code with the demo-only validator → invalid.
	ctxBad := WithGateInputs(ctx, GateInputs{ChangeRequestCode: "CR-1"})
	if _, err := p.Promote(ctxBad, testApp, "int"); !errors.Is(err, ErrChangeRequestInvalid) {
		t.Fatalf("want ErrChangeRequestInvalid, got %v", err)
	}

	// The universal demo code passes.
	ctxDemo := WithGateInputs(ctx, GateInputs{ChangeRequestCode: "test"})
	res, err := p.Promote(ctxDemo, testApp, "int")
	if err != nil || !res.Success {
		t.Fatalf("demo code should let promotion succeed: res=%+v err=%v", res, err)
	}
}

// A real validator accepts a valid code and rejects an unknown one.
func TestGate_ChangeRequestWithValidator(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	pol := &config.PromotionPolicy{ChangeRequest: &config.ChangeRequestPolicy{Rings: []string{"test"}, Provider: "jira"}}
	p, _, _ := gatedHarness(t, now, pol)
	p.SetChangeRequestValidators(map[string]changerequest.Validator{testApp: fakeValidator{ok: "CR-OK"}})
	ctx := context.Background()
	mustSeed(t, p, "int", "v1")

	if _, err := p.Promote(WithGateInputs(ctx, GateInputs{ChangeRequestCode: "CR-NOPE"}), testApp, "int"); !errors.Is(err, ErrChangeRequestInvalid) {
		t.Fatalf("unknown code should be invalid, got %v", err)
	}
	res, err := p.Promote(WithGateInputs(ctx, GateInputs{ChangeRequestCode: "CR-OK"}), testApp, "int")
	if err != nil || !res.Success {
		t.Fatalf("valid code should let promotion succeed: res=%+v err=%v", res, err)
	}
}

// Seeding directly into a gated ring is gated too.
func TestGate_SeedIntoGatedRing(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	pol := &config.PromotionPolicy{QASignoff: &config.GatePolicy{Rings: []string{"acc"}}}
	p, _, st := gatedHarness(t, now, pol)
	ctx := context.Background()

	if _, err := p.Seed(ctx, testApp, "acc", "v9"); !errors.Is(err, ErrSignoffRequired) {
		t.Fatalf("seeding into gated ring without sign-off should fail: %v", err)
	}
	_ = st.UpsertSignoff(ctx, store.Signoff{App: testApp, Ring: "acc", Version: "v9", Decision: store.DecisionGo, Engineer: "jp"})
	res, err := p.Seed(ctx, testApp, "acc", "v9")
	if err != nil || !res.Success {
		t.Fatalf("seed after GO should succeed: res=%+v err=%v", res, err)
	}
}

// An un-gated ring (int/test with default acc+prod gates) is unaffected.
func TestGate_UngatedRingUnaffected(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	// Default rings (acc, prod) for every gate; promoting int→test is untouched.
	pol := &config.PromotionPolicy{
		QASignoff:     &config.GatePolicy{},
		ChangeRequest: &config.ChangeRequestPolicy{},
	}
	p, _, _ := gatedHarness(t, now, pol)
	ctx := context.Background()
	mustSeed(t, p, "int", "v1")
	res, err := p.Promote(ctx, testApp, "int") // int → test, not gated
	if err != nil || !res.Success {
		t.Fatalf("ungated int→test should succeed: res=%+v err=%v", res, err)
	}
}

// ValidatePromote surfaces a gate failure without deploying anything (used by
// the API to reject before spawning an async job).
func TestGate_ValidatePromote(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	pol := &config.PromotionPolicy{QASignoff: &config.GatePolicy{Rings: []string{"test"}}}
	p, _, _ := gatedHarness(t, now, pol)
	ctx := context.Background()
	mustSeed(t, p, "int", "v1")
	if err := p.ValidatePromote(ctx, testApp, "int"); !errors.Is(err, ErrSignoffRequired) {
		t.Fatalf("ValidatePromote want ErrSignoffRequired, got %v", err)
	}
}

// fakeValidator accepts exactly one code.
type fakeValidator struct{ ok string }

func (f fakeValidator) Validate(_ context.Context, _, _, code string) error {
	if code == f.ok {
		return nil
	}
	return changerequest.ErrInvalidCode
}
