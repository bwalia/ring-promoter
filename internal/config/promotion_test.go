package config

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, layout, v string) time.Time {
	t.Helper()
	tm, err := time.Parse(layout, v)
	if err != nil {
		t.Fatalf("parse time %q: %v", v, err)
	}
	return tm
}

// A gate with no explicit rings guards the default set (acc, prod) and nothing
// else.
func TestGatePolicy_DefaultRings(t *testing.T) {
	g := &GatePolicy{}
	if !g.Guards("acc") || !g.Guards("prod") {
		t.Fatal("default gate should guard acc and prod")
	}
	if g.Guards("int") || g.Guards("test") {
		t.Fatal("default gate should not guard int/test")
	}
	// nil gate guards nothing.
	var nilGate *GatePolicy
	if nilGate.Guards("prod") {
		t.Fatal("nil gate should guard nothing")
	}
}

func TestGatePolicy_ExplicitRings(t *testing.T) {
	g := &GatePolicy{Rings: []string{"prod"}}
	if !g.Guards("prod") {
		t.Fatal("should guard prod")
	}
	if g.Guards("acc") {
		t.Fatal("should not guard acc when only prod listed")
	}
}

// Same-day window is open strictly inside [start, end) on allowed days.
func TestRecurringWindow_SameDay(t *testing.T) {
	w := RecurringWindow{Days: []string{"Sat"}, Start: "02:00", End: "04:00", Timezone: "UTC"}
	tests := []struct {
		v    string
		want bool
	}{
		{"2026-07-18T03:00:00Z", true},  // Sat 03:00
		{"2026-07-18T02:00:00Z", true},  // Sat 02:00 (inclusive start)
		{"2026-07-18T04:00:00Z", false}, // Sat 04:00 (exclusive end)
		{"2026-07-18T01:59:00Z", false}, // before
		{"2026-07-19T03:00:00Z", false}, // Sun, wrong day
	}
	for _, tt := range tests {
		if got := w.Active(mustTime(t, time.RFC3339, tt.v)); got != tt.want {
			t.Errorf("Active(%s) = %v, want %v", tt.v, got, tt.want)
		}
	}
}

// A window whose end is before its start crosses midnight; the start weekday is
// what gates it.
func TestRecurringWindow_CrossesMidnight(t *testing.T) {
	w := RecurringWindow{Days: []string{"Sat"}, Start: "23:00", End: "02:00", Timezone: "UTC"}
	tests := []struct {
		v    string
		want bool
	}{
		{"2026-07-18T23:30:00Z", true},  // Sat 23:30 (late part, opened Sat)
		{"2026-07-19T01:00:00Z", true},  // Sun 01:00 (early part, opened Sat)
		{"2026-07-19T02:00:00Z", false}, // Sun 02:00 (exclusive end)
		{"2026-07-18T22:00:00Z", false}, // Sat 22:00 (before)
		{"2026-07-18T01:00:00Z", false}, // Sat 01:00 opened Fri, not allowed
	}
	for _, tt := range tests {
		if got := w.Active(mustTime(t, time.RFC3339, tt.v)); got != tt.want {
			t.Errorf("Active(%s) = %v, want %v", tt.v, got, tt.want)
		}
	}
}

// An empty day list means every day.
func TestRecurringWindow_AllDays(t *testing.T) {
	w := RecurringWindow{Start: "09:00", End: "17:00", Timezone: "UTC"}
	if !w.Active(mustTime(t, time.RFC3339, "2026-07-19T12:00:00Z")) { // Sunday
		t.Fatal("empty days should match any weekday")
	}
}

// The timezone is honored: 02:30 Europe/London in July (BST, +01:00) is 01:30
// UTC, which must fall inside a London 02:00-04:00 window.
func TestRecurringWindow_Timezone(t *testing.T) {
	w := RecurringWindow{Start: "02:00", End: "04:00", Timezone: "Europe/London"}
	if !w.Active(mustTime(t, time.RFC3339, "2026-07-18T01:30:00Z")) {
		t.Fatal("01:30 UTC is 02:30 BST and should be inside the London window")
	}
	if w.Active(mustTime(t, time.RFC3339, "2026-07-18T03:30:00Z")) {
		t.Fatal("03:30 UTC is 04:30 BST and should be outside the London window")
	}
}

