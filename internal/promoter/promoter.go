// Package promoter implements the ring-promotion rules that tie together the
// store, deployer and health checker. It is the heart of the service and is the
// primary unit under test.
//
// Rules enforced here:
//   - Promote one ring at a time; never skip a ring (order comes from package ring).
//   - The source ring must be healthy before promoting.
//   - After deploying to the target ring, run a health check with configurable retries.
//   - If it still fails, automatically roll the target ring back to its previous version.
//   - Record every seed / promote / rollback in history.
package promoter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/deployer"
	"github.com/example/ring-promoter/internal/health"
	"github.com/example/ring-promoter/internal/ring"
	"github.com/example/ring-promoter/internal/store"
)

// Precondition errors. The API layer maps these to 4xx responses; every other
// (attempted-but-failed) outcome is reported in a Result with Success=false.
var (
	ErrAppNotFound       = errors.New("application not found")
	ErrRingNotConfigured = errors.New("ring not configured for application")
	ErrNoNextRing        = errors.New("no next ring: already at the last ring")
	ErrEmptyVersion      = errors.New("version must not be empty")
	ErrNothingToPromote  = errors.New("source ring has no version to promote")
	ErrNothingToRollback = errors.New("ring has no previous version to roll back to")
)

// Result describes the outcome of a seed / promote / rollback operation.
type Result struct {
	App        string          `json:"app"`
	Action     string          `json:"action"`
	Ring       string          `json:"ring"`                // the affected (target) ring
	FromRing   string          `json:"from_ring,omitempty"` // promote source ring
	Version    string          `json:"version"`             // version acted upon
	Success    bool            `json:"success"`             // deploy + health succeeded
	RolledBack bool            `json:"rolled_back,omitempty"`
	Message    string          `json:"message"`
	State      store.RingState `json:"state"`
}

