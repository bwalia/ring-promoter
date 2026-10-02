package promoter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/example/ring-promoter/internal/config"
	"github.com/example/ring-promoter/internal/progress"
	"github.com/example/ring-promoter/internal/store"
)

// A test kit answers "this version just landed — where do I go to try it?".
// It is assembled from three sources, in this order:
//
//   - config: the app's and ring's `links`, rendered for the version;
//   - health: the ring's health URL host, when config names no web link, so
//     every ring has at least "open the app" without any config;
//   - the deploy itself: the workflow run, its artifacts and release, and the
//     URLs printed in its logs — captured at deploy time and stored per
//     (app, ring, version), so a rollback brings back the kit of the version
//     it restores.
//
// The AI test plan is built on top: the model only ever picks among these
// candidate links (anything else is dropped), so a log line cannot steer a
// tester to a URL the deploy did not actually produce.

// Caps on what a deploy contributes to its kit.
const (
	maxLogLinks       = 25
	maxCollectedLines = 5000
	excerptURLLines   = 60
	excerptTailLines  = 80
	maxExcerptBytes   = 12_000
	maxPlanChecklist  = 10
	maxPlanLinks      = 8
	maxPlanTextRunes  = 600
	maxPlanItemRunes  = 200
	maxLinkLabelRunes = 80
)

// kitCollector wraps the operation's reporter for the duration of one deploy:
// every call passes through, and it additionally keeps the deploy's log lines
// and the links its execution backend reported (progress.OutputSink).
type kitCollector struct {
	Reporter
	mu     sync.Mutex
	lines  []string
	output []string
	links  []progress.Link
}

func newKitCollector(inner Reporter) *kitCollector { return &kitCollector{Reporter: inner} }

func (c *kitCollector) Log(line string) {
	c.Reporter.Log(line)
	c.mu.Lock()
	if len(c.lines) < maxCollectedLines {
		c.lines = append(c.lines, line)
	} else {
		// Keep the most recent lines: the tail is where deploys report URLs.
		copy(c.lines, c.lines[1:])
		c.lines[len(c.lines)-1] = line
	}
	c.mu.Unlock()
}

// AddLink implements progress.OutputSink.
func (c *kitCollector) AddLink(l progress.Link) {
	c.mu.Lock()
	c.links = append(c.links, l)
	c.mu.Unlock()
}

// AddOutput implements progress.OutputSink.
func (c *kitCollector) AddOutput(text string) {
	c.mu.Lock()
	c.output = append(c.output, text)
	c.mu.Unlock()
}

// logLines returns every collected line: streamed ones, then unstreamed
// output (e.g. a GitHub run's log archive).
func (c *kitCollector) logLines() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := append([]string(nil), c.lines...)
	for _, text := range c.output {
		out = append(out, strings.Split(text, "\n")...)
	}
	return out
}

// deployCollecting runs a deploy with a kitCollector on the context.
func (p *Promoter) deployCollecting(ctx context.Context, deploy func(ctx context.Context) error) (*kitCollector, error) {
	c := newKitCollector(reporterFrom(ctx))
	err := deploy(progress.WithReporter(ctx, c))
	return c, err
}

// saveTestKit stores what a healthy deploy of version into ringName produced.
// Best effort: a store error is logged, never fails the operation.
func (p *Promoter) saveTestKit(ctx context.Context, app, ringName, version string, c *kitCollector) {
	if c == nil {
		return
	}
	lines := c.logLines()
	var links []store.TestLink
	seen := map[string]bool{}
	add := func(l store.TestLink) {
		if l.URL == "" || seen[l.URL] {
			return
		}
		seen[l.URL] = true
		links = append(links, l)
	}
	c.mu.Lock()
	reported := append([]progress.Link(nil), c.links...)
	c.mu.Unlock()
	for _, l := range reported {
		if safeLinkURL(l.URL) {
			add(store.TestLink{Label: clip(l.Label, maxLinkLabelRunes), URL: l.URL, Kind: l.Kind, Source: "run"})
		}
	}
	n := 0
	for _, u := range ExtractURLs(lines) {
		if n >= maxLogLinks {
			break
		}
		if !seen[u] {
			add(store.TestLink{Label: labelForURL(u), URL: u, Kind: kindForURL(u), Source: "log"})
			n++
		}
	}
	kit := store.TestKit{App: app, Ring: ringName, Version: version, Links: links, LogExcerpt: logExcerpt(lines)}
	if err := p.store.SaveTestKit(ctx, kit); err != nil {
		p.log.Error("save test kit failed", "err", err, "app", app, "ring", ringName, "version", version)
	}
}

