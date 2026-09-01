package promoter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/example/ring-promoter/internal/changerequest"
	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/ring"
	"github.com/example/ring-promoter/internal/store"
)

// Gate precondition errors. Like the other precondition errors in this package
// they are returned BEFORE anything is deployed, and the API layer maps them to
// 4xx responses. They gate a version from entering a sensitive ring until an
// operator has satisfied the app's promotion policy.
var (
	// ErrMaintenanceWindowClosed: the target ring requires an active
	// maintenance window (config-recurring or ad-hoc) and none is open now.
	ErrMaintenanceWindowClosed = errors.New("maintenance window closed")
	// ErrSignoffRequired: no QA/release Go-No-Go sign-off exists for this exact
	// version and target ring.
	ErrSignoffRequired = errors.New("qa/release sign-off required")
	// ErrSignoffNoGo: a sign-off exists but its decision is no-go.
	ErrSignoffNoGo = errors.New("qa/release sign-off is no-go")
	// ErrChangeRequestRequired: the target ring requires a change-request code
	// and none was supplied.
	ErrChangeRequestRequired = errors.New("change-request code required")
	// ErrChangeRequestInvalid: a change-request code was supplied but did not
	// validate against the configured business system.
	ErrChangeRequestInvalid = errors.New("change-request code invalid")
	// ErrGrafanaNoGo: the target ring's Grafana dashboard reports no-go for the
	// release. Unlike the other gate errors this one is overridable — see
	// GateInputs.OverrideGrafana.
	ErrGrafanaNoGo = errors.New("grafana dashboard reports no-go")
	// ErrGrafanaOverrideReason: an override was requested without a reason. The
	// override is recorded in history, so it must say why.
	ErrGrafanaOverrideReason = errors.New("grafana override needs a reason")
)

// demoCRCode is the universal change-request code accepted by every provider so
// demos and tests never need a real business system. It is checked here, before
// any Validator is consulted.
const demoCRCode = "test"

// GateInputs carries the per-operation inputs a promotion gate may need — today
// just the change-request code the caller supplied. It is threaded through the
// operation context (like the progress Reporter) so Seed/Promote keep their
// signatures; the API attaches it from the request body, and it applies to the
// operation including any auto-promote hops it triggers.
type GateInputs struct {
	ChangeRequestCode string
	// OverrideGrafana lets a release engineer promote past a Grafana no-go. It
	// applies ONLY to the Grafana gate — the maintenance window, sign-off and
	// change-request gates have their own ways to be satisfied and are never
	// bypassed. An override requires OverrideReason and is written into the job
	// log and history so the decision is attributable after the fact.
	OverrideGrafana bool
	// OverrideReason explains why the dashboard was overruled.
	OverrideReason string
}

type gateInputsKey struct{}

// WithGateInputs returns a context carrying gate inputs for the operation.
func WithGateInputs(ctx context.Context, in GateInputs) context.Context {
	return context.WithValue(ctx, gateInputsKey{}, in)
}

// gateInputsFrom extracts gate inputs from the context (zero value if absent).
func gateInputsFrom(ctx context.Context) GateInputs {
	in, _ := ctx.Value(gateInputsKey{}).(GateInputs)
	return in
}

type validateOnlyKey struct{}

// withValidateOnly marks a context as a read-only pre-check. Gates evaluated
// under it must not emit audit events: the async API pre-validates every
// operation before spawning its job, so without this marker each accepted
// override would be recorded twice.
func withValidateOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, validateOnlyKey{}, true)
}

// isValidateOnly reports whether the context is a pre-check.
func isValidateOnly(ctx context.Context) bool {
	v, _ := ctx.Value(validateOnlyKey{}).(bool)
	return v
}

// SetChangeRequestValidators installs the per-app change-request validators
// (built from config in main). Apps absent from the map fall back to a
// validator that accepts only the demo code — so an enabled change-request gate
// is never silently a no-op.
func (p *Promoter) SetChangeRequestValidators(v map[string]changerequest.Validator) {
	p.crValidators = v
}

// policy returns the app's promotion policy, or nil when it has none.
func (p *Promoter) policy(app string) *config.PromotionPolicy {
	ac, ok := p.cfg.App(app)
	if !ok {
		return nil
	}
	return ac.PromotionPolicy
}

