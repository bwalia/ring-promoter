package promoter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/example/ring-promoter/internal/changerequest"
	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/deployer"
	"github.com/example/ring-promoter/internal/grafana"
)

// customGatesGate runs every operator-named extra check that guards the target
// ring. Built-in QA / AppSec / Grafana still run first; these sit on top.
type customGatesGate struct{}

func (customGatesGate) Name() string { return "custom" }

func (g customGatesGate) Evaluate(ctx context.Context, p *Promoter, req GateRequest) GateResult {
	if req.Policy == nil || len(req.Policy.CustomGates) == 0 {
		return skipped(g.Name())
	}
	var passed []string
	for _, cg := range req.Policy.CustomGates {
		if !cg.Guards(req.TargetRing) {
			continue
		}
		res := p.evalCustom(ctx, req, cg, !isValidateOnly(ctx))
		if res.Verdict == GateVerdictSkipped {
			continue
		}
		if res.Err != nil {
			return res
		}
		passed = append(passed, res.Detail)
	}
	if len(passed) == 0 {
		return skipped(g.Name())
	}
	return pass(g.Name(), strings.Join(passed, "; "))
}

func (p *Promoter) evalCustom(ctx context.Context, req GateRequest, g config.CustomGate, attachRollout bool) GateResult {
	name := "custom:" + g.Name
	switch g.KindOr() {
	case config.CustomGateTest, config.CustomGateJIRA, config.CustomGateOpsAPI:
		return p.evalTicketGate(ctx, req, g, name)
	case config.CustomGateMetric:
		return p.evalMetricGate(ctx, req, g, name)
	case config.CustomGateReplicas:
		return p.evalReplicaGate(ctx, req, g, name, attachRollout)
	default:
		return fail(name, fmt.Errorf("%w: unknown custom gate kind %q", ErrCustomGate, g.Kind))
	}
}

func ticketCode(in GateInputs, kind string) string {
	sr := strings.TrimSpace(in.ServiceRequestCode)
	cr := strings.TrimSpace(in.ChangeRequestCode)
	if kind == config.CustomGateOpsAPI {
		if sr != "" {
			return sr
		}
		return cr
	}
	if cr != "" {
		return cr
	}
	return sr
}

func (p *Promoter) evalTicketGate(ctx context.Context, req GateRequest, g config.CustomGate, name string) GateResult {
	code := ticketCode(req.Inputs, g.KindOr())
	if code == "" {
		return fail(name, fmt.Errorf("%w: %s requires a ticket code to enter %s", ErrCustomGate, g.LabelOr(), req.TargetRing))
	}
	if strings.EqualFold(code, demoCRCode) {
		return pass(name, fmt.Sprintf("%s %q accepted (demo code)", g.LabelOr(), code))
	}
	if g.KindOr() == config.CustomGateTest {
		return fail(name, fmt.Errorf("%w: %s only accepts the demo code %q", ErrCustomGate, g.LabelOr(), demoCRCode))
	}
	insp := p.inspectorFor(req.App, g)
	if insp == nil {
		return fail(name, fmt.Errorf("%w: %s has no ticket backend configured", ErrCustomGate, g.LabelOr()))
	}
	tkt, err := insp.Inspect(ctx, code, changerequest.InspectOpts{
		VersionField: strings.TrimSpace(g.VersionField),
		Approvals:    g.MinApprovals > 0,
	})
	if err != nil {
		if errors.Is(err, changerequest.ErrInvalidCode) || errors.Is(err, changerequest.ErrCodeRequired) {
			return fail(name, fmt.Errorf("%w: %s: %s", ErrCustomGate, g.LabelOr(), err.Error()))
		}
		return fail(name, fmt.Errorf("%w: %s: %v", ErrCustomGate, g.LabelOr(), err))
	}
	if !statusAllowed(g, tkt.Status) {
		return fail(name, fmt.Errorf("%w: %s %q is %q, not an allowed status", ErrCustomGate, g.LabelOr(), code, tkt.Status))
	}
	if g.MinApprovals > 0 && tkt.Approvals < g.MinApprovals {
		return fail(name, fmt.Errorf("%w: %s %q has %d approval(s), need %d", ErrCustomGate, g.LabelOr(), code, tkt.Approvals, g.MinApprovals))
	}
	if g.MatchesVersion() {
		got := strings.TrimSpace(tkt.Version)
		if got == "" {
			return fail(name, fmt.Errorf("%w: %s %q has no version (field %q) — the request must name this build",
				ErrCustomGate, g.LabelOr(), code, versionField(g)))
		}
		if got != req.Version {
			return fail(name, fmt.Errorf("%w: %s %q is for version %q, not %q", ErrCustomGate, g.LabelOr(), code, got, req.Version))
		}
	}
	return pass(name, fmt.Sprintf("%s %q ok for %s", g.LabelOr(), code, req.TargetRing))
}

