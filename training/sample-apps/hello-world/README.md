# hello-world

> **Ring Promoter feature focus:** seed → promote → rollback, version-aware
> health checks (`health_version_field`), auto-promote chains, the `kubectl`
> deployer. The smallest app — start here.

A dependency-free Go HTTP service that reports the version it is running. It is
the reference app: every other training app copies this shape (Go service +
Dockerfile + Helm chart + CI + `architecture.md`).

## Endpoints

| Path       | Purpose                                             |
|------------|-----------------------------------------------------|
| `/`        | Human greeting + version                             |
| `/healthz` | `{"status":"ok","version":"..."}` — liveness + version |
| `/readyz`  | Readiness (503 during a 2s warm-up, then 200)       |
| `/metrics` | Prometheus text (`helloworld_requests_total`, build info) |

The version comes from `RP_VERSION` (set by the Helm chart / Ring Promoter to
the deployed tag), falling back to the `-X main.version` build ldflag, then
`dev`.

## Run it locally

```bash
go run ./cmd            # http://localhost:8080
RP_VERSION=v1.2.3 go run ./cmd
curl -s localhost:8080/healthz     # {"status":"ok","version":"v1.2.3"}
```

## Build the image

```bash
docker build --build-arg VERSION=v0.1.0 -t ghcr.io/bwalia/rp-training-hello-world:v0.1.0 .
```

## Deploy via Helm (what Ring Promoter does per ring)

```bash
helm upgrade --install hello-world ./chart \
  --namespace hello-world-int --create-namespace \
  --set image.tag=v0.1.0 \
  --set ingress.host=hello-world.fictionally.org
```

The Ingress uses `ingressClassName: traefik` (k3s1) and annotates the host so
**external-dns publishes a Cloudflare CNAME → `pop0.wslproxy.com`** (the wslproxy
POP). See [`architecture.md`](./architecture.md).

## How Ring Promoter manages it

Registered in [`training/config/apps.training.yaml`](../../config/apps.training.yaml)
with four rings (int → test → acc → prod). Each ring's health check requires the
endpoint to report the exact deployed version, so a promotion only "sticks" when
the new build is genuinely live. Walk the flow in
[Lab 01](../../labs/lab-01-first-deployment.md).