// evaluateGates enforces the app's promotion policy for deploying `version`
// into `targetRing` by walking the ordered promotionGates pipeline (see
// gate.go). It returns nil when the app has no policy or no gate guards the
// target ring, and the first failing gate's typed error otherwise. Every check
// runs read-only and before any deploy, so a gate failure leaves all state
// untouched. Non-skipped verdicts are audited (category "gate") under the
// operation's correlation id — except during validate-only pre-checks, so one
// operation records each gate once.
func (p *Promoter) evaluateGates(ctx context.Context, app, targetRing, version string, in GateInputs) error {
	pol := p.policy(app)
	if pol == nil {
		return nil
	}
	rep := reporterFrom(ctx)
	req := GateRequest{App: app, TargetRing: targetRing, Version: version, Inputs: in, Policy: pol}

	for _, g := range promotionGates {
		res := g.Evaluate(ctx, p, req)
		if res.Verdict == GateVerdictSkipped {
			continue
		}
		if res.Err == nil {
			rep.Log("gate: " + res.Detail)
		}
		if !isValidateOnly(ctx) {
			p.audit(ctx, store.AuditEvent{
				App: app, Ring: targetRing, Category: store.AuditGate, Action: res.Gate,
				Version: version,
				Detail:  auditDetail(map[string]string{"verdict": res.Verdict, "detail": res.Detail}),
			})
		}
		if res.Err != nil {
			return res.Err
		}
	}
	return nil
}

// dashboardName renders the dashboard behind a Grafana gate for an error
// message, falling back to a generic noun when it is unnamed.
func dashboardName(g *config.GrafanaPolicy) string {
	if t := g.DashboardTitle(); t != "" {
		return t
	}
	return "the release dashboard"
}

// validateChangeRequest runs the app's CR validator, mapping a rejection to the
// ErrChangeRequestInvalid precondition and a reachability failure to a plain
// error (which the API surfaces as 5xx — the business system, not the caller,
// is at fault).
func (p *Promoter) validateChangeRequest(ctx context.Context, app, ring, code string) error {
	v := p.crValidators[app]
	if v == nil {
		v = changerequest.Test{}
	}
	if err := v.Validate(ctx, app, ring, code); err != nil {
		if errors.Is(err, changerequest.ErrInvalidCode) || errors.Is(err, changerequest.ErrCodeRequired) {
			return fmt.Errorf("%w: %s", ErrChangeRequestInvalid, err)
		}
		return fmt.Errorf("validate change-request %q: %w", code, err)
	}
	return nil
}

// maintenanceOpenAt reports whether a maintenance window covering ring is open
// at t: any config-defined recurring window OR any active operator-created
// window. Rings the maintenance gate does not guard are reported open (there is
// nothing to gate).
func (p *Promoter) maintenanceOpenAt(ctx context.Context, app, ring string, t time.Time) (bool, error) {
	pol := p.policy(app)
	if pol == nil || pol.MaintenanceWindow == nil {
		return true, nil
	}
	if pol.MaintenanceWindow.OpenAt(t) {
		return true, nil
	}
	wins, err := p.store.ListMaintenanceWindows(ctx, app)
	if err != nil {
		return false, err
	}
	for _, w := range wins {
		if w.Covers(ring) && w.Active(t) {
			return true, nil
		}
	}
	return false, nil
}

// ValidatePromote checks a promotion's gates (and basic preconditions) without
// performing it, so the API can reject a gated promotion with a 4xx before
// spawning a doomed async job. It resolves the version to be promoted (the
// source ring's current version) and evaluates the target ring's gates.
func (p *Promoter) ValidatePromote(ctx context.Context, app, fromRing string) error {
	if _, err := p.ringConfig(app, fromRing); err != nil {
		return err
	}
	next, ok := ring.Next(fromRing)
	if !ok {
		return ErrNoNextRing
	}
	if _, err := p.ringConfig(app, next.Name); err != nil {
		return fmt.Errorf("target %s: %w", next.Name, err)
	}
	st, err := p.store.GetRingState(ctx, app, fromRing)
	if err != nil || st.CurrentVersion == "" {
		return ErrNothingToPromote
	}
	return p.evaluateGates(withValidateOnly(ctx), app, next.Name, st.CurrentVersion, gateInputsFrom(ctx))
}

// signoffBy renders a short attribution for a sign-off.
func signoffBy(s store.Signoff) string {
	who := s.Engineer
	if who == "" {
		who = "unknown"
	}
	if s.QAStatus != "" {
		return fmt.Sprintf("%s, qa=%s", who, s.QAStatus)
	}
	return who
}
