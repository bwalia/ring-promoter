package promoter

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/store"
)

func TestAppControls_SeedHopWhenIntEmpty(t *testing.T) {
	p, _, _, _ := newHarness(t, 1)
	view, err := p.AppControls(context.Background(), testApp)
	if err != nil {
		t.Fatalf("controls: %v", err)
	}
	if len(view.Hops) != 1 || view.Hops[0].From != "" || view.Hops[0].To != "int" {
		t.Fatalf("want one seed hop into int, got %+v", view.Hops)
	}
	if view.Hops[0].Ready {
		t.Fatalf("empty seed should not be ready without a version: %+v", view.Hops[0])
	}
}

func TestAppControls_PromoteBlockedBySignoff(t *testing.T) {
	p, _, _, st := newHarness(t, 1)
	ctx := context.Background()
	cfg := testConfig(1)
	cfg.Apps[0].PromotionPolicy = &config.PromotionPolicy{
		QASignoff: &config.GatePolicy{Rings: []string{"test"}},
	}
	p.cfg = cfg
	mustSeed(t, p, "int", "v1")
	view, err := p.AppControls(ctx, testApp)
	if err != nil {
		t.Fatalf("controls: %v", err)
	}
	var hop *HopScore
	for i := range view.Hops {
		if view.Hops[i].From == "int" && view.Hops[i].To == "test" {
			hop = &view.Hops[i]
		}
	}
	if hop == nil {
		t.Fatalf("missing int→test hop: %+v", view.Hops)
	}
	if hop.Ready {
		t.Fatalf("sign-off required hop should not be ready: %+v", hop)
	}
	found := false
	for _, c := range hop.Controls {
		if c.ID == "qa_signoff" && c.Status == ControlBlock {
			found = true
		}
	}
	if !found {
		t.Fatalf("want blocked qa_signoff, got %+v", hop.Controls)
	}

	if err := st.UpsertSignoff(ctx, store.Signoff{
		App: testApp, Ring: "test", Version: "v1", Decision: store.DecisionGo,
		Engineer: "qa", QAStatus: "passed", UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("signoff: %v", err)
	}
	view, err = p.AppControls(ctx, testApp)
	if err != nil {
		t.Fatalf("controls after signoff: %v", err)
	}
	for i := range view.Hops {
		if view.Hops[i].From == "int" && view.Hops[i].To == "test" {
			hop = &view.Hops[i]
		}
	}
	for _, c := range hop.Controls {
		if c.ID == "qa_signoff" && c.Status != ControlPass {
			t.Fatalf("sign-off should pass, got %+v", c)
		}
	}
}

func TestAppControls_QAAgentWeightsNoGo(t *testing.T) {
	p, _, _, _ := newHarness(t, 1)
	ctx := context.Background()
	cfg := testConfig(1)
	cfg.QAAgent = &config.QAAgentConfig{Name: "qa-bot"}
	p.cfg = cfg
	mustSeed(t, p, "int", "v1")
	if _, err := p.RecordQAReport(ctx, testApp, "int", "no_go", nil, "e2e red", "", time.Time{}); err != nil {
		t.Fatalf("qa report: %v", err)
	}
	hop, err := p.HopControls(ctx, testApp, "int", "test")
	if err != nil {
		t.Fatalf("hop: %v", err)
	}
	if hop.Ready {
		t.Fatalf("QA no-go should block: %+v", hop)
	}
	blocked := false
	for _, c := range hop.Controls {
		if c.ID == "qa_agent" && c.Status == ControlBlock && c.Score == 0 {
			blocked = true
		}
	}
	if !blocked {
		t.Fatalf("want qa_agent block, got %+v", hop.Controls)
	}
}

func TestControlsBriefText_IncludesNextAction(t *testing.T) {
	text := ControlsBriefText(HopScore{
		From: "int", To: "test", Version: "v1", Score: 40, Ready: false,
		Blockers: []string{"QA agent"},
		Summary:  "blocked",
		Controls: []ControlScore{{
			ID: "qa_agent", Label: "QA agent", Status: ControlBlock, Score: 0, Weight: 16,
			Detail: "NO-GO", NextAction: "Hold the promotion.",
		}},
	})
	if !strings.Contains(text, "QA agent") || !strings.Contains(text, "Hold the promotion") {
		t.Fatalf("brief missing action: %s", text)
	}
}
