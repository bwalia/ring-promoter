// Recovery of operations interrupted by a process restart.
//
// Every seed / promote-hop / rollback writes a PendingOp journal record just
// before its deploy and clears it once the outcome is recorded (see the
// journalStart calls in promoter.go). Deploys run out-of-process (a Kubernetes
// Job, a GitHub Actions workflow), so when this service is redeployed mid-
// operation the deploy itself usually still lands — but before this journal
// existed, nothing recorded it: ring state kept the old version and history
// showed nothing, as if the operation had never happened.
//
// At start-up the API layer lists the leftover journal records and resumes each
// one as a normal job: ResumePendingOp waits for the ring to report the
// deployed version healthy, then writes the ring state and history entry the
// interrupted operation would have written — and continues its auto-promote
// chain. An outcome that cannot be proven is recorded as a failure, so the
// operation is visible in history either way.
package promoter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/store"
)

// journalStart writes the write-ahead record for a deploy about to start and
// returns its ID (0 when journaling failed). A journaling failure must not
// block the deploy itself: it is logged and the operation proceeds
// unjournaled — a restart during it is then lost, exactly as before the
// journal existed.
func (p *Promoter) journalStart(ctx context.Context, op store.PendingOp) int64 {
	id, err := p.store.CreatePendingOp(ctx, op)
	if err != nil {
		p.log.Warn("write-ahead journal failed; a restart during this operation cannot be recovered",
			"err", err, "app", op.App, "ring", op.Ring, "action", op.Action, "version", op.Version)
		return 0
	}
	return id
}

// journalEnd clears a write-ahead record once the operation's outcome is
// recorded. It deliberately uses a fresh context: the operation's own context
// may already be canceled or past its deadline, and failing to clear the
// journal would make the next start-up "recover" an operation that finished.
func (p *Promoter) journalEnd(id int64) {
	if id == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := p.store.DeletePendingOp(ctx, id); err != nil {
		p.log.Error("clear write-ahead journal failed; the next start-up may re-resolve this operation",
			"err", err, "id", id)
	}
}

// PendingOps returns the journaled operations a previous process left in
// flight (it died mid-deploy). Called once at start-up by the API layer, which
// runs ResumePendingOp for each as a regular job.
func (p *Promoter) PendingOps(ctx context.Context) ([]store.PendingOp, error) {
	return p.store.ListPendingOps(ctx)
}

