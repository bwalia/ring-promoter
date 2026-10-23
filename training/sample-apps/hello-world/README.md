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

The version comes from the `-X main.version` build ldflag (CI bakes in the image
tag), falling back to `RP_VERSION` for images built without one, then `dev`.
The baked-in version wins because Ring Promoter's kubectl deployer swaps only
the image: a `RP_VERSION` left on the Deployment by an earlier Helm install
would otherwise make every new build report the old version.

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

On the training instance hello-world is the worked example of every gate
family. [`hello-world-qa.yml`](../../../.github/workflows/hello-world-qa.yml)
produces the evidence on every change and nightly, using open-source tools only:

| Control | Tool | Evidence → gate |
|---------|------|-----------------|
| Unit tests (QA) | `go test -race -cover` | metrics → Grafana gate |
| SAST | gosec, Semgrep (`p/golang`, `p/dockerfile`) | metrics + SARIF |
| SCA | govulncheck (only reachable vulnerabilities block) | metrics + SARIF |
| Secrets | gitleaks | metrics + SARIF |
| IaC / CIS compliance | Checkov (CIS Docker + Kubernetes) on the Dockerfile and chart | metrics + SARIF |
| SBOM | Syft (CycloneDX), plus a buildx SPDX attestation on the image | metrics |
| Image CVEs | Grype over the SBOM | metrics + SARIF |
| Licences | licence policy over the SBOM (strong copyleft blocks) | metrics + SARIF |
| Provenance | buildx SLSA provenance (`mode=max`) | metrics |
| Release sign-off | a release engineer's GO for the exact version | Ring Promoter |

Metrics go to the rp-qa Pushgateway and show on grafana.workstation.co.uk:
[QA Unit Tests](https://grafana.workstation.co.uk/d/rp-qa-unit-tests?var-app=hello-world)
and [Security & Compliance](https://grafana.workstation.co.uk/d/rp-security-compliance?var-app=hello-world).
Ring Promoter enforces them in two ways:

- **Grafana gate (int, test, acc, prod):** one Go/No-Go check per control,
  covering the latest build. A release engineer can overrule it from the
  ring's panel with a recorded reason, for example for a known flake.
- **Security gate (test, acc, prod):** judges the SARIF for the exact version
  being promoted. All six scan kinds are required, and no critical or high
  finding may be left unwaived. It cannot be overridden: waive the individual
  finding instead, with a reason and an expiry.
- **Release sign-off (prod).**

CIS exceptions live in [`.checkov.yaml`](./.checkov.yaml), each with its
reason. Run the checks locally:

```bash
go test ./... -race -cover
go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
```

To see a gate go red, break a test on a branch and merge it. The dashboard
flips to NO-GO and Promote turns disabled on the training instance.

## How Ring Promoter manages it

Registered in [`training/config/apps.training.yaml`](../../config/apps.training.yaml)
with four rings (int → test → acc → prod). Each ring's health check requires the
endpoint to report the exact deployed version, so a promotion only "sticks" when
the new build is genuinely live. Walk the flow in
[Lab 01](../../labs/lab-01-first-deployment.md).
