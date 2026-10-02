package github

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/example/ring-promoter/internal/executor"
)

// Caps on what Outputs pulls from GitHub, so one chatty or artifact-heavy run
// cannot bloat a deploy's test kit.
const (
	maxLogArchiveBytes = 32 << 20 // compressed run-log archive downloaded
	maxLogTextBytes    = 1 << 20  // log text handed back (the tail is kept)
	maxArtifactLinks   = 10
	maxReleaseAssets   = 10
)

// Outputs implements executor.OutputReporter: links to the run's artifacts and
// to the GitHub release for the deployed version (with its downloadable
// assets, e.g. an .ipa or .apk), plus the run's log text. Each source is
// independent and best effort — a missing release or an expired log archive
// just contributes nothing.
func (x *execution) Outputs(ctx context.Context) (executor.Outputs, error) {
	var out executor.Outputs
	var errs []string

	if links, err := x.artifactLinks(ctx); err != nil {
		errs = append(errs, err.Error())
	} else {
		out.Links = append(out.Links, links...)
	}
	if links, err := x.releaseLinks(ctx); err != nil {
		errs = append(errs, err.Error())
	} else {
		out.Links = append(out.Links, links...)
	}
	if text, err := x.logText(ctx); err != nil {
		errs = append(errs, err.Error())
	} else {
		out.LogText = text
	}

	if len(out.Links) == 0 && out.LogText == "" && len(errs) > 0 {
		return out, fmt.Errorf("github outputs: %s", strings.Join(errs, "; "))
	}
	return out, nil
}

func (x *execution) repoEndpoint(format string, args ...any) string {
	return fmt.Sprintf("%s/repos/%s/%s/", x.e.cfg.APIBaseURL, x.e.cfg.Owner, x.e.cfg.Repo) +
		fmt.Sprintf(format, args...)
}

// artifactLinks lists the run's unexpired artifacts. Each links to the run's
// artifact page on github.com, where a signed-in operator can download it.
func (x *execution) artifactLinks(ctx context.Context) ([]executor.Link, error) {
	resp, err := x.e.do(ctx, http.MethodGet, x.repoEndpoint("actions/runs/%d/artifacts?per_page=%d", x.runID, maxArtifactLinks), nil)
	if err != nil {
		return nil, err
	}
	var body struct {
		Artifacts []struct {
			ID      int64  `json:"id"`
			Name    string `json:"name"`
			Expired bool   `json:"expired"`
		} `json:"artifacts"`
	}
	if err := decode(resp, &body); err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	var links []executor.Link
	for _, a := range body.Artifacts {
		if a.Expired || x.url == "" {
			continue
		}
		links = append(links, executor.Link{
			Label: "Artifact: " + a.Name,
			URL:   x.url + "/artifacts/" + strconv.FormatInt(a.ID, 10),
			Kind:  kindForName(a.Name, "artifact"),
		})
	}
	return links, nil
}

// releaseLinks finds the GitHub release whose tag is the deployed version. A
// branch or SHA has no release, which is normal and not an error.
func (x *execution) releaseLinks(ctx context.Context) ([]executor.Link, error) {
	if x.version == "" {
		return nil, nil
	}
	resp, err := x.e.do(ctx, http.MethodGet, x.repoEndpoint("releases/tags/%s", url.PathEscape(x.version)), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, nil
	}
	var rel struct {
		Name    string `json:"name"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := decode(resp, &rel); err != nil {
		return nil, fmt.Errorf("get release: %w", err)
	}
	var links []executor.Link
	if rel.HTMLURL != "" {
		name := rel.Name
		if name == "" {
			name = x.version
		}
		links = append(links, executor.Link{Label: "Release " + name, URL: rel.HTMLURL, Kind: "release"})
	}
	for i, a := range rel.Assets {
		if i >= maxReleaseAssets {
			break
		}
		links = append(links, executor.Link{Label: a.Name, URL: a.URL, Kind: kindForName(a.Name, "artifact")})
	}
	return links, nil
}

// logText downloads the run's log archive and returns its text, files in name
// order (GitHub numbers them by job and step), keeping the tail when long.
func (x *execution) logText(ctx context.Context) (string, error) {
	resp, err := x.e.do(ctx, http.MethodGet, x.repoEndpoint("actions/runs/%d/logs", x.runID), nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", apiError("download run logs", resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxLogArchiveBytes+1))
	if err != nil {
		return "", fmt.Errorf("download run logs: %w", err)
	}
	if len(data) > maxLogArchiveBytes {
		return "", fmt.Errorf("download run logs: archive larger than %d bytes", maxLogArchiveBytes)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("read run logs: %w", err)
	}
	files := make([]*zip.File, 0, len(zr.File))
	for _, f := range zr.File {
		// The archive holds each job's log at the top level and the same
		// lines again split per step in a sub-directory; read only the former.
		if !f.FileInfo().IsDir() && !strings.Contains(f.Name, "/") {
			files = append(files, f)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	var b strings.Builder
	for _, f := range files {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "==> %s <==\n", f.Name)
		_, _ = io.Copy(&b, io.LimitReader(rc, maxLogTextBytes))
		rc.Close()
		b.WriteByte('\n')
	}
	text := b.String()
	if len(text) > maxLogTextBytes {
		text = text[len(text)-maxLogTextBytes:]
	}
	return text, nil
}

// kindForName classifies a file or artifact by its name: mobile builds get
// their platform so the UI can offer them to the right tester. Whole name
// tokens are matched ("RingPromoter-ipa", "app.apk", "android-release").
func kindForName(name, fallback string) string {
	tokens := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	for _, t := range tokens {
		switch t {
		case "ipa", "ios", "testflight", "iphone", "ipad":
			return "ios"
		case "apk", "aab", "android":
			return "android"
		}
	}
	return fallback
}
