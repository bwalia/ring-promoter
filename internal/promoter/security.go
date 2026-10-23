// Security & compliance gates: scanners (SAST, SCA, secrets, container, IaC,
// DAST, licence) report per exact (app, version); a SecurityPolicy says which
// scans must exist and how many un-waived findings each severity may have; the
// promotion gate and the pipeline "security" gate both read the same
// evaluation. Waivers are the only escape hatch — the gate is never overridden.
package promoter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/ring"
	"github.com/example/ring-promoter/internal/store"
)

// Security errors.
var (
	// ErrInvalidSecurityScan: a scan report failed validation.
	ErrInvalidSecurityScan = errors.New("invalid security scan")
	// ErrInvalidWaiver: a waiver request failed validation.
	ErrInvalidWaiver = errors.New("invalid security waiver")
	// ErrSecurityScanMissing: a scan the security policy requires has not
	// reported for the version being deployed.
	ErrSecurityScanMissing = errors.New("security scan missing")
	// ErrSecurityNoGo: the version's scans breach the security policy.
	ErrSecurityNoGo = errors.New("security policy is no-go")
)

// Security evaluation verdicts.
const (
	SecurityGo      = "go"
	SecurityNoGo    = "no_go"
	SecurityPending = "pending" // a required scan has not reported yet
	SecurityNone    = "none"    // no version to evaluate
)

// Security check statuses.
const (
	CheckPass    = "pass"
	CheckFail    = "fail"
	CheckMissing = "missing"
	CheckStale   = "stale"
)

const (
	defaultWaiverDays = 30
	maxWaiverDays     = 90
)

// SeverityCounts are finding counts per severity.
type SeverityCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
}

func (c SeverityCounts) get(sev string) int {
	switch sev {
	case config.SeverityCritical:
		return c.Critical
	case config.SeverityHigh:
		return c.High
	case config.SeverityMedium:
		return c.Medium
	case config.SeverityLow:
		return c.Low
	}
	return 0
}

func (c *SeverityCounts) add(sev string, n int) {
	switch sev {
	case config.SeverityCritical:
		c.Critical += n
	case config.SeverityHigh:
		c.High += n
	case config.SeverityMedium:
		c.Medium += n
	case config.SeverityLow:
		c.Low += n
	}
}

func (c *SeverityCounts) plus(o SeverityCounts) {
	c.Critical += o.Critical
	c.High += o.High
	c.Medium += o.Medium
	c.Low += o.Low
}

// breaches renders the severities over their limit ("2 high > max 0"), most
// severe first; empty when within policy.
func (c SeverityCounts) breaches(limits map[string]int) []string {
	var out []string
	for _, sev := range config.Severities {
		max, ok := limits[sev]
		if ok && c.get(sev) > max {
			out = append(out, fmt.Sprintf("%d %s > max %d", c.get(sev), sev, max))
		}
	}
	return out
}

// SecurityCheck is one scan kind's result within an evaluation.
type SecurityCheck struct {
	Kind     string   `json:"kind"`
	Status   string   `json:"status"`
	Scanners []string `json:"scanners"`
	SeverityCounts
	Waived    int        `json:"waived"`
	ScannedAt *time.Time `json:"scanned_at,omitempty"`
	Detail    string     `json:"detail,omitempty"`
}

// SecurityEvaluation is a version's security verdict against a policy.
type SecurityEvaluation struct {
	Ring    string          `json:"ring,omitempty"`
	Version string          `json:"version"`
	Guarded bool            `json:"guarded"`
	Verdict string          `json:"verdict"`
	Detail  string          `json:"detail"`
	Checks  []SecurityCheck `json:"checks"`
	Counts  SeverityCounts  `json:"counts"`
	Waived  int             `json:"waived"`
}

// SecurityPolicyView is the JSON view of an app's effective security policy.
type SecurityPolicyView struct {
	Rings         []string       `json:"rings"`
	Require       []string       `json:"require"`
	Max           map[string]int `json:"max"`
	MaxAgeSeconds int64          `json:"max_age_seconds"`
	// Source is "app" (promotion_policy.security), "default" (security_policy)
	// or "builtin" (nothing configured: pipeline gates only, never enforced on
	// promotions).
	Source string `json:"source"`
}

// AppSecurity is an app's security overview.
type AppSecurity struct {
	Policy  SecurityPolicyView     `json:"policy"`
	Posture []SecurityEvaluation   `json:"posture"`
	Scans   []store.SecurityScan   `json:"scans"`
	Waivers []store.SecurityWaiver `json:"waivers"`
}

