// The promotion-policy engine: an ordered pipeline of independent gates, each
// deciding whether a version may enter a guarded ring. The four V1 gates
// (maintenance window, QA sign-off, change request, Grafana) are Gate
// implementations; new policy kinds are added by appending to promotionGates,
// not by growing a single function. Every non-skipped verdict is audited under
// the operation's correlation id.
package promoter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/store"
)

// Gate verdicts recorded in the audit ledger.
const (
	GateVerdictPass       = "pass"
	GateVerdictFail       = "fail"
	GateVerdictOverridden = "overridden"
	GateVerdictSkipped    = "skipped" // the gate does not guard this ring
)

// GateRequest is everything a gate may inspect: the operation's coordinates,
// the caller-supplied inputs, and the app's promotion policy (never nil —
// evaluateGates returns early for apps without one).
type GateRequest struct {
	App        string
	TargetRing string
	Version    string
	Inputs     GateInputs
	Policy     *config.PromotionPolicy
}

// GateResult is one gate's verdict. Err carries the exact typed error the API
// maps to a status code; it is non-nil only for GateVerdictFail. Detail is the
// human line the job log shows (already produced via the progress reporter).
type GateResult struct {
	Gate    string
	Verdict string
	Err     error
	Detail  string
}

// Gate is one independent promotion-policy check. Evaluate must be read-only:
// a gate failure leaves all state untouched. Implementations receive the
// Promoter for its store, validators, caches and clock.
type Gate interface {
	Name() string
	Evaluate(ctx context.Context, p *Promoter, req GateRequest) GateResult
}

// promotionGates is the fixed, ordered policy pipeline. Order is part of the
// contract: earlier gates' errors win when several would fail.
var promotionGates = []Gate{
	maintenanceGate{},
	signoffGate{},
	changeRequestGate{},
	grafanaGate{},
}

func pass(name, detail string) GateResult {
	return GateResult{Gate: name, Verdict: GateVerdictPass, Detail: detail}
}

func fail(name string, err error) GateResult {
	return GateResult{Gate: name, Verdict: GateVerdictFail, Err: err, Detail: err.Error()}
}

func skipped(name string) GateResult {
	return GateResult{Gate: name, Verdict: GateVerdictSkipped}
}

// ---- 1. Maintenance window (config-recurring OR operator-created ad-hoc) ----

type maintenanceGate struct{}

func (maintenanceGate) Name() string { return "maintenance_window" }

func (g maintenanceGate) Evaluate(ctx context.Context, p *Promoter, req GateRequest) GateResult {
	if !req.Policy.MaintenanceWindow.Guards(req.TargetRing) {
		return skipped(g.Name())
	}
	open, err := p.maintenanceOpenAt(ctx, req.App, req.TargetRing, p.now())
	if err != nil {
		return fail(g.Name(), fmt.Errorf("check maintenance windows: %w", err))
	}
	if !open {
		return fail(g.Name(), fmt.Errorf("%w: no active maintenance window for %s (open one, or wait for a scheduled window)",
			ErrMaintenanceWindowClosed, req.TargetRing))
	}
	return pass(g.Name(), fmt.Sprintf("maintenance window open for %s", req.TargetRing))
}

// ---- 2. QA / release Go-No-Go sign-off for the exact version ----

type signoffGate struct{}

func (signoffGate) Name() string { return "qa_signoff" }

func (g signoffGate) Evaluate(ctx context.Context, p *Promoter, req GateRequest) GateResult {
	if !req.Policy.QASignoff.Guards(req.TargetRing) {
		return skipped(g.Name())
	}
	s, err := p.store.GetSignoff(ctx, req.App, req.TargetRing, req.Version)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return fail(g.Name(), fmt.Errorf("%w: %s needs a release-engineer sign-off for %s before it can be promoted",
			ErrSignoffRequired, req.Version, req.TargetRing))
	case err != nil:
		return fail(g.Name(), fmt.Errorf("check sign-off: %w", err))
	case !s.IsGo():
		return fail(g.Name(), fmt.Errorf("%w: %s sign-off for %s is %q (%s)",
			ErrSignoffNoGo, req.Version, req.TargetRing, s.Decision, signoffBy(s)))
	}
	return pass(g.Name(), fmt.Sprintf("%s signed off for %s by %s", req.Version, req.TargetRing, signoffBy(s)))
}

