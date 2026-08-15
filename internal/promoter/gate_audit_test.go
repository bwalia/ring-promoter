package promoter

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/store"
)

func gateAudits(t *testing.T, st store.Store) []store.AuditEvent {
	t.Helper()
	events, err := st.ListAudit(context.Background(), store.AuditFilter{Category: store.AuditGate})
	if err != nil {
		t.Fatalf("list gate audits: %v", err)
	}
	return events
}

func gateDetail(t *testing.T, e store.AuditEvent) map[string]string {
	t.Helper()
	var d map[string]string
	if err := json.Unmarshal([]byte(e.Detail), &d); err != nil {
		t.Fatalf("decode detail %q: %v", e.Detail, err)
	}
	return d
}

// A promotion through a gated ring audits each guarding gate's verdict exactly
// once, under the operation's correlation id.
func TestGateAudit_PassingGatesRecorded(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	pol := &config.PromotionPolicy{
		QASignoff:     &config.GatePolicy{Rings: []string{"test"}},
		ChangeRequest: &config.ChangeRequestPolicy{Rings: []string{"test"}},
	}
	p, _, st := gatedHarness(t, now, pol)
	mustSeed(t, p, "int", "v1")
	if _, err := p.RecordSignoff(context.Background(), testApp, "test", "v1", store.DecisionGo, "qa-lead", "passed", ""); err != nil {
		t.Fatalf("signoff: %v", err)
	}

	ctx := WithGateInputs(context.Background(), GateInputs{ChangeRequestCode: "test"})
	ctx = WithCorrelationID(ctx, "corr-gates-1")
	if _, err := p.Promote(ctx, testApp, "int"); err != nil {
		t.Fatalf("promote: %v", err)
	}

	events := gateAudits(t, st)
	if len(events) != 2 {
		t.Fatalf("want 2 gate audits (qa_signoff + change_request), got %d: %+v", len(events), events)
	}
	byGate := map[string]store.AuditEvent{}
	for _, e := range events {
		byGate[e.Action] = e
		if e.CorrelationID != "corr-gates-1" {
			t.Fatalf("gate audit missing correlation: %+v", e)
		}
		if d := gateDetail(t, e); d["verdict"] != GateVerdictPass {
			t.Fatalf("gate %s verdict = %q, want pass", e.Action, d["verdict"])
		}
	}
	if _, ok := byGate["qa_signoff"]; !ok {
		t.Fatalf("qa_signoff not audited: %+v", events)
	}
	if _, ok := byGate["change_request"]; !ok {
		t.Fatalf("change_request not audited: %+v", events)
	}
}

// A refused promotion audits the failing gate with its error text, and gates
// ordered after the failing one are not evaluated (order is the contract).
func TestGateAudit_FailureRecordedAndOrderPreserved(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	pol := &config.PromotionPolicy{
		QASignoff:     &config.GatePolicy{Rings: []string{"test"}},
		ChangeRequest: &config.ChangeRequestPolicy{Rings: []string{"test"}},
	}
	p, dep, st := gatedHarness(t, now, pol)
	mustSeed(t, p, "int", "v1")

	before := dep.deployCount()
	if _, err := p.Promote(context.Background(), testApp, "int"); !errors.Is(err, ErrSignoffRequired) {
		t.Fatalf("want ErrSignoffRequired, got %v", err)
	}
	if dep.deployCount() != before {
		t.Fatal("deployed despite failing gate")
	}

	events := gateAudits(t, st)
	if len(events) != 1 {
		t.Fatalf("want only the failing gate audited (later gates unevaluated), got %d: %+v", len(events), events)
	}
	if events[0].Action != "qa_signoff" {
		t.Fatalf("failing gate = %q, want qa_signoff", events[0].Action)
	}
	d := gateDetail(t, events[0])
	if d["verdict"] != GateVerdictFail || d["detail"] == "" {
		t.Fatalf("failure detail wrong: %v", d)
	}
}

// An overridden Grafana no-go audits the gate verdict as "overridden" and
// still writes the dedicated override row (two distinct categories).
func TestGateAudit_OverrideVerdictRecorded(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, _, st := gatedHarness(t, now, grafanaPolicy())
	p.SetGrafanaClient(testApp, &stubGrafana{value: 0}) // 0 = NO GO
	mustSeed(t, p, "int", "v1")

	ctx := WithGateInputs(context.Background(), GateInputs{
		OverrideGrafana: true, OverrideReason: "flaky suite, rerun green",
	})
	if _, err := p.Promote(ctx, testApp, "int"); err != nil {
		t.Fatalf("override promote: %v", err)
	}

	gates := gateAudits(t, st)
	if len(gates) != 1 || gates[0].Action != "grafana" {
		t.Fatalf("want 1 grafana gate audit, got %+v", gates)
	}
	if d := gateDetail(t, gates[0]); d["verdict"] != GateVerdictOverridden {
		t.Fatalf("verdict = %q, want overridden", d["verdict"])
	}
	overrides, err := st.ListAudit(context.Background(), store.AuditFilter{Category: store.AuditOverride})
	if err != nil || len(overrides) != 1 {
		t.Fatalf("want 1 dedicated override row, got %d (%v)", len(overrides), err)
	}
}

// Pre-validation (the async request check) must not add gate audits — one
// operation records each gate once.
func TestGateAudit_ValidateOnlyNotRecorded(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	pol := &config.PromotionPolicy{QASignoff: &config.GatePolicy{Rings: []string{"test"}}}
	p, _, st := gatedHarness(t, now, pol)
	mustSeed(t, p, "int", "v1")
	if _, err := p.RecordSignoff(context.Background(), testApp, "test", "v1", store.DecisionGo, "qa-lead", "passed", ""); err != nil {
		t.Fatalf("signoff: %v", err)
	}

	if err := p.ValidatePromote(context.Background(), testApp, "int"); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if events := gateAudits(t, st); len(events) != 0 {
		t.Fatalf("validate-only pass must not audit, got %+v", events)
	}

	if _, err := p.Promote(context.Background(), testApp, "int"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if events := gateAudits(t, st); len(events) != 1 {
		t.Fatalf("operation must audit exactly once, got %d", len(events))
	}
}

// Apps without a policy, and rings no gate guards, leave no gate audits.
func TestGateAudit_UngatedSilent(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, _, st := gatedHarness(t, now, nil)
	mustSeed(t, p, "int", "v1")
	if _, err := p.Promote(context.Background(), testApp, "int"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if events := gateAudits(t, st); len(events) != 0 {
		t.Fatalf("ungated app must not audit gates, got %+v", events)
	}
}
