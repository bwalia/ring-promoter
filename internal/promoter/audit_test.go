package promoter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/store"
)

// A successful promotion past an overridden Grafana no-go must leave exactly
// one durable override record — history only keeps step logs for failures, so
// before the audit ledger a successful override left no queryable trace.
func TestAudit_GrafanaOverrideRecordedOnSuccess(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, _, st := gatedHarness(t, now, grafanaPolicy())
	p.SetGrafanaClient(testApp, &stubGrafana{value: 0}) // 0 = NO GO
	mustSeed(t, p, "int", "v1")

	ctx := WithGateInputs(context.Background(), GateInputs{
		OverrideGrafana: true,
		OverrideReason:  "known flaky e2e, re-run passed",
	})
	ctx = WithActor(ctx, Actor{Type: store.ActorHuman, Name: "release-eng"})
	ctx = WithCorrelationID(ctx, "corr-override-1")
	if _, err := p.Promote(ctx, testApp, "int"); err != nil {
		t.Fatalf("override with a reason: %v", err)
	}

	events, err := st.ListAudit(context.Background(), store.AuditFilter{Category: store.AuditOverride})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("want exactly 1 override event (pre-validation must not double-record), got %d: %+v", len(events), events)
	}
	e := events[0]
	if e.Action != "grafana.override" || e.App != testApp || e.Actor != "release-eng" || e.CorrelationID != "corr-override-1" {
		t.Fatalf("override event wrong: %+v", e)
	}
	if !strings.Contains(e.Detail, "known flaky e2e") {
		t.Fatalf("override reason not in detail: %s", e.Detail)
	}
}

// A refused override (no reason) must not be recorded as an override.
func TestAudit_RefusedOverrideNotRecorded(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, _, st := gatedHarness(t, now, grafanaPolicy())
	p.SetGrafanaClient(testApp, &stubGrafana{value: 0})
	mustSeed(t, p, "int", "v1")

	ctx := WithGateInputs(context.Background(), GateInputs{OverrideGrafana: true})
	if _, err := p.Promote(ctx, testApp, "int"); !errors.Is(err, ErrGrafanaOverrideReason) {
		t.Fatalf("want ErrGrafanaOverrideReason, got %v", err)
	}
	events, err := st.ListAudit(context.Background(), store.AuditFilter{Category: store.AuditOverride})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("refused override must leave no override record, got %+v", events)
	}
}

// Operations record history and audit under the same correlation id, and the
// pending-op journal carries it too so recovery lands in the same correlation.
func TestAudit_CorrelationStampedOnHistory(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	p, _, st := gatedHarness(t, now, nil)

	ctx := WithCorrelationID(context.Background(), "corr-seed-9")
	if _, err := p.Seed(ctx, testApp, "int", "v1"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	hist, err := st.ListHistory(context.Background(), testApp)
	if err != nil || len(hist) != 1 {
		t.Fatalf("history: %v (%d rows)", err, len(hist))
	}
	if hist[0].CorrelationID != "corr-seed-9" {
		t.Fatalf("history correlation = %q, want corr-seed-9", hist[0].CorrelationID)
	}
	ops, err := st.ListAudit(context.Background(), store.AuditFilter{Category: store.AuditOperation})
	if err != nil || len(ops) != 1 {
		t.Fatalf("audit: %v (%d rows)", err, len(ops))
	}
	if ops[0].CorrelationID != "corr-seed-9" {
		t.Fatalf("audit correlation = %q, want corr-seed-9", ops[0].CorrelationID)
	}
}
