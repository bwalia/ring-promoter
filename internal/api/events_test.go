package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/deployer"
	"github.com/example/ring-promoter/internal/health"
	"github.com/example/ring-promoter/internal/promoter"
	"github.com/example/ring-promoter/internal/ring"
	"github.com/example/ring-promoter/internal/store"
)

// newEventsTestServer builds the same in-memory server as newTestServerFull
// but keeps the *Server so the test can mutate jobs directly.
func newEventsTestServer(t *testing.T) *Server {
	t.Helper()
	rings := map[string]config.RingConfig{}
	for _, r := range ring.Names() {
		rings[r] = config.RingConfig{
			Namespace: r, Deployment: "web", Container: "web",
			Image: "repo/web", HealthURL: "health://web/" + r,
		}
	}
	zero := 0
	delay := config.Duration(time.Millisecond)
	cfg := &config.Config{
		APIToken: "tok",
		Retry:    config.RetryConfig{Count: &zero, Delay: &delay},
		Apps:     []config.AppConfig{{Name: "web", Rings: rings}},
	}
	prom := promoter.New(cfg, store.NewMemory(), nil, deployer.NewLogDeployer(nil), health.AlwaysHealthy{}, nil)
	return NewServer(prom, "tok", "", http.NotFoundHandler(), time.Minute, nil, BuildInfo{}, nil)
}

// readEvent reads one SSE event (up to a blank line) and returns its
// event name and data payload, skipping heartbeat comments.
func readEvent(t *testing.T, r *bufio.Reader) (string, string) {
	t.Helper()
	var event, data string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read event: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "" && (event != "" || data != ""):
			return event, data
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func TestEventStream(t *testing.T) {
	old := eventsCoalesce
	eventsCoalesce = 10 * time.Millisecond
	defer func() { eventsCoalesce = old }()

	srv := newEventsTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer tok")
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}

	r := bufio.NewReader(res.Body)

	// The initial snapshot arrives without any job activity.
	event, data := readEvent(t, r)
	if event != "jobs" {
		t.Fatalf("first event = %q, want jobs", event)
	}
	var payload struct {
		Jobs []jobState `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatalf("initial payload: %v", err)
	}
	if len(payload.Jobs) != 0 {
		t.Fatalf("initial jobs = %d, want 0", len(payload.Jobs))
	}

	// A job mutation pushes a fresh snapshot.
	job := srv.jobs.create("demo", "seed")
	job.StartStep("deploy", "Deploying")
	job.Log("hello from the deploy")

	event, data = readEvent(t, r)
	if event != "jobs" {
		t.Fatalf("second event = %q, want jobs", event)
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatalf("second payload: %v", err)
	}
	if len(payload.Jobs) != 1 || payload.Jobs[0].App != "demo" {
		t.Fatalf("jobs = %+v, want one for demo", payload.Jobs)
	}
	if len(payload.Jobs[0].Steps) != 1 || len(payload.Jobs[0].Steps[0].Logs) == 0 {
		t.Fatalf("steps = %+v, want one step with logs", payload.Jobs[0].Steps)
	}
}

func TestEventStreamRequiresAuth(t *testing.T) {
	srv := newEventsTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
}
