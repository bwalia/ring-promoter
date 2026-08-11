package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/example/ring-promoter/internal/ring"
)

// Change-request provider values.
const (
	// CRProviderTest is the demo/offline provider: it validates no external
	// system, so the only code it accepts is the universal demo code "test"
	// (which every provider accepts — see the promoter). Use it when an app
	// wants the gate enforced but has no business system wired up yet.
	CRProviderTest = "test"
	// CRProviderJIRA validates a change-request code against a JIRA issue.
	CRProviderJIRA = "jira"
)

// defaultGateRings is the ring set a gate guards when it is enabled but lists
// no explicit rings: acceptance and production, the promotion targets that
// carry real risk. Kept in sync with the ring pipeline by name.
func defaultGateRings() []string { return []string{"acc", "prod"} }

// gateRings resolves a gate's configured rings, applying the default set when
// none are listed.
func gateRings(rings []string) []string {
	if len(rings) == 0 {
		return defaultGateRings()
	}
	return rings
}

// ringInSet reports whether ringName is one of rings (case-sensitive; ring
// names are lower-case identifiers).
func ringInSet(ringName string, rings []string) bool {
	for _, r := range rings {
		if r == ringName {
			return true
		}
	}
	return false
}

// PromotionPolicy configures the extra gates an application requires before a
// version may be seeded into, or promoted into, a sensitive ring. Every gate is
// optional and independent; a nil gate is not enforced, so an app with no
// promotion_policy behaves exactly as before.
type PromotionPolicy struct {
	// MaintenanceWindow requires an active maintenance window (config-recurring
	// or operator-created) for the target ring.
	MaintenanceWindow *MaintenanceWindowPolicy `yaml:"maintenance_window"`
	// QASignoff requires a recorded GO sign-off for the exact version.
	QASignoff *GatePolicy `yaml:"qa_signoff"`
	// ChangeRequest requires a valid change-request code for the target ring.
	ChangeRequest *ChangeRequestPolicy `yaml:"change_request"`
	// Grafana requires a Grafana dashboard query to report "go" for the target
	// ring — the release-quality/build-status signal that already drives the
	// team's own go/no-go call.
	Grafana *GrafanaPolicy `yaml:"grafana"`
}

// GatePolicy is a gate that applies to a set of target rings and needs no
// further configuration.
type GatePolicy struct {
	// Rings are the target rings this gate guards (deploying/promoting INTO
	// them). Empty means the default set (acc, prod).
	Rings []string `yaml:"rings"`
}

// Guards reports whether the gate applies to operations targeting ringName.
func (g *GatePolicy) Guards(ringName string) bool {
	return g != nil && ringInSet(ringName, gateRings(g.Rings))
}

// MaintenanceWindowPolicy gates a set of rings behind an active maintenance
// window. A promotion is allowed when "now" falls within EITHER a config-
// defined recurring window here OR an operator-created ad-hoc window (persisted
// at runtime in the store). The two sources are a union: either one opens the
// gate.
type MaintenanceWindowPolicy struct {
	// Rings are the target rings this gate guards. Empty means the default set.
	Rings []string `yaml:"rings"`
	// Recurring are the permanent weekly windows defined in configuration.
	Recurring []RecurringWindow `yaml:"recurring"`
}

// Guards reports whether the gate applies to operations targeting ringName.
func (m *MaintenanceWindowPolicy) Guards(ringName string) bool {
	return m != nil && ringInSet(ringName, gateRings(m.Rings))
}

// OpenAt reports whether any configured recurring window is open at t. The
// recurring windows apply to every ring the policy guards, so no ring argument
// is needed.
func (m *MaintenanceWindowPolicy) OpenAt(t time.Time) bool {
	if m == nil {
		return false
	}
	for _, w := range m.Recurring {
		if w.Active(t) {
			return true
		}
	}
	return false
}

// RecurringWindow is a weekly-recurring maintenance window: on the listed days,
// between Start and End (interpreted in Timezone). When End is earlier than
// Start the window crosses midnight into the following day.
type RecurringWindow struct {
	// Days limits the window to these weekdays ("Mon".."Sun" or full names,
	// case-insensitive). Empty means every day.
	Days []string `yaml:"days"`
	// Start and End are "HH:MM" 24-hour clock times in Timezone.
	Start string `yaml:"start"`
	End   string `yaml:"end"`
	// Timezone is an IANA name (e.g. "Europe/London"); empty means UTC.
	Timezone string `yaml:"timezone"`
}

