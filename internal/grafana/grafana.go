// Package grafana reads a go/no-go verdict out of a Grafana dashboard.
//
// The teams that run these pipelines already answer "is this release good?" on
// a dashboard — diytaxreturn's "Build & Release Status" has a literal Go/No-Go
// panel. Rather than reimplement that judgement, Ring Promoter runs the panel's
// own query through Grafana's /api/ds/query endpoint and turns the single
// number it returns into a verdict, so the gate and the dashboard can never
// disagree.
package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Verdict is the gate's answer for one ring.
type Verdict string

const (
	// VerdictGo: the query cleared the go threshold — promotion may proceed.
	VerdictGo Verdict = "go"
	// VerdictCheck: between the thresholds. Worth a look, but not blocking —
	// the dashboard's own "PENDING" state maps here.
	VerdictCheck Verdict = "check"
	// VerdictNoGo: at or below the no-go threshold. Blocks the promotion.
	VerdictNoGo Verdict = "no_go"
	// VerdictUnknown: the query could not be answered (Grafana unreachable, no
	// data, bad credentials). Never blocks — an observability outage must not
	// stop a release — but the UI shows it so nobody mistakes it for a GO.
	VerdictUnknown Verdict = "unknown"
)

// Blocks reports whether the verdict should stop a promotion. Only an explicit
// NO-GO does; "check" and "unknown" are advisory.
func (v Verdict) Blocks() bool { return v == VerdictNoGo }

// Result is one evaluation of a ring's gate: the rolled-up verdict plus the
// individual checks behind it, so the UI can show WHY the gate says what it
// says (e.g. which of the four E2E suites is red).
type Result struct {
	Verdict Verdict `json:"verdict"`
	// Checks are the individual dashboard queries, in config order.
	Checks []CheckResult `json:"checks,omitempty"`
	// Dashboard is the human name of the dashboard behind the verdict.
	Dashboard string `json:"dashboard,omitempty"`
	// DashboardURL deep-links to that dashboard for the ring in question.
	DashboardURL string `json:"dashboard_url,omitempty"`
	// Error explains a gate-level failure in one line, for the UI.
	Error string `json:"error,omitempty"`
	// CheckedAt is when the queries ran.
	CheckedAt time.Time `json:"checked_at"`
	// Demo reports that no Grafana was contacted — the verdict is configured.
	Demo bool `json:"demo,omitempty"`
}

// CheckResult is one dashboard query's contribution to the gate.
type CheckResult struct {
	// Name identifies the check in the UI, e.g. "Register & Login".
	Name    string  `json:"name"`
	Verdict Verdict `json:"verdict"`
	// Value is the number the query returned (absent when Verdict is unknown).
	Value *float64 `json:"value,omitempty"`
	// Unit suffixes the value in the UI (e.g. "%").
	Unit string `json:"unit,omitempty"`
	// GoMin and NoGoMax are the thresholds the value was judged against, so the
	// UI can show "2 (≥ 2)" without re-reading the config.
	GoMin   float64 `json:"go_min"`
	NoGoMax float64 `json:"no_go_max"`
	// RanAt is when the underlying test run happened (not when we queried), when
	// the query reports it. This is what the staleness rule judges.
	RanAt *time.Time `json:"ran_at,omitempty"`
	// Stale reports that RanAt is older than the gate's max_age, which is why a
	// nightly suite that silently stopped running does not keep showing its last
	// GO forever.
	Stale bool `json:"stale,omitempty"`
	// Error explains a VerdictUnknown in one line, for the UI.
	Error string `json:"error,omitempty"`
}

// Rollup reduces the individual checks to the gate's verdict. A single no-go
// blocks; otherwise anything short of a clean pass is advisory:
//
//   - any no-go            -> no-go   (something is definitely broken)
//   - every check unknown  -> unknown (Grafana is not answering at all)
//   - any check / unknown  -> check   (partial or ambiguous picture)
//   - otherwise            -> go
func Rollup(checks []CheckResult) Verdict {
	if len(checks) == 0 {
		return VerdictUnknown
	}
	unknown, degraded := 0, 0
	for _, c := range checks {
		switch c.Verdict {
		case VerdictNoGo:
			return VerdictNoGo
		case VerdictUnknown:
			unknown++
		case VerdictCheck:
			degraded++
		}
	}
	switch {
	case unknown == len(checks):
		return VerdictUnknown
	case unknown > 0 || degraded > 0:
		return VerdictCheck
	default:
		return VerdictGo
	}
}

// Query describes one evaluation: which datasource, what to run, over what
// window. Dashboard variables must already be substituted.
type Query struct {
	DatasourceUID  string
	DatasourceType string
	// Expr is raw SQL for SQL datasources, or PromQL for Prometheus.
	Expr     string
	Lookback time.Duration
}

// Client queries a Grafana instance. The zero value is not usable; use New.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New builds a client for a Grafana base URL and service-account token.
func New(baseURL, token string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		baseURL: strings.TrimSuffix(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		http:    &http.Client{Timeout: timeout},
	}
}

// dsQueryRequest is the /api/ds/query body. Grafana expands its own macros
// ($__timeFrom, $__timeTo) server-side from the "from"/"to" range, so a
// dashboard panel's SQL can be sent verbatim.
type dsQueryRequest struct {
	From    string          `json:"from"`
	To      string          `json:"to"`
	Queries []dsQueryTarget `json:"queries"`
}

