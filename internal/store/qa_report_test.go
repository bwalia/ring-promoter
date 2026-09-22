package store

import (
	"context"
	"testing"
	"time"
)

func TestQAReport_UpsertAndList(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	healthy := false
	if err := m.UpsertQAReport(ctx, QAReport{
		App: "web", Ring: "int", WorkflowVerdict: QAVerdictNoGo,
		EnvHealthy: &healthy, Summary: "E2E red", Source: "qa-bot",
		CheckedAt: time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	healthy = true
	if err := m.UpsertQAReport(ctx, QAReport{
		App: "web", Ring: "int", WorkflowVerdict: QAVerdictGo,
		EnvHealthy: &healthy, Summary: "E2E green", Source: "qa-bot",
	}); err != nil {
		t.Fatal(err)
	}
	list, err := m.ListQAReports(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 report after upsert, got %d", len(list))
	}
	if list[0].WorkflowVerdict != QAVerdictGo {
		t.Fatalf("want go, got %s", list[0].WorkflowVerdict)
	}
	if list[0].EnvHealthy == nil || !*list[0].EnvHealthy {
		t.Fatal("want env healthy")
	}
	all, err := m.ListQAReports(ctx, "")
	if err != nil || len(all) != 1 {
		t.Fatalf("list all: %v len=%d", err, len(all))
	}
}