// SecurityScanInput is a scan report to record.
type SecurityScanInput struct {
	App, Version, Kind, Scanner string
	URL, Summary                string
	ScannedAt                   time.Time
	// Counts are used only when neither Findings nor SARIF is given.
	Counts   SeverityCounts
	Findings []store.SecurityFinding
	// SARIF, when set, is a SARIF 2.1.0 document the findings are read from.
	SARIF json.RawMessage
}

// effectiveSecurityPolicy returns the app's configured policy and its source,
// or the built-in default (configured=false).
func (p *Promoter) effectiveSecurityPolicy(app string) (pol config.SecurityPolicy, source string, configured bool) {
	if sp, src, ok := p.cfg.SecurityPolicyFor(app); ok {
		return *sp, src, true
	}
	return config.DefaultSecurityPolicy(), "builtin", false
}

// RecordSecurityScan validates and stores a scanner's report for one version.
// Rescanning with the same (version, kind, scanner) replaces the previous one.
func (p *Promoter) RecordSecurityScan(ctx context.Context, in SecurityScanInput) (store.SecurityScan, error) {
	if _, ok := p.cfg.App(in.App); !ok {
		return store.SecurityScan{}, ErrAppNotFound
	}
	in.Version = strings.TrimSpace(in.Version)
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	in.Scanner = strings.TrimSpace(in.Scanner)
	if in.Version == "" {
		return store.SecurityScan{}, fmt.Errorf("%w: version is required", ErrInvalidSecurityScan)
	}
	if !config.IsScanKind(in.Kind) {
		return store.SecurityScan{}, fmt.Errorf("%w: kind must be one of %s", ErrInvalidSecurityScan, strings.Join(config.ScanKinds, ", "))
	}
	findings := in.Findings
	if len(in.SARIF) > 0 && string(in.SARIF) != "null" {
		parsed, tool, err := parseSARIF(in.SARIF, in.Kind)
		if err != nil {
			return store.SecurityScan{}, err
		}
		findings = parsed
		if in.Scanner == "" {
			in.Scanner = tool
		}
	}
	if in.Scanner == "" {
		return store.SecurityScan{}, fmt.Errorf("%w: scanner is required", ErrInvalidSecurityScan)
	}
	counts := in.Counts
	if findings != nil {
		counts = SeverityCounts{}
		for i := range findings {
			f := &findings[i]
			f.ID = strings.TrimSpace(f.ID)
			f.Severity = strings.ToLower(strings.TrimSpace(f.Severity))
			f.Waived = false
			if f.ID == "" {
				return store.SecurityScan{}, fmt.Errorf("%w: findings[%d] has no id", ErrInvalidSecurityScan, i)
			}
			if !config.IsSeverity(f.Severity) {
				return store.SecurityScan{}, fmt.Errorf("%w: findings[%d] severity %q (want %s)", ErrInvalidSecurityScan, i, f.Severity, strings.Join(config.Severities, ", "))
			}
			counts.add(f.Severity, 1)
		}
	} else {
		findings = []store.SecurityFinding{}
	}
	if counts.Critical < 0 || counts.High < 0 || counts.Medium < 0 || counts.Low < 0 {
		return store.SecurityScan{}, fmt.Errorf("%w: counts must not be negative", ErrInvalidSecurityScan)
	}
	if len(findings) > store.MaxStoredFindings {
		sort.SliceStable(findings, func(i, j int) bool {
			return severityRank(findings[i].Severity) < severityRank(findings[j].Severity)
		})
		findings = findings[:store.MaxStoredFindings]
	}
	id, err := randomID("scan-")
	if err != nil {
		return store.SecurityScan{}, err
	}
	scannedAt := in.ScannedAt
	if scannedAt.IsZero() {
		scannedAt = p.now()
	}
	scan := store.SecurityScan{
		ID: id, App: in.App, Version: in.Version, Kind: in.Kind, Scanner: in.Scanner,
		Critical: counts.Critical, High: counts.High, Medium: counts.Medium, Low: counts.Low,
		Findings: findings, Summary: strings.TrimSpace(in.Summary), URL: strings.TrimSpace(in.URL),
		ScannedAt: scannedAt.UTC(),
	}
	if err := p.store.UpsertSecurityScan(ctx, scan); err != nil {
		return store.SecurityScan{}, err
	}
	p.audit(ctx, store.AuditEvent{
		App: in.App, Category: store.AuditSecurity, Action: "scan.record", Version: in.Version,
		Detail: auditDetail(map[string]string{
			"kind": scan.Kind, "scanner": scan.Scanner,
			"critical": strconv.Itoa(scan.Critical), "high": strconv.Itoa(scan.High),
			"medium": strconv.Itoa(scan.Medium), "low": strconv.Itoa(scan.Low),
		}),
	})
	if list, err := p.store.ListSecurityScans(ctx, in.App, in.Version); err == nil {
		for _, s := range list {
			if s.ID == id {
				return s, nil
			}
		}
	}
	return scan, nil
}

