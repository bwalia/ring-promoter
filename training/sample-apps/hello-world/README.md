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
**external-dns publishes a Cloudflare CNAME → `lon1.pop0.uk`** (the wslproxy
POP). See [`architecture.md`](./architecture.md).

## QA, security and compliance gates

On the training instance hello-world is the worked example of all three gate
families. [`hello-world-qa.yml`](../../../.github/workflows/hello-world-qa.yml)
produces the evidence on every change and nightly:

| Gate | Evidence | Rings | Blocks when |
|------|----------|-------|-------------|
| **QA (Grafana)** | `go test` results → rp-qa Pushgateway → [QA Unit Tests dashboard](https://grafana.workstation.co.uk/d/rp-qa-unit-tests?var-app=hello-world) | int, test, acc, prod | a test fails, pass rate ≤ 95%, coverage ≤ 50% |
| **AppSec** | gosec (SAST), govulncheck (SCA), gitleaks (secrets) SARIF → `POST /api/security/scans` | test, acc, prod | a scan is missing or older than 30 days, or any critical/high finding is unwaived |
| **Compliance** | release-engineer sign-off for the exact version | prod | no GO sign-off recorded |

The QA gate can be overruled from a ring's panel with a recorded reason (for a
known flake); the AppSec gate cannot — waive the individual finding instead,
with a reason and an expiry. Run the unit tests locally:

```bash
go test ./... -race -cover
```

To see the gate go red, break a test on a branch, merge it, and watch the
dashboard flip to NO-GO and Promote turn disabled on the training instance.

## How Ring Promoter manages it

Registered in [`training/config/apps.training.yaml`](../../config/apps.training.yaml)
with four rings (int → test → acc → prod). Each ring's health check requires the
endpoint to report the exact deployed version, so a promotion only "sticks" when
the new build is genuinely live. Walk the flow in
[Lab 01](../../labs/lab-01-first-deployment.md).
