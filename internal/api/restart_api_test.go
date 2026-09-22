package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRestart_SyncAsyncAndProdPassword(t *testing.T) {
	h := newTestServer(t, "s3cret")

	// Preconditions: unknown app / ring → 404, never-deployed ring → 409.
	if rec := doJSON(t, h, "POST", "/api/apps/nope/rings/int/restart", `{}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown app: expected 404, got %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, "POST", "/api/apps/web/rings/ring99/restart?async=1", `{}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown ring: expected 404, got %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, "POST", "/api/apps/web/rings/int/restart?async=1", `{}`); rec.Code != http.StatusConflict {
		t.Fatalf("nothing deployed: expected 409, got %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, "POST", "/api/apps/web/rings/int/restart", `{"bogus":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: expected 400, got %d %s", rec.Code, rec.Body)
	}

	for _, step := range []struct{ path, body string }{
		{"/api/apps/web/seed", `{"ring":"int","version":"v1"}`},
		{"/api/apps/web/seed", `{"ring":"prod","version":"v1","password":"s3cret"}`},
	} {
		if rec := doJSON(t, h, "POST", step.path, step.body); rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d body %s", step.path, rec.Code, rec.Body)
		}
	}

	// Sync: 200 with the seed-shaped Result; the version is unchanged.
	rec := doJSON(t, h, "POST", "/api/apps/web/rings/int/restart", `{"reason":"rotate db password"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("sync restart: expected 200, got %d %s", rec.Code, rec.Body)
	}
	var res struct {
		Action  string `json:"action"`
		Ring    string `json:"ring"`
		Version string `json:"version"`
		Success bool   `json:"success"`
		State   struct {
			CurrentVersion string `json:"current_version"`
		} `json:"state"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Action != "restart" || res.Ring != "int" || res.Version != "v1" || !res.Success || res.State.CurrentVersion != "v1" {
		t.Fatalf("unexpected result: %s", rec.Body)
	}

	// Async: 202 + job id, pollable like a seed job.
	id := startJob(t, h, "/api/apps/web/rings/int/restart?async=1", `{}`)
	waitForJob(t, h, id, jobSuccess)
	if rec := doJSON(t, h, "GET", "/api/apps/web/jobs/"+id, ""); !strings.Contains(rec.Body.String(), `"action":"restart"`) {
		t.Fatalf("job action: %s", rec.Body)
	}

	// Production: password required exactly like seeding prod (async included).
	if rec := doJSON(t, h, "POST", "/api/apps/web/rings/prod/restart?async=1", `{}`); rec.Code != http.StatusForbidden {
		t.Fatalf("prod without password: expected 403, got %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, "POST", "/api/apps/web/rings/prod/restart", `{"password":"nope"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("prod wrong password: expected 403, got %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, "POST", "/api/apps/web/rings/prod/restart", `{"password":"s3cret"}`); rec.Code != http.StatusOK {
		t.Fatalf("prod with password: expected 200, got %d %s", rec.Code, rec.Body)
	}

	// Named deployments: DNS-1123 labels, at most 20 — else 400, before any job.
	if rec := doJSON(t, h, "POST", "/api/apps/web/rings/int/restart?async=1", `{"deployments":["Bad_Name"]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid deployment name: expected 400, got %d %s", rec.Code, rec.Body)
	}
	many := `"d0"` + strings.Repeat(`,"d0"`, 20)
	if rec := doJSON(t, h, "POST", "/api/apps/web/rings/int/restart", `{"deployments":[`+many+`]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("21 deployments: expected 400, got %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, h, "POST", "/api/apps/web/rings/int/restart", `{"deployments":["jobshout-api","jobshout-web"]}`); rec.Code != http.StatusOK {
		t.Fatalf("named deployments: expected 200, got %d %s", rec.Code, rec.Body)
	}

	// History shows the restarts with the reason and the deployment list.
	rec = doJSON(t, h, "GET", "/api/apps/web/history", "")
	if !strings.Contains(rec.Body.String(), `"action":"restart"`) || !strings.Contains(rec.Body.String(), "rotate db password") ||
		!strings.Contains(rec.Body.String(), "[deployments: jobshout-api jobshout-web]") {
		t.Fatalf("history missing restart entries: %s", rec.Body)
	}
}