// ---- 3. Change-request code, validated against the app's business system ----

type changeRequestGate struct{}

func (changeRequestGate) Name() string { return "change_request" }

func (g changeRequestGate) Evaluate(ctx context.Context, p *Promoter, req GateRequest) GateResult {
	if !req.Policy.ChangeRequest.Guards(req.TargetRing) {
		return skipped(g.Name())
	}
	code := strings.TrimSpace(req.Inputs.ChangeRequestCode)
	if code == "" {
		return fail(g.Name(), fmt.Errorf("%w: promotion to %s requires a valid change-request code", ErrChangeRequestRequired, req.TargetRing))
	}
	if strings.EqualFold(code, demoCRCode) {
		return pass(g.Name(), fmt.Sprintf("change-request %q accepted (demo code)", code))
	}
	if err := p.validateChangeRequest(ctx, req.App, req.TargetRing, code); err != nil {
		return fail(g.Name(), err)
	}
	return pass(g.Name(), fmt.Sprintf("change-request %q validated for %s", code, req.TargetRing))
}

// ---- 4. Grafana go/no-go, read from the app's release dashboard ----
//
// Only an explicit no-go blocks: a "check" verdict is advisory, and a Grafana
// that cannot be reached must not become an outage of its own. This is the one
// overridable gate — see GateInputs.OverrideGrafana.

type grafanaGate struct{}

func (grafanaGate) Name() string { return "grafana" }

func (g grafanaGate) Evaluate(ctx context.Context, p *Promoter, req GateRequest) GateResult {
	if !req.Policy.Grafana.Guards(req.TargetRing) {
		return skipped(g.Name())
	}
	res := p.grafanaVerdict(ctx, req.App, req.TargetRing)
	switch {
	case !res.Verdict.Blocks():
		return pass(g.Name(), fmt.Sprintf("grafana %s for %s (%s)", res.Verdict, req.TargetRing, describeGrafana(res)))
	case req.Inputs.OverrideGrafana:
		reason := strings.TrimSpace(req.Inputs.OverrideReason)
		if reason == "" {
			return fail(g.Name(), fmt.Errorf("%w: state why the %s no-go is being overruled", ErrGrafanaOverrideReason, req.TargetRing))
		}
		p.log.Warn("grafana gate overridden",
			"app", req.App, "ring", req.TargetRing, "version", req.Version,
			"verdict", describeGrafana(res), "reason", reason)
		// Durably record the override even when the promotion then succeeds
		// (history only keeps step logs for failures, so without this row a
		// successful overridden promotion left no queryable trace). Skipped
		// during pre-validation so one operation records one override.
		if !isValidateOnly(ctx) {
			p.audit(ctx, store.AuditEvent{
				App: req.App, Ring: req.TargetRing, Category: store.AuditOverride, Action: "grafana.override",
				Version: req.Version,
				Detail:  auditDetail(map[string]string{"verdict": describeGrafana(res), "reason": reason}),
			})
		}
		return GateResult{
			Gate: g.Name(), Verdict: GateVerdictOverridden,
			Detail: fmt.Sprintf("grafana NO-GO for %s (%s) OVERRIDDEN — %s", req.TargetRing, describeGrafana(res), reason),
		}
	default:
		return fail(g.Name(), fmt.Errorf("%w: %s reports %s for %s (open the ring's panel to override)",
			ErrGrafanaNoGo, dashboardName(req.Policy.Grafana), describeGrafana(res), req.TargetRing))
	}
}
