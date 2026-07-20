// Command ai-chat is a Ring Promoter training academy service that proxies chat
// prompts to a local Ollama server. It is the app whose ecosystem showcases
// Ring Promoter's Ollama-based AI failure diagnosis: the same Ollama that this
// app talks to is what Ring Promoter uses to explain why a ring went unhealthy.
//
// It is dependency-free (standard library only) so it builds and runs anywhere,
// and it exposes the exact endpoints Ring Promoter's health checks understand:
//
//	POST /chat       -> {"prompt":"..."} proxied to Ollama /api/generate
//	GET  /           -> greeting + version (human)
//	GET  /healthz    -> {"status":"ok","version":"..."}  (liveness + version)
//	GET  /readyz     -> 200 once warmed up                (readiness)
//	GET  /metrics    -> Prometheus text exposition        (observability)
//
// Every response also carries the X-App-Version header so a proxy or probe can
// read the running version without parsing a body.
//
// The version comes from the RP_VERSION environment variable (Ring Promoter and
// the Helm chart set it to the image tag being deployed) or the -X ldflag,
// falling back to "dev". Because /healthz echoes the version, a ring configured
// with `health_version_field: version` will only pass once the endpoint is
// actually serving the promoted build — the core "did the new version really go
// live?" guarantee.
//
// Ollama is optional: if OLLAMA_URL is empty or the upstream call fails, /chat
// still returns HTTP 200 with a canned body so demos work fully offline.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

// version is overridable at build time: go build -ldflags "-X main.version=v1.2.3".
// At runtime RP_VERSION wins so the same image can report the tag it was
// deployed as.
var version = "dev"

const offlineResponse = "(ollama unavailable in this environment)"

func resolveVersion() string {
	if v := os.Getenv("RP_VERSION"); v != "" {
		return v
	}
	return version
}

// chatRequest is the request body accepted by POST /chat.
type chatRequest struct {
	Prompt string `json:"prompt"`
}

// chatResponse is what POST /chat returns to the caller.
type chatResponse struct {
	Response string `json:"response"`
	Version  string `json:"version"`
}

// ollamaRequest is the JSON sent to Ollama's /api/generate endpoint.
type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// ollamaResponse is the (non-streaming) JSON returned by Ollama /api/generate.
type ollamaResponse struct {
	Response string `json:"response"`
}

func main() {
	addr := ":" + envOr("PORT", "8080")
	ver := resolveVersion()
	ollamaURL := os.Getenv("OLLAMA_URL")
	ollamaModel := envOr("OLLAMA_MODEL", "qwen3-coder:30b")

	var requests atomic.Int64
	var chatCalls atomic.Int64
	var ollamaErrors atomic.Int64
	ready := &atomic.Bool{}
	// Simulate a short warm-up so readiness is meaningful in demos.
	go func() {
		time.Sleep(2 * time.Second)
		ready.Store(true)
	}()

	client := &http.Client{Timeout: 60 * time.Second}

	mux := http.NewServeMux()

	// versioned wraps a handler so every response reports the running version.
	versioned := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-App-Version", ver)
			h(w, r)
		}
	}

	mux.HandleFunc("GET /", versioned(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprintf(w, "ai-chat — Ring Promoter training 🤖\nversion: %s\nhost: %s\nmodel: %s\nPOST /chat {\"prompt\":\"...\"}\n", ver, hostname(), ollamaModel)
	}))

	mux.HandleFunc("POST /chat", versioned(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		chatCalls.Add(1)

		var req chatRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil || req.Prompt == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be JSON {\"prompt\":\"...\"} with a non-empty prompt"})
			return
		}

		text, err := generate(r.Context(), client, ollamaURL, ollamaModel, req.Prompt)
		if err != nil {
			// Ollama optional: keep demos working offline by returning a canned
			// 200 rather than surfacing the upstream failure.
			ollamaErrors.Add(1)
			log.Printf("ollama call failed (returning canned response): %v", err)
			writeJSON(w, http.StatusOK, chatResponse{Response: offlineResponse, Version: ver})
			return
		}
		writeJSON(w, http.StatusOK, chatResponse{Response: text, Version: ver})
	}))

	mux.HandleFunc("GET /healthz", versioned(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": ver})
	}))

	mux.HandleFunc("GET /readyz", versioned(func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "warming-up"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}))

	mux.HandleFunc("GET /metrics", versioned(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP aichat_requests_total Total requests served.\n")
		fmt.Fprintf(w, "# TYPE aichat_requests_total counter\n")
		fmt.Fprintf(w, "aichat_requests_total %d\n", requests.Load())
		fmt.Fprintf(w, "# HELP aichat_chat_requests_total Total /chat requests served.\n")
		fmt.Fprintf(w, "# TYPE aichat_chat_requests_total counter\n")
		fmt.Fprintf(w, "aichat_chat_requests_total %d\n", chatCalls.Load())
		fmt.Fprintf(w, "# HELP aichat_ollama_errors_total Total upstream Ollama failures (served canned).\n")
		fmt.Fprintf(w, "# TYPE aichat_ollama_errors_total counter\n")
		fmt.Fprintf(w, "aichat_ollama_errors_total %d\n", ollamaErrors.Load())
		fmt.Fprintf(w, "# HELP aichat_build_info Build info as labels.\n")
		fmt.Fprintf(w, "# TYPE aichat_build_info gauge\n")
		fmt.Fprintf(w, "aichat_build_info{version=%q,model=%q} 1\n", ver, ollamaModel)
	}))

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("ai-chat %s listening on %s (ollama_url=%q model=%q)", ver, addr, ollamaURL, ollamaModel)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// generate posts the prompt to Ollama's /api/generate and returns the model's
// response text. An empty ollamaURL is treated as "not configured" so the
// caller falls back to the canned offline response.
func generate(ctx context.Context, client *http.Client, ollamaURL, model, prompt string) (string, error) {
	if ollamaURL == "" {
		return "", fmt.Errorf("OLLAMA_URL not set")
	}

	body, err := json.Marshal(ollamaRequest{Model: model, Prompt: prompt, Stream: false})
	if err != nil {
		return "", err
	}

	endpoint := ollamaURL + "/api/generate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var out ollamaResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&out); err != nil {
		return "", err
	}
	if out.Response == "" {
		return "", fmt.Errorf("ollama returned an empty response")
	}
	return out.Response, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}
