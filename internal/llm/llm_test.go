package llm

import (
	"strings"
	"testing"
)

type verdict struct {
	Status string `json:"status"`
	Score  int    `json:"score"`
}

func TestDecodeStrict(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    verdict
		wantErr string
	}{
		{name: "plain json", in: `{"status":"ok","score":3}`, want: verdict{"ok", 3}},
		{name: "surrounding whitespace", in: "\n  {\"status\":\"ok\"}  \n", want: verdict{Status: "ok"}},
		{name: "json fence", in: "```json\n{\"status\":\"ok\",\"score\":1}\n```", want: verdict{"ok", 1}},
		{name: "bare fence", in: "```\n{\"status\":\"ok\"}\n```", want: verdict{Status: "ok"}},
		{name: "unknown field", in: `{"status":"ok","extra":true}`, wantErr: "unknown field"},
		{name: "trailing value", in: `{"status":"ok"} {"status":"again"}`, wantErr: "trailing content"},
		{name: "not json", in: "Sure! Here is the answer.", wantErr: "decode model output"},
		{name: "wrong type", in: `{"score":"high"}`, wantErr: "decode model output"},
		{name: "empty", in: "   ", wantErr: "decode model output"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got verdict
			err := DecodeStrict(tc.in, &got)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestStripFence(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                     `{"a":1}`,
		"  {\"a\":1}\n":               `{"a":1}`,
		"```json\n{\"a\":1}\n```":     `{"a":1}`,
		"```\n{\"a\":1}\n```  \n":     `{"a":1}`,
		"```json\n{\"a\":1}\n":        `{"a":1}`, // unterminated fence
		"```json\n\n  {\"a\":1}\n```": `{"a":1}`,
		"```JSON\n{\"a\":1}\n```":     `{"a":1}`,
		"```jsonc\n{\"a\":1}\n```":    `{"a":1}`,
		"```json{\"a\":1}```":         `{"a":1}`, // tag on the fence line
		"```{\"a\":1}```":             `{"a":1}`,
	}
	for in, want := range cases {
		if got := string(stripFence([]byte(in))); got != want {
			t.Errorf("stripFence(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDecodeIgnoresUnknownFields(t *testing.T) {
	var got verdict
	if err := Decode("```json\n{\"status\":\"ok\",\"score\":2,\"priority\":\"high\"}\n```", &got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != (verdict{"ok", 2}) {
		t.Fatalf("got %+v", got)
	}
	// Leniency is about extra fields only: malformed or chatty answers still fail.
	for _, in := range []string{`{"status":"ok"} and that's it`, "Sure!", `{"score":"high"}`} {
		if err := Decode(in, &got); err == nil {
			t.Errorf("Decode(%q) = nil, want error", in)
		}
	}
}
