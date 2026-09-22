package promoter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/store"
)

func TestRecordQAReport_Disabled(t *testing.T) {
	p, _, _, _ := newHarness(t, 1)
	_, err := p.RecordQAReport(context.Background(), testApp, "int", store.QAVerdictGo, nil, "", "", time.Time{})
	if !errors.Is(err, ErrQAAgentDisabled) {
		t.Fatalf("got %v, want ErrQAAgentDisabled", err)
	}
}

func TestRecordQAReport_HappyPath(t *testing.T) {
	p, _, _, _ := newHarness(t, 1)
	p.cfg.QAAgent = &config.QAAgentConfig{Name: "qa-bot"}
	healthy := true
	got, err := p.RecordQAReport(context.Background(), testApp, "int", "GO", &healthy, "all green", "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkflowVerdict != store.QAVerdictGo {
		t.Fatalf("verdict=%s", got.WorkflowVerdict)
	}
	if got.Source != "qa-bot" {
		t.Fatalf("source=%s", got.Source)
	}
	list, err := p.ListQAReports(context.Background(), testApp)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v len=%d", err, len(list))
	}
}

func TestRecordQAReport_Scope(t *testing.T) {
	p, _, _, _ := newHarness(t, 1)
	p.cfg.QAAgent = &config.QAAgentConfig{Name: "qa-bot", Apps: []string{"other"}}
	_, err := p.RecordQAReport(context.Background(), testApp, "int", store.QAVerdictGo, nil, "", "", time.Time{})
	if !errors.Is(err, ErrQAAgentScope) {
		t.Fatalf("got %v, want ErrQAAgentScope", err)
	}
}
