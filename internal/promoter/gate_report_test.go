package promoter

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/progress"
)

// gateRecorder is a Reporter that also collects gate verdicts.
type gateRecorder struct {
	mu    sync.Mutex
	gates []progress.GateVerdict
}

func (*gateRecorder) StartStep(string, string)  {}
func (*gateRecorder) Log(string)                {}
func (*gateRecorder) FinishStep(string, string) {}
func (g *gateRecorder) AddGate(v progress.GateVerdict) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gates = append(g.gates, v)
}

// Gate verdicts reach the reporter — the failing one included, with its
// reason — so a live view can draw the Go/No-Go decision; a validate-only
// pre-check reports nothing.
func TestGateVerdictsReported(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	pol := &config.PromotionPolicy{
		ChangeRequest: &config.ChangeRequestPolicy{Rings: []string{"test"}},
		QASignoff:     &config.GatePolicy{Rings: []string{"test"}},
	}
	p, _, _ := gatedHarness(t, now, pol)
	mustSeed(t, p, "int", "v1")

	rec := &gateRecorder{}
	ctx := WithReporter(WithGateInputs(context.Background(), GateInputs{ChangeRequestCode: "test"}), rec)
	if err := p.ValidatePromote(ctx, testApp, "int"); err == nil {
		t.Fatal("expected the missing sign-off to fail validation")
	}
	if len(rec.gates) != 0 {
		t.Fatalf("validate-only must not report gates: %+v", rec.gates)
	}
	if _, err := p.Promote(ctx, testApp, "int"); err == nil {
		t.Fatal("expected the missing sign-off to block the promotion")
	}
	if len(rec.gates) != 1 {
		t.Fatalf("gates = %+v", rec.gates)
	}
	g := rec.gates[0]
	if g.Gate != "qa_signoff" || g.Verdict != GateVerdictFail || g.Detail == "" || g.Ring != "test" {
		t.Fatalf("gate = %+v", g)
	}
}
