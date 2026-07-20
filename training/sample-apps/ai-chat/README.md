# ai-chat

> **Ring Promoter feature focus:** AI failure-diagnosis (Ollama), a **ref-pinned
> ring** (`acc` → `release`), multiple environments with **optional GPU
> deployment**, and version-aware health checks (`health_version_field: version`).

A dependency-free Go HTTP service that proxies chat prompts to a local
[Ollama](https://ollama.com) server. It follows the same shape as the
[`hello-world`](../hello-world) reference app (Go service + Dockerfile + Helm
chart + CI + `architecture.md`) and adds a real upstream dependency so the
academy can teach the AI-centric Ring Promoter features.

## Why this app exists — the Ring Promoter tie-ins

### (a) AI failure-diagnosis (Ollama) — where Ring Promoter shines

Ring Promoter can call an **Ollama** model to diagnose *why* a ring went
unhealthy — it feeds the failing pod's logs, events, and the diff between the
old and new version to a local model and gets back a plain-English explanation
("readiness never went green because `OLLAMA_URL` points at an unreachable
service"). `ai-chat` is the natural showcase for this because **its whole
ecosystem already runs Ollama**: the same in-cluster model that `POST /chat`
proxies to is the one Ring Promoter's diagnosis uses. Break a promotion of this
app on purpose (point `ollama.url` at nothing, or ship a bad tag) and Ring
Promoter's Ollama-backed diagnosis narrates the failure — a self-contained,
GPU-free-friendly demo of the feature.

### (b) A ref-pinned ring (`acc` → `release`)

Unlike `hello-world` (where every ring tracks whatever is promoted to it), the
`acc` ring here is **ref-pinned**: it always ships the `release` ref, never a
rolling SHA. See [`chart/values-acc.yaml`](./chart/values-acc.yaml) — `image.tag`
is pinned to `release`, and the Ring Promoter app config declares `acc` (and
`prod`) with `ref: release`. A promotion *into* `acc` therefore only ever
deploys the last blessed candidate. This teaches "some rings float, some rings
pin."

### (c) Multiple environments + optional GPU deployment

The chart deploys across `int → test → acc → prod` and carries an **optional GPU
scheduling block** (`nodeSelector` + `tolerations` + a `nvidia.com/gpu` resource
request). It is **disabled by default** (`gpu.enabled=false`) so the chart runs
on any node; enable it in a ring that hosts Ollama on a GPU box.

### (d) Version-aware health checks (`health_version_field: version`)

`/healthz` echoes `RP_VERSION` and every response carries an `X-App-Version`
header, so a ring configured with `health_version_field: version` only passes
once the endpoint actually serves the promoted build.

## Endpoints

| Path       | Purpose                                             |
|------------|-----------------------------------------------------|
| `POST /chat` | `{"prompt":"..."}` → proxied to Ollama `/api/generate`; returns `{"response":"...","version":"..."}` |
| `/`        | Human greeting + version + configured model          |
| `/healthz` | `{"status":"ok","version":"..."}` — liveness + version |
| `/readyz`  | Readiness (503 during a 2s warm-up, then 200)       |
| `/metrics` | Prometheus text (`aichat_requests_total`, `aichat_chat_requests_total`, `aichat_ollama_errors_total`, build info) |

Every response also sets the **`X-App-Version`** response header.

The version comes from `RP_VERSION` (set by the Helm chart / Ring Promoter to
the deployed tag), falling back to the `-X main.version` build ldflag, then
`dev`.

## Ollama is optional (offline-friendly)

`POST /chat` calls Ollama's `/api/generate` at `OLLAMA_URL` with
`{model, prompt, stream:false}`. If `OLLAMA_URL` is empty **or** the upstream
call fails, it still returns **HTTP 200** with a canned body so demos work with
no GPU and no model:

```json
{"response":"(ollama unavailable in this environment)","version":"v0.1.0"}
```

`OLLAMA_MODEL` defaults to `qwen3-coder:30b`.

## Run it locally

```bash
go run ./cmd            # http://localhost:8080
curl -s localhost:8080/healthz     # {"status":"ok","version":"dev"}

# Offline (no Ollama) — returns the canned response:
curl -s -X POST localhost:8080/chat -d '{"prompt":"hello"}'

# Against a real Ollama:
OLLAMA_URL=http://localhost:11434 OLLAMA_MODEL=qwen3-coder:30b go run ./cmd
curl -s -X POST localhost:8080/chat -d '{"prompt":"Explain ring-based promotion"}'
```

## Build the image

```bash
docker build --build-arg VERSION=v0.1.0 -t ghcr.io/bwalia/rp-training-ai-chat:v0.1.0 .
```

## Deploy via Helm (what Ring Promoter does per ring)

```bash
helm upgrade --install ai-chat ./chart \
  --namespace ai-chat-int --create-namespace \
  --set image.tag=v0.1.0 \
  --set ingress.host=ai-chat.fictionally.org \
  --set ollama.url=http://ollama.ollama.svc.cluster.local:11434

# The ref-pinned acc ring:
helm upgrade --install ai-chat ./chart -f chart/values-acc.yaml \
  --namespace ai-chat-acc --create-namespace

# Enable optional GPU scheduling for a ring that hosts Ollama on a GPU node:
helm upgrade --install ai-chat ./chart --set gpu.enabled=true ...
```

The Ingress uses `ingressClassName: traefik` (k3s1) and annotates the host so
**external-dns publishes a Cloudflare CNAME → `pop0.wslproxy.com`** (the wslproxy
POP). See [`architecture.md`](./architecture.md).

## How Ring Promoter manages it

Registered in
[`training/config/apps.training.yaml`](../../config/apps.training.yaml) with
four rings (int → test → acc → prod). `acc` and `prod` are ref-pinned to
`release`; `int`/`test` track promoted builds. Each ring's health check requires
the endpoint to report the exact deployed version, and Ring Promoter's
Ollama-based diagnosis explains any ring that stays unhealthy.
