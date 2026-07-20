# kuard

> **Ring Promoter feature focus:** the `kubectl` deployer running a **third-party
> image** (`gcr.io/kuar-demo/kuard-amd64`), **STATUS-ONLY health checks** (kuard's
> `/healthy` returns `200` with no version field), and **blue/green promotion via
> image tag** (`blue` <-> `green`). Display name **"KUARD Demo"**, with
> `auto_promote` enabled. This is the **canonical demo app the labs use most**.

KUARD (Kubernetes Up And Running Demo) is the upstream training image from the
"Kubernetes Up & Running" book. We build **nothing** here — no Go, no Dockerfile.
This app exists to show how Ring Promoter promotes an image it does not own, and
what health checking looks like when the app reports **only** a status and no
version.

## Why it is the canonical lab app

- **Third-party image** — proves promotions don't require building your own code.
- **STATUS-ONLY health check** — `/healthy` returns `200 OK` with no JSON version
  field. Ring Promoter verifies liveness/readiness only; the **deployed image tag
  is the version**. Contrast with `hello-world`, which reports its version and
  uses a version-aware health check (`health_version_field`).
- **Blue/green by tag** — promotion flips `image.tag` between `blue` and `green`,
  the simplest possible "new version" signal.

## Endpoints

| Path       | Purpose                                              |
|------------|------------------------------------------------------|
| `/`        | kuard web UI (env, liveness, memory, etc.)           |
| `/healthy` | Liveness + readiness — `200 OK`, **no version field** |

## Run it locally

```bash
docker run --rm -p 8080:8080 gcr.io/kuar-demo/kuard-amd64:blue
curl -s -o /dev/null -w '%{http_code}\n' localhost:8080/healthy   # 200
# swap blue -> green to see the other colour
docker run --rm -p 8080:8080 gcr.io/kuar-demo/kuard-amd64:green
```

## Deploy via Helm (what Ring Promoter does per ring)

```bash
helm upgrade --install kuard ./chart \
  --namespace kuard-int --create-namespace \
  --set image.tag=blue \
  --set ingress.host=kuard.fictionally.org
```

Promotion is just re-running with the other tag:

```bash
helm upgrade --install kuard ./chart --set image.tag=green
```

The Ingress uses `ingressClassName: traefik` (k3s1) and annotates the host so
**external-dns publishes a Cloudflare CNAME → `pop0.wslproxy.com`** (the wslproxy
POP). See [`architecture.md`](./architecture.md).

## How Ring Promoter manages it

Registered in [`training/config/apps.training.yaml`](../../config/apps.training.yaml)
with `display_name: "KUARD Demo"`, `auto_promote` enabled, and a **status-only**
health check (no `health_version_field`). Blue/green is expressed as promoting
the `blue` and `green` tags across the rings (int → test → acc → prod).
