package changerequest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OpsAPIParams configures an OPSAPI service-request inspector. The token is
// never read from YAML — the caller passes it from a Secret / env var.
type OpsAPIParams struct {
	BaseURL   string
	Path      string
	Token     string
	Namespace string
	Client    httpDoer
	Log       *slog.Logger
}

// OpsAPI looks up a service request by code. It never creates one.
type OpsAPI struct {
	http      httpDoer
	baseURL   string
	path      string
	token     string
	namespace string
	log       *slog.Logger
}

// NewOpsAPI constructs an inspector from params.
func NewOpsAPI(p OpsAPIParams) *OpsAPI {
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	log := p.Log
	if log == nil {
		log = slog.Default()
	}
	path := strings.TrimRight(p.Path, "/")
	if path == "" {
		path = "/api/v2/namespace/service-requests"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return &OpsAPI{
		http:      client,
		baseURL:   strings.TrimRight(p.BaseURL, "/"),
		path:      path,
		token:     strings.TrimSpace(p.Token),
		namespace: strings.TrimSpace(p.Namespace),
		log:       log,
	}
}

// Inspect implements Inspector against OPSAPI GET {path}/{code}.
func (o *OpsAPI) Inspect(ctx context.Context, code string, opts InspectOpts) (Ticket, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return Ticket{}, ErrCodeRequired
	}
	endpoint := o.baseURL + o.path + "/" + url.PathEscape(code)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Ticket{}, err
	}
	req.Header.Set("Accept", "application/json")
	if o.token != "" {
		req.Header.Set("Authorization", "Bearer "+o.token)
	}
	if o.namespace != "" {
		req.Header.Set("X-Namespace-Id", o.namespace)
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return Ticket{}, fmt.Errorf("reach OPSAPI: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusNotFound:
		return Ticket{}, fmt.Errorf("%w: OPSAPI service request %q does not exist", ErrInvalidCode, code)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return Ticket{}, fmt.Errorf("OPSAPI rejected the request (status %d): check %s", resp.StatusCode, "RP_OPSAPI_TOKEN")
	default:
		return Ticket{}, fmt.Errorf("OPSAPI lookup of %q failed: %s", code, apiSnippet(resp))
	}

	var raw map[string]any
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return Ticket{}, fmt.Errorf("decode OPSAPI response for %q: %w", code, err)
	}
	// Some APIs wrap the body in {data: {...}}.
	body := raw
	if inner, ok := raw["data"].(map[string]any); ok {
		body = inner
	}
	tkt := Ticket{Code: code, Fields: map[string]string{}}
	tkt.Status = firstNonEmpty(
		jsonText(body["status"]),
		jsonText(body["state"]),
		jsonText(body["approval_status"]),
	)
	vf := strings.TrimSpace(opts.VersionField)
	if vf == "" {
		vf = "version"
	}
	tkt.Version = firstNonEmpty(jsonPath(body, vf), jsonText(body["version"]), jsonPath(raw, vf))
	tkt.Fields[vf] = tkt.Version
	tkt.Approvals = jsonInt(body["approvals"])
	if tkt.Approvals == 0 {
		tkt.Approvals = jsonInt(body["approved_count"])
	}
	if tkt.Approvals == 0 {
		tkt.Approvals = jsonInt(body["approval_count"])
	}
	_ = opts
	return tkt, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
