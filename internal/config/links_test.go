package config

import (
	"strings"
	"testing"
)

func TestLinksLoadAndRender(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	body := `
apps:
  - name: shop
    links:
      - { label: Web UI, url: "https://{ring}.shop.example/" }
      - { label: TestFlight, url: "https://testflight.apple.com/join/AbC", kind: ios }
    rings:
      int:
        health_url: "http://x/healthz"
        links:
          - { label: Release notes, url: "https://github.com/o/r/releases/tag/{version}", kind: release }
`
	cfg, err := Load(writeConfig(t, body))
	if err != nil {
		t.Fatalf("valid links should load: %v", err)
	}
	app := cfg.Apps[0]
	if got := app.Links[0].Render("shop", "int", "", "v1"); got != "https://int.shop.example/" {
		t.Fatalf("render = %q", got)
	}
	if got := app.Rings["int"].Links[0].Render("shop", "int", "", "feat/a b"); got != "https://github.com/o/r/releases/tag/feat%2Fa%20b" {
		t.Fatalf("version not path-escaped: %q", got)
	}
	if app.Links[0].KindOrDefault() != "web" || app.Links[1].KindOrDefault() != "ios" {
		t.Fatal("unexpected kinds")
	}
}

func TestLinksRejected(t *testing.T) {
	t.Setenv("RP_API_TOKEN", "tok")
	for name, link := range map[string]string{
		"no label":     `{ url: "https://x/" }`,
		"not absolute": `{ label: L, url: "/relative" }`,
		"bad scheme":   `{ label: L, url: "javascript:alert(1)" }`,
		"bad kind":     `{ label: L, url: "https://x/", kind: banana }`,
	} {
		body := "\napps:\n  - name: shop\n    links:\n      - " + link + "\n    rings:\n      int: { health_url: \"http://x/\" }\n"
		_, err := Load(writeConfig(t, body))
		if err == nil || !strings.Contains(err.Error(), "link") {
			t.Errorf("%s: want a link error, got %v", name, err)
		}
	}
}
