package promoter

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/grafana"
)

// The Grafana gate asks a dashboard whether a release is good enough to move on.
// Two things make it different from the other gates:
//
//   - It is READ from the dashboard rather than recorded by an operator, so its
//     verdict is fetched on every rings poll. Those polls are frequent, so
//     verdicts are cached briefly (grafanaCacheTTL) — the dashboard's own data
//     refreshes on the order of minutes anyway.
//   - It is the only gate that can be OVERRIDDEN. A no-go is a metric-based
//     opinion, and a release engineer looking at the same dashboard may
//     legitimately disagree. The override is per-operation and always carries a
//     reason into the job log and history.
const grafanaCacheTTL = 60 * time.Second

// grafanaClient is the slice of *grafana.Client the gate needs, so tests can
// substitute a stub without standing up an HTTP server.
type grafanaClient interface {
	Query(ctx context.Context, q grafana.Query) (grafana.Sample, error)
}

type grafanaCacheEntry struct {
	res grafana.Result
	at  time.Time
}

// grafanaVerdict returns the gate's verdict for deploying into targetRing,
// serving a cached answer when one is fresh enough. It never returns an error:
// a Grafana that cannot be reached yields VerdictUnknown, which does not block.
func (p *Promoter) grafanaVerdict(ctx context.Context, app, targetRing string) grafana.Result {
	pol := p.policy(app)
	if pol == nil || !pol.Grafana.Guards(targetRing) {
		return grafana.Result{Verdict: grafana.VerdictUnknown}
	}
	g := pol.Grafana
	key := app + "/" + targetRing

	p.grafanaMu.Lock()
	if e, ok := p.grafanaCache[key]; ok && p.now().Sub(e.at) < grafanaCacheTTL {
		p.grafanaMu.Unlock()
		return e.res
	}
	p.grafanaMu.Unlock()

	res := p.queryGrafana(ctx, app, targetRing, g)

	p.grafanaMu.Lock()
	if p.grafanaCache == nil {
		p.grafanaCache = map[string]grafanaCacheEntry{}
	}
	p.grafanaCache[key] = grafanaCacheEntry{res: res, at: p.now()}
	p.grafanaMu.Unlock()
	return res
}

// queryGrafana evaluates every check behind the gate once, without touching the
// cache, and rolls the results up into one verdict.
func (p *Promoter) queryGrafana(ctx context.Context, app, targetRing string, g *config.GrafanaPolicy) grafana.Result {
	targetEnv := p.targetEnv(app, targetRing)
	res := grafana.Result{
		Dashboard:    g.DashboardTitle(),
		DashboardURL: g.DashboardURL(targetEnv),
		CheckedAt:    p.now(),
	}

	// Demo mode: no Grafana configured, so the verdict is whatever the config
	// says. Keeps the gate demonstrable offline, like the "log" deployer.
	if g.DemoMode() {
		res.Demo = true
		switch g.DemoVerdictFor(targetRing) {
		case config.GrafanaVerdictNoGo:
			res.Verdict = grafana.VerdictNoGo
		case config.GrafanaVerdictCheck:
			res.Verdict = grafana.VerdictCheck
		default:
			res.Verdict = grafana.VerdictGo
		}
		// Demo checks mirror the configured verdict so the UI still lists them.
		for _, c := range g.Checks {
			res.Checks = append(res.Checks, grafana.CheckResult{
				Name: c.NameOr(), Verdict: res.Verdict,
			})
		}
		return res
	}

	client := p.grafanaClientFor(app, g)
	if client == nil {
		res.Verdict = grafana.VerdictUnknown
		res.Error = "no grafana client configured"
		return res
	}

	for _, c := range g.Checks {
		res.Checks = append(res.Checks, p.runCheck(ctx, app, targetRing, targetEnv, g, c, client))
	}
	res.Verdict = grafana.Rollup(res.Checks)
	return res
}

// runCheck evaluates one check: query, judge against its thresholds, then apply
// the staleness rule.
func (p *Promoter) runCheck(
	ctx context.Context,
	app, targetRing, targetEnv string,
	g *config.GrafanaPolicy,
	c config.GrafanaCheck,
	client grafanaClient,
) grafana.CheckResult {
	goMin, noGoMax := c.Thresholds(g)
	out := grafana.CheckResult{
		Name:    c.NameOr(),
		Unit:    c.UnitOr(g),
		GoMin:   goMin,
		NoGoMax: noGoMax,
	}

	sample, err := client.Query(ctx, grafana.Query{
		DatasourceUID:  g.DatasourceUID,
		DatasourceType: g.DatasourceTypeOrDefault(),
		Expr:           expandDashboardVars(c.Query, targetEnv, g.WorkflowVar()),
		Lookback:       g.LookbackOrDefault(),
	})
	if err != nil {
		out.Verdict = grafana.VerdictUnknown
		out.Error = err.Error()
		p.log.Warn("grafana gate check could not be evaluated",
			"app", app, "ring", targetRing, "check", out.Name, "error", err)
		return out
	}

	out.Value = &sample.Value
	out.Verdict = grafana.Judge(sample.Value, goMin, noGoMax)
	if !sample.At.IsZero() {
		at := sample.At
		out.RanAt = &at
	}

	// Staleness. A nightly suite that stopped running would otherwise keep
	// reporting the verdict of its last run indefinitely; past max_age the
	// answer is "we don't know" — advisory, never blocking, and visible.
	if maxAge := g.MaxAge; maxAge > 0 {
		switch {
		case out.RanAt == nil:
			out.Verdict = grafana.VerdictUnknown
			out.Error = "query reports no run time, so its age cannot be checked " +
				"(select the run timestamp, e.g. created_at AS \"time\")"
		case p.now().Sub(*out.RanAt) > maxAge:
			out.Stale = true
			out.Verdict = grafana.VerdictUnknown
			out.Error = fmt.Sprintf("last run was %s ago, older than the %s limit",
				roundedAge(p.now().Sub(*out.RanAt)), maxAge)
		}
	}
	return out
}

