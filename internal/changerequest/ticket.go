package changerequest

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Ticket is one JIRA issue or OPSAPI service request, as far as a custom
// promotion gate needs it: status, optional version field, and how many
// approvers have already said yes.
type Ticket struct {
	Code      string
	Status    string
	Version   string
	Approvals int
	Fields    map[string]string
}

// InspectOpts tells an Inspector which extras to fetch. Approvals is a second
// round-trip on Jira Service Management; skip it unless min_approvals is set.
type InspectOpts struct {
	VersionField string
	Approvals    bool
}

// Inspector looks up a ticket without mutating it. Custom gates are read-only
// — they never create the JIRA issue or the service request.
type Inspector interface {
	Inspect(ctx context.Context, code string, opts InspectOpts) (Ticket, error)
}

func jsonText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case map[string]any:
		for _, k := range []string{"name", "value", "displayName", "version", "status"} {
			if s := jsonText(t[k]); s != "" {
				return s
			}
		}
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func jsonPath(m map[string]any, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || m == nil {
		return ""
	}
	cur := any(m)
	for _, p := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = obj[p]
	}
	return jsonText(cur)
}

func jsonInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	case []any:
		n := 0
		for _, item := range t {
			if obj, ok := item.(map[string]any); ok {
				dec := strings.ToLower(jsonText(obj["finalDecision"]))
				if dec == "" {
					dec = strings.ToLower(jsonText(obj["decision"]))
				}
				if dec == "approved" || dec == "approve" {
					n++
				}
			} else {
				n++
			}
		}
		return n
	}
	return 0
}