func TestMaintenanceWindowPolicy_OpenAt(t *testing.T) {
	m := &MaintenanceWindowPolicy{
		Recurring: []RecurringWindow{
			{Days: []string{"Sat"}, Start: "02:00", End: "04:00", Timezone: "UTC"},
		},
	}
	if !m.OpenAt(mustTime(t, time.RFC3339, "2026-07-18T03:00:00Z")) {
		t.Fatal("expected open inside recurring window")
	}
	if m.OpenAt(mustTime(t, time.RFC3339, "2026-07-18T05:00:00Z")) {
		t.Fatal("expected closed outside recurring window")
	}
}

// A full promotion_policy loads and its gates resolve.
func TestPromotionPolicy_Load(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	body := `
apps:
  - name: web
    promotion_policy:
      maintenance_window:
        rings: [prod]
        recurring:
          - days: [Sat]
            start: "02:00"
            end: "04:00"
            timezone: Europe/London
      qa_signoff:
        rings: [acc, prod]
      change_request:
        provider: jira
        jira:
          base_url: https://acme.atlassian.net
          email: rel@acme.com
          allowed_statuses: [Approved]
    rings:
      int: { namespace: ring0, deployment: web, container: web, image: repo/web, health_url: "http://x/health" }
      acc: { namespace: acc, deployment: web, container: web, image: repo/web, health_url: "http://x/health" }
      prod: { namespace: prod, deployment: web, container: web, image: repo/web, health_url: "http://x/health" }
`
	cfg, err := Load(writeConfig(t, body))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	app, _ := cfg.App("web")
	p := app.PromotionPolicy
	if p == nil {
		t.Fatal("promotion policy missing")
	}
	if !p.MaintenanceWindow.Guards("prod") || p.MaintenanceWindow.Guards("acc") {
		t.Fatal("maintenance window should guard prod only")
	}
	if !p.QASignoff.Guards("acc") || !p.QASignoff.Guards("prod") {
		t.Fatal("qa signoff should guard acc and prod")
	}
	if p.ChangeRequest.ProviderKind() != CRProviderJIRA {
		t.Fatalf("provider = %q, want jira", p.ChangeRequest.ProviderKind())
	}
	if got := p.ChangeRequest.JIRA.TokenEnvName(); got != "RP_JIRA_TOKEN" {
		t.Fatalf("token env = %q, want RP_JIRA_TOKEN", got)
	}
}

func TestPromotionPolicy_UnknownRingRejected(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	body := `
apps:
  - name: web
    promotion_policy:
      qa_signoff:
        rings: [banana]
    rings:
      int: { namespace: ring0, deployment: web, container: web, image: repo/web, health_url: "http://x/health" }
`
	if _, err := Load(writeConfig(t, body)); err == nil {
		t.Fatal("expected error for unknown ring in promotion_policy")
	}
}

func TestPromotionPolicy_JIRARequiresConfig(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	body := `
apps:
  - name: web
    promotion_policy:
      change_request:
        provider: jira
    rings:
      int: { namespace: ring0, deployment: web, container: web, image: repo/web, health_url: "http://x/health" }
`
	if _, err := Load(writeConfig(t, body)); err == nil {
		t.Fatal("expected error: jira provider without jira block")
	}
}

func TestPromotionPolicy_BadWindowRejected(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	body := `
apps:
  - name: web
    promotion_policy:
      maintenance_window:
        recurring:
          - start: "25:00"
            end: "04:00"
    rings:
      int: { namespace: ring0, deployment: web, container: web, image: repo/web, health_url: "http://x/health" }
`
	if _, err := Load(writeConfig(t, body)); err == nil {
		t.Fatal("expected error for invalid window time")
	}
}
