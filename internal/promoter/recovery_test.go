package promoter

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/deployer"
	"github.com/example/ring-promoter/internal/store"
)

// verifyingHarness builds a harness whose rings report their running version
// (health_version_field set), which is what recovery needs to prove an
// interrupted deploy landed.
func verifyingHarness(t *testing.T) (*Promoter, *fakeDeployer, *scriptedChecker, store.Store) {
	t.Helper()
	p, dep, chk, st := newHarnessWithRings(t, 0, func(_ string, rc *config.RingConfig) {
		rc.HealthVersionField = "version"
	})
	p.recoveryPoll = time.Millisecond
	return p, dep, chk, st
}

func pendingOps(t *testing.T, st store.Store) []store.PendingOp {
	t.Helper()
	ops, err := st.ListPendingOps(context.Background())
	if err != nil {
		t.Fatalf("list pending ops: %v", err)
	}
	return ops
}

// Every completed operation — successful or failed — must leave no journal
// entry behind: a leftover record means the next start-up would try to
// "recover" an operation that already recorded its outcome.
func TestJournal_ClearedAfterCompletedOperations(t *testing.T) {
	p, dep, _, st := verifyingHarness(t)
	ctx := context.Background()

	if _, err := p.Seed(ctx, testApp, "int", "v1"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := p.Promote(ctx, testApp, "int"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	dep.failVersion("v9")
	if res, err := p.Seed(ctx, testApp, "int", "v9"); err != nil || res.Success {
		t.Fatalf("expected failed seed result, got %+v err=%v", res, err)
	}
	if _, err := p.Seed(ctx, testApp, "int", "v2"); err != nil {
		t.Fatalf("seed v2: %v", err)
	}
	if _, err := p.Rollback(ctx, testApp, "int"); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	if ops := pendingOps(t, st); len(ops) != 0 {
		t.Fatalf("expected no pending ops after completed operations, got %+v", ops)
	}
}

// The incident this journal exists for: the deploy landed in the ring, but the
// process died before recording it. Recovery must write the ring state and the
// history entry — and continue the auto-promote chain the seed would have run.
func TestResumePendingOp_RecoversLandedSeedAndAutoChains(t *testing.T) {
	p, dep, _, st := verifyingHarness(t)
	ctx := context.Background()

	// Baseline: v1 healthy in int, auto-promote onward enabled.
	mustSeed(t, p, "int", "v1")
	if err := p.SetAutoPromote(ctx, testApp, "int", true); err != nil {
		t.Fatalf("set auto-promote: %v", err)
	}

	// Simulate the crash: the ring-exec deploy of v2 landed in the cluster...
	if err := dep.Deploy(ctx, deployer.Target{App: testApp, Ring: "int"}, "v2"); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	// ...but the process died before recording anything: only the journal row
	// survives, ring state still says v1.
	opID, err := st.CreatePendingOp(ctx, store.PendingOp{
		App: testApp, Ring: "int", Action: store.ActionSeed, Version: "v2", PrevVersion: "v1",
	})
	if err != nil {
		t.Fatalf("create pending op: %v", err)
	}

	op, err := st.GetPendingOp(ctx, opID)
	if err != nil {
		t.Fatalf("get pending op: %v", err)
	}
	res, err := p.ResumePendingOp(ctx, op)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected recovered success, got %+v", res)
	}

	if s := mustState(t, st, testApp, "int"); s.CurrentVersion != "v2" || s.PreviousVersion != "v1" || !s.Healthy {
		t.Fatalf("int state not recovered: %+v", s)
	}
	// Auto-promote continued the chain into test.
	if s := mustState(t, st, testApp, "test"); s.CurrentVersion != "v2" {
		t.Fatalf("auto-promote after recovery did not reach test: %+v", s)
	}

	hist, err := st.ListHistory(ctx, testApp)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	var recovered, promoted bool
	for _, h := range hist {
		if h.Action == store.ActionSeed && h.ToVersion == "v2" && h.Result == store.ResultSuccess &&
			strings.Contains(h.Message, "recovered after a ring-promoter restart") {
			recovered = true
		}
		if h.Action == store.ActionPromote && h.Ring == "test" && h.ToVersion == "v2" && h.Result == store.ResultSuccess {
			promoted = true
		}
	}
	if !recovered || !promoted {
		t.Fatalf("expected recovered seed + follow-on promote in history, got %+v", hist)
	}

	if ops := pendingOps(t, st); len(ops) != 0 {
		t.Fatalf("journal not cleared after recovery: %+v", ops)
	}
}

// A ring that cannot report its running version cannot prove the interrupted
// deploy landed: recovery must record an explicit failure — never a guessed
// success, and never nothing at all.
func TestResumePendingOp_UnverifiableRingRecordsFailure(t *testing.T) {
	p, _, _, st := newHarness(t, 0) // rings have a health URL but no version field
	p.recoveryPoll = time.Millisecond
	ctx := context.Background()

	opID, err := st.CreatePendingOp(ctx, store.PendingOp{
		App: testApp, Ring: "int", Action: store.ActionSeed, Version: "v2",
	})
	if err != nil {
		t.Fatalf("create pending op: %v", err)
	}
	op, _ := st.GetPendingOp(ctx, opID)

	res, err := p.ResumePendingOp(ctx, op)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if res.Success {
		t.Fatalf("expected unverifiable recovery to fail, got %+v", res)
	}
	hist, _ := st.ListHistory(ctx, testApp)
	if len(hist) != 1 || hist[0].Result != store.ResultFailure ||
		!strings.Contains(hist[0].Message, "cannot be verified") {
		t.Fatalf("expected one unverifiable-failure entry, got %+v", hist)
	}
	// State must be untouched — recovery proved nothing.
	if _, err := st.GetRingState(ctx, testApp, "int"); err != store.ErrNotFound {
		t.Fatalf("expected no ring state, got err=%v", err)
	}
	if ops := pendingOps(t, st); len(ops) != 0 {
		t.Fatalf("journal not cleared: %+v", ops)
	}
}

// When the deployed version never becomes healthy within the operation budget,
// recovery records the failure instead of hanging or losing the operation.
func TestResumePendingOp_VersionNeverHealthyRecordsFailure(t *testing.T) {
	p, dep, _, st := verifyingHarness(t)
	bg := context.Background()

	// The ring still runs v1; the interrupted deploy of v2 never landed.
	if err := dep.Deploy(bg, deployer.Target{App: testApp, Ring: "int"}, "v1"); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	opID, err := st.CreatePendingOp(bg, store.PendingOp{
		App: testApp, Ring: "int", Action: store.ActionSeed, Version: "v2", PrevVersion: "v1",
	})
	if err != nil {
		t.Fatalf("create pending op: %v", err)
	}
	op, _ := st.GetPendingOp(bg, opID)

	ctx, cancel := context.WithTimeout(bg, 50*time.Millisecond)
	defer cancel()
	res, err := p.ResumePendingOp(ctx, op)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if res.Success {
		t.Fatalf("expected recovery failure, got %+v", res)
	}
	hist, _ := st.ListHistory(bg, testApp)
	if len(hist) != 1 || hist[0].Result != store.ResultFailure ||
		!strings.Contains(hist[0].Message, "never reported it healthy") {
		t.Fatalf("expected one never-healthy failure entry, got %+v", hist)
	}
	if ops := pendingOps(t, st); len(ops) != 0 {
		t.Fatalf("journal not cleared: %+v", ops)
	}
}

// During a rolling update the old pod can finish the operation (and clear the
// journal) while the new pod waits for the app lock: resuming a record that is
// gone must be a no-op, not a duplicate history entry.
func TestResumePendingOp_AlreadyCompletedIsNoOp(t *testing.T) {
	p, _, _, st := verifyingHarness(t)
	ctx := context.Background()

	opID, err := st.CreatePendingOp(ctx, store.PendingOp{
		App: testApp, Ring: "int", Action: store.ActionSeed, Version: "v2",
	})
	if err != nil {
		t.Fatalf("create pending op: %v", err)
	}
	op, _ := st.GetPendingOp(ctx, opID)
	// The original owner finished and cleared the journal.
	if err := st.DeletePendingOp(ctx, opID); err != nil {
		t.Fatalf("delete pending op: %v", err)
	}

	res, err := p.ResumePendingOp(ctx, op)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !res.Success || !strings.Contains(res.Message, "nothing to recover") {
		t.Fatalf("expected completed-elsewhere no-op, got %+v", res)
	}
	if hist, _ := st.ListHistory(ctx, testApp); len(hist) != 0 {
		t.Fatalf("no-op recovery must not write history, got %+v", hist)
	}
}