// TestKitView is a ring's test kit for its current version, as the API shows
// it: every link (config, health host, deploy) and the stored AI plan.
type TestKitView struct {
	App     string           `json:"app"`
	Ring    string           `json:"ring"`
	Version string           `json:"version"`
	Links   []store.TestLink `json:"links"`
	Plan    *store.TestPlan  `json:"plan,omitempty"`
	// Captured is when the deploy's links were captured; nil when the version
	// predates test kits (only config and health links are then known).
	Captured *time.Time `json:"captured_at,omitempty"`
}

// ErrNoVersion is returned when a ring has no deployed version to test.
var ErrNoVersion = errors.New("no version deployed in this ring")

// TestKit returns the test kit of a ring's current version.
func (p *Promoter) TestKit(ctx context.Context, app, ringName string) (TestKitView, store.TestKit, error) {
	ac, ok := p.cfg.App(app)
	if !ok {
		return TestKitView{}, store.TestKit{}, ErrAppNotFound
	}
	rc, err := p.ringConfig(app, ringName)
	if err != nil {
		return TestKitView{}, store.TestKit{}, err
	}
	version := p.currentVersion(ctx, app, ringName)
	if version == "" {
		return TestKitView{}, store.TestKit{}, ErrNoVersion
	}
	view := TestKitView{App: app, Ring: ringName, Version: version}
	kit, err := p.store.GetTestKit(ctx, app, ringName, version)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return TestKitView{}, store.TestKit{}, err
	}
	view.Links = ringLinks(ac, ringName, rc, version, kit.Links)
	if err == nil {
		view.Plan = kit.Plan
		t := kit.CreatedAt
		view.Captured = &t
	}
	return view, kit, nil
}

