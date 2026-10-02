package github

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOutputs_ArtifactsReleaseAndLogs(t *testing.T) {
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	for name, body := range map[string]string{
		"0_deploy.txt":        "2026-10-02T10:00:00Z deployed to https://test.app.example/\n",
		"1_ios.txt":           "2026-10-02T10:01:00Z TestFlight: https://testflight.apple.com/join/abc\n",
		"deploy/1_Step 1.txt": "duplicate per-step copy\n",
	} {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}
	_ = zw.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/repos/o/r/actions/runs/7/artifacts":
			_, _ = w.Write([]byte(`{"artifacts":[{"id":11,"name":"RingPromoter-ipa","expired":false},{"id":12,"name":"old","expired":true}]}`))
		case "/repos/o/r/releases/tags/v1.2.0":
			_, _ = w.Write([]byte(`{"name":"v1.2.0","html_url":"https://github.com/o/r/releases/tag/v1.2.0","assets":[{"name":"app.apk","browser_download_url":"https://github.com/o/r/releases/download/v1.2.0/app.apk"}]}`))
		case "/repos/o/r/actions/runs/7/logs":
			_, _ = w.Write(zbuf.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	e := New(nil, Config{Owner: "o", Repo: "r", Token: "tok", APIBaseURL: srv.URL}, srv.Client())
	x := &execution{e: e, runID: 7, url: "https://github.com/o/r/actions/runs/7", version: "v1.2.0"}
	out, err := x.Outputs(context.Background())
	if err != nil {
		t.Fatalf("outputs: %v", err)
	}
	var got []string
	for _, l := range out.Links {
		got = append(got, l.Kind+" "+l.URL)
	}
	want := []string{
		"ios https://github.com/o/r/actions/runs/7/artifacts/11",
		"release https://github.com/o/r/releases/tag/v1.2.0",
		"android https://github.com/o/r/releases/download/v1.2.0/app.apk",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("links:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(out.LogText, "testflight.apple.com/join/abc") || strings.Contains(out.LogText, "duplicate per-step") {
		t.Fatalf("log text = %q", out.LogText)
	}
	if strings.Index(out.LogText, "0_deploy") > strings.Index(out.LogText, "1_ios") {
		t.Fatal("log files should be in name order")
	}

	// A branch has no release: not an error.
	x.version = "main"
	if out, err := x.Outputs(context.Background()); err != nil || len(out.Links) != 1 {
		t.Fatalf("branch outputs = %+v, %v", out.Links, err)
	}
}
