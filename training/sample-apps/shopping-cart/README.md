# shopping-cart

> **Ring Promoter feature focus:** a multi-service app (frontend + backend +
> Redis) promoted together as a Ring Promoter **group**, plus **canary** and
> **blue/green** deployment patterns. The backend keeps version-aware health
> checks (`health_version_field: version`) so a group promotion only "sticks"
> when the new build is genuinely live.

A small but realistic multi-service workload built on the same shape as
[`hello-world`](../hello-world) (Go service + Dockerfile + Helm chart + CI +
`architecture.md`), scaled up to three cooperating pieces:

- **Backend API** (`cmd/api`, standard library only) — stores cart items in
  Redis over a tiny hand-written RESP client, and **degrades gracefully**: an
  empty cart when Redis is down, an in-memory store when `REDIS_ADDR` is unset.
  It never crashes because of Redis.
- **Frontend** (`cmd/web`) — a tiny static server that ships one embedded
  `index.html`; the page fetches the backend's `/api/cart`.
- **Redis** — an ephemeral `redis:7-alpine` Deployment + Service (no auth, no
  persistence), templated by the chart.

The frontend and backend share one version and are deployed by **one Helm
release**, so Ring Promoter promotes them as a group.

## Endpoints

### Backend (`cmd/api`)

| Path            | Purpose                                                       |
|-----------------|--------------------------------------------------------------|
| `GET /api/cart` | `{"items":[...],"backend":"redis\|memory"}` (empty if Redis down) |
| `POST /api/cart`| Body `{"item":"milk"}` — appends an item                     |
| `/healthz`      | `{"status":"ok","version":"..."}` — liveness + version        |
| `/readyz`       | `200` when Redis is reachable, `503 not-ready` when down (never crashes) |
| `/metrics`      | Prometheus text (`cart_requests_total`, `cart_items_added_total`, build info) |

### Frontend (`cmd/web`)

| Path       | Purpose                                             |
|------------|-----------------------------------------------------|
| `GET /`    | The embedded single-page cart UI                    |
| `/healthz` | `{"status":"ok","version":"..."}`                    |
| `/readyz`  | `200` (no external dependency)                      |
| `/metrics` | Prometheus text (`web_requests_total`, build info)  |

Version comes from `RP_VERSION` (set by the Helm chart / Ring Promoter to the
deployed tag), falling back to the `-X main.version` build ldflag, then `dev` —
exactly like `hello-world`.

## Run it locally

```bash
# Backend — in-memory store (no Redis needed)
RP_VERSION=v1.2.3 go run ./cmd/api          # http://localhost:8080
curl -s localhost:8080/healthz              # {"status":"ok","version":"v1.2.3"}
curl -s -XPOST localhost:8080/api/cart -d '{"item":"milk"}'
curl -s localhost:8080/api/cart             # {"items":["milk"],"backend":"memory"}

# Backend — with Redis
docker run -d --rm -p 6379:6379 redis:7-alpine
REDIS_ADDR=127.0.0.1:6379 go run ./cmd/api

# Frontend (points the page at the backend via API_BASE)
API_BASE=http://localhost:8080 go run ./cmd/web   # http://localhost:8080 (use a different PORT)
```

## Build the images

```bash
docker build --build-arg VERSION=v0.1.0 -f Dockerfile     -t ghcr.io/bwalia/rp-training-shopping-cart-api:v0.1.0 .
docker build --build-arg VERSION=v0.1.0 -f Dockerfile.web -t ghcr.io/bwalia/rp-training-shopping-cart-web:v0.1.0 .
```

## Deploy via Helm (what Ring Promoter does per ring)

One release deploys all three workloads. Backend and frontend share `image.tag`,
so the group is promoted atomically:

```bash
helm upgrade --install shopping-cart ./chart \
  --namespace shopping-cart-int --create-namespace \
  --set image.tag=v0.1.0
```

This renders:

- `shopping-cart-backend` Deployment + Service, `shopping-cart-frontend`
  Deployment + Service, `shopping-cart-redis` Deployment + Service.
- Two Ingresses using `ingressClassName: traefik` (k3s1), annotated so
  **external-dns publishes a Cloudflare CNAME → `pop0.wslproxy.com`**:
  - frontend → `shop.fictionally.org`
  - backend → `shopping-cart-api.fictionally.org`

## Deployment patterns

The backend is set up so the training academy can demonstrate two progressive
delivery patterns on top of Ring Promoter's ring flow. See
[`architecture.md`](./architecture.md) for the mechanics:

- **Canary** — run two backend Deployments (stable + canary) behind one Service
  and split traffic with Traefik weighting; watch `/healthz` version + metrics,
  then widen or roll back.
- **Blue/green** — run two full-version Deployments and flip the Service
  selector from `blue` to `green` for an instant, reversible cutover.

## How Ring Promoter manages it

Registered as a **group** (frontend + backend) with the usual rings
(int → test → acc → prod). Each ring's health check requires the backend
`/healthz` to report the exact deployed version (`health_version_field:
version`), so a group promotion only passes when both services are genuinely
serving the promoted build.
