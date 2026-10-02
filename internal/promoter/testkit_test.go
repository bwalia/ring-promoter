package promoter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/deployer"
	"github.com/example/ring-promoter/internal/progress"
	"github.com/example/ring-promoter/internal/store"
)

func TestExtractURLs_KeepsOutputsDropsNoiseAndSecrets(t *testing.T) {
	lines := []string{
		"Pulling https://registry-1.docker.io/v2/library/node/manifests/20",
		"npm http fetch GET 200 https://registry.npmjs.org/react 12ms",
		"Uploading to https://results-receiver.actions.githubusercontent.com/x",
		"signed: https://bucket.s3.amazonaws.com/a.zip?X-Amz-Signature=abc&X-Amz-Credential=k",
		"creds: https://user:pass@example.net/private",
		"internal: http://api.default.svc.cluster.local:8080/health",
		"Deployed: https://test.app.example.net/ (healthy).",
		"TestFlight build 42 ready: https://testflight.apple.com/join/AbCdEf",
		"again https://test.app.example.net/",
	}
	got := ExtractURLs(lines)
	want := []string{"https://test.app.example.net/", "https://testflight.apple.com/join/AbCdEf"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("ExtractURLs = %v, want %v", got, want)
	}
	if k := kindForURL("https://testflight.apple.com/join/AbCdEf"); k != "ios" {
		t.Errorf("testflight kind = %q, want ios", k)
	}
}

func TestLogExcerpt_RedactsSignedURLs(t *testing.T) {
	ex := logExcerpt([]string{"download https://x.blob.core.windows.net/f?sig=SECRET&se=1", "done"})
	if strings.Contains(ex, "SECRET") {
		t.Fatalf("excerpt leaked a signed URL: %q", ex)
	}
	if !strings.Contains(ex, "[redacted url]") {
		t.Fatalf("excerpt should mark the redaction: %q", ex)
	}
}

