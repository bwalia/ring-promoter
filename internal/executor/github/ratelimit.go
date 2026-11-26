package github

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GitHub's REST quota is per token (5,000 requests an hour for a PAT), and
// every app in a promoter — and anything else using the same PAT, such as a
// developer's gh CLI — draws from it. On 2026-11-26 it ran dry and deploys
// failed outright with "github api: status 403: API rate limit exceeded",
// including runs that were already dispatched and only being watched.
//
// Two things follow from that:
//
//   - A rate-limited request did nothing, so it is safe to wait until the
//     quota resets and ask again, even for the dispatch POST. A deploy in
//     flight should outlive a dry quota rather than be recorded as failed.
//   - Polling should spend as little quota as it can. GitHub does not count a
//     conditional request answered 304 Not Modified, and an in-progress run
//     polled every 15s is unchanged most of the time — so GETs carry the ETag
//     of the last answer (see etagCache).

// rateLimitGate remembers, per token, until when GitHub has told us the quota
// is exhausted, so every executor sharing that token waits instead of each
// spending requests to discover the same thing.
type rateLimitGate struct {
	mu    sync.Mutex
	until map[[32]byte]time.Time
}

var sharedRateLimits = &rateLimitGate{until: map[[32]byte]time.Time{}}

// rateLimitFloor is the least a rate-limited request waits (a var for tests).
var rateLimitFloor = 5 * time.Second

func tokenKey(token string) [32]byte { return sha256.Sum256([]byte(token)) }

// blockedUntil returns when the token's quota resets, or the zero time when
// it is not known to be exhausted.
func (g *rateLimitGate) blockedUntil(token string, now time.Time) time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	k := tokenKey(token)
	t, ok := g.until[k]
	if !ok {
		return time.Time{}
	}
	if !t.After(now) {
		delete(g.until, k)
		return time.Time{}
	}
	return t
}

func (g *rateLimitGate) block(token string, until time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	k := tokenKey(token)
	if until.After(g.until[k]) {
		g.until[k] = until
	}
}

// rateLimitedUntil reports whether resp is GitHub refusing a request for rate
// limiting (primary or secondary), and if so when it is worth asking again.
// A plain 403 (missing permission) is not rate limiting and returns false.
func rateLimitedUntil(resp *http.Response, now time.Time) (time.Time, bool) {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return time.Time{}, false
	}
	// Secondary limits say how long to wait outright.
	if s := resp.Header.Get("Retry-After"); s != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && secs >= 0 {
			return now.Add(time.Duration(secs) * time.Second), true
		}
	}
	// Primary limit: the quota is spent and resets at X-RateLimit-Reset.
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		if s := resp.Header.Get("X-RateLimit-Reset"); s != "" {
			if epoch, err := strconv.ParseInt(s, 10, 64); err == nil {
				// A second past the reset, so the first retry lands after it.
				return time.Unix(epoch, 0).Add(time.Second), true
			}
		}
		return now.Add(time.Minute), true
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return now.Add(time.Minute), true
	}
	// A 403 without the headers: only rate limiting if the body says so
	// (secondary limits sometimes arrive like that).
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if strings.Contains(strings.ToLower(string(body)), "rate limit") {
		return now.Add(time.Minute), true
	}
	return time.Time{}, false
}

// rateLimitError is what a caller gets when the quota will not reset within
// the executor's patience: it says when it will, which the raw 403 did not.
func rateLimitError(until, now time.Time) error {
	return fmt.Errorf("github api: rate limit exhausted for this token (shared by every app and tool using it); resets at %s UTC, in %s",
		until.UTC().Format("15:04:05"), until.Sub(now).Round(time.Second))
}

// ---- conditional requests ----

// etagCache keeps the last 200 answer of each GET, with its ETag, so the next
// GET of the same URL can be sent conditionally. Only small JSON bodies are
// kept (not log archives), and the cache is bounded.
type etagCache struct {
	mu      sync.Mutex
	entries map[string]etagEntry
}

type etagEntry struct {
	etag   string
	header http.Header
	body   []byte
}

const (
	etagMaxEntries = 256
	etagMaxBody    = 1 << 20
)

func (c *etagCache) get(url string) (etagEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[url]
	return e, ok
}

func (c *etagCache) put(url string, e etagEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil || len(c.entries) >= etagMaxEntries {
		// Cheap bound: start over rather than track recency. Old run URLs
		// are never polled again, so losing them costs nothing.
		c.entries = map[string]etagEntry{}
	}
	c.entries[url] = e
}

// remember stores a 200 JSON answer that carries an ETag and hands back a
// response whose body is still readable.
func (c *etagCache) remember(url string, resp *http.Response) *http.Response {
	etag := resp.Header.Get("ETag")
	if resp.StatusCode != http.StatusOK || etag == "" ||
		!strings.Contains(resp.Header.Get("Content-Type"), "json") ||
		resp.ContentLength > etagMaxBody {
		return resp
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, etagMaxBody+1))
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil || len(body) > etagMaxBody {
		// Too big or unreadable to keep; hand back exactly what was read and
		// let the decoder report any error.
		return resp
	}
	c.put(url, etagEntry{etag: etag, header: resp.Header.Clone(), body: body})
	return resp
}

// replay turns a 304 into the 200 it stands for.
func (e etagEntry) replay(req *http.Request) *http.Response {
	return &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        e.header.Clone(),
		Body:          io.NopCloser(bytes.NewReader(e.body)),
		ContentLength: int64(len(e.body)),
		Request:       req,
	}
}