func versionField(g config.CustomGate) string {
	if s := strings.TrimSpace(g.VersionField); s != "" {
		return s
	}
	return "version"
}

func statusAllowed(g config.CustomGate, status string) bool {
	allowed := g.AllowedStatuses
	if len(allowed) == 0 {
		if g.KindOr() == config.CustomGateOpsAPI {
			allowed = []string{"approved"}
		} else {
			return true
		}
	}
	for _, a := range allowed {
		if strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(status)) {
			return true
		}
	}
	return false
}

func (p *Promoter) evalMetricGate(ctx context.Context, req GateRequest, g config.CustomGate, name string) GateResult {
	rule := g.Metric
	if rule == nil {
		return fail(name, fmt.Errorf("%w: %s has no metric rule", ErrCustomGate, g.LabelOr()))
	}
	var value float64
	if rule.DemoValue != nil {
		value = *rule.DemoValue
	} else {
		sample, err := p.queryMetric(ctx, req.App, req.TargetRing, rule)
		if err != nil {
			return fail(name, fmt.Errorf("%w: %s: %v", ErrCustomGate, g.LabelOr(), err))
		}
		value = sample.Value
	}
	if !rule.Compare(value) {
		return fail(name, fmt.Errorf("%w: %s is %g, need %s", ErrCustomGate, g.LabelOr(), value, rule.Describe()))
	}
	return pass(name, fmt.Sprintf("%s is %g (need %s)", g.LabelOr(), value, rule.Describe()))
}

func (p *Promoter) queryMetric(ctx context.Context, app, ringName string, rule *config.MetricRule) (grafana.Sample, error) {
	pol := p.policy(app)
	if pol == nil || pol.Grafana == nil || strings.TrimSpace(pol.Grafana.URL) == "" {
		return grafana.Sample{}, fmt.Errorf("no grafana url — set metric.demo_value or promotion_policy.grafana.url")
	}
	g := pol.Grafana
	uid := strings.TrimSpace(rule.DatasourceUID)
	if uid == "" {
		uid = g.DatasourceUID
	}
	dsType := strings.TrimSpace(rule.DatasourceType)
	if dsType == "" {
		dsType = g.DatasourceTypeOrDefault()
	}
	client := p.grafanaClientFor(app, g)
	env := p.targetEnv(app, ringName)
	return client.Query(ctx, grafana.Query{
		DatasourceUID:  uid,
		DatasourceType: dsType,
		Expr:           expandDashboardVars(rule.Query, env, g.WorkflowVar()),
		Lookback:       g.LookbackOrDefault(),
	})
}

func (p *Promoter) evalReplicaGate(ctx context.Context, req GateRequest, g config.CustomGate, name string, attachRollout bool) GateResult {
	rule := g.Replicas
	if rule == nil {
		return fail(name, fmt.Errorf("%w: %s has no replicas rule", ErrCustomGate, g.LabelOr()))
	}
	ready, err := p.readyReplicas(ctx, req.App, req.TargetRing, rule)
	if err != nil {
		return fail(name, fmt.Errorf("%w: %s: %v", ErrCustomGate, g.LabelOr(), err))
	}
	need := rule.MinReadyOr()
	if ready < need {
		return fail(name, fmt.Errorf("%w: %s has %d ready replica(s); need at least %d (a single replica cannot roll out gradually)",
			ErrCustomGate, g.LabelOr(), ready, need))
	}
	if attachRollout && rule.Gradual != nil {
		p.rememberRollout(req.App, req.TargetRing, rule.Gradual)
	}
	detail := fmt.Sprintf("%s: %d ready replica(s) (≥ %d)", g.LabelOr(), ready, need)
	if rule.Gradual != nil {
		detail += fmt.Sprintf("; gradual maxUnavailable=%s maxSurge=%s",
			rule.Gradual.MaxUnavailableOr(), rule.Gradual.MaxSurgeOr())
	}
	return pass(name, detail)
}