// Active reports whether the window is open at instant t. A window that fails
// to parse (guarded against at config load) reports closed.
func (w RecurringWindow) Active(t time.Time) bool {
	loc, err := w.location()
	if err != nil {
		return false
	}
	startMin, err := parseClock(w.Start)
	if err != nil {
		return false
	}
	endMin, err := parseClock(w.End)
	if err != nil {
		return false
	}
	lt := t.In(loc)
	nowMin := lt.Hour()*60 + lt.Minute()

	if startMin < endMin {
		// Same-day window: [start, end) on an allowed weekday.
		return w.dayAllowed(lt.Weekday()) && nowMin >= startMin && nowMin < endMin
	}
	// Crosses midnight: [start, 24:00) on the start day, then [00:00, end) on
	// the next day. When now is in the late part, the window opened today; when
	// in the early part, it opened on the previous weekday.
	if nowMin >= startMin {
		return w.dayAllowed(lt.Weekday())
	}
	if nowMin < endMin {
		return w.dayAllowed(lt.Weekday() - 1) // prev weekday; wraps below
	}
	return false
}

// dayAllowed reports whether wd is one of the window's days (all days when the
// list is empty). Weekday arithmetic that produces -1 (from Sunday) wraps to
// Saturday.
func (w RecurringWindow) dayAllowed(wd time.Weekday) bool {
	if wd < 0 {
		wd = time.Saturday
	}
	if len(w.Days) == 0 {
		return true
	}
	for _, d := range w.Days {
		if pd, err := parseWeekday(d); err == nil && pd == wd {
			return true
		}
	}
	return false
}

func (w RecurringWindow) location() (*time.Location, error) {
	if strings.TrimSpace(w.Timezone) == "" {
		return time.UTC, nil
	}
	return time.LoadLocation(w.Timezone)
}

// validate checks that a recurring window parses (times, days, timezone) and is
// non-degenerate.
func (w RecurringWindow) validate() error {
	start, err := parseClock(w.Start)
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	end, err := parseClock(w.End)
	if err != nil {
		return fmt.Errorf("end: %w", err)
	}
	if start == end {
		return fmt.Errorf("start and end are equal (%s); a window must span time", w.Start)
	}
	if _, err := w.location(); err != nil {
		return fmt.Errorf("timezone %q: %w", w.Timezone, err)
	}
	for _, d := range w.Days {
		if _, err := parseWeekday(d); err != nil {
			return err
		}
	}
	return nil
}

// parseClock parses "HH:MM" into minutes-since-midnight.
func parseClock(s string) (int, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("invalid time %q (want HH:MM): %w", s, err)
	}
	return t.Hour()*60 + t.Minute(), nil
}

// weekdayNames maps accepted spellings to a weekday. Both 3-letter and full
// names are accepted, case-insensitively.
var weekdayNames = map[string]time.Weekday{
	"sun": time.Sunday, "sunday": time.Sunday,
	"mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tuesday": time.Tuesday,
	"wed": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thursday": time.Thursday,
	"fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
}

func parseWeekday(s string) (time.Weekday, error) {
	if wd, ok := weekdayNames[strings.ToLower(strings.TrimSpace(s))]; ok {
		return wd, nil
	}
	return 0, fmt.Errorf("invalid day %q (want Mon..Sun)", s)
}

// ChangeRequestPolicy requires a valid change-request (CR) code before a
// version may be deployed into a guarded ring. The universal demo code "test"
// is always accepted (enforced by the promoter) regardless of provider, so
// demos never need a real business system.
type ChangeRequestPolicy struct {
	// Rings are the target rings this gate guards. Empty means the default set.
	Rings []string `yaml:"rings"`
	// Provider selects the validation backend: "test" (default; only the demo
	// code "test" passes) or "jira".
	Provider string `yaml:"provider"`
	// JIRA configures the "jira" provider.
	JIRA *JIRAConfig `yaml:"jira"`
}

