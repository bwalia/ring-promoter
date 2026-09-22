package promoter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/deployer"
	"github.com/example/ring-promoter/internal/store"
)

func TestRestart_RestartsTargetAndRecordsHistory(t *testing.T) {
	p, dep, chk, st := newHarness(t, 1)
	ctx := context.Background()
	mustSeed(t, p, "int", "v1")
	mustSeed(t, p, "int", "v2") // previous = v1, current = v2
	deploysBefore := dep.deployCount()
	checksBefore := chk.checkCount(testApp, "int")

	res, err := p.Restart(ctx, testApp, "int", "rotated db password")
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if !res.Success || res.Action != store.ActionRestart || res.Ring != "int" || res.Version != "v2" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if got := dep.restartCalls(); len(got) != 1 || got[0] != key(testApp, "int") {
		t.Fatalf("restart calls = %v, want exactly [%s]", got, key(testApp, "int"))
	}
	if dep.deployCount() != deploysBefore {
		t.Fatalf("restart must not deploy: deploys %d -> %d", deploysBefore, dep.deployCount())
	}
	if chk.checkCount(testApp, "int") <= checksBefore {
		t.Fatal("restart must health-check the ring")
	}

	// Versions are untouched.
	s := mustState(t, st, testApp, "int")
	if s.CurrentVersion != "v2" || s.PreviousVersion != "v1" || !s.Healthy {
		t.Fatalf("bad state after restart: %+v", s)
	}

	// A "restart" history entry at the current version, carrying the reason.
	hist, err := p.History(ctx, testApp)
	if err != nil {
		t.Fatal(err)
	}
	h := hist[0]
	if h.Action != store.ActionRestart || h.Ring != "int" || h.ToVersion != "v2" ||
		h.FromVersion != "" || h.Result != store.ResultSuccess ||
		!strings.Contains(h.Message, "rotated db password") {
		t.Fatalf("bad history entry: %+v", h)
	}
}

func TestRestart_UnhealthyRecordsFailureKeepsVersions(t *testing.T) {
	p, dep, chk, st := newHarness(t, 0)
	mustSeed(t, p, "int", "v1")
	chk.markUnhealthy(testApp, "int", "v1")

	res, err := p.Restart(context.Background(), testApp, "int", "")
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if res.Success || res.RolledBack {
		t.Fatalf("expected an unsuccessful restart without rollback, got %+v", res)
	}
	if dep.deployCount() != 1 {
		t.Fatalf("restart must never redeploy or roll back, deploys = %d", dep.deployCount())
	}
	s := mustState(t, st, testApp, "int")
	if s.CurrentVersion != "v1" || s.PreviousVersion != "" || s.Healthy {
		t.Fatalf("bad state after failed restart: %+v", s)
	}
	hist, _ := p.History(context.Background(), testApp)
	if hist[0].Action != store.ActionRestart || hist[0].Result != store.ResultFailure {
		t.Fatalf("expected a failed restart entry, got %+v", hist[0])
	}
}

func TestRestart_DeployerErrorRecordsFailure(t *testing.T) {
	p, dep, _, st := newHarness(t, 1)
	mustSeed(t, p, "int", "v1")
	dep.restartErr = errors.New("rollout status: timed out")

	res, err := p.Restart(context.Background(), testApp, "int", "")
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if res.Success || !strings.Contains(res.Message, "timed out") {
		t.Fatalf("unexpected result: %+v", res)
	}
	if s := mustState(t, st, testApp, "int"); s.CurrentVersion != "v1" {
		t.Fatalf("version changed: %+v", s)
	}
	hist, _ := p.History(context.Background(), testApp)
	if hist[0].Action != store.ActionRestart || hist[0].Result != store.ResultFailure {
		t.Fatalf("expected a failed restart entry, got %+v", hist[0])
	}
}

func TestRestart_UnsupportedDeployerIsPrecondition(t *testing.T) {
	p, dep, _, _ := newHarness(t, 1)
	mustSeed(t, p, "int", "v1")
	dep.restartErr = fmt.Errorf("%w: test", deployer.ErrRestartUnsupported)

	if _, err := p.Restart(context.Background(), testApp, "int", ""); !errors.Is(err, deployer.ErrRestartUnsupported) {
		t.Fatalf("expected ErrRestartUnsupported, got %v", err)
	}
	hist, _ := p.History(context.Background(), testApp)
	if hist[0].Action == store.ActionRestart {
		t.Fatalf("an unsupported restart must not be recorded: %+v", hist[0])
	}
}

func TestRestart_Preconditions(t *testing.T) {
	p, dep, _, _ := newHarness(t, 1)
	ctx := context.Background()
	if _, err := p.Restart(ctx, "nope", "int", ""); !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("unknown app: expected ErrAppNotFound, got %v", err)
	}
	if _, err := p.Restart(ctx, testApp, "ring99", ""); !errors.Is(err, ErrRingNotConfigured) {
		t.Fatalf("unknown ring: expected ErrRingNotConfigured, got %v", err)
	}
	if _, err := p.Restart(ctx, testApp, "int", ""); !errors.Is(err, ErrNothingToRestart) {
		t.Fatalf("never-deployed ring: expected ErrNothingToRestart, got %v", err)
	}
	if err := p.ValidateRestart(ctx, testApp, "int"); !errors.Is(err, ErrNothingToRestart) {
		t.Fatalf("ValidateRestart: expected ErrNothingToRestart, got %v", err)
	}
	if n := len(dep.restartCalls()); n != 0 {
		t.Fatalf("no restart may run on a precondition failure, got %d", n)
	}
}

// A restart is not a promotion: gates that guard version changes into a ring
// (closed maintenance window, missing QA sign-off) must not block it.
func TestRestart_NotBlockedByPromotionGates(t *testing.T) {
	wed := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC) // outside the window
	p, dep, _ := gatedHarness(t, wed, nil)
	ctx := context.Background()
	mustSeed(t, p, "test", "v1") // deployed before the policy existed

	p.cfg.Apps[0].PromotionPolicy = &config.PromotionPolicy{
		MaintenanceWindow: &config.MaintenanceWindowPolicy{
			Rings:     []string{"test"},
			Recurring: []config.RecurringWindow{{Days: []string{"Sat"}, Start: "02:00", End: "04:00", Timezone: "UTC"}},
		},
		QASignoff: &config.GatePolicy{Rings: []string{"test"}},
	}
	if err := p.ValidateSeed(ctx, testApp, "test", "v1"); err == nil {
		t.Fatal("test setup: expected the gates to block a seed into test")
	}

	res, err := p.Restart(ctx, testApp, "test", "")
	if err != nil || !res.Success {
		t.Fatalf("restart must bypass promotion gates: res=%+v err=%v", res, err)
	}
	if n := len(dep.restartCalls()); n != 1 {
		t.Fatalf("restart calls = %d, want 1", n)
	}
}
