package changerequest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// httpDoer is the subset of *http.Client used here; it lets tests substitute a
// fake transport.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// JIRAParams configures a JIRA validator. The token is passed in directly (it
// is read from an environment variable / Secret by the caller, never the config
// file).
type JIRAParams struct {
	// BaseURL is the JIRA site base, e.g. "https://acme.atlassian.net".
	BaseURL string
	// Email + Token authenticate to JIRA Cloud (HTTP basic auth).
	Email string
	Token string
	// AllowedStatuses, when non-empty, requires the issue's status name to be
	// one of these (compared case-insensitively).
	AllowedStatuses []string
	// ProjectKeys, when non-empty, requires the code's project prefix to be one
	// of these (e.g. "CR"), checked before any network call.
	ProjectKeys []string
	// Client overrides the HTTP client (tests). Default: 10s timeout.
	Client httpDoer
	// Log receives warnings; defaults to slog.Default().
	Log *slog.Logger
}

// JIRA validates a change-request code by resolving it to a JIRA issue and
// (optionally) checking the issue's status and project.
type JIRA struct {
	http            httpDoer
	baseURL         string
	authHeader      string
	allowedStatuses []string
	projectKeys     []string
	log             *slog.Logger
}

// NewJIRA constructs a JIRA validator from params, filling defaults.
func NewJIRA(p JIRAParams) *JIRA {
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	log := p.Log
	if log == nil {
		log = slog.Default()
	}
	cred := base64.StdEncoding.EncodeToString([]byte(p.Email + ":" + p.Token))
	return &JIRA{
		http:            client,
		baseURL:         strings.TrimRight(p.BaseURL, "/"),
		authHeader:      "Basic " + cred,
		allowedStatuses: p.AllowedStatuses,
		projectKeys:     p.ProjectKeys,
		log:             log,
	}
}

// Validate implements Validator against JIRA Cloud's REST API.
func (j *JIRA) Validate(ctx context.Context, app, ring, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return ErrCodeRequired
	}
	if len(j.projectKeys) > 0 && !projectAllowed(code, j.projectKeys) {
		return fmt.Errorf("%w: %q is not in an allowed JIRA project (%s)",
			ErrInvalidCode, code, strings.Join(j.projectKeys, ", "))
	}

	endpoint := fmt.Sprintf("%s/rest/api/3/issue/%s?fields=status", j.baseURL, url.PathEscape(code))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", j.authHeader)

	resp, err := j.http.Do(req)
	if err != nil {
		return fmt.Errorf("reach JIRA: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		// fall through to status check below
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: JIRA issue %q does not exist", ErrInvalidCode, code)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("JIRA rejected the request (status %d): check RP_JIRA_TOKEN / email", resp.StatusCode)
	default:
		return fmt.Errorf("JIRA lookup of %q failed: %s", code, apiSnippet(resp))
	}

	var issue struct {
		Fields struct {
			Status struct {
				Name string `json:"name"`
			} `json:"status"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return fmt.Errorf("decode JIRA response for %q: %w", code, err)
	}

	if len(j.allowedStatuses) == 0 {
		return nil // any existing issue is acceptable
	}
	status := issue.Fields.Status.Name
	for _, allowed := range j.allowedStatuses {
		if strings.EqualFold(status, allowed) {
			return nil
		}
	}
	return fmt.Errorf("%w: JIRA issue %q is %q, not one of the approved statuses (%s)",
		ErrInvalidCode, code, status, strings.Join(j.allowedStatuses, ", "))
}

// projectAllowed reports whether code's project prefix (the part before the
// first "-", e.g. "CR" in "CR-123") is one of keys (case-insensitive).
func projectAllowed(code string, keys []string) bool {
	prefix := code
	if i := strings.IndexByte(code, '-'); i >= 0 {
		prefix = code[:i]
	}
	for _, k := range keys {
		if strings.EqualFold(prefix, k) {
			return true
		}
	}
	return false
}

// apiSnippet returns a short, trimmed snippet of a response body for errors.
func apiSnippet(resp *http.Response) string {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	msg := strings.TrimSpace(string(b))
	if msg == "" {
		return fmt.Sprintf("status %d", resp.StatusCode)
	}
	return fmt.Sprintf("status %d: %s", resp.StatusCode, msg)
}