// Guards reports whether the gate applies to operations targeting ringName.
func (c *ChangeRequestPolicy) Guards(ringName string) bool {
	return c != nil && ringInSet(ringName, gateRings(c.Rings))
}

// ProviderKind returns the resolved provider (defaulting to "test").
func (c *ChangeRequestPolicy) ProviderKind() string {
	if c == nil || c.Provider == "" {
		return CRProviderTest
	}
	return c.Provider
}

// JIRAConfig configures change-request validation against a JIRA instance. The
// API token is NOT stored here — it comes from the environment variable named
// by TokenEnv (populated from a Secret), like the github deployer's token.
type JIRAConfig struct {
	// BaseURL is the JIRA site base, e.g. "https://acme.atlassian.net".
	BaseURL string `yaml:"base_url"`
	// Email is the account the API token belongs to (JIRA Cloud uses basic auth
	// of email + API token).
	Email string `yaml:"email"`
	// TokenEnv names the environment variable holding the API token. Default
	// "RP_JIRA_TOKEN".
	TokenEnv string `yaml:"token_env"`
	// AllowedStatuses, when set, requires the issue's status to be one of these
	// (case-insensitive, e.g. "Approved", "In Progress"). Empty means any
	// existing, non-closed issue is acceptable.
	AllowedStatuses []string `yaml:"allowed_statuses"`
	// ProjectKeys, when set, requires the CR code's project prefix to be one of
	// these (e.g. "CR", "OPS") — a cheap guard before calling JIRA.
	ProjectKeys []string `yaml:"project_keys"`
}

// TokenEnvName returns the environment variable holding the JIRA API token.
func (j *JIRAConfig) TokenEnvName() string {
	if j != nil && j.TokenEnv != "" {
		return j.TokenEnv
	}
	return "RP_JIRA_TOKEN"
}

// GrafanaPolicy gates a set of rings behind a Grafana dashboard query. The
// query returns ONE number and the thresholds below turn it into a verdict, so
// the same gate fits a purpose-built "Go/No-Go" panel (2 = go, 1 = pending,
// 0 = no-go) and a plain quality metric (e.g. success rate: go_min 95,
// no_go_max 80) without any code change.
//
// The API token is NOT stored here — it comes from the environment variable
// named by TokenEnv, like the JIRA and GitHub tokens.
type GrafanaPolicy struct {
	// Rings are the target rings this gate guards. Empty means the default set.
	Rings []string `yaml:"rings"`
	// URL is the Grafana base URL, e.g. "https://grafana.example.com". Empty
	// puts the gate in demo mode: no HTTP call is made and the verdict comes
	// from DemoVerdict, so the gate can be shown off without a live Grafana.
	URL string `yaml:"url"`
	// TokenEnv names the environment variable holding a Grafana service-account
	// token with Viewer access to the datasource. Default "RP_GRAFANA_TOKEN".
	TokenEnv string `yaml:"token_env"`
	// DashboardUID is the dashboard the query comes from (e.g.
	// "sre-build-release-status"). Used to build the "Open dashboard" link the
	// UI offers, and to name the gate in messages.
	DashboardUID string `yaml:"dashboard_uid"`
	// DashboardName is the human title shown in the UI ("Build & Release
	// Status"). Optional; empty falls back to the UID.
	DashboardName string `yaml:"dashboard_name"`
	// DatasourceUID is the datasource the query runs against (e.g.
	// "QA-PostgreSQL"). Required unless the gate is in demo mode.
	DatasourceUID string `yaml:"datasource_uid"`
	// DatasourceType is the Grafana datasource plugin id, e.g.
	// "grafana-postgresql-datasource" or "prometheus". Default is the
	// PostgreSQL datasource, which is what the diytaxreturn QA dashboards use.
	DatasourceType string `yaml:"datasource_type"`
	// Checks are the dashboard queries behind the gate, evaluated together: one
	// no-go blocks the promotion, and the UI lists each by name so it is obvious
	// WHICH suite is red. A single-panel gate is just a one-entry list.
	Checks []GrafanaCheck `yaml:"checks"`
	// MaxAge bounds how old the run behind a check may be. It matters for
	// NIGHTLY suites: "the last E2E run passed" is worthless if that run was
	// four days ago, and without this a suite that silently stopped running
	// would keep showing its last GO forever. A check older than MaxAge becomes
	// "no data" (advisory, never blocking) rather than a false GO. Zero disables
	// the rule. Requires the check query to select the run's timestamp.
	MaxAge time.Duration `yaml:"max_age"`
	// Workflow substitutes the dashboard's $workflow variable. Default ".*"
	// (every workflow), matching the dashboard's "All" selection.
	Workflow string `yaml:"workflow"`
	// Lookback is the time range the query covers, ending now. Default 24h.
	Lookback time.Duration `yaml:"lookback"`
	// GoMin is the value at or above which the verdict is GO. Default 1.
	GoMin *float64 `yaml:"go_min"`
	// NoGoMax is the value at or below which the verdict is NO-GO. Values
	// between NoGoMax and GoMin are "check" — shown, but not blocking. Default
	// 0. Must be below GoMin.
	NoGoMax *float64 `yaml:"no_go_max"`
	// Unit suffixes the value in the UI (e.g. "%"). Optional; a check may
	// override it.
	Unit string `yaml:"unit"`
	// DemoVerdict fixes the verdict when URL is empty: "go", "check" or
	// "no_go". Default "go". Demo mode only — it is ignored once URL is set.
	DemoVerdict string `yaml:"demo_verdict"`
	// DemoVerdicts fixes the verdict per target ring, overriding DemoVerdict for
	// the rings it names. Demo mode only; it exists so a walkthrough can show
	// every state at once (go into test, check into acc, no-go into prod).
	DemoVerdicts map[string]string `yaml:"demo_verdicts"`
}

