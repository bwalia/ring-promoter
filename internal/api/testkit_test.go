package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// fakePlanner is a Diagnoser that can also write test plans.
type fakePlanner struct {
	fakeDiagnoser
	plans  atomic.Int64
	answer string
}

func (f *fakePlanner) TestPlan(_ context.Context, report string) (string, error) {
	f.plans.Add(1)
	f.report.Store(report)
	return f.answer, nil
}

func getKit(t *testing.T, h http.Handler, ring string) (int, testKitResponse) {
	t.Helper()
	rec := doJSON(t, h, "GET", "/api/apps/web/rings/"+ring+"/test-kit", "")
	var out testKitResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v (%s)", err, rec.Body)
		}
	}
	return rec.Code, out
}

func TestTestKit_EndpointsAndAIPlan(t *testing.T) {
	fp := &fakePlanner{answer: `{"summary":"Check the new build.","checklist":["Open it"],"links":[{"id":1,"why":"x"},{"url":"https://evil.example/"}]}`}
	h, _ := newTestServerWithDiag(t, "", fp)

	if code, _ := getKit(t, h, "int"); code != http.StatusConflict {
		t.Fatalf("empty ring: want 409, got %d", code)
	}
	if rec := doJSON(t, h, "POST", "/api/apps/web/seed", `{"ring":"int","version":"v1"}`); rec.Code != http.StatusOK {
		t.Fatalf("seed: %d %s", rec.Code, rec.Body)
	}
	code, kit := getKit(t, h, "int")
	if code != http.StatusOK || kit.Version != "v1" || !kit.AIEnabled || kit.PlanStatus != "none" {
		t.Fatalf("kit = %d %+v", code, kit)
	}

	// The log deployer produces no links and the test health URL is not
	// http(s), so give the model a candidate via the response we check below:
	// with no candidates every proposed link must be dropped.
	if rec := doJSON(t, h, "POST", "/api/apps/web/rings/int/test-kit/plan", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("plan: want 202, got %d %s", rec.Code, rec.Body)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, kit = getKit(t, h, "int")
		if kit.PlanStatus != diagRunning || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if kit.PlanStatus != diagDone || kit.Plan == nil || kit.Plan.Summary != "Check the new build." {
		t.Fatalf("plan not stored: %+v", kit)
	}
	if len(kit.Plan.Links) != 0 {
		t.Fatalf("plan kept links that were not candidates: %+v", kit.Plan.Links)
	}

	// A stored plan is reused; refresh=1 regenerates it.
	if rec := doJSON(t, h, "POST", "/api/apps/web/rings/int/test-kit/plan", ""); rec.Code != http.StatusOK {
		t.Fatalf("reuse: want 200, got %d", rec.Code)
	}
	if n := fp.plans.Load(); n != 1 {
		t.Fatalf("planner calls = %d, want 1", n)
	}
	if rec := doJSON(t, h, "POST", "/api/apps/web/rings/int/test-kit/plan?refresh=1", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("refresh: want 202, got %d", rec.Code)
	}
}

func TestTestKit_PlanNotConfigured(t *testing.T) {
	h := newTestServer(t, "")
	doJSON(t, h, "POST", "/api/apps/web/seed", `{"ring":"int","version":"v1"}`)
	if rec := doJSON(t, h, "POST", "/api/apps/web/rings/int/test-kit/plan", ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("want 501, got %d", rec.Code)
	}
	if _, kit := getKit(t, h, "int"); kit.AIEnabled {
		t.Fatal("ai_enabled should be false without a planner")
	}
}