// ResumePendingOp resolves one operation whose owning process died mid-deploy:
// it waits for the target ring to report the deployed version healthy, then
// writes the ring state and history entry the interrupted operation would have
// written, and continues its auto-promote chain. When the outcome cannot be
// proven (the ring never reports the version, or cannot report versions at
// all), a failure entry is recorded instead so the operation never disappears
// silently.
func (p *Promoter) ResumePendingOp(ctx context.Context, op store.PendingOp) (Result, error) {
	rc, err := p.ringConfig(op.App, op.Ring)
	if err != nil {
		// The app/ring is no longer configured, so there is nothing to verify
		// the outcome against; drop the record instead of retrying forever.
		p.journalEnd(op.ID)
		return Result{}, fmt.Errorf("recover %s of %s to %s/%s: %w", op.Action, op.Version, op.App, op.Ring, err)
	}

	unlock, err := p.store.Lock(ctx, "app:"+op.App)
	if err != nil {
		return Result{}, fmt.Errorf("lock application: %w", err)
	}
	defer unlock()

	// During a rolling update the previous pod may still have been RUNNING this
	// operation while we waited for the lock — and may have finished it and
	// cleared the journal. Only resume a record that is still there.
	if _, err := p.store.GetPendingOp(ctx, op.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return Result{
				App: op.App, Action: op.Action, Ring: op.Ring, FromRing: op.FromRing, Version: op.Version,
				Success: true, Message: "operation was completed by its original owner; nothing to recover",
			}, nil
		}
		return Result{}, fmt.Errorf("recheck pending operation: %w", err)
	}
	defer p.journalEnd(op.ID)

	rep := reporterFrom(ctx)
	res := Result{App: op.App, Action: op.Action, Ring: op.Ring, FromRing: op.FromRing, Version: op.Version}
	rep.StartStep("recover", fmt.Sprintf("Recover interrupted %s of %s to %s", op.Action, op.Version, op.Ring))
	rep.Log("a previous ring-promoter process was stopped mid-operation; verifying whether the deploy landed")

	// Only a ring that reports its running version can PROVE the interrupted
	// deploy landed — on any other ring a stale old build answering "200 OK" is
	// indistinguishable from success. Ref-pinned rings are excluded too: their
	// pipeline decides the concrete version, so there is none to await.
	if rc.HealthURL == "" || !verifiesVersion(rc) || rc.Ref != "" {
		res.Message = fmt.Sprintf("%s of %s to %s was interrupted by a ring-promoter restart and its outcome cannot be verified (ring does not report its running version) — check the ring and re-run if needed",
			op.Action, op.Version, op.Ring)
		rep.FinishStep(StepFailed, res.Message)
		p.record(ctx, op.App, op.Ring, op.Action, op.PrevVersion, op.Version, store.ResultFailure, res.Message)
		res.State, _ = p.store.GetRingState(ctx, op.App, op.Ring)
		return res, nil
	}

	if healthErr := p.awaitVersion(ctx, rc, op.Version); healthErr != nil {
		res.Message = fmt.Sprintf("%s of %s to %s was interrupted by a ring-promoter restart and the ring never reported it healthy: %s",
			op.Action, op.Version, op.Ring, healthErr)
		rep.FinishStep(StepFailed, res.Message)
		// awaitVersion normally gives up BECAUSE ctx expired — record through a
		// detached context or this failure would itself vanish from history.
		rctx := context.WithoutCancel(ctx)
		p.record(rctx, op.App, op.Ring, op.Action, op.PrevVersion, op.Version, store.ResultFailure, res.Message)
		res.State, _ = p.store.GetRingState(rctx, op.App, op.Ring)
		return res, nil
	}
	rep.FinishStep(StepSuccess, fmt.Sprintf("%s is healthy and running %s", op.Ring, op.Version))

	res.State = p.saveState(ctx, op.App, op.Ring, op.PrevVersion, op.Version, true)
	res.Success = true
	switch op.Action {
	case store.ActionPromote:
		res.Message = fmt.Sprintf("promoted %s from %s to %s and healthy (recovered after a ring-promoter restart)", op.Version, op.FromRing, op.Ring)
	case store.ActionRollback:
		res.Message = fmt.Sprintf("rolled back to %s (recovered after a ring-promoter restart)", op.Version)
	default:
		res.Message = fmt.Sprintf("seeded %s and healthy (recovered after a ring-promoter restart)", op.Version)
	}
	p.record(ctx, op.App, op.Ring, op.Action, op.PrevVersion, op.Version, store.ResultSuccess, res.Message)
	if op.Action == store.ActionRollback {
		return res, nil
	}
	// The recovered ring may have auto-promote on: continue the chain exactly
	// as the interrupted operation would have.
	return p.autoChain(ctx, op.App, res)
}

// awaitVersion polls the ring's health endpoint until it reports `version` and
// healthy, or ctx expires. Unlike checkWithRetries it is not bounded by the
// retry config: the interrupted deploy may still be running out-of-process
// (e.g. a Kubernetes Job mid-build), so recovery keeps waiting for the whole
// operation budget for it to land. Returns the last health error on timeout.
func (p *Promoter) awaitVersion(ctx context.Context, rc config.RingConfig, version string) error {
	rep := reporterFrom(ctx)
	pr := probe(rc, version)
	for attempt := 1; ; attempt++ {
		err := p.checker.Check(ctx, pr)
		if err == nil {
			return nil
		}
		rep.Log(fmt.Sprintf("attempt %d: %s", attempt, err))
		select {
		case <-ctx.Done():
			return err
		case <-time.After(p.recoveryPoll):
		}
	}
}