// GrafanaCheck is one query behind a Grafana gate — typically one panel, e.g.
// one E2E suite's "Go / No-Go" stat.
type GrafanaCheck struct {
	// Name identifies the check in the UI, e.g. "Register & Login". Required.
	Name string `yaml:"name"`
	// Query is the SQL (or PromQL) producing the check's number. Dashboard
	// variables are substituted before the query is sent:
	//   $env / ${env:raw}           -> the target ring's target_env (or name)
	//   $workflow / ${workflow:raw} -> the policy's Workflow
	// Grafana's own $__timeFrom()/$__timeTo() macros are expanded server-side
	// from Lookback, so leave them in the query as the dashboard writes them.
	//
	// To make the gate's max_age rule work, also select the run's timestamp
	// (e.g. `created_at AS "time"`): the first non-time column is the value, and
	// the time column dates the run behind it.
	Query string `yaml:"query"`
	// GoMin / NoGoMax override the policy's thresholds for this check only.
	GoMin   *float64 `yaml:"go_min"`
	NoGoMax *float64 `yaml:"no_go_max"`
	// Unit overrides the policy's unit for this check (e.g. "%").
	Unit string `yaml:"unit"`
}

// Grafana gate defaults.
const (
	grafanaDefaultWorkflow = ".*"
	grafanaDefaultLookback = 24 * time.Hour
	grafanaDefaultDSType   = "grafana-postgresql-datasource"
	grafanaDefaultLabel    = "Go/No-Go"
)

// Thresholds returns the check's effective (goMin, noGoMax), falling back to the
// policy's.
func (c GrafanaCheck) Thresholds(pol *GrafanaPolicy) (goMin, noGoMax float64) {
	goMin, noGoMax = pol.Thresholds()
	if c.GoMin != nil {
		goMin = *c.GoMin
	}
	if c.NoGoMax != nil {
		noGoMax = *c.NoGoMax
	}
	return goMin, noGoMax
}

// UnitOr returns the check's unit, falling back to the policy's.
func (c GrafanaCheck) UnitOr(pol *GrafanaPolicy) string {
	if strings.TrimSpace(c.Unit) != "" {
		return c.Unit
	}
	if pol != nil {
		return pol.Unit
	}
	return ""
}

// NameOr returns the check's display name, falling back to a generic label so
// the UI never renders a blank row.
func (c GrafanaCheck) NameOr() string {
	if strings.TrimSpace(c.Name) != "" {
		return c.Name
	}
	return grafanaDefaultLabel
}

