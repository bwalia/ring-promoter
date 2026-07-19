package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/deployer"
	"github.com/example/ring-promoter/internal/health"
	"github.com/example/ring-promoter/internal/promoter"
	"github.com/example/ring-promoter/internal/ring"
	"github.com/example/ring-promoter/internal/store"
)

// newGatedServer builds an API server whose "web" app has the given promotion
// policy, on the in-memory backends with an always-healthy checker.
func newGatedServer(t *testing.T, policy *config.PromotionPolicy) http.Handler {
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
		Apps:     []config.AppConfig{{Name: "web", Rings: rings, PromotionPolicy: policy}},
	}
	st := store.NewMemory()
	prom := promoter.New(cfg, st, nil, deployer.NewLogDeployer(nil), health.AlwaysHealthy{}, nil)
	return NewServer(prom, "tok", "", http.NotFoundHandler(), time.Minute, nil, BuildInfo{}, nil).Handler()
}

// A QA sign-off gate blocks a promotion until a GO is recorded via the API.
func TestAPI_SignoffGate(t *testing.T) {
	h := newGatedServer(t, &config.PromotionPolicy{
		QASignoff: &config.GatePolicy{Rings: []string{"test"}},
	})
	// Seed int with v1.
	if rec := doJSON(t, h, "POST", "/api/apps/web/seed", `{"ring":"int","version":"v1"}`); rec.Code != http.StatusOK {
		t.Fatalf("seed int: %d %s", rec.Code, rec.Body)
	}
	// Promote int→test without a sign-off → 409 Conflict.
	if rec := doJSON(t, h, "POST", "/api/apps/web/promote", `{"from_ring":"int"}`); rec.Code != http.StatusConflict {
		t.Fatalf("promote without signoff: want 409, got %d %s", rec.Code, rec.Body)
	}
	// Record a GO sign-off for (test, v1).
	body := `{"ring":"test","version":"v1","decision":"go","engineer":"J. Patel","qa_status":"passed"}`
	if rec := doJSON(t, h, "POST", "/api/apps/web/signoffs", body); rec.Code != http.StatusCreated {
		t.Fatalf("record signoff: %d %s", rec.Code, rec.Body)
	}
	// A sign-off missing the engineer is rejected.
	if rec := doJSON(t, h, "POST", "/api/apps/web/signoffs", `{"ring":"test","version":"v1","decision":"go"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("signoff without engineer: want 400, got %d %s", rec.Code, rec.Body)
	}
	// Now the promotion succeeds.
	if rec := doJSON(t, h, "POST", "/api/apps/web/promote", `{"from_ring":"int"}`); rec.Code != http.StatusOK {
		t.Fatalf("promote after GO: want 200, got %d %s", rec.Code, rec.Body)
	}
	// Sign-offs are listed.
	rec := doJSON(t, h, "GET", "/api/apps/web/signoffs", "")
	var listed struct {
		Signoffs []store.Signoff `json:"signoffs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil || len(listed.Signoffs) != 1 {
		t.Fatalf("list signoffs: %d %s err=%v", rec.Code, rec.Body, err)
	}
}

// A change-request gate requires a code; the demo code "test" passes, a real
// code fails against the demo validator.
func TestAPI_ChangeRequestGate(t *testing.T) {
	h := newGatedServer(t, &config.PromotionPolicy{
		ChangeRequest: &config.ChangeRequestPolicy{Rings: []string{"acc"}, Provider: "test"},
	})
	// Seeding into acc without a code → 400.
	if rec := doJSON(t, h, "POST", "/api/apps/web/seed", `{"ring":"acc","version":"v1"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("seed acc no code: want 400, got %d %s", rec.Code, rec.Body)
	}
	// A non-demo code against the demo-only validator → 400 (invalid).
	if rec := doJSON(t, h, "POST", "/api/apps/web/seed", `{"ring":"acc","version":"v1","cr_code":"CR-1"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("seed acc bad code: want 400, got %d %s", rec.Code, rec.Body)
	}
	// The demo code passes.
	if rec := doJSON(t, h, "POST", "/api/apps/web/seed", `{"ring":"acc","version":"v1","cr_code":"test"}`); rec.Code != http.StatusOK {
		t.Fatalf("seed acc demo code: want 200, got %d %s", rec.Code, rec.Body)
	}
}

// Ad-hoc maintenance windows: create → list (open) → promote passes → delete.
func TestAPI_MaintenanceWindowGate(t *testing.T) {
	h := newGatedServer(t, &config.PromotionPolicy{
		MaintenanceWindow: &config.MaintenanceWindowPolicy{Rings: []string{"test"}},
	})
	if rec := doJSON(t, h, "POST", "/api/apps/web/seed", `{"ring":"int","version":"v1"}`); rec.Code != http.StatusOK {
		t.Fatalf("seed int: %d %s", rec.Code, rec.Body)
	}
	// No window → promotion into test blocked (409).
	if rec := doJSON(t, h, "POST", "/api/apps/web/promote", `{"from_ring":"int"}`); rec.Code != http.StatusConflict {
		t.Fatalf("promote no window: want 409, got %d %s", rec.Code, rec.Body)
	}
	// Open an ad-hoc window covering now for the "test" ring.
	now := time.Now().UTC()
	win := `{"ring":"test","starts_at":"` + now.Add(-time.Hour).Format(time.RFC3339) +
		`","ends_at":"` + now.Add(time.Hour).Format(time.RFC3339) + `","reason":"release","created_by":"J. Patel"}`
	rec := doJSON(t, h, "POST", "/api/apps/web/maintenance-windows", win)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create window: %d %s", rec.Code, rec.Body)
	}
	var created store.MaintenanceWindow
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("decode window: %v body=%s", err, rec.Body)
	}
	// List shows the window and reports the ring open.
	lrec := doJSON(t, h, "GET", "/api/apps/web/maintenance-windows", "")
	var view promoter.MaintenanceView
	if err := json.Unmarshal(lrec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode view: %v body=%s", err, lrec.Body)
	}
	if len(view.Windows) != 1 || !view.OpenRings["test"] {
		t.Fatalf("expected 1 window and test open, got %+v", view)
	}
	// Promotion now passes.
	if rec := doJSON(t, h, "POST", "/api/apps/web/promote", `{"from_ring":"int"}`); rec.Code != http.StatusOK {
		t.Fatalf("promote in window: want 200, got %d %s", rec.Code, rec.Body)
	}
	// Delete the window; a second delete 404s.
	if rec := doJSON(t, h, "DELETE", "/api/apps/web/maintenance-windows/"+created.ID, ""); rec.Code != http.StatusOK {
		t.Fatalf("delete window: %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, "DELETE", "/api/apps/web/maintenance-windows/"+created.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("delete twice: want 404, got %d %s", rec.Code, rec.Body)
	}
}

// The rings read model advertises which gates guard each ring so the UI can
// prompt appropriately.
func TestAPI_RingsExposeGates(t *testing.T) {
	h := newGatedServer(t, &config.PromotionPolicy{
		ChangeRequest: &config.ChangeRequestPolicy{Rings: []string{"prod"}, Provider: "test"},
	})
	rec := doJSON(t, h, "GET", "/api/apps/web/rings", "")
	var resp struct {
		Rings []promoter.RingView `json:"rings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode rings: %v", err)
	}
	for _, rv := range resp.Rings {
		want := rv.Ring.Name == "prod"
		if rv.Gates.ChangeRequest != want {
			t.Errorf("ring %s change_request gate = %v, want %v", rv.Ring.Name, rv.Gates.ChangeRequest, want)
		}
	}
}