type dsQueryTarget struct {
	RefID         string       `json:"refId"`
	Datasource    dsRef        `json:"datasource"`
	RawSQL        string       `json:"rawSql,omitempty"`
	Expr          string       `json:"expr,omitempty"`
	Format        string       `json:"format,omitempty"`
	IntervalMs    int          `json:"intervalMs"`
	MaxDataPoints int          `json:"maxDataPoints"`
	TimeRange     *dsTimeRange `json:"timeRange,omitempty"`
}

type dsRef struct {
	UID  string `json:"uid"`
	Type string `json:"type"`
}

type dsTimeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// dsQueryResponse is the slice of the response we need: the first frame's first
// numeric column. Grafana returns arrow-ish JSON frames; we only reduce them to
// one number, exactly as the panel's "lastNotNull" reducer does.
type dsQueryResponse struct {
	Results map[string]struct {
		Error  string `json:"error"`
		Frames []struct {
			Schema struct {
				Fields []struct {
					Name string `json:"name"`
					Type string `json:"type"`
				} `json:"fields"`
			} `json:"schema"`
			Data struct {
				Values [][]json.RawMessage `json:"values"`
			} `json:"data"`
		} `json:"frames"`
	} `json:"results"`
}

// isPrometheus reports whether the datasource speaks PromQL rather than SQL.
func isPrometheus(dsType string) bool {
	return strings.Contains(strings.ToLower(dsType), "prometheus")
}

// Sample is one row read off a dashboard query: the number the panel would
// show, and — when the query selects it — when the underlying run happened.
type Sample struct {
	Value float64
	// At is the time column's value, zero when the query selects none. The
	// staleness rule needs it: "the last E2E run said GO" means nothing without
	// "and it ran four hours ago".
	At time.Time
}

// Query runs one query and reduces it to a single sample. It returns an error
// only when no number could be obtained; the caller maps that to
// VerdictUnknown.
func (c *Client) Query(ctx context.Context, q Query) (Sample, error) {
	var zero Sample
	if c.baseURL == "" {
		return zero, fmt.Errorf("no grafana url configured")
	}
	lookback := q.Lookback
	if lookback <= 0 {
		lookback = 24 * time.Hour
	}
	now := time.Now()
	from := fmt.Sprintf("%d", now.Add(-lookback).UnixMilli())
	to := fmt.Sprintf("%d", now.UnixMilli())

	target := dsQueryTarget{
		RefID:         "A",
		Datasource:    dsRef{UID: q.DatasourceUID, Type: q.DatasourceType},
		IntervalMs:    60_000,
		MaxDataPoints: 1,
		TimeRange:     &dsTimeRange{From: from, To: to},
	}
	if isPrometheus(q.DatasourceType) {
		target.Expr = q.Expr
	} else {
		target.RawSQL = q.Expr
		target.Format = "table"
	}

	body, err := json.Marshal(dsQueryRequest{From: from, To: to, Queries: []dsQueryTarget{target}})
	if err != nil {
		return zero, fmt.Errorf("encode query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/ds/query", bytes.NewReader(body))
	if err != nil {
		return zero, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return zero, fmt.Errorf("call grafana: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return zero, fmt.Errorf("read grafana response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return zero, fmt.Errorf("grafana returned %s: %s", resp.Status, firstLine(raw))
	}

	var parsed dsQueryResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return zero, fmt.Errorf("decode grafana response: %w", err)
	}
	res, ok := parsed.Results["A"]
	if !ok {
		return zero, fmt.Errorf("grafana returned no result for the query")
	}
	if res.Error != "" {
		return zero, fmt.Errorf("grafana query failed: %s", res.Error)
	}
	// The value is the first NON-time column (like the panel's own reducer); the
	// time column, when the query selects one, dates the underlying run and
	// feeds the staleness rule. Both are read from the same frame so they always
	// describe the same row.
	for _, frame := range res.Frames {
		var (
			sample Sample
			found  bool
		)
		for col, field := range frame.Schema.Fields {
			if col >= len(frame.Data.Values) {
				break
			}
			isTime := strings.EqualFold(field.Type, "time")
			v, ok := lastNumber(frame.Data.Values[col])
			if !ok {
				continue
			}
			switch {
			case isTime && sample.At.IsZero():
				// Grafana renders time fields as epoch milliseconds.
				sample.At = time.UnixMilli(int64(v))
			case !isTime && !found:
				sample.Value, found = v, true
			}
		}
		if found {
			return sample, nil
		}
	}
	return zero, fmt.Errorf("grafana returned no data for the query")
}

// lastNumber returns the last non-null numeric value in a frame column, which
// is what Grafana's "lastNotNull" reducer displays on a stat panel.
func lastNumber(col []json.RawMessage) (float64, bool) {
	for i := len(col) - 1; i >= 0; i-- {
		var f *float64
		if err := json.Unmarshal(col[i], &f); err != nil || f == nil {
			continue
		}
		return *f, true
	}
	return 0, false
}

// firstLine trims a response body to something loggable.
func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// Judge turns a queried number into a verdict using the gate's thresholds.
// Values at or above goMin are GO, at or below noGoMax are NO-GO, and anything
// in between is "check".
func Judge(value, goMin, noGoMax float64) Verdict {
	switch {
	case value >= goMin:
		return VerdictGo
	case value <= noGoMax:
		return VerdictNoGo
	default:
		return VerdictCheck
	}
}