// Guards reports whether the gate applies to operations targeting ringName.
func (g *GrafanaPolicy) Guards(ringName string) bool {
	return g != nil && ringInSet(ringName, gateRings(g.Rings))
}

// DemoMode reports whether the gate answers from DemoVerdict instead of calling
// a real Grafana (no URL configured).
func (g *GrafanaPolicy) DemoMode() bool {
	return g != nil && strings.TrimSpace(g.URL) == ""
}

// TokenEnvName returns the environment variable holding the Grafana token.
func (g *GrafanaPolicy) TokenEnvName() string {
	if g != nil && g.TokenEnv != "" {
		return g.TokenEnv
	}
	return "RP_GRAFANA_TOKEN"
}

// WorkflowVar returns the $workflow substitution (default: every workflow).
func (g *GrafanaPolicy) WorkflowVar() string {
	if g != nil && strings.TrimSpace(g.Workflow) != "" {
		return g.Workflow
	}
	return grafanaDefaultWorkflow
}

// LookbackOrDefault returns the query's time range.
func (g *GrafanaPolicy) LookbackOrDefault() time.Duration {
	if g != nil && g.Lookback > 0 {
		return g.Lookback
	}
	return grafanaDefaultLookback
}

// DatasourceTypeOrDefault returns the datasource plugin id.
func (g *GrafanaPolicy) DatasourceTypeOrDefault() string {
	if g != nil && strings.TrimSpace(g.DatasourceType) != "" {
		return g.DatasourceType
	}
	return grafanaDefaultDSType
}

// DashboardTitle returns the human name of the dashboard behind the gate.
func (g *GrafanaPolicy) DashboardTitle() string {
	if g == nil {
		return ""
	}
	if strings.TrimSpace(g.DashboardName) != "" {
		return g.DashboardName
	}
	return g.DashboardUID
}

// Thresholds returns the effective (goMin, noGoMax) pair.
func (g *GrafanaPolicy) Thresholds() (goMin, noGoMax float64) {
	goMin, noGoMax = 1, 0
	if g == nil {
		return goMin, noGoMax
	}
	if g.GoMin != nil {
		goMin = *g.GoMin
	}
	if g.NoGoMax != nil {
		noGoMax = *g.NoGoMax
	}
	return goMin, noGoMax
}

// DashboardURL returns a link to the dashboard for the target ring, or "" when
// there is nothing to link to (demo mode, or no dashboard UID).
func (g *GrafanaPolicy) DashboardURL(targetEnv string) string {
	if g == nil || strings.TrimSpace(g.URL) == "" || strings.TrimSpace(g.DashboardUID) == "" {
		return ""
	}
	u := strings.TrimSuffix(g.URL, "/") + "/d/" + g.DashboardUID
	q := url.Values{}
	q.Set("var-env", targetEnv)
	q.Set("var-workflow", g.WorkflowVar())
	q.Set("from", "now-"+shortDuration(g.LookbackOrDefault()))
	q.Set("to", "now")
	return u + "?" + q.Encode()
}