func severityRank(sev string) int {
	for i, s := range config.Severities {
		if s == sev {
			return i
		}
	}
	return len(config.Severities)
}

// activeWaivers returns the app's waivers in force now.
func (p *Promoter) activeWaivers(ctx context.Context, app string) ([]store.SecurityWaiver, error) {
	all, err := p.store.ListSecurityWaivers(ctx, app)
	if err != nil {
		return nil, err
	}
	now := p.now()
	var out []store.SecurityWaiver
	for _, w := range all {
		if w.ActiveAt(now) {
			out = append(out, w)
		}
	}
	return out, nil
}

// EvaluateSecurity evaluates app@version's scans against the app's effective
// policy for ringName ("" = not tied to a ring: only ring-wide waivers count).
func (p *Promoter) EvaluateSecurity(ctx context.Context, app, ringName, version string) (SecurityEvaluation, error) {
	if _, ok := p.cfg.App(app); !ok {
		return SecurityEvaluation{}, ErrAppNotFound
	}
	pol, _, configured := p.effectiveSecurityPolicy(app)
	ev, err := p.evaluateSecurity(ctx, app, ringName, version, pol)
	ev.Guarded = configured && ringName != "" && pol.Guards(ringName)
	return ev, err
}

func (p *Promoter) evaluateSecurity(ctx context.Context, app, ringName, version string, pol config.SecurityPolicy) (SecurityEvaluation, error) {
	ev := SecurityEvaluation{Ring: ringName, Version: version, Checks: []SecurityCheck{}}
	if version == "" {
		ev.Verdict, ev.Detail = SecurityNone, "no version deployed"
		return ev, nil
	}
	scans, err := p.store.ListSecurityScans(ctx, app, version)
	if err != nil {
		return ev, err
	}
	waivers, err := p.activeWaivers(ctx, app)
	if err != nil {
		return ev, err
	}
	limits := pol.Limits()
	byKind := map[string]*SecurityCheck{}
	var order []string
	check := func(kind string) *SecurityCheck {
		c, ok := byKind[kind]
		if !ok {
			c = &SecurityCheck{Kind: kind, Scanners: []string{}}
			byKind[kind] = c
			order = append(order, kind)
		}
		return c
	}
	for _, k := range pol.RequiredKinds() {
		check(k)
	}
	required := map[string]bool{}
	for _, k := range pol.RequiredKinds() {
		required[k] = true
	}
	// Scans come newest first; sort the extra kinds for a stable order.
	sort.SliceStable(scans, func(i, j int) bool {
		if required[scans[i].Kind] != required[scans[j].Kind] {
			return required[scans[i].Kind]
		}
		return scans[i].Kind < scans[j].Kind
	})
	for _, s := range scans {
		c := check(s.Kind)
		c.Scanners = append(c.Scanners, s.Scanner)
		counts := SeverityCounts{Critical: s.Critical, High: s.High, Medium: s.Medium, Low: s.Low}
		for _, f := range s.Findings {
			for _, w := range waivers {
				if w.Covers(f.ID, ringName) {
					counts.add(f.Severity, -1)
					c.Waived++
					break
				}
			}
		}
		for _, sev := range config.Severities {
			if counts.get(sev) < 0 {
				counts.add(sev, -counts.get(sev))
			}
		}
		c.SeverityCounts.plus(counts)
		if c.ScannedAt == nil || s.ScannedAt.After(*c.ScannedAt) {
			t := s.ScannedAt
			c.ScannedAt = &t
		}
	}

	now := p.now()
	missing, failed := false, false
	var parts []string
	for _, kind := range order {
		c := byKind[kind]
		switch {
		case len(c.Scanners) == 0:
			c.Status, c.Detail = CheckMissing, "no "+kind+" scan for "+version
			missing = true
			parts = append(parts, kind+" missing")
		case required[kind] && pol.MaxAgeStd() > 0 && now.Sub(*c.ScannedAt) > pol.MaxAgeStd():
			c.Status = CheckStale
			c.Detail = fmt.Sprintf("scanned %s ago (max %s)", now.Sub(*c.ScannedAt).Round(time.Minute), pol.MaxAgeStd())
			failed = true
			parts = append(parts, kind+" stale")
		default:
			if b := c.breaches(limits); len(b) > 0 {
				c.Status, c.Detail = CheckFail, strings.Join(b, ", ")
				failed = true
				parts = append(parts, kind+" "+c.Detail)
			} else {
				c.Status = CheckPass
				parts = append(parts, kind+" pass")
			}
		}
		ev.Counts.plus(c.SeverityCounts)
		ev.Waived += c.Waived
		ev.Checks = append(ev.Checks, *c)
	}
	if b := ev.Counts.breaches(limits); len(b) > 0 && !failed {
		failed = true
		parts = append(parts, "total "+strings.Join(b, ", "))
	}
	switch {
	case failed:
		ev.Verdict = SecurityNoGo
	case missing:
		ev.Verdict = SecurityPending
	default:
		ev.Verdict = SecurityGo
	}
	if len(parts) == 0 {
		parts = append(parts, "no scans required")
	}
	ev.Detail = strings.Join(parts, " · ")
	return ev, nil
}

