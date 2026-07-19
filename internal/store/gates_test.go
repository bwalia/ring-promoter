package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemory_MaintenanceWindowCRUD(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)

	w := MaintenanceWindow{
		ID: "w1", App: "web", Ring: "prod",
		StartsAt: now, EndsAt: now.Add(2 * time.Hour),
		Reason: "release", CreatedBy: "jpatel",
	}
	if err := m.CreateMaintenanceWindow(ctx, w); err != nil {
		t.Fatalf("create: %v", err)
	}
	// A window for a different app must not leak.
	if err := m.CreateMaintenanceWindow(ctx, MaintenanceWindow{ID: "w2", App: "other", StartsAt: now, EndsAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	list, err := m.ListMaintenanceWindows(ctx, "web")
	if err != nil || len(list) != 1 || list[0].ID != "w1" {
		t.Fatalf("list web = %+v, err %v", list, err)
	}
	if got := list[0].CreatedAt; got.IsZero() {
		t.Fatal("created_at should be stamped")
	}

	// Active/Covers helpers.
	if !w.Active(now.Add(time.Hour)) || w.Active(now.Add(3*time.Hour)) {
		t.Fatal("Active window bounds wrong")
	}
	if !w.Covers("prod") || w.Covers("acc") {
		t.Fatal("Covers should match prod only")
	}
	allRings := MaintenanceWindow{Ring: ""}
	if !allRings.Covers("acc") || !allRings.Covers("prod") {
		t.Fatal("empty ring should cover all")
	}

	// Delete is app-scoped.
	if err := m.DeleteMaintenanceWindow(ctx, "other", "w1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-app delete should be ErrNotFound, got %v", err)
	}
	if err := m.DeleteMaintenanceWindow(ctx, "web", "w1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if list, _ := m.ListMaintenanceWindows(ctx, "web"); len(list) != 0 {
		t.Fatalf("expected empty after delete, got %+v", list)
	}
}

// A create prunes windows that ended more than pruneWindowAfter ago.
func TestMemory_MaintenanceWindowPrune(t *testing.T) {
	m := NewMemory()
	fixed := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return fixed }
	ctx := context.Background()

	old := MaintenanceWindow{ID: "old", App: "web", StartsAt: fixed.Add(-30 * 24 * time.Hour), EndsAt: fixed.Add(-20 * 24 * time.Hour)}
	if err := m.CreateMaintenanceWindow(ctx, old); err != nil {
		t.Fatal(err)
	}
	// Creating a fresh window triggers the prune of the long-expired one.
	if err := m.CreateMaintenanceWindow(ctx, MaintenanceWindow{ID: "new", App: "web", StartsAt: fixed, EndsAt: fixed.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	list, _ := m.ListMaintenanceWindows(ctx, "web")
	if len(list) != 1 || list[0].ID != "new" {
		t.Fatalf("prune failed, got %+v", list)
	}
}

func TestMemory_SignoffUpsertGet(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	if _, err := m.GetSignoff(ctx, "web", "prod", "v1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing signoff should be ErrNotFound, got %v", err)
	}

	if err := m.UpsertSignoff(ctx, Signoff{App: "web", Ring: "prod", Version: "v1", Decision: DecisionNoGo, Engineer: "jpatel", QAStatus: "failed"}); err != nil {
		t.Fatal(err)
	}
	got, err := m.GetSignoff(ctx, "web", "prod", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if got.IsGo() {
		t.Fatal("no_go should not be a go")
	}
	// Upsert replaces the decision for the same key.
	if err := m.UpsertSignoff(ctx, Signoff{App: "web", Ring: "prod", Version: "v1", Decision: DecisionGo, Engineer: "jpatel", QAStatus: "passed"}); err != nil {
		t.Fatal(err)
	}
	got, _ = m.GetSignoff(ctx, "web", "prod", "v1")
	if !got.IsGo() || got.QAStatus != "passed" {
		t.Fatalf("upsert did not replace: %+v", got)
	}
	// Version is part of the key: a different version has no sign-off.
	if _, err := m.GetSignoff(ctx, "web", "prod", "v2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("different version should be ErrNotFound, got %v", err)
	}

	list, err := m.ListSignoffs(ctx, "web")
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %+v, err %v", list, err)
	}
}