// roundedAge renders a staleness age readably ("38h", "3m").
func roundedAge(d time.Duration) string {
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return "moments"
	}
}

// grafanaClientFor returns (and memoises) the app's Grafana client.
func (p *Promoter) grafanaClientFor(app string, g *config.GrafanaPolicy) grafanaClient {
	p.grafanaMu.Lock()
	defer p.grafanaMu.Unlock()
	if c, ok := p.grafanaClients[app]; ok {
		return c
	}
	c := grafanaClient(grafana.New(g.URL, os.Getenv(g.TokenEnvName()), 10*time.Second))
	if p.grafanaClients == nil {
		p.grafanaClients = map[string]grafanaClient{}
	}
	p.grafanaClients[app] = c
	return c
}

// SetGrafanaClient installs a Grafana client for an app, replacing the one
// built from config. Used by tests.
func (p *Promoter) SetGrafanaClient(app string, c grafanaClient) {
	p.grafanaMu.Lock()
	defer p.grafanaMu.Unlock()
	if p.grafanaClients == nil {
		p.grafanaClients = map[string]grafanaClient{}
	}
	p.grafanaClients[app] = c
	// Verdicts are cached per app/ring, so swapping the client has to drop every
	// ring's entry for that app or the old client's answers linger.
	for key := range p.grafanaCache {
		if strings.HasPrefix(key, app+"/") {
			delete(p.grafanaCache, key)
		}
	}
}

// targetEnv resolves the environment name a ring deploys to, which is what the
// dashboard's $env variable expects. Falls back to the ring name (int, test,
// acc, prod), which is what the diytaxreturn dashboards use anyway.
func (p *Promoter) targetEnv(app, ringName string) string {
	if ac, ok := p.cfg.App(app); ok {
		if rc, ok := ac.Rings[ringName]; ok && strings.TrimSpace(rc.TargetEnv) != "" {
			return rc.TargetEnv
		}
	}
	return ringName
}

// expandDashboardVars substitutes the dashboard variables a panel query carries
// ($env and $workflow, in both their bare and ${...:raw} spellings) so a query
// copied straight out of a dashboard JSON runs unmodified. Grafana's own
// $__timeFrom()/$__timeTo() macros are left alone — the server expands those.
func expandDashboardVars(query, env, workflow string) string {
	return strings.NewReplacer(
		"${env:raw}", env,
		"${env}", env,
		"$env", env,
		"${workflow:raw}", workflow,
		"${workflow}", workflow,
		"$workflow", workflow,
	).Replace(query)
}

// grafanaGateState is the per-Promoter state backing the gate: a small verdict
// cache and the per-app clients. Both are guarded by grafanaMu.
type grafanaGateState struct {
	grafanaMu      sync.Mutex
	grafanaCache   map[string]grafanaCacheEntry
	grafanaClients map[string]grafanaClient
}

// describeGrafana renders a verdict for a log line or an error message. A no-go
// names the checks responsible — "which suite is red?" is the first question
// anyone asks, and the answer belongs in the error, not a second lookup.
func describeGrafana(res grafana.Result) string {
	if res.Demo {
		return fmt.Sprintf("%s (demo)", res.Verdict)
	}
	if len(res.Checks) == 0 {
		if res.Error != "" {
			return fmt.Sprintf("unavailable (%s)", res.Error)
		}
		return "unavailable"
	}
	var blocking, other []string
	for _, c := range res.Checks {
		switch c.Verdict {
		case grafana.VerdictNoGo:
			blocking = append(blocking, describeCheck(c))
		case grafana.VerdictGo:
		default:
			other = append(other, describeCheck(c))
		}
	}
	switch {
	case len(blocking) > 0:
		return strings.Join(blocking, ", ")
	case len(other) > 0:
		return strings.Join(other, ", ")
	default:
		return fmt.Sprintf("all %d checks passing", len(res.Checks))
	}
}

// describeCheck renders one check as "name = value" (or the reason it has none).
func describeCheck(c grafana.CheckResult) string {
	switch {
	case c.Stale:
		return fmt.Sprintf("%s stale (%s)", c.Name, c.Error)
	case c.Value != nil:
		return fmt.Sprintf("%s = %s", c.Name, formatValue(*c.Value, c.Unit))
	case c.Error != "":
		return fmt.Sprintf("%s unavailable (%s)", c.Name, c.Error)
	default:
		return fmt.Sprintf("%s %s", c.Name, c.Verdict)
	}
}

// formatValue renders a metric without trailing zeroes ("2", "98.4%").
func formatValue(v float64, unit string) string {
	s := strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
	return s + unit
}
