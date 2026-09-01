package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/ring-promoter/internal/store"
)

func doJSONAs(t *testing.T, h http.Handler, method, path, body, actor string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	if actor != "" {
		req.Header.Set("X-Actor", actor)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func listAudit(t *testing.T, h http.Handler, query string) []store.AuditEvent {
	t.Helper()
	rec := doJSON(t, h, "GET", "/api/audit"+query, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/audit%s: status %d body %s", query, rec.Code, rec.Body)
	}
	var resp struct {
		Audit        []store.AuditEvent `json:"audit"`
		NextBeforeID int64              `json:"next_before_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode audit response: %v", err)
	}
	return resp.Audit
}

func TestAudit_OperationsAreRecordedWithActorAndCorrelation(t *testing.T) {
	h := newTestServer(t, "")

	if rec := doJSONAs(t, h, "POST", "/api/apps/web/seed", `{"ring":"int","version":"v1"}`, "alice"); rec.Code != http.StatusOK {
		t.Fatalf("seed: status %d body %s", rec.Code, rec.Body)
	}
	if rec := doJSONAs(t, h, "POST", "/api/apps/web/promote", `{"from_ring":"int"}`, "bob"); rec.Code != http.StatusOK {
		t.Fatalf("promote: status %d body %s", rec.Code, rec.Body)
	}

	events := listAudit(t, h, "?category=operation")
	if len(events) != 2 {
		t.Fatalf("want 2 operation events, got %d: %+v", len(events), events)
	}
	// Newest first: promote then seed.
	promote, seed := events[0], events[1]
	if promote.Action != "promote" || promote.Actor != "bob" || promote.ActorType != store.ActorHuman {
		t.Fatalf("promote event wrong: %+v", promote)
	}
	if seed.Action != "seed" || seed.Actor != "alice" || seed.App != "web" || seed.Ring != "int" || seed.Version != "v1" {
		t.Fatalf("seed event wrong: %+v", seed)
	}
	if seed.CorrelationID == "" || promote.CorrelationID == "" {
		t.Fatalf("missing correlation ids: %+v", events)
	}
	if seed.CorrelationID == promote.CorrelationID {
		t.Fatalf("separate requests must have separate correlation ids")
	}

	// The history rows carry the same correlation ids as their audit events.
	rec := doJSON(t, h, "GET", "/api/apps/web/history", "")
	var hist struct {
		History []store.HistoryEntry `json:"history"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &hist); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(hist.History) != 2 {
		t.Fatalf("want 2 history rows, got %d", len(hist.History))
	}
	if hist.History[0].CorrelationID != promote.CorrelationID {
		t.Fatalf("promote history correlation %q != audit %q", hist.History[0].CorrelationID, promote.CorrelationID)
	}
	if hist.History[1].CorrelationID != seed.CorrelationID {
		t.Fatalf("seed history correlation %q != audit %q", hist.History[1].CorrelationID, seed.CorrelationID)
	}
}

func TestAudit_AnonymousWhenNoActorHeader(t *testing.T) {
	h := newTestServer(t, "")
	if rec := doJSON(t, h, "POST", "/api/apps/web/seed", `{"ring":"int","version":"v1"}`); rec.Code != http.StatusOK {
		t.Fatalf("seed: status %d body %s", rec.Code, rec.Body)
	}
	events := listAudit(t, h, "?category=operation")
	if len(events) != 1 || events[0].Actor != "anonymous" || events[0].ActorType != store.ActorHuman {
		t.Fatalf("want one anonymous human event, got %+v", events)
	}
}

func TestAudit_AsyncJobCarriesActorAndCorrelation(t *testing.T) {
	h := newTestServer(t, "")

	rec := doJSONAs(t, h, "POST", "/api/apps/web/seed?async=1", `{"ring":"int","version":"v1"}`, "carol")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("async seed: status %d body %s", rec.Code, rec.Body)
	}
	// The detached job finishes quickly with the log deployer; poll the ledger.
	var events []store.AuditEvent
	for i := 0; i < 100; i++ {
		events = listAudit(t, h, "?category=operation")
		if len(events) > 0 {
			break
		}
	}
	if len(events) != 1 {
		t.Fatalf("want 1 operation event from async job, got %d", len(events))
	}
	if events[0].Actor != "carol" || events[0].CorrelationID == "" {
		t.Fatalf("async job lost attribution: %+v", events[0])
	}
}

func TestAudit_ConfigMutationsAreRecorded(t *testing.T) {
	h := newTestServer(t, "")

	if rec := doJSONAs(t, h, "PUT", "/api/apps/web/rings/int/auto-promote", `{"enabled":true}`, "dave"); rec.Code != http.StatusOK {
		t.Fatalf("auto-promote: status %d body %s", rec.Code, rec.Body)
	}
	if rec := doJSONAs(t, h, "POST", "/api/groups", `{"name":"Core","apps":["web"]}`, "dave"); rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create group: status %d body %s", rec.Code, rec.Body)
	}
	if rec := doJSONAs(t, h, "POST", "/api/apps/web/signoffs", `{"ring":"acc","version":"v1","decision":"go","engineer":"dave"}`, "dave"); rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("signoff: status %d body %s", rec.Code, rec.Body)
	}

	events := listAudit(t, h, "?category=config")
	actions := map[string]bool{}
	for _, e := range events {
		actions[e.Action] = true
		if e.Actor != "dave" {
			t.Fatalf("config event not attributed: %+v", e)
		}
	}
	for _, want := range []string{"auto_promote.set", "group.create", "signoff.record"} {
		if !actions[want] {
			t.Fatalf("missing config audit %q; have %v", want, actions)
		}
	}
}

func TestAudit_FilterAndPagingOverHTTP(t *testing.T) {
	h := newTestServer(t, "")
	for i := 0; i < 3; i++ {
		if rec := doJSON(t, h, "POST", "/api/apps/web/seed", `{"ring":"int","version":"v1"}`); rec.Code != http.StatusOK {
			t.Fatalf("seed %d: status %d body %s", i, rec.Code, rec.Body)
		}
	}

	page := listAudit(t, h, "?category=operation&limit=2")
	if len(page) != 2 {
		t.Fatalf("limit=2: got %d", len(page))
	}
	rest := listAudit(t, h, "?category=operation&limit=2&before_id="+jsonInt(page[1].ID))
	if len(rest) != 1 {
		t.Fatalf("second page: got %d", len(rest))
	}

	if rec := doJSON(t, h, "GET", "/api/audit?before_id=abc", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad before_id: expected 400, got %d", rec.Code)
	}
	if rec := doJSON(t, h, "GET", "/api/audit?limit=-1", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad limit: expected 400, got %d", rec.Code)
	}
}

func jsonInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}