// ringLinks merges a ring's links: config (app-level, then ring-level), the
// health host when config names no web link, then what the deploy produced.
func ringLinks(ac config.AppConfig, ringName string, rc config.RingConfig, version string, deployed []store.TestLink) []store.TestLink {
	var out []store.TestLink
	seen := map[string]bool{}
	add := func(l store.TestLink) {
		if l.URL != "" && !seen[l.URL] {
			seen[l.URL] = true
			out = append(out, l)
		}
	}
	hasWeb := false
	for _, l := range slices.Concat(ac.Links, rc.Links) {
		kind := l.KindOrDefault()
		hasWeb = hasWeb || kind == "web"
		add(store.TestLink{Label: l.Label, URL: l.Render(ac.Name, ringName, rc.TargetEnv, version), Kind: kind, Source: "config"})
	}
	if !hasWeb {
		if host := healthHost(rc.HealthURL); host != "" {
			add(store.TestLink{Label: "Open " + strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://"), "/"), URL: host, Kind: "web", Source: "health"})
		}
	}
	for _, l := range deployed {
		add(l)
	}
	if out == nil {
		out = []store.TestLink{}
	}
	return out
}

// healthHost returns scheme://host/ of a health URL, or "".
func healthHost(health string) string {
	u, err := url.Parse(health)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/"
}

// ---- URL extraction from deploy logs ----

var urlRE = regexp.MustCompile(`https?://[^\s"'<>` + "`" + `\]\[(){}|\\^]+`)

// noiseHosts are hosts deploy logs mention that are never what a tester wants:
// package registries, base-image mirrors, GitHub's own plumbing.
var noiseHosts = []string{
	"api.github.com", "codeload.github.com", "objects.githubusercontent.com",
	"raw.githubusercontent.com", "pipelines.actions.githubusercontent.com",
	"results-receiver.actions.githubusercontent.com", "github-releases.githubusercontent.com",
	"blob.core.windows.net", "registry-1.docker.io", "auth.docker.io",
	"production.cloudflare.docker.com", "index.docker.io", "docker.io",
	"deb.debian.org", "security.debian.org", "archive.ubuntu.com", "security.ubuntu.com",
	"dl-cdn.alpinelinux.org", "registry.npmjs.org", "registry.yarnpkg.com", "npmjs.com",
	"proxy.golang.org", "sum.golang.org", "go.dev", "golang.org", "pypi.org",
	"files.pythonhosted.org", "rubygems.org", "nodejs.org", "get.helm.sh",
	"dl.k8s.io", "storage.googleapis.com", "git-scm.com", "w3.org", "schema.org",
	"example.com", "example.org", "localhost", "127.0.0.1", "0.0.0.0",
	"kubernetes.default.svc", "cluster.local",
}

// noisePaths are github.com paths that are plumbing, not outputs.
var noisePaths = []string{"github.com/actions/", "github.com/features/", "docs.github.com"}

// ExtractURLs returns the distinct, test-worthy URLs found in log lines, in
// order of last appearance (most recent first) — deploys print where they
// landed at the end. Signed or credential-bearing URLs are dropped entirely:
// they must never be stored or shown.
func ExtractURLs(lines []string) []string {
	var out []string
	seen := map[string]bool{}
	for i := len(lines) - 1; i >= 0; i-- {
		for _, m := range urlRE.FindAllString(lines[i], -1) {
			u := strings.TrimRight(m, ".,;:!?'\"")
			if seen[u] {
				continue
			}
			seen[u] = true
			if testWorthy(u) {
				out = append(out, u)
			}
		}
	}
	return out
}

func testWorthy(raw string) bool {
	if !safeLinkURL(raw) {
		return false
	}
	u, _ := url.Parse(raw)
	host := strings.ToLower(u.Hostname())
	for _, n := range noiseHosts {
		if host == n || strings.HasSuffix(host, "."+n) {
			return false
		}
	}
	if strings.HasSuffix(host, ".svc") || strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".local") {
		return false
	}
	hp := host + u.EscapedPath()
	for _, n := range noisePaths {
		if strings.HasPrefix(hp, n) {
			return false
		}
	}
	return true
}

// secretParams are query parameters that make a URL a credential.
var secretParams = []string{"sig", "signature", "token", "access_token", "x-amz-signature",
	"x-amz-credential", "x-goog-signature", "se", "sp", "sv", "code", "password", "secret", "key", "apikey", "api_key"}

// safeLinkURL reports whether raw is an absolute http(s) URL with no embedded
// credentials or signature-style query parameters.
func safeLinkURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return false
	}
	for k := range u.Query() {
		if slices.Contains(secretParams, strings.ToLower(k)) {
			return false
		}
	}
	return true
}

// kindForURL classifies a URL found in logs.
func kindForURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "other"
	}
	host := strings.ToLower(u.Hostname())
	p := strings.ToLower(u.Path)
	switch {
	case host == "testflight.apple.com" || host == "apps.apple.com" || host == "appstoreconnect.apple.com" || strings.HasSuffix(p, ".ipa"):
		return "ios"
	case host == "play.google.com" || strings.HasSuffix(p, ".apk") || strings.HasSuffix(p, ".aab"):
		return "android"
	case host == "github.com" && strings.Contains(p, "/actions/runs/"):
		return "ci"
	case host == "github.com" && strings.Contains(p, "/releases"):
		return "release"
	case strings.Contains(host, "grafana") || strings.Contains(p, "/d/"):
		return "dashboard"
	case strings.Contains(p, "swagger") || strings.Contains(p, "openapi") || strings.Contains(p, "/docs"):
		return "docs"
	case strings.HasPrefix(host, "api.") || strings.HasPrefix(p, "/api"):
		return "api"
	}
	return "web"
}

// labelForURL is a short human label: host plus a trimmed path.
func labelForURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return clip(raw, maxLinkLabelRunes)
	}
	label := u.Host
	if p := strings.TrimSuffix(u.Path, "/"); p != "" {
		label += p
	}
	return clip(label, maxLinkLabelRunes)
}