// RingView is the read model for one ring of an application (for GET .../rings).
type RingView struct {
	Ring            ring.Ring `json:"ring"`
	Configured      bool      `json:"configured"`
	CurrentVersion  string    `json:"current_version"`
	PreviousVersion string    `json:"previous_version"`
	LiveVersion     string    `json:"live_version"`
	Healthy         bool      `json:"healthy"`      // last stored health
	LiveHealthy     bool      `json:"live_healthy"` // fresh check at read time
	LiveHealthError string    `json:"live_health_error,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
	CanPromoteFrom  bool      `json:"can_promote_from"`
}

// Promoter orchestrates deployments across rings for all configured apps.
type Promoter struct {
	cfg        *config.Config
	store      store.Store
	deployer   deployer.Deployer
	checker    health.Checker
	log        *slog.Logger
	retryCount int
	retryDelay time.Duration
}

// New constructs a Promoter.
func New(cfg *config.Config, st store.Store, dep deployer.Deployer, chk health.Checker, log *slog.Logger) *Promoter {
	if log == nil {
		log = slog.Default()
	}
	return &Promoter{
		cfg:        cfg,
		store:      st,
		deployer:   dep,
		checker:    chk,
		log:        log,
		retryCount: cfg.Retry.RetryCount(),
		retryDelay: cfg.Retry.RetryDelay(),
	}
}

// Apps returns the configured application names.
func (p *Promoter) Apps() []string {
	names := make([]string, 0, len(p.cfg.Apps))
	for _, a := range p.cfg.Apps {
		names = append(names, a.Name)
	}
	return names
}

// History returns an application's history, newest first.
func (p *Promoter) History(ctx context.Context, app string) ([]store.HistoryEntry, error) {
	if _, ok := p.cfg.App(app); !ok {
		return nil, ErrAppNotFound
	}
	return p.store.ListHistory(ctx, app)
}

// Rings returns the read model for every ring of an application, including a
// fresh (live) health check and live version where available.
func (p *Promoter) Rings(ctx context.Context, app string) ([]RingView, error) {
	ac, ok := p.cfg.App(app)
	if !ok {
		return nil, ErrAppNotFound
	}

	all := ring.All()
	views := make([]RingView, len(all))
	var wg sync.WaitGroup

	for i, r := range all {
		rc, configured := ac.Rings[r.Name]
		v := RingView{Ring: r, Configured: configured}
		if st, err := p.store.GetRingState(ctx, app, r.Name); err == nil {
			v.CurrentVersion = st.CurrentVersion
			v.PreviousVersion = st.PreviousVersion
			v.Healthy = st.Healthy
			v.UpdatedAt = st.UpdatedAt
		}
		_, hasNext := ring.Next(r.Name)
		v.CanPromoteFrom = configured && hasNext && v.CurrentVersion != ""
		views[i] = v

		if !configured {
			continue
		}
		wg.Add(1)
		go func(idx int, rc config.RingConfig, tgt deployer.Target) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			if err := p.checker.Check(cctx, rc.HealthURL); err != nil {
				views[idx].LiveHealthy = false
				views[idx].LiveHealthError = err.Error()
			} else {
				views[idx].LiveHealthy = true
			}
			if lv, ok := p.deployer.(deployer.LiveVersioner); ok {
				if ver, err := lv.LiveVersion(cctx, tgt); err == nil {
					views[idx].LiveVersion = ver
				}
			}
		}(i, rc, p.target(app, r.Name, rc))
	}
	wg.Wait()
	return views, nil
}

// Seed sets an initial version for one ring, deploys it and health-checks it.
// It does not roll back on failure (there is no baseline to return to).
func (p *Promoter) Seed(ctx context.Context, app, ringName, version string) (Result, error) {
	if version == "" {
		return Result{}, ErrEmptyVersion
	}
	rc, err := p.ringConfig(app, ringName)
	if err != nil {
		return Result{}, err
	}
	unlock, err := p.store.Lock(ctx, "app:"+app)
	if err != nil {
		return Result{}, fmt.Errorf("lock application: %w", err)
	}
	defer unlock()

	tgt := p.target(app, ringName, rc)
	prev := p.currentVersion(ctx, app, ringName)
	res := Result{App: app, Action: store.ActionSeed, Ring: ringName, Version: version}

	if err := p.deployer.Deploy(ctx, tgt, version); err != nil {
		// The deploy never happened, so leave the stored state untouched.
		res.Message = "deploy failed: " + err.Error()
		res.State, _ = p.store.GetRingState(ctx, app, ringName)
		p.record(ctx, app, ringName, store.ActionSeed, prev, version, store.ResultFailure, res.Message)
		return res, nil
	}

	healthErr := p.checkWithRetries(ctx, rc.HealthURL)
	healthy := healthErr == nil
	res.State = p.saveState(ctx, app, ringName, prev, version, healthy)
	res.Success = healthy
	if healthy {
		res.Message = fmt.Sprintf("seeded %s and healthy", version)
		p.record(ctx, app, ringName, store.ActionSeed, prev, version, store.ResultSuccess, res.Message)
	} else {
		res.Message = "seeded but health check failed: " + healthErr.Error()
		p.record(ctx, app, ringName, store.ActionSeed, prev, version, store.ResultFailure, res.Message)
	}
	return res, nil
}

// Promote copies the source ring's current version to the next ring, then
// health-checks the target and auto-rolls-back on failure.
func (p *Promoter) Promote(ctx context.Context, app, fromRing string) (Result, error) {
	srcRC, err := p.ringConfig(app, fromRing)
	if err != nil {
		return Result{}, err
	}
	nextRing, ok := ring.Next(fromRing)
	if !ok {
		return Result{}, ErrNoNextRing
	}
	dstRC, err := p.ringConfig(app, nextRing.Name)
	if err != nil {
		return Result{}, fmt.Errorf("target %s: %w", nextRing.Name, err)
	}

	unlock, err := p.store.Lock(ctx, "app:"+app)
	if err != nil {
		return Result{}, fmt.Errorf("lock application: %w", err)
	}
	defer unlock()

	res := Result{App: app, Action: store.ActionPromote, Ring: nextRing.Name, FromRing: fromRing}

	// Source must have a version to promote.
	srcState, err := p.store.GetRingState(ctx, app, fromRing)
	if err != nil || srcState.CurrentVersion == "" {
		return Result{}, ErrNothingToPromote
	}
	version := srcState.CurrentVersion
	res.Version = version

	// Rule: source ring must be healthy before promoting (live check).
	if err := p.checker.Check(ctx, srcRC.HealthURL); err != nil {
		srcState.Healthy = false
		_ = p.store.UpsertRingState(ctx, srcState)
		res.Message = fmt.Sprintf("source ring %s is unhealthy, promotion aborted: %s", fromRing, err.Error())
		p.record(ctx, app, nextRing.Name, store.ActionPromote, "", version, store.ResultFailure, res.Message)
		return res, nil
	}

	dstTgt := p.target(app, nextRing.Name, dstRC)
	dstPrev := p.currentVersion(ctx, app, nextRing.Name)

	// Deploy to the target ring.
	if err := p.deployer.Deploy(ctx, dstTgt, version); err != nil {
		res.Message = "deploy to target failed: " + err.Error()
		p.record(ctx, app, nextRing.Name, store.ActionPromote, dstPrev, version, store.ResultFailure, res.Message)
		res.State, _ = p.store.GetRingState(ctx, app, nextRing.Name)
		return res, nil
	}

	// The deploy succeeded, so the cluster now runs `version`. Persist that
	// immediately (health not yet confirmed) so the stored state never lags the
	// cluster — even if the health check AND the auto-rollback below both fail.
	p.saveState(ctx, app, nextRing.Name, dstPrev, version, false)

	// Health-check the target with retries.
	if healthErr := p.checkWithRetries(ctx, dstRC.HealthURL); healthErr == nil {
		res.State = p.saveState(ctx, app, nextRing.Name, dstPrev, version, true)
		res.Success = true
		res.Message = fmt.Sprintf("promoted %s from %s to %s and healthy", version, fromRing, nextRing.Name)
		p.record(ctx, app, nextRing.Name, store.ActionPromote, dstPrev, version, store.ResultSuccess, res.Message)
		return res, nil
	} else {
		p.record(ctx, app, nextRing.Name, store.ActionPromote, dstPrev, version, store.ResultFailure,
			"health check failed after retries: "+healthErr.Error())
		p.log.Warn("promote health check failed, rolling back",
			"app", app, "ring", nextRing.Name, "version", version, "previous", dstPrev, "err", healthErr)
	}

	// Auto-rollback if there is a previous version. The promotion has failed
	// regardless of whether the rollback succeeds.
	if dstPrev == "" {
		res.State = p.saveState(ctx, app, nextRing.Name, dstPrev, version, false)
		res.Message = "promote failed health check and there is no previous version to roll back to"
		return res, nil
	}
	st, healthy, derr := p.rollbackTo(ctx, app, nextRing.Name, dstTgt, dstRC, version, dstPrev)
	res.State = st
	res.RolledBack = derr == nil
	switch {
	case derr != nil:
		res.Message = fmt.Sprintf("promote of %s failed health check and %s", version, derr.Error())
	case healthy:
		res.Message = fmt.Sprintf("promote of %s failed health check; rolled back to %s", version, dstPrev)
	default:
		res.Message = fmt.Sprintf("promote of %s failed health check; rolled back to %s but it is unhealthy", version, dstPrev)
	}
	return res, nil
}

// Rollback returns a ring to its previous version.
func (p *Promoter) Rollback(ctx context.Context, app, ringName string) (Result, error) {
	rc, err := p.ringConfig(app, ringName)
	if err != nil {
		return Result{}, err
	}
	unlock, err := p.store.Lock(ctx, "app:"+app)
	if err != nil {
		return Result{}, fmt.Errorf("lock application: %w", err)
	}
	defer unlock()

	st, err := p.store.GetRingState(ctx, app, ringName)
	if err != nil || st.PreviousVersion == "" {
		return Result{}, ErrNothingToRollback
	}
	from, to := st.CurrentVersion, st.PreviousVersion
	tgt := p.target(app, ringName, rc)

	res := Result{App: app, Action: store.ActionRollback, Ring: ringName, Version: to}
	newState, healthy, derr := p.rollbackTo(ctx, app, ringName, tgt, rc, from, to)
	res.State = newState
	switch {
	case derr != nil:
		res.Message = derr.Error()
	case healthy:
		res.RolledBack = true
		res.Success = true
		res.Message = fmt.Sprintf("rolled back %s from %s to %s", ringName, from, to)
	default:
		res.RolledBack = true
		res.Message = fmt.Sprintf("rolled back to %s but it is unhealthy", to)
	}
	return res, nil
}

// rollbackTo deploys `to` on the target, health-checks it, persists the state
// (current=to, previous=from) and records a rollback history entry. It returns
// the resulting state, whether the rolled-back version is healthy, and a
// non-nil error only if the rollback deploy itself failed.
func (p *Promoter) rollbackTo(ctx context.Context, app, ringName string, tgt deployer.Target, rc config.RingConfig, from, to string) (store.RingState, bool, error) {
	if err := p.deployer.Deploy(ctx, tgt, to); err != nil {
		msg := fmt.Sprintf("rollback deploy to %s failed: %s", to, err.Error())
		p.record(ctx, app, ringName, store.ActionRollback, from, to, store.ResultFailure, msg)
		st, _ := p.store.GetRingState(ctx, app, ringName)
		return st, false, errors.New(msg)
	}
	healthy := p.checkWithRetries(ctx, rc.HealthURL) == nil
	st := p.saveState(ctx, app, ringName, from, to, healthy)
	if healthy {
		p.record(ctx, app, ringName, store.ActionRollback, from, to, store.ResultSuccess,
			fmt.Sprintf("rolled back to %s", to))
	} else {
		p.record(ctx, app, ringName, store.ActionRollback, from, to, store.ResultFailure,
			fmt.Sprintf("rolled back to %s but it is unhealthy", to))
	}
	return st, healthy, nil
}

// checkWithRetries runs the health check up to retry.Count+1 times, waiting
// retry.Delay between attempts.
func (p *Promoter) checkWithRetries(ctx context.Context, url string) error {
	attempts := p.retryCount + 1
	if attempts < 1 {
		attempts = 1
	}
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(p.retryDelay):
			}
		}
		if err = p.checker.Check(ctx, url); err == nil {
			return nil
		}
		p.log.Warn("health check attempt failed", "url", url, "attempt", i+1, "attempts", attempts, "err", err)
	}
	return err
}

// saveState persists current=cur, previous=prev, healthy and returns the state.
func (p *Promoter) saveState(ctx context.Context, app, ringName, prev, cur string, healthy bool) store.RingState {
	st := store.RingState{
		App:             app,
		Ring:            ringName,
		CurrentVersion:  cur,
		PreviousVersion: prev,
		Healthy:         healthy,
	}
	if err := p.store.UpsertRingState(ctx, st); err != nil {
		p.log.Error("save ring state failed", "err", err, "app", app, "ring", ringName)
	}
	// Reflect the stored timestamp in the returned value.
	if saved, err := p.store.GetRingState(ctx, app, ringName); err == nil {
		return saved
	}
	return st
}

func (p *Promoter) record(ctx context.Context, app, ringName, action, from, to, result, msg string) {
	err := p.store.AddHistory(ctx, store.HistoryEntry{
		App: app, Ring: ringName, Action: action,
		FromVersion: from, ToVersion: to, Result: result, Message: msg,
	})
	if err != nil {
		p.log.Error("record history failed", "err", err, "app", app, "ring", ringName, "action", action)
	}
}

func (p *Promoter) currentVersion(ctx context.Context, app, ringName string) string {
	if st, err := p.store.GetRingState(ctx, app, ringName); err == nil {
		return st.CurrentVersion
	}
	return ""
}

func (p *Promoter) ringConfig(app, ringName string) (config.RingConfig, error) {
	ac, ok := p.cfg.App(app)
	if !ok {
		return config.RingConfig{}, ErrAppNotFound
	}
	if !ring.IsValid(ringName) {
		return config.RingConfig{}, ErrRingNotConfigured
	}
	rc, ok := ac.Rings[ringName]
	if !ok {
		return config.RingConfig{}, ErrRingNotConfigured
	}
	return rc, nil
}

func (p *Promoter) target(app, ringName string, rc config.RingConfig) deployer.Target {
	return deployer.Target{
		App:        app,
		Ring:       ringName,
		Namespace:  rc.Namespace,
		Deployment: rc.Deployment,
		Container:  rc.Container,
		Image:      rc.Image,
	}
}
