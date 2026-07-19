package changerequest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) Do(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResp(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewBufferString(body))}
}

func TestTestValidator_RejectsEverything(t *testing.T) {
	// The demo "test" code never reaches a Validator (the promoter handles it),
	// so the Test validator rejects any real code.
	if err := (Test{}).Validate(context.Background(), "web", "prod", "CR-1"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("want ErrInvalidCode, got %v", err)
	}
}

func TestJIRA_ValidIssueAnyStatus(t *testing.T) {
	j := NewJIRA(JIRAParams{
		BaseURL: "https://acme.atlassian.net", Email: "e@x.com", Token: "tok",
		Client: roundTrip(func(r *http.Request) (*http.Response, error) {
			if got := r.Header.Get("Authorization"); got == "" {
				t.Error("missing basic auth header")
			}
			return jsonResp(200, `{"fields":{"status":{"name":"In Progress"}}}`), nil
		}),
	})
	if err := j.Validate(context.Background(), "web", "prod", "CR-42"); err != nil {
		t.Fatalf("valid issue should pass: %v", err)
	}
}

func TestJIRA_UnknownIssueRejected(t *testing.T) {
	j := NewJIRA(JIRAParams{
		BaseURL: "https://acme.atlassian.net", Email: "e@x.com", Token: "tok",
		Client: roundTrip(func(r *http.Request) (*http.Response, error) {
			return jsonResp(404, `{"errorMessages":["Issue does not exist"]}`), nil
		}),
	})
	if err := j.Validate(context.Background(), "web", "prod", "CR-999"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("unknown issue should be ErrInvalidCode, got %v", err)
	}
}

func TestJIRA_StatusMustBeAllowed(t *testing.T) {
	mk := func(status string) *JIRA {
		return NewJIRA(JIRAParams{
			BaseURL: "https://acme.atlassian.net", Email: "e@x.com", Token: "tok",
			AllowedStatuses: []string{"Approved", "Ready for Release"},
			Client: roundTrip(func(r *http.Request) (*http.Response, error) {
				return jsonResp(200, `{"fields":{"status":{"name":"`+status+`"}}}`), nil
			}),
		})
	}
	if err := mk("approved").Validate(context.Background(), "web", "prod", "CR-1"); err != nil {
		t.Fatalf("approved (case-insensitive) should pass: %v", err)
	}
	if err := mk("In Progress").Validate(context.Background(), "web", "prod", "CR-1"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("disallowed status should be ErrInvalidCode, got %v", err)
	}
}

func TestJIRA_ProjectKeyGuard(t *testing.T) {
	called := false
	j := NewJIRA(JIRAParams{
		BaseURL: "https://acme.atlassian.net", Email: "e@x.com", Token: "tok",
		ProjectKeys: []string{"CR", "OPS"},
		Client: roundTrip(func(r *http.Request) (*http.Response, error) {
			called = true
			return jsonResp(200, `{"fields":{"status":{"name":"Approved"}}}`), nil
		}),
	})
	// Wrong project prefix is rejected without ever calling JIRA.
	if err := j.Validate(context.Background(), "web", "prod", "BUG-5"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("wrong project should be ErrInvalidCode, got %v", err)
	}
	if called {
		t.Fatal("JIRA should not be called for a disallowed project prefix")
	}
}

func TestJIRA_EmptyCodeRequired(t *testing.T) {
	j := NewJIRA(JIRAParams{BaseURL: "https://x", Email: "e", Token: "t"})
	if err := j.Validate(context.Background(), "web", "prod", "  "); !errors.Is(err, ErrCodeRequired) {
		t.Fatalf("blank code should be ErrCodeRequired, got %v", err)
	}
}

func TestJIRA_AuthErrorSurfaced(t *testing.T) {
	j := NewJIRA(JIRAParams{
		BaseURL: "https://acme.atlassian.net", Email: "e@x.com", Token: "bad",
		Client: roundTrip(func(r *http.Request) (*http.Response, error) {
			return jsonResp(401, `unauthorized`), nil
		}),
	})
	err := j.Validate(context.Background(), "web", "prod", "CR-1")
	if err == nil || errors.Is(err, ErrInvalidCode) {
		t.Fatalf("401 should be a reachability/auth error, not ErrInvalidCode: %v", err)
	}
}
