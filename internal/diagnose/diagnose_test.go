package diagnose

import (
	"context"
	"errors"
	"testing"

	"github.com/example/ring-promoter/internal/llm"
)

// fakeProvider records the last request and returns a canned answer.
type fakeProvider struct {
	got  llm.Request
	text string
	err  error
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	f.got = req
	if f.err != nil {
		return llm.Response{}, f.err
	}
	return llm.Response{Text: f.text}, nil
}

func TestDiagnoseSendsReportAndReturnsAnswer(t *testing.T) {
	p := &fakeProvider{text: "The health check failed.\n- Check the URL"}
	answer, err := New(p, nil).Diagnose(context.Background(), "promote failed: health check timeout")
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if answer != p.text {
		t.Errorf("answer = %q, want %q", answer, p.text)
	}
	if p.got.System != systemPrompt {
		t.Error("system prompt is not the diagnosis prompt")
	}
	if len(p.got.Messages) != 1 || p.got.Messages[0] != (llm.Message{Role: "user", Content: "promote failed: health check timeout"}) {
		t.Errorf("messages = %+v", p.got.Messages)
	}
	if p.got.Format != nil {
		t.Errorf("format = %s, want none (plain-text answer)", p.got.Format)
	}
	if p.got.Temperature != 0.2 {
		t.Errorf("temperature = %v, want 0.2", p.got.Temperature)
	}
}

func TestTestPlanAsksForJSON(t *testing.T) {
	p := &fakeProvider{text: `{"summary":"s","checklist":[],"links":[]}`}
	answer, err := New(p, nil).TestPlan(context.Background(), "kit report")
	if err != nil {
		t.Fatalf("TestPlan: %v", err)
	}
	if answer != p.text {
		t.Errorf("answer = %q", answer)
	}
	if p.got.System != testPlanPrompt {
		t.Error("system prompt is not the test-plan prompt")
	}
	if string(p.got.Format) != `"json"` {
		t.Errorf("format = %s, want \"json\"", p.got.Format)
	}
	if len(p.got.Messages) != 1 || p.got.Messages[0].Content != "kit report" {
		t.Errorf("messages = %+v", p.got.Messages)
	}
}

func TestProviderErrorIsReturnedUnchanged(t *testing.T) {
	cause := errors.New("gateway said no")
	p := &fakeProvider{err: cause}
	c := New(p, nil)
	if _, err := c.Diagnose(context.Background(), "r"); !errors.Is(err, cause) {
		t.Errorf("Diagnose err = %v, want %v", err, cause)
	}
	p.err = llm.ErrUnavailable
	if _, err := c.TestPlan(context.Background(), "r"); !errors.Is(err, llm.ErrUnavailable) {
		t.Errorf("TestPlan err = %v, want ErrUnavailable", err)
	}
}