// logExcerpt keeps the lines that mention URLs plus the tail, as AI evidence.
func logExcerpt(lines []string) string {
	var urlLines []string
	for _, l := range lines {
		if strings.Contains(l, "http://") || strings.Contains(l, "https://") {
			urlLines = append(urlLines, redactSecrets(l))
		}
	}
	if len(urlLines) > excerptURLLines {
		urlLines = urlLines[len(urlLines)-excerptURLLines:]
	}
	tail := lines
	if len(tail) > excerptTailLines {
		tail = tail[len(tail)-excerptTailLines:]
	}
	var b strings.Builder
	if len(urlLines) > 0 {
		b.WriteString("Lines mentioning URLs:\n")
		for _, l := range urlLines {
			b.WriteString(clip(l, 300) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("Last log lines:\n")
	for _, l := range tail {
		b.WriteString(clip(redactSecrets(l), 300) + "\n")
	}
	text := b.String()
	if len(text) > maxExcerptBytes {
		text = text[len(text)-maxExcerptBytes:]
	}
	return text
}

// redactSecrets drops URLs that carry credentials from a log line.
func redactSecrets(line string) string {
	return urlRE.ReplaceAllStringFunc(line, func(m string) string {
		if safeLinkURL(strings.TrimRight(m, ".,;:!?'\"")) {
			return m
		}
		return "[redacted url]"
	})
}

func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "…"
}

// ---- AI test plan ----

// TestPlanReport renders the kit as the evidence handed to the model.
func TestPlanReport(view TestKitView, kit store.TestKit) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Application: %s\nRing: %s\nVersion: %s\n\n", view.App, view.Ring, view.Version)
	b.WriteString("Candidate links (id. kind | label | url):\n")
	for i, l := range view.Links {
		fmt.Fprintf(&b, "%d. %s | %s | %s\n", i+1, l.Kind, l.Label, l.URL)
	}
	if kit.LogExcerpt != "" {
		b.WriteString("\nDeploy log excerpt:\n")
		b.WriteString(kit.LogExcerpt)
	} else {
		b.WriteString("\nNo deploy logs were captured for this version.\n")
	}
	return b.String()
}

// ParseTestPlan validates the model's JSON answer. Links are accepted only
// when they reference a candidate by number or exact URL; everything else the
// model proposes is dropped, so the plan can never introduce a new URL.
func ParseTestPlan(raw string, candidates []store.TestLink, now time.Time) (store.TestPlan, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	var ans struct {
		Summary   string   `json:"summary"`
		Checklist []string `json:"checklist"`
		Links     []struct {
			ID  json.RawMessage `json:"id"`
			URL string          `json:"url"`
			Why string          `json:"why"`
		} `json:"links"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &ans); err != nil {
		return store.TestPlan{}, fmt.Errorf("the model did not return valid JSON: %w", err)
	}
	plan := store.TestPlan{Summary: clip(ans.Summary, maxPlanTextRunes), GeneratedAt: now.UTC()}
	for _, item := range ans.Checklist {
		if item = clip(item, maxPlanItemRunes); item != "" && len(plan.Checklist) < maxPlanChecklist {
			plan.Checklist = append(plan.Checklist, item)
		}
	}
	picked := map[string]bool{}
	for _, l := range ans.Links {
		if len(plan.Links) >= maxPlanLinks {
			break
		}
		var c *store.TestLink
		var id int
		if json.Unmarshal(l.ID, &id) == nil && id >= 1 && id <= len(candidates) {
			c = &candidates[id-1]
		} else if l.URL != "" {
			for i := range candidates {
				if candidates[i].URL == l.URL {
					c = &candidates[i]
					break
				}
			}
		}
		if c == nil || picked[c.URL] {
			continue
		}
		picked[c.URL] = true
		pick := *c
		pick.Why = clip(l.Why, maxPlanItemRunes)
		plan.Links = append(plan.Links, pick)
	}
	if plan.Summary == "" && len(plan.Checklist) == 0 {
		return store.TestPlan{}, errors.New("the model returned an empty plan")
	}
	if plan.Checklist == nil {
		plan.Checklist = []string{}
	}
	if plan.Links == nil {
		plan.Links = []store.TestLink{}
	}
	return plan, nil
}

// SetTestPlan stores an AI test plan on the kit of (app, ring, version). A
// version deployed before test kits existed gets an empty kit created first.
func (p *Promoter) SetTestPlan(ctx context.Context, app, ringName, version string, plan store.TestPlan) error {
	err := p.store.SetTestPlan(ctx, app, ringName, version, plan)
	if errors.Is(err, store.ErrNotFound) {
		if err := p.store.SaveTestKit(ctx, store.TestKit{App: app, Ring: ringName, Version: version}); err != nil {
			return err
		}
		return p.store.SetTestPlan(ctx, app, ringName, version, plan)
	}
	return err
}
