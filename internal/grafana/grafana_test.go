package grafana

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// capture records what the Grafana stub was asked for. It is filled in while
// the request is being served, so it must only be read after Query returns.
type capture struct {
	header http.Header
	path   string
	body   []byte
}

// serve stands up a Grafana stub returning a canned /api/ds/query body, and
// hands back the capture so the caller can assert on the request.
func serve(t *testing.T, status int, body string) (*Client, *capture) {
	t.Helper()
	got := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.header = r.Header.Clone()
		got.path = r.URL.Path
		got.body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "svc-token", 5*time.Second), got
}

// The shape of a real /api/ds/query response for the diytaxreturn "Go/No-Go"
// stat panel: one frame, one column, one row holding the value 2 (GO).
const goNoGoFrame = `{
  "results": {
    "A": {
      "frames": [{
        "schema": {"fields": [{"name": "Go/No-Go", "type": "number"}]},
        "data": {"values": [[2]]}
      }]
    }
  }
}`

func TestQuery_ReadsTheValue(t *testing.T) {
	c, got := serve(t, http.StatusOK, goNoGoFrame)
	value, err := c.Query(context.Background(), Query{
		DatasourceUID:  "QA-PostgreSQL",
		DatasourceType: "grafana-postgresql-datasource",
		Expr:           "SELECT 2",
		Lookback:       24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if value.Value != 2 {
		t.Fatalf("want 2, got %v", value.Value)
	}
	if got.path != "/api/ds/query" {
		t.Fatalf("want the ds/query endpoint, got %q", got.path)
	}
}

// A SQL datasource is sent rawSql (with a table format); Prometheus is sent
// expr. Getting this backwards makes every query fail against a real Grafana.
func TestQuery_SendsTheRightFieldPerDatasource(t *testing.T) {
	for _, tc := range []struct {
		dsType    string
		wantField string
		deadField string
	}{
		{"grafana-postgresql-datasource", "rawSql", "expr"},
		{"prometheus", "expr", "rawSql"},
	} {
		t.Run(tc.dsType, func(t *testing.T) {
			c, got := serve(t, http.StatusOK, goNoGoFrame)
			if _, err := c.Query(context.Background(), Query{
				DatasourceUID:  "ds",
				DatasourceType: tc.dsType,
				Expr:           "THE QUERY",
			}); err != nil {
				t.Fatal(err)
			}
			var sent struct {
				From, To string
				Queries  []map[string]any
			}
			if err := json.Unmarshal(got.body, &sent); err != nil {
				t.Fatal(err)
			}
			q := sent.Queries[0]
			if q[tc.wantField] != "THE QUERY" {
				t.Fatalf("%s should carry the query, got %v", tc.wantField, q)
			}
			if _, dead := q[tc.deadField]; dead {
				t.Fatalf("%s should not be sent for %s", tc.deadField, tc.dsType)
			}
		})
	}
}

func TestQuery_AuthorizationHeader(t *testing.T) {
	c, got := serve(t, http.StatusOK, goNoGoFrame)
	if _, err := c.Query(context.Background(), Query{DatasourceUID: "ds", Expr: "x"}); err != nil {
		t.Fatal(err)
	}
	if h := got.header.Get("Authorization"); h != "Bearer svc-token" {
		t.Fatalf("want a bearer token, got %q", h)
	}
}

// Frames whose first column is the time axis must not be read as the value.
func TestQuery_SkipsTheTimeColumn(t *testing.T) {
	c, _ := serve(t, http.StatusOK, `{
      "results": {"A": {"frames": [{
        "schema": {"fields": [{"name":"Time","type":"time"},{"name":"Value","type":"number"}]},
        "data": {"values": [[1750000000000, 1750000060000], [41, 42]]}
      }]}}
    }`)
	got, err := c.Query(context.Background(), Query{DatasourceUID: "ds", Expr: "x"})
	if err != nil {
		t.Fatal(err)
	}
	// The last non-null value, matching a stat panel's lastNotNull reducer.
	if got.Value != 42 {
		t.Fatalf("want 42, got %v", got.Value)
	}
	// The time column dates the run, which is what the staleness rule needs.
	if want := time.UnixMilli(1750000060000); !got.At.Equal(want) {
		t.Fatalf("want run time %v, got %v", want, got.At)
	}
}

func TestQuery_Failures(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"unauthorized":  {http.StatusUnauthorized, `{"message":"invalid API key"}`, "401"},
		"query error":   {http.StatusOK, `{"results":{"A":{"error":"relation does not exist"}}}`, "relation does not exist"},
		"empty frame":   {http.StatusOK, `{"results":{"A":{"frames":[]}}}`, "no data"},
		"null only":     {http.StatusOK, `{"results":{"A":{"frames":[{"schema":{"fields":[{"name":"v","type":"number"}]},"data":{"values":[[null]]}}]}}}`, "no data"},
		"no result set": {http.StatusOK, `{"results":{}}`, "no result"},
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := serve(t, tc.status, tc.body)
			_, err := c.Query(context.Background(), Query{DatasourceUID: "ds", Expr: "x"})
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q should mention %q", err, tc.want)
			}
		})
	}
}

func TestJudge(t *testing.T) {
	// The diytaxreturn Go/No-Go panel's three states.
	for _, tc := range []struct {
		value float64
		want  Verdict
	}{
		{2, VerdictGo},    // success
		{1, VerdictCheck}, // pending
		{0, VerdictNoGo},  // failure
	} {
		if got := Judge(tc.value, 2, 0); got != tc.want {
			t.Fatalf("Judge(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
	// A percentage-style gate: go at 95, no-go at or below 80.
	if got := Judge(98.4, 95, 80); got != VerdictGo {
		t.Fatalf("98.4%% should be a go, got %q", got)
	}
	if got := Judge(90, 95, 80); got != VerdictCheck {
		t.Fatalf("90%% should need a look, got %q", got)
	}
	if got := Judge(80, 95, 80); got != VerdictNoGo {
		t.Fatalf("80%% should be a no-go, got %q", got)
	}
}

// Only an explicit no-go blocks; unknown and check are advisory.
func TestBlocks(t *testing.T) {
	for v, want := range map[Verdict]bool{
		VerdictNoGo:    true,
		VerdictGo:      false,
		VerdictCheck:   false,
		VerdictUnknown: false,
	} {
		if got := v.Blocks(); got != want {
			t.Fatalf("%q.Blocks() = %v, want %v", v, got, want)
		}
	}
}