func (p *Promoter) readyReplicas(ctx context.Context, app, ringName string, rule *config.ReplicaRule) (int, error) {
	if rule.DemoReady != nil {
		return *rule.DemoReady, nil
	}
	rc, err := p.ringConfig(app, ringName)
	if err != nil {
		return 0, err
	}
	rr, ok := p.deployerFor(app).(deployer.ReplicaReader)
	if !ok {
		return 0, fmt.Errorf("this deployer cannot read readyReplicas — set replicas.demo_ready")
	}
	return rr.ReadyReplicas(ctx, p.target(app, ringName, rc))
}

func (p *Promoter) inspectorFor(app string, g config.CustomGate) changerequest.Inspector {
	key := app + "/" + g.Name
	p.customMu.Lock()
	if p.customInspectors != nil {
		if insp, ok := p.customInspectors[key]; ok {
			p.customMu.Unlock()
			return insp
		}
	}
	p.customMu.Unlock()

	switch g.KindOr() {
	case config.CustomGateJIRA:
		jc := g.JIRA
		if jc == nil {
			if pol := p.policy(app); pol != nil && pol.ChangeRequest != nil {
				jc = pol.ChangeRequest.JIRA
			}
		}
		if jc == nil || strings.TrimSpace(jc.BaseURL) == "" {
			return nil
		}
		return changerequest.NewJIRA(changerequest.JIRAParams{
			BaseURL:         jc.BaseURL,
			Email:           jc.Email,
			Token:           os.Getenv(jc.TokenEnvName()),
			AllowedStatuses: g.AllowedStatuses,
			ProjectKeys:     g.ProjectKeys,
			Log:             p.log,
		})
	case config.CustomGateOpsAPI:
		if g.OpsAPI == nil {
			return nil
		}
		return changerequest.NewOpsAPI(changerequest.OpsAPIParams{
			BaseURL:   g.OpsAPI.BaseURL,
			Path:      g.OpsAPI.PathOr(),
			Token:     os.Getenv(g.OpsAPI.TokenEnvName()),
			Namespace: g.OpsAPI.Namespace,
			Log:       p.log,
		})
	default:
		return nil
	}
}

// SetCustomInspector installs a ticket inspector for tests (app + gate name).
func (p *Promoter) SetCustomInspector(app, gate string, insp changerequest.Inspector) {
	p.customMu.Lock()
	defer p.customMu.Unlock()
	if p.customInspectors == nil {
		p.customInspectors = map[string]changerequest.Inspector{}
	}
	p.customInspectors[app+"/"+gate] = insp
}

func (p *Promoter) rememberRollout(app, ringName string, g *config.GradualRollout) {
	if g == nil {
		return
	}
	p.rolloutMu.Lock()
	defer p.rolloutMu.Unlock()
	if p.pendingRollout == nil {
		p.pendingRollout = map[string]*config.GradualRollout{}
	}
	p.pendingRollout[app+"/"+ringName] = g
}

func (p *Promoter) takeRollout(app, ringName string) *config.GradualRollout {
	p.rolloutMu.Lock()
	defer p.rolloutMu.Unlock()
	k := app + "/" + ringName
	g := p.pendingRollout[k]
	delete(p.pendingRollout, k)
	return g
}

func (p *Promoter) withPendingRollout(ctx context.Context, app, ringName string) context.Context {
	g := p.takeRollout(app, ringName)
	if g == nil {
		return ctx
	}
	return deployer.WithGradualRollout(ctx, deployer.GradualRollout{
		MaxUnavailable: g.MaxUnavailableOr(),
		MaxSurge:       g.MaxSurgeOr(),
	})
}
