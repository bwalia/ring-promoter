package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// The event stream replaces the UI's fixed 2-second /api/jobs polling: a
// connected browser gets a push whenever job state changes and nothing at all
// while the system is idle. Updates within the coalesce window collapse into
// one snapshot (a deploy emits log lines far faster than a human can read),
// and heartbeat comments keep idle connections alive through proxies.
var (
	eventsCoalesce  = time.Second
	eventsHeartbeat = 25 * time.Second
)

// eventHub fans "job state changed" wake-ups out to connected event streams.
type eventHub struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

func newEventHub() *eventHub {
	return &eventHub{subs: make(map[chan struct{}]struct{})}
}

// wake nudges every subscriber without blocking. Subscriber channels hold one
// pending wake-up; more would be pointless because the stream sends whole
// snapshots, not deltas.
func (h *eventHub) wake() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (h *eventHub) subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

// handleEvents streams the shared jobs view as Server-Sent Events: one `jobs`
// event with the full latest-per-app snapshot on connect and after every
// (coalesced) change. The UI falls back to polling when the stream is down, so
// this endpoint can be dropped by any proxy without loss of function.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	// ResponseController reaches Flush through the logging/metrics wrappers
	// (they expose Unwrap for exactly this).
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	// Tell buffering reverse proxies (nginx-style) to pass events through.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if err := rc.Flush(); err != nil {
		return // writer cannot stream
	}

	wake, cancel := s.jobs.hub.subscribe()
	defer cancel()

	send := func() bool {
		payload, err := json.Marshal(map[string]any{"jobs": s.jobs.latestPerApp()})
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: jobs\ndata: %s\n\n", payload); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !send() {
		return
	}

	heartbeat := time.NewTicker(eventsHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-wake:
			// Let a burst of updates (log lines) collapse into one snapshot.
			t := time.NewTimer(eventsCoalesce)
			select {
			case <-r.Context().Done():
				t.Stop()
				return
			case <-t.C:
			}
			if !send() {
				return
			}
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}
