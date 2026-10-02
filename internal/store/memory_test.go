package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// Detailed failure logs are kept for only the newest KeepFailureLogs entries
// per app; older ones are trimmed back to the summary fields.
func TestMemoryTrimsFailureLogsToNewestThree(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	for i := 1; i <= KeepFailureLogs+2; i++ {
		if err := m.AddHistory(ctx, HistoryEntry{
			App: "web", Ring: "int", Action: ActionSeed,
			Result: ResultFailure, Message: fmt.Sprintf("fail %d", i),
			Logs: fmt.Sprintf("logs %d", i),
		}); err != nil {
			t.Fatalf("add history %d: %v", i, err)
		}
	}
	// Another app's failure logs must not count against web's quota.
	if err := m.AddHistory(ctx, HistoryEntry{
		App: "other", Ring: "int", Action: ActionSeed,
		Result: ResultFailure, Logs: "other logs",
	}); err != nil {
		t.Fatal(err)
	}

	entries, err := m.ListHistory(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != KeepFailureLogs+2 {
		t.Fatalf("got %d entries", len(entries))
	}
	// ListHistory never exposes logs.
	for _, e := range entries {
		if e.Logs != "" {
			t.Errorf("ListHistory leaked logs for entry %d", e.ID)
		}
	}

	// GetHistoryEntry: newest 3 keep logs, the 2 oldest were trimmed.
	for i, e := range entries {
		full, err := m.GetHistoryEntry(ctx, "web", e.ID)
		if err != nil {
			t.Fatalf("get %d: %v", e.ID, err)
		}
		wantLogs := i < KeepFailureLogs // entries are newest first
		if (full.Logs != "") != wantLogs {
			t.Errorf("entry %d (newest-first index %d): logs %q, want kept=%v", e.ID, i, full.Logs, wantLogs)
		}
	}

	// The other app's logs survived.
	others, _ := m.ListHistory(ctx, "other")
	full, err := m.GetHistoryEntry(ctx, "other", others[0].ID)
	if err != nil || full.Logs != "other logs" {
		t.Errorf("other app logs = %q, err %v", full.Logs, err)
	}
}

func TestMemory_TestKitsKeyedByVersionAndTrimmed(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0)
	m := NewMemoryWithClock(func() time.Time { now = now.Add(time.Second); return now })
	for i := 0; i < KeepTestKits+2; i++ {
		v := fmt.Sprintf("v%d", i)
		if err := m.SaveTestKit(ctx, TestKit{App: "a", Ring: "int", Version: v, Links: []TestLink{{URL: "https://x/" + v}}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.GetTestKit(ctx, "a", "int", "v0"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("oldest kit should be trimmed, got %v", err)
	}
	if err := m.SetTestPlan(ctx, "a", "int", "v11", TestPlan{Summary: "s"}); err != nil {
		t.Fatal(err)
	}
	k, err := m.GetTestKit(ctx, "a", "int", "v11")
	if err != nil || k.Plan == nil || k.Links[0].URL != "https://x/v11" {
		t.Fatalf("kit = %+v, %v", k, err)
	}
	// Re-saving a version replaces its evidence and clears its plan.
	_ = m.SaveTestKit(ctx, TestKit{App: "a", Ring: "int", Version: "v11"})
	if k, _ := m.GetTestKit(ctx, "a", "int", "v11"); k.Plan != nil {
		t.Fatal("plan should be cleared on re-save")
	}
	if err := m.SetTestPlan(ctx, "a", "test", "v1", TestPlan{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
