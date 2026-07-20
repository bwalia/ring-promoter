# image-proc

> **Ring Promoter feature focus:** the **k8sjob deployer** (each deploy runs as
> a Kubernetes Job in the `ring-exec` namespace), **async worker autoscaling**,
> and **version-aware health via the `X-App-Version` response header**
> (`health_version_header: X-App-Version`) rather than a JSON field.

An async, queue-backed workload. An **API** accepts jobs and pushes them onto a
Redis list; a horizontally-scaled **worker** drains the list and "processes"
each job. It builds on the [hello-world](../hello-world/) reference app (same Go
service + Dockerfile + Helm chart + CI + `architecture.md` shape) and adds a
queue, a second binary, and an HPA.

## Components

| Component | Ingress | Endpoints | Scales |
|-----------|---------|-----------|--------|
| `cmd/api`    | yes (traefik) | `POST /jobs`, `GET /jobs/{id}`, `/healthz`, `/readyz`, `/metrics` | fixed replicas |
| `cmd/worker` | no (side port only) | `/healthz`, `/metrics` | **HPA** |
| Redis        | no | — | single replica (ephemeral) |

## API endpoints

| Path             | Purpose |
|------------------|---------|
| `POST /jobs`     | Enqueue a job onto the Redis list. `202` + `{"id","status":"queued"}`. **`503` if Redis is down** (the queue is sick, not the API). |
| `GET /jobs/{id}` | Job status: `queued` \| `processing` \| `done` (`404` if unknown). |
| `/healthz`       | Liveness. **Stays `ok` even when Redis is down.** |
| `/readyz`        | Readiness (503 during a 2s warm-up). |
| `/metrics`       | Prometheus text: `imageproc_queue_depth` gauge, `imageproc_jobs_submitted_total` counter, build info. |

### Version-aware health — the HEADER variant

Unlike hello-world (which puts the version in a JSON field), **every** API
response — including `/healthz` — carries an `X-App-Version` response header
equal to the running version:

```
$ curl -si https://imageproc.fictionally.org/healthz
HTTP/1.1 200 OK
X-App-Version: v1.2.3
Content-Type: application/json
{"status":"ok","version":"v1.2.3"}
```

A Ring Promoter ring configured with `health_version_header: X-App-Version`
reads the header to confirm the running build matches the promoted version — a
stale replica answering `200 OK` with the old version fails the check and is
rolled back. The version comes from `RP_VERSION` (set by the chart / Ring
Promoter to the deployed tag), falling back to the `-X main.version` build
ldflag, then `dev`.

## Run it locally

```bash
# 1. Redis
docker run --rm -p 6379:6379 redis:7-alpine

# 2. API (terminal 2) and worker (terminal 3)
RP_VERSION=v1.2.3 REDIS_ADDR=localhost:6379 go run ./cmd/api      # :8080
RP_VERSION=v1.2.3 REDIS_ADDR=localhost:6379 go run ./cmd/worker   # metrics :9090

# 3. Drive it
id=$(curl -s -X POST localhost:8080/jobs | jq -r .id)
curl -s localhost:8080/jobs/$id         # -> processing, then done
curl -s localhost:8080/metrics | grep queue_depth
```

## Build the images (two, from one multi-target Dockerfile)

```bash
docker build --target api    --build-arg VERSION=v0.1.0 -t ghcr.io/bwalia/rp-training-image-proc-api:v0.1.0 .
docker build --target worker --build-arg VERSION=v0.1.0 -t ghcr.io/bwalia/rp-training-image-proc-worker:v0.1.0 .
```

## Deploy via Helm (what the k8sjob deploy script runs per ring)

```bash
helm upgrade --install image-proc ./chart \
  --namespace image-proc-int --create-namespace \
  --set image.tag=v0.1.0 \
  --set ingress.host=imageproc.fictionally.org
```

The API Ingress uses `ingressClassName: traefik` (k3s1) and annotates the host
so **external-dns publishes a Cloudflare CNAME → `pop0.wslproxy.com`** (the
wslproxy POP). Redis is an in-cluster `redis:7-alpine` Deployment+Service
(ephemeral). See [`architecture.md`](./architecture.md).

### Chart value toggles

| Value | Default | Effect |
|-------|---------|--------|
| `api.enabled` / `worker.enabled` / `redis.enabled` | `true` | Turn each workload on/off. |
| `worker.autoscaling.enabled` | `true` | HPA on the worker (CPU stand-in). |
| `worker.autoscaling.{min,max}Replicas` | `2` / `8` | Worker scale bounds. |
| `worker.processDelay` | `500ms` | Per-job "work" time (deeper queue under load). |
| `ingress.enabled` / `ingress.host` | `true` / `imageproc.<domain>` | API ingress. |
| `image.tag` | `""`→appVersion | Promoted version (Ring Promoter sets this). |

## How Ring Promoter manages it — the k8sjob deployer

image-proc is registered with Ring Promoter using the **`k8sjob` deployer**.
Instead of Ring Promoter shelling out to `kubectl`/`helm` from its own process
(as hello-world's `kubectl` deployer does), **each deploy is executed as a
Kubernetes Job in the `ring-exec` namespace** that runs a deploy script. This
isolates deploy credentials and tooling in a short-lived pod per deploy.

The app-config shape Ring Promoter uses (documented here; the orchestrator wires
the real config):

```yaml
apps:
  - name: image-proc
    deployer: k8sjob            # run each deploy as a Kubernetes Job
    health_version_header: X-App-Version   # read the version from the response header
    k8sjob:
      namespace: ring-exec      # where the deploy Job runs
      image: ghcr.io/bwalia/rp-training-deploy:latest   # image with helm+kubectl
      command:                  # the deploy script the Job executes
        - /bin/sh
        - -c
        - |
          helm upgrade --install image-proc ./chart \
            --namespace image-proc-$RING --create-namespace \
            --set image.tag=$VERSION \
            --set ingress.host=imageproc.fictionally.org
    rings:
      - { name: int,  auto_promote: true }
      - { name: test, auto_promote: true }
      - { name: acc,  auto_promote: false }
      - { name: prod, auto_promote: false }
```

Each ring's health check requires the API's `X-App-Version` header to equal the
promoted version, so a promotion only "sticks" when the new build is genuinely
live. Compare with hello-world's JSON-field variant in
[Lab 01](../../labs/lab-01-first-deployment.md).
