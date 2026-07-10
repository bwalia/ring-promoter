package api

import (
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

// newTestServer builds a full API server on the in-memory backends, with an
// app configured in every ring.
func newTestServer(t *testing.T, prodPass string) http.Handler {
	t.Helper()
	h, _ := newTestServerWithDiag(t, prodPass, nil)
	return h
}

// newTestServerWithDiag additionally wires a Diagnoser and returns the backing
// store so tests can seed history entries directly.
func newTestServerWithDiag(t *testing.T, prodPass string, diag Diagnoser) (http.Handler, store.Store) {
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
	st := store.NewMemory()
	prom := promoter.New(cfg, st, nil, deployer.NewLogDeployer(nil), health.AlwaysHealthy{}, nil)
	return NewServer(prom, "tok", prodPass, http.NotFoundHandler(), time.Minute, nil, BuildInfo{}, diag).Handler(), st
}

func doJSON(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestProdPassword_GuardsProductionDeploys(t *testing.T) {
	h := newTestServer(t, "s3cret")

	// Get a version up to acc first (no password needed below prod).
	for _, step := range []struct{ path, body string }{
		{"/api/apps/web/seed", `{"ring":"int","version":"v1"}`},
		{"/api/apps/web/promote", `{"from_ring":"int"}`},
		{"/api/apps/web/promote", `{"from_ring":"test"}`},
	} {
		if rec := doJSON(t, h, "POST", step.path, step.body); rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d body %s", step.path, rec.Code, rec.Body)
		}
	}

	// Promote acc -> prod: missing, wrong, then correct password.
	if rec := doJSON(t, h, "POST", "/api/apps/web/promote", `{"from_ring":"acc"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("no password: expected 403, got %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, "POST", "/api/apps/web/promote", `{"from_ring":"acc","password":"nope"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong password: expected 403, got %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, "POST", "/api/apps/web/promote", `{"from_ring":"acc","password":"s3cret"}`); rec.Code != http.StatusOK {
		t.Fatalf("correct password: expected 200, got %d %s", rec.Code, rec.Body)
	}

	// Seeding prod directly is guarded too (async included).
	if rec := doJSON(t, h, "POST", "/api/apps/web/seed?async=1", `{"ring":"prod","version":"v2"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("seed prod without password: expected 403, got %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, "POST", "/api/apps/web/seed", `{"ring":"prod","version":"v2","password":"s3cret"}`); rec.Code != http.StatusOK {
		t.Fatalf("seed prod with password: expected 200, got %d %s", rec.Code, rec.Body)
	}

	// Enabling auto-promote INTO prod (on acc) is guarded; disabling is not.
	if rec := doJSON(t, h, "PUT", "/api/apps/web/rings/acc/auto-promote", `{"enabled":true}`); rec.Code != http.StatusForbidden {
		t.Fatalf("enable auto->prod without password: expected 403, got %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, "PUT", "/api/apps/web/rings/acc/auto-promote", `{"enabled":true,"password":"s3cret"}`); rec.Code != http.StatusOK {
		t.Fatalf("enable auto->prod with password: expected 200, got %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, "PUT", "/api/apps/web/rings/acc/auto-promote", `{"enabled":false}`); rec.Code != http.StatusOK {
		t.Fatalf("disable auto->prod: expected 200, got %d %s", rec.Code, rec.Body)
	}
	// Lower rings never need it.
	if rec := doJSON(t, h, "PUT", "/api/apps/web/rings/test/auto-promote", `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable auto on test: expected 200, got %d %s", rec.Code, rec.Body)
	}

	// Rollback of prod stays password-free (incident response).
	if rec := doJSON(t, h, "POST", "/api/apps/web/rollback", `{"ring":"prod"}`); rec.Code != http.StatusOK {
		t.Fatalf("rollback prod: expected 200, got %d %s", rec.Code, rec.Body)
	}
}

func TestProdPassword_DisabledWhenUnset(t *testing.T) {
	h := newTestServer(t, "")
	doJSON(t, h, "POST", "/api/apps/web/seed", `{"ring":"acc","version":"v1"}`)
	if rec := doJSON(t, h, "POST", "/api/apps/web/promote", `{"from_ring":"acc"}`); rec.Code != http.StatusOK {
		t.Fatalf("no password configured: expected 200, got %d %s", rec.Code, rec.Body)
	}
}

func TestGroups_CRUDAndValidation(t *testing.T) {
	h := newTestServer(t, "")

	// Empty at first.
	if rec := doJSON(t, h, "GET", "/api/groups", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"groups":[]`) {
		t.Fatalf("initial list: %d %s", rec.Code, rec.Body)
	}

	// Create (with a duplicate member that must be deduplicated).
	rec := doJSON(t, h, "POST", "/api/groups", `{"name":" Core ","apps":["web","web"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var created struct {
		ID   string   `json:"id"`
		Name string   `json:"name"`
		Apps []string `json:"apps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.ID == "" || created.Name != "Core" || len(created.Apps) != 1 {
		t.Fatalf("bad created group: %+v", created)
	}

	// Validation: empty name, unknown app.
	if rec := doJSON(t, h, "POST", "/api/groups", `{"name":"  ","apps":[]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty name: %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, "POST", "/api/groups", `{"name":"x","apps":["nope"]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown app: %d %s", rec.Code, rec.Body)
	}

	// Update.
	if rec := doJSON(t, h, "PUT", "/api/groups/"+created.ID, `{"name":"Platform","apps":["web"]}`); rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, "PUT", "/api/groups/missing", `{"name":"x","apps":[]}`); rec.Code != http.StatusNotFound {
		t.Fatalf("update missing: %d %s", rec.Code, rec.Body)
	}

	// List reflects the update and is shared state (no cookies/session involved).
	if rec := doJSON(t, h, "GET", "/api/groups", ""); !strings.Contains(rec.Body.String(), "Platform") {
		t.Fatalf("list after update: %s", rec.Body)
	}

	// Delete.
	if rec := doJSON(t, h, "DELETE", "/api/groups/"+created.ID, ""); rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, "DELETE", "/api/groups/"+created.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("delete twice: %d %s", rec.Code, rec.Body)
	}
}
