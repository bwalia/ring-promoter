package config

import (
	"fmt"
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
	return nil
}