// shortDuration renders a duration the way Grafana's time picker writes it
// ("24h", "7d", "90m") rather than Go's "24h0m0s".
func shortDuration(d time.Duration) string {
	switch {
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
}

// Grafana demo verdicts, mirroring the dashboard's own three states.
const (
	GrafanaVerdictGo    = "go"
	GrafanaVerdictCheck = "check"
	GrafanaVerdictNoGo  = "no_go"
)

// DemoVerdictOrDefault returns the fixed verdict used in demo mode.
func (g *GrafanaPolicy) DemoVerdictOrDefault() string {
	if g != nil && strings.TrimSpace(g.DemoVerdict) != "" {
		return strings.ToLower(strings.TrimSpace(g.DemoVerdict))
	}
	return GrafanaVerdictGo
}

// DemoVerdictFor returns the demo verdict for one target ring, falling back to
// the app-wide DemoVerdictOrDefault.
func (g *GrafanaPolicy) DemoVerdictFor(ringName string) string {
	if g != nil {
		if v, ok := g.DemoVerdicts[ringName]; ok && strings.TrimSpace(v) != "" {
			return strings.ToLower(strings.TrimSpace(v))
		}
	}
	return g.DemoVerdictOrDefault()
}

// validatePromotionPolicy checks an app's promotion_policy: ring names must be
// valid, recurring windows must parse, and the change-request provider must be
// known (with the config its provider needs).
func validatePromotionPolicy(a AppConfig) error {
	p := a.PromotionPolicy
	if p == nil {
		return nil
	}
	checkRings := func(gate string, rings []string) error {
		for _, r := range rings {
			if !ring.IsValid(r) {
				return fmt.Errorf("application %q promotion_policy.%s references unknown ring %q", a.Name, gate, r)
			}
		}
		return nil
	}
	if m := p.MaintenanceWindow; m != nil {
		if err := checkRings("maintenance_window", m.Rings); err != nil {
			return err
		}
		for i, w := range m.Recurring {
			if err := w.validate(); err != nil {
				return fmt.Errorf("application %q maintenance_window recurring[%d]: %w", a.Name, i, err)
			}
		}
	}
	if q := p.QASignoff; q != nil {
		if err := checkRings("qa_signoff", q.Rings); err != nil {
			return err
		}
	}
	if c := p.ChangeRequest; c != nil {
		if err := checkRings("change_request", c.Rings); err != nil {
			return err
		}
		switch c.ProviderKind() {
		case CRProviderTest:
			// No external configuration required.
		case CRProviderJIRA:
			if c.JIRA == nil || c.JIRA.BaseURL == "" || c.JIRA.Email == "" {
				return fmt.Errorf("application %q change_request uses the jira provider but is missing jira.base_url or jira.email", a.Name)
			}
		default:
			return fmt.Errorf("application %q change_request has unknown provider %q (want %q or %q)", a.Name, c.Provider, CRProviderTest, CRProviderJIRA)
		}
	}
	if g := p.Grafana; g != nil {
		if err := checkRings("grafana", g.Rings); err != nil {
			return err
		}
		demoVerdicts := map[string]string{"demo_verdict": g.DemoVerdictOrDefault()}
		for r, v := range g.DemoVerdicts {
			if !ring.IsValid(r) {
				return fmt.Errorf("application %q promotion_policy.grafana.demo_verdicts references unknown ring %q", a.Name, r)
			}
			demoVerdicts["demo_verdicts."+r] = strings.ToLower(strings.TrimSpace(v))
		}
		for field, v := range demoVerdicts {
			switch v {
			case GrafanaVerdictGo, GrafanaVerdictCheck, GrafanaVerdictNoGo:
			default:
				return fmt.Errorf("application %q grafana %s is %q (want %q, %q or %q)",
					a.Name, field, v, GrafanaVerdictGo, GrafanaVerdictCheck, GrafanaVerdictNoGo)
			}
		}
		goMin, noGoMax := g.Thresholds()
		if noGoMax >= goMin {
			return fmt.Errorf("application %q grafana has no_go_max (%v) >= go_min (%v); no_go_max must be the lower bound",
				a.Name, noGoMax, goMin)
		}
		// Per-check threshold overrides get the same sanity check.
		for i, c := range g.Checks {
			cGoMin, cNoGoMax := c.Thresholds(g)
			if cNoGoMax >= cGoMin {
				return fmt.Errorf("application %q grafana checks[%d] (%s) has no_go_max (%v) >= go_min (%v)",
					a.Name, i, c.NameOr(), cNoGoMax, cGoMin)
			}
		}
		// A live gate must be able to run its queries; a demo gate needs none.
		if !g.DemoMode() {
			if strings.TrimSpace(g.DatasourceUID) == "" {
				return fmt.Errorf("application %q grafana sets url but is missing datasource_uid", a.Name)
			}
			if len(g.Checks) == 0 {
				return fmt.Errorf("application %q grafana sets url but declares no checks", a.Name)
			}
			for i, c := range g.Checks {
				if strings.TrimSpace(c.Name) == "" {
					return fmt.Errorf("application %q grafana checks[%d] has no name", a.Name, i)
				}
				if strings.TrimSpace(c.Query) == "" {
					return fmt.Errorf("application %q grafana check %q has no query", a.Name, c.Name)
				}
			}
			if _, err := url.Parse(g.URL); err != nil {
				return fmt.Errorf("application %q grafana url %q: %w", a.Name, g.URL, err)
			}
		}
	}
	return nil
}