// SecurityVerdict evaluates the version a pipeline security gate checks for
// app on ringName: version when given, else the ring's pinned ref for the
// first ring, else the previous ring's current version (what a promotion into
// ringName deploys). It uses the app's policy, or the built-in default.
func (p *Promoter) SecurityVerdict(ctx context.Context, app, ringName, version string) (SecurityEvaluation, error) {
	if version == "" {
		if prev, ok := ring.Prev(ringName); ok {
			if st, err := p.store.GetRingState(ctx, app, prev.Name); err == nil {
				version = st.CurrentVersion
			}
		} else {
			version = p.RingRef(app, ringName)
		}
	}
	return p.EvaluateSecurity(ctx, app, ringName, version)
}

func policyView(pol config.SecurityPolicy, source string) SecurityPolicyView {
	max := map[string]int{}
	for k, v := range pol.Limits() {
		max[k] = v
	}
	return SecurityPolicyView{
		Rings: append([]string{}, pol.GuardedRings()...), Require: append([]string{}, pol.RequiredKinds()...),
		Max: max, MaxAgeSeconds: int64(pol.MaxAgeStd().Seconds()), Source: source,
	}
}

// AppSecurity returns the app's security policy, the posture of each
// configured ring (its current version evaluated against the policy), the
// newest scans and every waiver.
func (p *Promoter) AppSecurity(ctx context.Context, app string) (AppSecurity, error) {
	ac, ok := p.cfg.App(app)
	if !ok {
		return AppSecurity{}, ErrAppNotFound
	}
	pol, source, configured := p.effectiveSecurityPolicy(app)
	out := AppSecurity{Policy: policyView(pol, source), Posture: []SecurityEvaluation{}}
	for _, r := range ring.All() {
		if _, ok := ac.Rings[r.Name]; !ok {
			continue
		}
		version := ""
		if st, err := p.store.GetRingState(ctx, app, r.Name); err == nil {
			version = st.CurrentVersion
		} else if !errors.Is(err, store.ErrNotFound) {
			return AppSecurity{}, err
		}
		ev, err := p.evaluateSecurity(ctx, app, r.Name, version, pol)
		if err != nil {
			return AppSecurity{}, err
		}
		ev.Guarded = configured && pol.Guards(r.Name)
		out.Posture = append(out.Posture, ev)
	}
	scans, err := p.store.ListSecurityScans(ctx, app, "")
	if err != nil {
		return AppSecurity{}, err
	}
	if len(scans) > 100 {
		scans = scans[:100]
	}
	active, err := p.activeWaivers(ctx, app)
	if err != nil {
		return AppSecurity{}, err
	}
	for i := range scans {
		for j := range scans[i].Findings {
			f := &scans[i].Findings[j]
			for _, w := range active {
				if w.Finding == f.ID {
					f.Waived = true
					break
				}
			}
		}
	}
	out.Scans = append([]store.SecurityScan{}, scans...)
	waivers, err := p.store.ListSecurityWaivers(ctx, app)
	if err != nil {
		return AppSecurity{}, err
	}
	out.Waivers = append([]store.SecurityWaiver{}, waivers...)
	return out, nil
}

