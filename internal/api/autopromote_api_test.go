package api

import (
	"encoding/json"
	"net/http"
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

// newAutoPromoteServer builds an API server whose "web" app declares
// auto_promote in config for the named rings, leaving the rest unset.
func newAutoPromoteServer(t *testing.T, decl map[string]bool) http.Handler {
	t.Helper()
	rings := map[string]config.RingConfig{}
	for _, r := range ring.Names() {
		rc := config.RingConfig{
			Namespace: r, Deployment: "web", Container: "web",
			Image: "repo/web", HealthURL: "health://web/" + r,
		}
		if want, ok := decl[r]; ok {
			v := want
			rc.AutoPromote = &v
		}
		rings[r] = rc
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
	return NewServer(prom, "tok", "", http.NotFoundHandler(), time.Minute, nil, BuildInfo{}, nil).Handler()
}

// A ring whose auto-promote is declared in config refuses the API toggle with
// 409; a ring config says nothing about keeps working as before.
func TestAPI_AutoPromote_ConfigOwnedRingIs409(t *testing.T) {
	h := newAutoPromoteServer(t, map[string]bool{"test": false})

	rec := doJSON(t, h, "PUT", "/api/apps/web/rings/test/auto-promote", `{"enabled":true}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("config-owned ring: want 409, got %d %s", rec.Code, rec.Body)
	}
	if body := rec.Body.String(); !strings.Contains(body, "config") {
		t.Fatalf("409 should say config owns it, got %s", body)
	}
	// Undeclared ring: unchanged behaviour.
	if rec := doJSON(t, h, "PUT", "/api/apps/web/rings/int/auto-promote", `{"enabled":true}`); rec.Code != http.StatusOK {
		t.Fatalf("undeclared ring: want 200, got %d %s", rec.Code, rec.Body)
	}
}

// The rings read model tells the UI which switches are config-managed, so it can
// disable the control instead of offering one that returns 409.
func TestAPI_Rings_ReportAutoPromoteManaged(t *testing.T) {
	h := newAutoPromoteServer(t, map[string]bool{"test": true})

	rec := doJSON(t, h, "GET", "/api/apps/web/rings", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get rings: %d %s", rec.Code, rec.Body)
	}
	var got struct {
		Rings []struct {
			Ring struct {
				Name string `json:"name"`
			} `json:"ring"`
			AutoPromoteManaged bool `json:"auto_promote_managed"`
		} `json:"rings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body)
	}
	if len(got.Rings) == 0 {
		t.Fatalf("no rings returned: %s", rec.Body)
	}
	for _, r := range got.Rings {
		want := r.Ring.Name == "test"
		if r.AutoPromoteManaged != want {
			t.Fatalf("ring %s: auto_promote_managed=%v, want %v", r.Ring.Name, r.AutoPromoteManaged, want)
		}
	}
}
