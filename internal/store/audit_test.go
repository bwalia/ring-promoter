package store

import (
	"context"
	"testing"
	"time"
)

func newAuditMemory(t *testing.T) *Memory {
	t.Helper()
	base := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	n := 0
	return NewMemoryWithClock(func() time.Time {
		n++
		return base.Add(time.Duration(n) * time.Second)
	})
}

func TestAudit_AppendAssignsIDsAndTimestamps(t *testing.T) {
	m := newAuditMemory(t)
	ctx := context.Background()

	for _, e := range []AuditEvent{
		{Category: AuditOperation, Action: "seed", App: "web", Ring: "int", Version: "v1", ActorType: ActorHuman, Actor: "alice"},
		{Category: AuditConfig, Action: "group.create", ActorType: ActorHuman, Actor: "bob"},
	} {
		if err := m.AppendAudit(ctx, e); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	got, err := m.ListAudit(ctx, AuditFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 events, got %d", len(got))
	}
	// Newest first.
	if got[0].Action != "group.create" || got[1].Action != "seed" {
		t.Fatalf("wrong order: %q then %q", got[0].Action, got[1].Action)
	}
	if got[0].ID <= got[1].ID {
		t.Fatalf("ids not increasing: %d then %d", got[1].ID, got[0].ID)
	}
	for _, e := range got {
		if e.OccurredAt.IsZero() {
			t.Fatalf("event %d has zero timestamp", e.ID)
		}
	}
}

func TestAudit_Filters(t *testing.T) {
	m := newAuditMemory(t)
	ctx := context.Background()

	events := []AuditEvent{
		{Category: AuditOperation, Action: "seed", App: "web", Ring: "int", Actor: "alice", ActorType: ActorHuman},
		{Category: AuditOperation, Action: "promote", App: "web", Ring: "test", Actor: "bob", ActorType: ActorHuman},
		{Category: AuditOperation, Action: "seed", App: "api", Ring: "int", Actor: "alice", ActorType: ActorHuman},
		{Category: AuditOverride, Action: "grafana.override", App: "web", Ring: "prod", Actor: "carol", ActorType: ActorHuman},
	}
	for _, e := range events {
		if err := m.AppendAudit(ctx, e); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	cases := []struct {
		name   string
		filter AuditFilter
		want   int
	}{
		{"by app", AuditFilter{App: "web"}, 3},
		{"by ring", AuditFilter{Ring: "int"}, 2},
		{"by category", AuditFilter{Category: AuditOverride}, 1},
		{"by actor", AuditFilter{Actor: "alice"}, 2},
		{"app+ring", AuditFilter{App: "web", Ring: "int"}, 1},
		{"no match", AuditFilter{App: "nope"}, 0},
	}
	for _, tc := range cases {
		got, err := m.ListAudit(ctx, tc.filter)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(got) != tc.want {
			t.Errorf("%s: want %d events, got %d", tc.name, tc.want, len(got))
		}
	}
}

func TestAudit_KeysetPaging(t *testing.T) {
	m := newAuditMemory(t)
	ctx := context.Background()

	for i := 0; i < 7; i++ {
		if err := m.AppendAudit(ctx, AuditEvent{Category: AuditOperation, Action: "seed", App: "web"}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	page1, err := m.ListAudit(ctx, AuditFilter{Limit: 3})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 3 || page1[0].ID != 7 || page1[2].ID != 5 {
		t.Fatalf("page1 wrong: %+v", page1)
	}

	page2, err := m.ListAudit(ctx, AuditFilter{Limit: 3, BeforeID: page1[2].ID})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 3 || page2[0].ID != 4 || page2[2].ID != 2 {
		t.Fatalf("page2 wrong: %+v", page2)
	}

	page3, err := m.ListAudit(ctx, AuditFilter{Limit: 3, BeforeID: page2[2].ID})
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if len(page3) != 1 || page3[0].ID != 1 {
		t.Fatalf("page3 wrong: %+v", page3)
	}
}

func TestAudit_LimitClamps(t *testing.T) {
	m := newAuditMemory(t)
	ctx := context.Background()
	for i := 0; i < AuditDefaultLimit+10; i++ {
		if err := m.AppendAudit(ctx, AuditEvent{Category: AuditOperation, Action: "seed"}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	got, err := m.ListAudit(ctx, AuditFilter{}) // zero limit -> default
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != AuditDefaultLimit {
		t.Fatalf("zero limit: want default %d, got %d", AuditDefaultLimit, len(got))
	}
	got, err = m.ListAudit(ctx, AuditFilter{Limit: AuditMaxLimit + 1000})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != AuditDefaultLimit+10 {
		t.Fatalf("over-max limit: want all %d, got %d", AuditDefaultLimit+10, len(got))
	}
}