// CreateSecurityWaiver accepts the risk of one finding id for app (on one ring,
// or all when ringName is empty) until expiresAt, or for days (default 30)
// when expiresAt is zero. A waiver lasts at most 90 days and must say why.
func (p *Promoter) CreateSecurityWaiver(ctx context.Context, app, finding, ringName, reason, createdBy string, expiresAt time.Time, days int) (store.SecurityWaiver, error) {
	ac, ok := p.cfg.App(app)
	if !ok {
		return store.SecurityWaiver{}, ErrAppNotFound
	}
	finding, ringName, reason = strings.TrimSpace(finding), strings.TrimSpace(ringName), strings.TrimSpace(reason)
	if finding == "" {
		return store.SecurityWaiver{}, fmt.Errorf("%w: finding is required", ErrInvalidWaiver)
	}
	if reason == "" {
		return store.SecurityWaiver{}, fmt.Errorf("%w: reason is required (it is kept as audit evidence)", ErrInvalidWaiver)
	}
	if ringName != "" {
		if _, ok := ac.Rings[ringName]; !ok || !ring.IsValid(ringName) {
			return store.SecurityWaiver{}, ErrRingNotConfigured
		}
	}
	now := p.now()
	if expiresAt.IsZero() {
		if days == 0 {
			days = defaultWaiverDays
		}
		if days < 0 {
			return store.SecurityWaiver{}, fmt.Errorf("%w: days must be positive", ErrInvalidWaiver)
		}
		expiresAt = now.Add(time.Duration(days) * 24 * time.Hour)
	}
	if !expiresAt.After(now) {
		return store.SecurityWaiver{}, fmt.Errorf("%w: expiry must be in the future", ErrInvalidWaiver)
	}
	if expiresAt.Sub(now) > maxWaiverDays*24*time.Hour {
		return store.SecurityWaiver{}, fmt.Errorf("%w: a waiver lasts at most %d days", ErrInvalidWaiver, maxWaiverDays)
	}
	id, err := randomID("wv-")
	if err != nil {
		return store.SecurityWaiver{}, err
	}
	w := store.SecurityWaiver{
		ID: id, App: app, Finding: finding, Ring: ringName, Reason: reason,
		CreatedBy: strings.TrimSpace(createdBy), CreatedAt: now.UTC(), ExpiresAt: expiresAt.UTC(),
	}
	if err := p.store.CreateSecurityWaiver(ctx, w); err != nil {
		return store.SecurityWaiver{}, err
	}
	p.audit(ctx, store.AuditEvent{
		App: app, Ring: ringName, Category: store.AuditSecurity, Action: "waiver.create",
		Detail: auditDetail(map[string]string{
			"id": id, "finding": finding, "reason": reason, "created_by": w.CreatedBy,
			"expires_at": w.ExpiresAt.Format(time.RFC3339),
		}),
	})
	return w, nil
}

// DeleteSecurityWaiver revokes one of app's waivers.
func (p *Promoter) DeleteSecurityWaiver(ctx context.Context, app, id string) error {
	if _, ok := p.cfg.App(app); !ok {
		return ErrAppNotFound
	}
	if err := p.store.DeleteSecurityWaiver(ctx, app, id); err != nil {
		return err
	}
	p.audit(ctx, store.AuditEvent{
		App: app, Category: store.AuditSecurity, Action: "waiver.delete",
		Detail: auditDetail(map[string]string{"id": id}),
	})
	return nil
}

// ---- 5. Security: the version's scans satisfy the app's security policy ----
//
// Enforced only when the app has a configured policy (its own or the
// top-level default) guarding the target ring. Not overridable: a finding is
// accepted only through an attributed, expiring waiver.

type securityGate struct{}

func (securityGate) Name() string { return "security" }

func (g securityGate) Evaluate(ctx context.Context, p *Promoter, req GateRequest) GateResult {
	pol, _, configured := p.effectiveSecurityPolicy(req.App)
	if !configured || !pol.Guards(req.TargetRing) {
		return skipped(g.Name())
	}
	ev, err := p.evaluateSecurity(ctx, req.App, req.TargetRing, req.Version, pol)
	if err != nil {
		return fail(g.Name(), fmt.Errorf("evaluate security scans: %w", err))
	}
	switch ev.Verdict {
	case SecurityGo:
		detail := fmt.Sprintf("security GO for %s on %s (%s)", req.Version, req.TargetRing, ev.Detail)
		if ev.Waived > 0 {
			detail += fmt.Sprintf(", %d finding(s) waived", ev.Waived)
		}
		return pass(g.Name(), detail)
	case SecurityPending:
		return fail(g.Name(), fmt.Errorf("%w: %s needs these scans before it can enter %s: %s",
			ErrSecurityScanMissing, req.Version, req.TargetRing, ev.Detail))
	default:
		return fail(g.Name(), fmt.Errorf("%w: %s on %s: %s (fix the findings or waive them)",
			ErrSecurityNoGo, req.Version, req.TargetRing, ev.Detail))
	}
}
