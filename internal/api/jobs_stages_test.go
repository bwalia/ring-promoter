package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/progress"
	"github.com/example/ring-promoter/internal/promoter"
)

func TestJob_StagesAndGatesJSON(t *testing.T) {
	m := NewJobManager()
	j := m.create("web", "promote")
	j.setRing("test")
	j.AddGate(progress.GateVerdict{Gate: "qa_signoff", Verdict: "pass", Detail: "v1 signed off", Ring: "test"})
	j.StartStep("deploy", "Deploy v1 to test")
	started := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	j.SetStages([]progress.Stage{{
		ID: "7", Name: "build", Status: progress.StageRunning, URL: "https://gh/job/7", StartedAt: &started,
		Steps: []progress.StageStep{{Number: 1, Name: "checkout", Status: progress.StageSuccess}},
	}})
	j.SetStages([]progress.Stage{{ID: "7", Name: "build", Status: progress.StageSuccess}})
	j.FinishStep(promoter.StepSuccess, "")

	b, err := json.Marshal(j.snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Ring  string `json:"ring"`
		Gates []struct {
			Gate, Verdict, Detail, Ring string
		} `json:"gates"`
		Steps []struct {
			Stages []map[string]any `json:"stages"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Ring != "test" || len(got.Gates) != 1 || got.Gates[0].Verdict != "pass" {
		t.Fatalf("job = %s", b)
	}
	if len(got.Steps) != 1 || len(got.Steps[0].Stages) != 1 || got.Steps[0].Stages[0]["status"] != "success" {
		t.Fatalf("stages should be replaced wholesale: %s", b)
	}

	// A job without stages or gates keeps the original JSON shape.
	plain, _ := json.Marshal(m.create("web", "seed").snapshot())
	if strings.Contains(string(plain), "gates") || strings.Contains(string(plain), "stages") || strings.Contains(string(plain), `"ring"`) {
		t.Fatalf("new fields should be omitted when empty: %s", plain)
	}
}
