package promoter

import (
	"encoding/json"
	"testing"
)

const trivySARIF = `{
 "version": "2.1.0",
 "runs": [{
  "tool": {"driver": {"name": "Trivy", "rules": [
    {"id": "CVE-2024-0001", "shortDescription": {"text": "openssl: overflow"}, "properties": {"security-severity": "9.8"}},
    {"id": "CVE-2024-0002", "shortDescription": {"text": "x/net: DoS"}, "properties": {"security-severity": "7.5"}},
    {"id": "CVE-2024-0003", "properties": {"security-severity": "5.3"}},
    {"id": "CVE-2024-0004", "properties": {"security-severity": "2.1"}}
  ]}},
  "results": [
    {"ruleId": "CVE-2024-0001", "ruleIndex": 0, "level": "error", "message": {"text": "Package: openssl"},
     "locations": [{"physicalLocation": {"artifactLocation": {"uri": "Dockerfile"}, "region": {"startLine": 3}}}]},
    {"ruleId": "CVE-2024-0002", "level": "error", "message": {"text": "x"}},
    {"ruleId": "CVE-2024-0003", "message": {"text": "x"}},
    {"ruleId": "CVE-2024-0004", "message": {"text": "x"}, "properties": {"security-severity": 9.1}}
  ]
 }]
}`

const gitleaksSARIF = `{
 "version": "2.1.0",
 "runs": [{
  "tool": {"driver": {"name": "gitleaks", "rules": [{"id": "aws-access-token"}]}},
  "results": [
    {"ruleId": "aws-access-token", "message": {"text": "AWS key found\nmore"},
     "locations": [{"physicalLocation": {"artifactLocation": {"uri": "config/prod.env"}, "region": {"startLine": 12}}}]},
    {"ruleId": "generic-api-key", "level": "note", "message": {"text": "key"}}
  ]
 }]
}`

func TestParseSARIF_Trivy(t *testing.T) {
	f, tool, err := parseSARIF(json.RawMessage(trivySARIF), "container")
	if err != nil {
		t.Fatal(err)
	}
	if tool != "Trivy" || len(f) != 4 {
		t.Fatalf("tool=%q findings=%+v", tool, f)
	}
	want := []string{"critical", "high", "medium", "critical"} // result property beats rule's 2.1
	for i, w := range want {
		if f[i].Severity != w {
			t.Fatalf("finding %d severity = %q, want %q", i, f[i].Severity, w)
		}
	}
	if f[0].ID != "CVE-2024-0001" || f[0].Title != "openssl: overflow" || f[0].Location != "Dockerfile:3" {
		t.Fatalf("finding 0 = %+v", f[0])
	}
}

func TestParseSARIF_SecretsAreCritical(t *testing.T) {
	f, tool, err := parseSARIF(json.RawMessage(gitleaksSARIF), "secrets")
	if err != nil {
		t.Fatal(err)
	}
	if tool != "gitleaks" || len(f) != 2 {
		t.Fatalf("tool=%q findings=%+v", tool, f)
	}
	for _, x := range f {
		if x.Severity != "critical" {
			t.Fatalf("secret severity = %q", x.Severity)
		}
	}
	if f[0].Title != "AWS key found" || f[0].Location != "config/prod.env:12" {
		t.Fatalf("finding = %+v", f[0])
	}
	// Outside a secrets scan the SARIF level decides.
	f, _, _ = parseSARIF(json.RawMessage(gitleaksSARIF), "sast")
	if f[0].Severity != "medium" || f[1].Severity != "low" {
		t.Fatalf("levels = %q %q", f[0].Severity, f[1].Severity)
	}
}

func TestParseSARIF_Invalid(t *testing.T) {
	if _, _, err := parseSARIF(json.RawMessage(`{"runs": 3}`), "sast"); err == nil {
		t.Fatal("want error")
	}
}