func TestParseTestPlan_OnlyCandidateLinks(t *testing.T) {
	cands := []store.TestLink{
		{Label: "Web", URL: "https://test.example.net/", Kind: "web", Source: "health"},
		{Label: "TestFlight", URL: "https://testflight.apple.com/join/x", Kind: "ios", Source: "log"},
	}
	raw := "```json\n" + `{"summary":"Shipped login fix.","checklist":["Sign in","Open the app"],
	  "links":[{"id":2,"why":"install build"},{"id":9,"why":"bogus id"},
	           {"url":"https://evil.example.com/","why":"injected"},
	           {"url":"https://test.example.net/","why":"try login"},{"id":1,"why":"dup"}]}` + "\n```"
	plan, err := ParseTestPlan(raw, cands, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(plan.Links) != 2 || plan.Links[0].URL != cands[1].URL || plan.Links[1].URL != cands[0].URL {
		t.Fatalf("links = %+v, want TestFlight then web only", plan.Links)
	}
	if plan.Links[0].Why != "install build" || len(plan.Checklist) != 2 {
		t.Fatalf("unexpected plan %+v", plan)
	}
	if _, err := ParseTestPlan("not json", cands, time.Now()); err == nil {
		t.Fatal("want an error for non-JSON output")
	}
	if _, err := ParseTestPlan(`{"links":[{"id":1}]}`, cands, time.Now()); err == nil {
		t.Fatal("want an error for an empty plan")
	}
}

func TestRingLinks_ConfigHealthAndDeploy(t *testing.T) {
	ac := config.AppConfig{Name: "shop", Links: []config.LinkConfig{
		{Label: "API docs", URL: "https://{target_env}.shop.example/docs", Kind: "docs"},
	}}
	rc := config.RingConfig{TargetEnv: "qa", HealthURL: "https://qa.shop.example/healthz",
		Links: []config.LinkConfig{{Label: "Release", URL: "https://github.com/o/r/releases/tag/{version}", Kind: "release"}}}
	got := ringLinks(ac, "test", rc, "feat/x", []store.TestLink{
		{Label: "Workflow run", URL: "https://github.com/o/r/actions/runs/1", Kind: "ci", Source: "run"},
		{Label: "dup", URL: "https://qa.shop.example/", Kind: "web", Source: "log"},
	})
	var urls []string
	for _, l := range got {
		urls = append(urls, l.Source+":"+l.URL)
	}
	want := []string{
		"config:https://qa.shop.example/docs",
		"config:https://github.com/o/r/releases/tag/feat%2Fx",
		"health:https://qa.shop.example/",
		"run:https://github.com/o/r/actions/runs/1",
	}
	if strings.Join(urls, " ") != strings.Join(want, " ") {
		t.Fatalf("links = %v\nwant    %v", urls, want)
	}

	// A configured web link replaces the health-host fallback.
	ac.Links = append(ac.Links, config.LinkConfig{Label: "UI", URL: "https://ui.example/"})
	for _, l := range ringLinks(ac, "test", rc, "v1", nil) {
		if l.Source == "health" {
			t.Fatal("health host shown although config names a web link")
		}
	}
}

func TestSeed_CapturesTestKitPerVersion(t *testing.T) {
	p, dep, _, st := newHarness(t, 0)
	dep.onDeploy = func(ctx context.Context, tg deployer.Target, version string) {
		progress.AddLink(ctx, progress.Link{Label: "Workflow run", URL: "https://github.com/o/r/actions/runs/" + version, Kind: "ci"})
		progress.FromContext(ctx).Log("live at https://" + tg.Ring + ".app.example.net/" + version)
		progress.AddOutput(ctx, "ipa uploaded\nhttps://testflight.apple.com/join/abc")
	}
	ctx := context.Background()
	for _, v := range []string{"v1", "v2"} {
		if res, err := p.Seed(ctx, testApp, "int", v); err != nil || !res.Success {
			t.Fatalf("seed %s: %+v %v", v, res, err)
		}
	}

	view, kit, err := p.TestKit(ctx, testApp, "int")
	if err != nil {
		t.Fatalf("test kit: %v", err)
	}
	if view.Version != "v2" || view.Captured == nil {
		t.Fatalf("view = %+v, want v2 captured", view)
	}
	var got []string
	for _, l := range view.Links {
		got = append(got, l.Kind+":"+l.URL)
	}
	want := []string{
		"ci:https://github.com/o/r/actions/runs/v2",
		"ios:https://testflight.apple.com/join/abc",
		"web:https://int.app.example.net/v2",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("links = %v\nwant    %v", got, want)
	}
	if !strings.Contains(kit.LogExcerpt, "ipa uploaded") {
		t.Fatalf("log excerpt missing unstreamed output: %q", kit.LogExcerpt)
	}

	// The previous version keeps its own kit, ready for a rollback.
	old, err := st.GetTestKit(ctx, testApp, "int", "v1")
	if err != nil || old.Links[0].URL != "https://github.com/o/r/actions/runs/v1" {
		t.Fatalf("v1 kit = %+v, %v", old, err)
	}

	// Ring views carry the same links.
	views, err := p.Rings(ctx, testApp)
	if err != nil {
		t.Fatal(err)
	}
	if len(views[0].Links) != 3 {
		t.Fatalf("ring view links = %+v", views[0].Links)
	}

	// Storing a plan, including for a version that predates test kits.
	if err := p.SetTestPlan(ctx, testApp, "int", "v2", store.TestPlan{Summary: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetTestPlan(ctx, testApp, "test", "v0", store.TestPlan{Summary: "s"}); err != nil {
		t.Fatal(err)
	}
	if view, _, _ := p.TestKit(ctx, testApp, "int"); view.Plan == nil || view.Plan.Summary != "s" {
		t.Fatalf("plan not stored: %+v", view.Plan)
	}
}

func TestSeed_FailedDeployStoresNoKit(t *testing.T) {
	p, dep, _, st := newHarness(t, 0)
	dep.failVer["bad"] = true
	if _, err := p.Seed(context.Background(), testApp, "int", "bad"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetTestKit(context.Background(), testApp, "int", "bad"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want no kit for a failed deploy, got %v", err)
	}
	if _, _, err := p.TestKit(context.Background(), testApp, "int"); !errors.Is(err, ErrNoVersion) {
		t.Fatalf("want ErrNoVersion, got %v", err)
	}
}

func TestRingLinks_SkipsClusterInternalHealthHosts(t *testing.T) {
	for _, h := range []string{
		"http://jobshout-api.int.svc.cluster.local:8080/health",
		"http://beacon-api.int.svc:8080/readyz",
		"http://localhost:8080/healthz",
		"http://10.43.0.12/healthz",
		"http://api/healthz",
		"http://box.internal/healthz",
	} {
		if got := ringLinks(config.AppConfig{Name: "a"}, "int", config.RingConfig{HealthURL: h}, "v1", nil); len(got) != 0 {
			t.Errorf("%s: want no Open link for an internal host, got %+v", h, got)
		}
	}
	got := ringLinks(config.AppConfig{Name: "a"}, "int", config.RingConfig{HealthURL: "https://int-opsapi.workstation.co.uk/ready"}, "v1", nil)
	if len(got) != 1 || got[0].URL != "https://int-opsapi.workstation.co.uk/" {
		t.Fatalf("public health host: got %+v", got)
	}
}
