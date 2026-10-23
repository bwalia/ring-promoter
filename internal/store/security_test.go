package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSecurityScan_UpsertAndList(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewMemoryWithClock(func() time.Time { return now })
	put := func(id, version, kind, scanner string, high int) {
		t.Helper()
		if err := m.UpsertSecurityScan(ctx, SecurityScan{ID: id, App: "web", Version: version, Kind: kind, Scanner: scanner, High: high}); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Minute)
	}
	put("s1", "v1", "sca", "trivy", 3)
	put("s2", "v1", "sast", "semgrep", 0)
	put("s3", "v2", "sca", "trivy", 1)
	put("s4", "v1", "sca", "trivy", 0) // rescan replaces s1

	v1, err := m.ListSecurityScans(ctx, "web", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if len(v1) != 2 || v1[0].ID != "s4" || v1[0].High != 0 || v1[1].ID != "s2" {
		t.Fatalf("v1 scans = %+v", v1)
	}
	if v1[0].ScannedAt.IsZero() {
		t.Fatal("scanned_at not defaulted")
	}
	all, _ := m.ListSecurityScans(ctx, "web", "")
	if len(all) != 3 {
		t.Fatalf("all = %d", len(all))
	}
	if other, _ := m.ListSecurityScans(ctx, "api", ""); len(other) != 0 {
		t.Fatalf("other app = %+v", other)
	}
}

func TestSecurityWaiver_CRUD(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewMemoryWithClock(func() time.Time { return now })
	w := SecurityWaiver{ID: "w1", App: "web", Finding: "CVE-1", Reason: "no fix", ExpiresAt: now.Add(time.Hour)}
	if err := m.CreateSecurityWaiver(ctx, w); err != nil {
		t.Fatal(err)
	}
	list, _ := m.ListSecurityWaivers(ctx, "web")
	if len(list) != 1 || list[0].CreatedAt.IsZero() {
		t.Fatalf("list = %+v", list)
	}
	if !list[0].ActiveAt(now) || list[0].ActiveAt(now.Add(2*time.Hour)) {
		t.Fatal("ActiveAt")
	}
	if !list[0].Covers("CVE-1", "prod") || list[0].Covers("CVE-2", "prod") {
		t.Fatal("Covers")
	}
	if err := m.DeleteSecurityWaiver(ctx, "api", "w1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-app delete = %v", err)
	}
	if err := m.DeleteSecurityWaiver(ctx, "web", "w1"); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteSecurityWaiver(ctx, "web", "w1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v", err)
	}
}
