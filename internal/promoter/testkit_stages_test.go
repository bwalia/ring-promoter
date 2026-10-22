package promoter

import (
	"testing"

	"github.com/example/ring-promoter/internal/progress"
)

// sinkReporter records stages and gates pushed through it.
type sinkReporter struct {
	noopTestReporter
	stages []progress.Stage
	gates  []progress.GateVerdict
}

type noopTestReporter struct{}

func (noopTestReporter) StartStep(string, string)  {}
func (noopTestReporter) Log(string)                {}
func (noopTestReporter) FinishStep(string, string) {}

func (s *sinkReporter) SetStages(st []progress.Stage)  { s.stages = st }
func (s *sinkReporter) AddGate(g progress.GateVerdict) { s.gates = append(s.gates, g) }

// The deploy-time collector must not swallow stage or gate reports: it wraps
// the job's reporter for exactly the window in which a backend reports stages.
func TestKitCollectorPassesStagesAndGatesThrough(t *testing.T) {
	inner := &sinkReporter{}
	c := newKitCollector(inner)

	var r Reporter = c
	if _, ok := r.(progress.StageSink); !ok {
		t.Fatal("kitCollector does not implement progress.StageSink")
	}
	c.SetStages([]progress.Stage{{ID: "1", Name: "Build IPA", Status: "running"}})
	c.AddGate(progress.GateVerdict{Gate: "qa_signoff", Verdict: GateVerdictPass})

	if len(inner.stages) != 1 || inner.stages[0].Name != "Build IPA" {
		t.Fatalf("stages not passed through: %+v", inner.stages)
	}
	if len(inner.gates) != 1 || inner.gates[0].Gate != "qa_signoff" {
		t.Fatalf("gates not passed through: %+v", inner.gates)
	}
}
