package promoter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/grafana"
	"github.com/example/ring-promoter/internal/ring"
	"github.com/example/ring-promoter/internal/store"
)

// Control statuses for one weighted check on a promotion hop.
const (
	ControlPass  = "pass"
	ControlWarn  = "warn"
	ControlBlock = "block"
	ControlSkip  = "skip"
)

// ControlScore is one daily DevOps check — health, QA, AppSec, a policy gate —
// scored 0–100 and weighted into the hop total. Skip means the control is off
// for this hop and is dropped from the denominator.
type ControlScore struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Kind       string `json:"kind"`
	Weight     int    `json:"weight"`
	Score      int    `json:"score"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	NextAction string `json:"next_action,omitempty"`
}

// HopScore is the scorecard for moving one version from From into To.
type HopScore struct {
	From     string         `json:"from"`
	To       string         `json:"to"`
	ToLabel  string         `json:"to_label"`
	Version  string         `json:"version"`
	Score    int            `json:"score"`
	Ready    bool           `json:"ready"`
	Blockers []string       `json:"blockers"`
	Controls []ControlScore `json:"controls"`
	Summary  string         `json:"summary"`
}

// ControlsView is GET /api/apps/{app}/controls: every configured hop, scored
// the way a release engineer asks "can I promote this today, and why not?".
type ControlsView struct {
	App       string     `json:"app"`
	AIEnabled bool       `json:"ai_enabled"`
	Hops      []HopScore `json:"hops"`
}

// AppControls scores every configured promotion hop for app.
func (p *Promoter) AppControls(ctx context.Context, app string) (ControlsView, error) {
	views, err := p.Rings(ctx, app)
	if err != nil {
		return ControlsView{}, err
	}
	hist, err := p.store.ListHistory(ctx, app)
	if err != nil {
		return ControlsView{}, err
	}
	weights := config.DefaultScoreWeights()
	if pol := p.policy(app); pol != nil {
		weights = pol.Score.OrDefault()
	}
	byName := map[string]RingView{}
	for _, v := range views {
		byName[v.Ring.Name] = v
	}
	out := ControlsView{App: app, Hops: []HopScore{}}
	// Seed the first configured ring when it has no version — the daily "what
	// do I put in int?" question CI never answers.
	first := firstConfigured(views)
	if first != nil && first.CurrentVersion == "" {
		out.Hops = append(out.Hops, p.scoreHop(ctx, app, nil, first, "", hist, weights))
	}
	for i := range views {
		v := views[i]
		if !v.Configured || v.CurrentVersion == "" || !v.CanPromoteFrom {
			continue
		}
		next, ok := ring.Next(v.Ring.Name)
		if !ok {
			continue
		}
		tgt, ok := byName[next.Name]
		if !ok || !tgt.Configured {
			continue
		}
		src := v
		out.Hops = append(out.Hops, p.scoreHop(ctx, app, &src, &tgt, v.CurrentVersion, hist, weights))
	}
	return out, nil
}

// HopControls scores one hop (from may be empty for a seed into to).
func (p *Promoter) HopControls(ctx context.Context, app, from, to string) (HopScore, error) {
	view, err := p.AppControls(ctx, app)
	if err != nil {
		return HopScore{}, err
	}
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	for _, h := range view.Hops {
		if h.From == from && h.To == to {
			return h, nil
		}
	}
	return HopScore{}, fmt.Errorf("%w: no hop %s → %s", ErrRingNotConfigured, from, to)
}

func firstConfigured(views []RingView) *RingView {
	for i := range views {
		if views[i].Configured {
			return &views[i]
		}
	}
	return nil
}

func (p *Promoter) scoreHop(ctx context.Context, app string, src, target *RingView, version string, hist []store.HistoryEntry, w config.ScoreWeights) HopScore {
	from, to := "", target.Ring.Name
	srcHealthy, srcErr := true, ""
	var ttfb *int64
	if src != nil {
		from = src.Ring.Name
		srcHealthy = src.LiveHealthy
		srcErr = src.LiveHealthError
		ttfb = src.TTFBms
		if version == "" {
			version = src.CurrentVersion
		}
	}
	controls := []ControlScore{
		scoreHealth(from, srcHealthy, srcErr, ttfb, w.Health),
		p.scoreQAAgent(ctx, app, from, to, w.QAAgent),
		p.scoreSignoff(ctx, app, to, version, *target, w.QASignoff),
		p.scoreSecurity(ctx, app, to, version, w.Security),
		scoreGrafana(*target, w.Grafana),
		scoreWindow(*target, w.Window),
		scoreChangeRequest(*target, w.ChangeRequest),
		scoreOps(from, to, hist, w.Ops),
	}
	if pol := p.policy(app); pol != nil {
		for _, cg := range pol.CustomGates {
			if cg.Guards(to) {
				controls = append(controls, p.scoreCustom(ctx, app, version, *target, cg, w.Custom))
			}
		}
	}
	hop := HopScore{
		From: from, To: to, ToLabel: target.Ring.Label, Version: version,
		Controls: controls, Blockers: []string{},
	}
	var weighted, denom int
	for _, c := range controls {
		if c.Status == ControlSkip {
			continue
		}
		weighted += c.Score * c.Weight
		denom += c.Weight
		if c.Status == ControlBlock {
			hop.Blockers = append(hop.Blockers, c.Label)
		}
	}
	if denom > 0 {
		hop.Score = (weighted + denom/2) / denom
	}
	hop.Ready = len(hop.Blockers) == 0 && hop.Score >= 70 && version != ""
	hop.Summary = hopSummary(hop)
	return hop
}

func hopSummary(h HopScore) string {
	move := "seed " + h.To
	if h.From != "" {
		move = "promote " + h.From + " → " + h.To
	}
	if h.Version != "" {
		move += " (" + h.Version + ")"
	}
	if len(h.Blockers) > 0 {
		return fmt.Sprintf("%s is blocked by %s. Score %d/100.", move, strings.Join(h.Blockers, ", "), h.Score)
	}
	if h.Ready {
		return fmt.Sprintf("%s is ready. Score %d/100.", move, h.Score)
	}
	return fmt.Sprintf("%s is not blocked but is only %d/100 — check warnings before hopping.", move, h.Score)
}

func ctrl(id, label, kind string, weight, score int, status, detail, next string) ControlScore {
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return ControlScore{ID: id, Label: label, Kind: kind, Weight: weight, Score: score, Status: status, Detail: detail, NextAction: next}
}

func skip(id, label, kind string, weight int, detail string) ControlScore {
	return ctrl(id, label, kind, weight, 0, ControlSkip, detail, "")
}

func scoreHealth(from string, healthy bool, liveErr string, ttfb *int64, weight int) ControlScore {
	if from == "" {
		return skip("health", "Source health", "health", weight, "No source ring yet — this is a seed.")
	}
	if !healthy {
		detail := "Source ring is not live-healthy"
		if liveErr != "" {
			detail += ": " + liveErr
		}
		return ctrl("health", "Source health", "health", weight, 0, ControlBlock, detail, "Fix the source ring (health URL / pods) before promoting. A hop copies a broken version.")
	}
	if ttfb != nil && *ttfb > 2000 {
		return ctrl("health", "Source health", "health", weight, 72, ControlWarn,
			fmt.Sprintf("Healthy, but TTFB is %dms (slow for a hop)", *ttfb),
			"Check origin latency before promoting — slow int often means a worse acc.")
	}
	return ctrl("health", "Source health", "health", weight, 100, ControlPass, "Source ring is live-healthy", "")
}

func (p *Promoter) scoreQAAgent(ctx context.Context, app, from, to string, weight int) ControlScore {
	if !p.QAAgentEnabled() {
		return skip("qa_agent", "QA agent", "qa", weight, "No QA agent is registered on this instance.")
	}
	ringName := from
	if ringName == "" {
		ringName = to
	}
	verdict, summary, err := p.QAVerdict(ctx, app, ringName)
	if err != nil {
		return ctrl("qa_agent", "QA agent", "qa", weight, 40, ControlWarn, err.Error(), "Re-check the QA agent.")
	}
	switch verdict {
	case QAVerdictGo:
		d := "QA agent GO"
		if summary != "" {
			d += " — " + summary
		}
		return ctrl("qa_agent", "QA agent", "qa", weight, 100, ControlPass, d, "")
	case QAVerdictCheck:
		return ctrl("qa_agent", "QA agent", "qa", weight, 65, ControlWarn, "QA agent CHECK — "+orDash(summary),
			"Open the failing suite or wait for the next green run before hopping.")
	case QAVerdictNoGo:
		return ctrl("qa_agent", "QA agent", "qa", weight, 0, ControlBlock, "QA agent NO-GO — "+orDash(summary),
			"Hold the promotion. Fix the failing tests the agent reported, then wait for a GO push.")
	default:
		return ctrl("qa_agent", "QA agent", "qa", weight, 35, ControlWarn, "No QA report yet for this hop",
			"Ask the QA agent (or CI) to POST /api/qa/reports for this app/ring. CI never stores this; RP does.")
	}
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "see the QA strip"
	}
	return s
}

func (p *Promoter) scoreSignoff(ctx context.Context, app, to, version string, target RingView, weight int) ControlScore {
	if !target.Gates.QASignoff {
		return skip("qa_signoff", "QA / release sign-off", "qa", weight, "Sign-off is not required to enter "+to+".")
	}
	if version == "" {
		return ctrl("qa_signoff", "QA / release sign-off", "qa", weight, 20, ControlWarn,
			"Need a version before a sign-off can be recorded", "Seed the source ring first.")
	}
	s, err := p.store.GetSignoff(ctx, app, to, version)
	if errors.Is(err, store.ErrNotFound) {
		return ctrl("qa_signoff", "QA / release sign-off", "qa", weight, 0, ControlBlock,
			"No GO sign-off for "+version+" → "+to,
			"Record a QA/release GO in Deployment Controls (or the promote dialog) for this exact version.")
	}
	if err != nil {
		return ctrl("qa_signoff", "QA / release sign-off", "qa", weight, 40, ControlWarn, err.Error(), "")
	}
	if s.IsGo() {
		return ctrl("qa_signoff", "QA / release sign-off", "qa", weight, 100, ControlPass,
			"GO by "+s.Engineer+" ("+s.QAStatus+")", "")
	}
	return ctrl("qa_signoff", "QA / release sign-off", "qa", weight, 0, ControlBlock,
		"NO-GO by "+s.Engineer, "Clear the NO-GO with a new GO sign-off after QA retests "+version+".")
}

func (p *Promoter) scoreSecurity(ctx context.Context, app, to, version string, weight int) ControlScore {
	ev, err := p.EvaluateSecurity(ctx, app, to, version)
	if err != nil {
		return ctrl("security", "Security & compliance", "security", weight, 40, ControlWarn, err.Error(), "")
	}
	if !ev.Guarded {
		return skip("security", "Security & compliance", "security", weight, "AppSec does not gate "+to+".")
	}
	switch ev.Verdict {
	case SecurityGo:
		return ctrl("security", "Security & compliance", "security", weight, 100, ControlPass, ev.Detail, "")
	case SecurityPending:
		return ctrl("security", "Security & compliance", "security", weight, 25, ControlWarn, ev.Detail,
			"Push the missing scans (SAST/SCA/secrets) for this exact version, or wait for CI to POST /api/security/scans.")
	case SecurityNoGo:
		return ctrl("security", "Security & compliance", "security", weight, 0, ControlBlock, ev.Detail,
			"Fix or waive findings on the Security panel. RP blocks the hop; GitOps would have shipped the image anyway.")
	default:
		return skip("security", "Security & compliance", "security", weight, ev.Detail)
	}
}

func scoreGrafana(target RingView, weight int) ControlScore {
	if !target.Gates.Grafana {
		return skip("grafana", "Grafana / E2E gate", "qa", weight, "No Grafana go/no-go on "+target.Ring.Name+".")
	}
	st := target.Gates.GrafanaStatus
	if st == nil {
		return ctrl("grafana", "Grafana / E2E gate", "qa", weight, 50, ControlWarn, "No verdict yet", "Open the release dashboard.")
	}
	switch st.Verdict {
	case grafana.VerdictGo:
		return ctrl("grafana", "Grafana / E2E gate", "qa", weight, 100, ControlPass, "Dashboard GO ("+st.Dashboard+")", "")
	case grafana.VerdictCheck:
		return ctrl("grafana", "Grafana / E2E gate", "qa", weight, 68, ControlWarn,
			"Dashboard CHECK — worth a look, not blocking", "Inspect "+st.Dashboard+" before hopping.")
	case grafana.VerdictNoGo:
		return ctrl("grafana", "Grafana / E2E gate", "qa", weight, 0, ControlBlock,
			"Dashboard NO-GO ("+st.Dashboard+")",
			"Fix the red suite, or promote with a Grafana override + reason if it is a known flake.")
	default:
		return ctrl("grafana", "Grafana / E2E gate", "qa", weight, 70, ControlWarn,
			"Grafana unknown (outage never blocks a release)", "Treat as unverified — do not confuse with GO.")
	}
}

func scoreWindow(target RingView, weight int) ControlScore {
	if !target.Gates.MaintenanceWindow {
		return skip("window", "Maintenance window", "policy", weight, "No window required to enter "+target.Ring.Name+".")
	}
	if target.Gates.MaintenanceWindowOpen {
		return ctrl("window", "Maintenance window", "policy", weight, 100, ControlPass, "A window is open now", "")
	}
	return ctrl("window", "Maintenance window", "policy", weight, 0, ControlBlock,
		"No window open for "+target.Ring.Label,
		"Open an ad-hoc window (hours + reason) from the promote dialog. CI has no notion of a freeze.")
}

func scoreChangeRequest(target RingView, weight int) ControlScore {
	if !target.Gates.ChangeRequest {
		return skip("change_request", "Change request", "policy", weight, "No CR code required for "+target.Ring.Name+".")
	}
	prov := target.Gates.ChangeRequestProvider
	if prov == "" {
		prov = "the change system"
	}
	return ctrl("change_request", "Change request", "policy", weight, 55, ControlWarn,
		"A valid "+prov+" change-request code is required at promote time",
		"Have the ticket number ready (demo code is \"test\"). RP will refuse the hop without it; a pipeline YAML will not.")
}

func scoreOps(from, to string, hist []store.HistoryEntry, weight int) ControlScore {
	failed, total := 0, 0
	var lastFail string
	for _, h := range hist {
		if h.Action != "promote" && h.Action != "seed" {
			continue
		}
		if to != "" && h.Ring != to {
			continue
		}
		total++
		if h.Result == "failure" {
			failed++
			if lastFail == "" {
				lastFail = h.Message
			}
		}
		if total >= 5 {
			break
		}
	}
	if total == 0 {
		return ctrl("ops", "Recent hops", "ops", weight, 80, ControlPass, "No recent hops into this ring to learn from", "")
	}
	if failed == 0 {
		return ctrl("ops", "Recent hops", "ops", weight, 100, ControlPass,
			fmt.Sprintf("Last %d hop(s) into %s succeeded", total, to), "")
	}
	if failed >= 2 || (total > 0 && failed == total) {
		return ctrl("ops", "Recent hops", "ops", weight, 35, ControlWarn,
			fmt.Sprintf("%d of last %d hops into %s failed — %s", failed, total, to, lastFail),
			"Diagnose the last failed job with AI before hopping again. CI will happily retry the same broken image.")
	}
	return ctrl("ops", "Recent hops", "ops", weight, 70, ControlWarn,
		fmt.Sprintf("1 of last %d hops failed — %s", total, lastFail),
		"Skim the last failure before promoting.")
}

func (p *Promoter) scoreCustom(ctx context.Context, app, version string, target RingView, g config.CustomGate, weight int) ControlScore {
	id := "custom:" + g.Name
	label := g.LabelOr()
	kind := "custom"
	switch g.KindOr() {
	case config.CustomGateJIRA, config.CustomGateOpsAPI, config.CustomGateTest:
		need := "a ticket code"
		if g.KindOr() == config.CustomGateOpsAPI {
			need = "an approved service request for this version"
		}
		if g.MinApprovals > 0 {
			need = fmt.Sprintf("a ticket with ≥%d approvals", g.MinApprovals)
		}
		return ctrl(id, label, kind, weight, 55, ControlWarn,
			"Required at promote time: "+need+" (demo code is \"test\"). Not live-checked on the scorecard — JIRA/OPSAPI stay off the rings poll.",
			"Have the ticket ready in the promote dialog. CI will ship without it; RP will not.")
	case config.CustomGateMetric, config.CustomGateReplicas:
		req := GateRequest{App: app, TargetRing: target.Ring.Name, Version: version, Policy: p.policy(app)}
		res := p.evalCustom(ctx, req, g, false)
		if res.Err != nil {
			return ctrl(id, label, kind, weight, 0, ControlBlock, res.Detail,
				"Satisfy "+label+" before hopping. This is a custom gate CI/CD does not run.")
		}
		return ctrl(id, label, kind, weight, 100, ControlPass, res.Detail, "")
	default:
		return skip(id, label, kind, weight, "unknown custom gate kind")
	}
}

// ControlsBriefText is the deterministic briefing (always available). The AI
// path rewrites it in plainer language when a model is configured.
func ControlsBriefText(h HopScore) string {
	var b strings.Builder
	b.WriteString(h.Summary)
	b.WriteByte('\n')
	for _, c := range h.Controls {
		if c.Status == ControlSkip {
			continue
		}
		fmt.Fprintf(&b, "\n- %s (%s, %d/100, weight %d): %s", c.Label, c.Status, c.Score, c.Weight, c.Detail)
		if c.NextAction != "" {
			fmt.Fprintf(&b, " Next: %s", c.NextAction)
		}
	}
	return strings.TrimSpace(b.String())
}

// MarshalHop is a compact JSON dump for the AI briefer.
func MarshalHop(h HopScore) string {
	b, err := json.Marshal(h)
	if err != nil {
		return h.Summary
	}
	return string(b)
}
