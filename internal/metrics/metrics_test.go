package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

// histSampleCount gathers the package Registry and returns the observation count
// of the histogram series named `name` carrying exactly the given labels. It
// returns 0 if no such series exists yet. Reading straight from the shared
// Registry (rather than a fresh one) is deliberate: these tests exercise the
// same global collectors production uses, so a regression in registration would
// surface here.
func histSampleCount(t *testing.T, name string, labels map[string]string) uint64 {
	t.Helper()
	families, err := Registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if labelsMatch(m.GetLabel(), labels) {
				return m.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

func labelsMatch(got []*dto.LabelPair, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for _, lp := range got {
		if want[lp.GetName()] != lp.GetValue() {
			return false
		}
	}
	return true
}

func TestObservePromotion_Success(t *testing.T) {
	// Unique app name keeps this series isolated from any other test touching
	// the shared Registry, so before/after deltas are unambiguous.
	const app = "test-obs-success"
	counter := promotionsTotal.WithLabelValues(app, "int", ActionSeed, "success")
	before := testutil.ToFloat64(counter)
	beforeHist := histSampleCount(t, "ringpromoter_promotion_duration_seconds",
		map[string]string{"application": app, "ring": "int", "action": ActionSeed})

	ObservePromotion(app, "int", ActionSeed, true, 3.2)

	if got := testutil.ToFloat64(counter) - before; got != 1 {
		t.Errorf("success counter delta = %v, want 1", got)
	}
	afterHist := histSampleCount(t, "ringpromoter_promotion_duration_seconds",
		map[string]string{"application": app, "ring": "int", "action": ActionSeed})
	if afterHist-beforeHist != 1 {
		t.Errorf("duration histogram sample delta = %d, want 1", afterHist-beforeHist)
	}
}

func TestObservePromotion_Failure(t *testing.T) {
	const app = "test-obs-failure"
	failure := promotionsTotal.WithLabelValues(app, "prod", ActionPromote, "failure")
	success := promotionsTotal.WithLabelValues(app, "prod", ActionPromote, "success")
	beforeFail := testutil.ToFloat64(failure)
	beforeOK := testutil.ToFloat64(success)

	ObservePromotion(app, "prod", ActionPromote, false, 1.0)

	if got := testutil.ToFloat64(failure) - beforeFail; got != 1 {
		t.Errorf("failure counter delta = %v, want 1", got)
	}
	if got := testutil.ToFloat64(success) - beforeOK; got != 0 {
		t.Errorf("success counter moved on a failure: delta = %v, want 0", got)
	}
}

func TestObservePromotion_UnknownFallback(t *testing.T) {
	// Empty app/ring/action must collapse to "unknown" to keep cardinality
	// bounded — never emit an empty-string label series.
	unknown := promotionsTotal.WithLabelValues("unknown", "unknown", "unknown", "success")
	before := testutil.ToFloat64(unknown)

	ObservePromotion("", "", "", true, 0.5)

	if got := testutil.ToFloat64(unknown) - before; got != 1 {
		t.Errorf("unknown-fallback counter delta = %v, want 1", got)
	}
}

func TestProvider(t *testing.T) {
	cases := map[string]string{
		"kubectl":         "kubectl",
		"KubeCtl":         "kubectl",
		"github":          "github_actions",
		"github_actions":  "github_actions",
		"gha":             "github_actions",
		"k8sjob":          "k8sjob",
		"k8s_job":         "k8sjob",
		"job":             "k8sjob",
		"log":             "log",
		"":                "log",
		"  kubectl  ":     "kubectl",
		"something-weird": "other",
	}
	for in, want := range cases {
		if got := Provider(in); got != want {
			t.Errorf("Provider(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStrategy(t *testing.T) {
	cases := map[string]string{
		"kubectl": "rolling",
		"github":  "workflow",
		"k8sjob":  "job",
		"log":     "noop",
		"":        "noop", // "" -> Provider "log" -> "noop"
		"mystery": "other",
	}
	for in, want := range cases {
		if got := Strategy(in); got != want {
			t.Errorf("Strategy(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClipStatusBool(t *testing.T) {
	if got := clip("green", "red", "green", "blue"); got != "green" {
		t.Errorf("clip allowed value = %q, want green", got)
	}
	if got := clip("purple", "red", "green"); got != "other" {
		t.Errorf("clip disallowed value = %q, want other", got)
	}
	if status(nil) != "success" || status(io.EOF) != "error" {
		t.Error("status did not map nil->success / err->error")
	}
	if boolStatus(true) != "success" || boolStatus(false) != "failure" {
		t.Error("boolStatus did not map true->success / false->failure")
	}
}

func TestHTTPMiddleware_MatchedRoute(t *testing.T) {
	// Route through a real ServeMux so r.Pattern is populated authentically,
	// exactly as in production — the middleware relies on that for the route label.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /widgets/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hi"))
	})
	h := HTTPMiddleware(mux)

	counter := httpRequestsTotal.WithLabelValues("GET", "GET /widgets/{id}", "418")
	before := testutil.ToFloat64(counter)

	// Two distinct wildcard values must collapse to the one route pattern.
	for _, id := range []string{"1", "2"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/widgets/"+id, nil))
		if rec.Code != http.StatusTeapot {
			t.Fatalf("status = %d, want 418", rec.Code)
		}
	}

	if got := testutil.ToFloat64(counter) - before; got != 2 {
		t.Errorf("requests_total{route=\"GET /widgets/{id}\",status=\"418\"} delta = %v, want 2", got)
	}
}

func TestHTTPMiddleware_Unmatched(t *testing.T) {
	mux := http.NewServeMux() // nothing registered -> 404, empty Pattern
	h := HTTPMiddleware(mux)

	counter := httpRequestsTotal.WithLabelValues("GET", "<unmatched>", "404")
	before := testutil.ToFloat64(counter)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/no/such/route", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	if got := testutil.ToFloat64(counter) - before; got != 1 {
		t.Errorf("unmatched route counter delta = %v, want 1", got)
	}
}

func TestHandlerServesRegistry(t *testing.T) {
	// Seed at least one series so the exposition is non-trivial. build_info is a
	// promauto GaugeVec that emits nothing until SetBuildInfo is called (main does
	// this at startup), so publish it here before asserting it appears.
	ObservePromotion("test-handler", "int", ActionSeed, true, 1)
	SetBuildInfo(BuildInfo{Version: "test", Commit: "test"})

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	text := string(body)
	for _, want := range []string{
		"ringpromoter_promotion_operations_total",
		"go_goroutines", // runtime collector wired in init
		"ringpromoter_build_info",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("/metrics body missing %q", want)
		}
	}
}

func TestSetBuildInfo(t *testing.T) {
	SetBuildInfo(BuildInfo{Version: "v9.9.9", Commit: "abc123"})
	// build_date/go_version/platform/arch were empty -> must render as "unknown".
	g := buildInfo.WithLabelValues("v9.9.9", "abc123", "unknown", "unknown", "unknown", "unknown")
	if got := testutil.ToFloat64(g); got != 1 {
		t.Errorf("build_info gauge = %v, want 1", got)
	}
	if orUnknown("") != "unknown" || orUnknown("x") != "x" {
		t.Error("orUnknown mapping wrong")
	}
}
