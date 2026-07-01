# Ring Promoter

A small, production-grade control plane that promotes versions of **multiple
applications** through an ordered set of deployment **rings**:

```
ring0 (Dev) → ring1 (Integration) → ring2 (Acceptance) → ring3 (Production)
```

It runs as a long-lived service on [k3s](https://k3s.io/), exposes a JSON REST
API and an embedded single-page web UI, and tracks current/previous versions and
full history **per (application, ring)**.

---

## Contents

- [How it works](#how-it-works)
- [Project layout](#project-layout)
- [Run locally](#run-locally)
- [REST API](#rest-api)
- [Add an application (config only)](#add-an-application-config-only)
- [Add a ring (single-place change)](#add-a-ring-single-place-change)
- [Deploy on k3s](#deploy-on-k3s)
- [How an app's CI calls the API](#how-an-apps-ci-calls-the-api)
- [Postgres schema](#postgres-schema)
- [Configuration reference](#configuration-reference)
- [Testing](#testing)

---

## How it works

The rings are **shared and ordered** (defined once in `internal/ring`). Each
managed application declares, per ring, *where* it lives and *how* to reach it —
namespace, Deployment, container, image repository, and a health URL. This lives
in **configuration**, not code.

### Promotion rules

- **One ring at a time; never skip.** `promote {from_ring}` always targets the
  *next* ring in the pipeline.
- **The source ring must be healthy** before a promotion proceeds (a live health
  check is performed).
- After deploying the source version to the target ring, the service runs a
  **health check with a configurable number of retries**.
- If the target is still unhealthy after all retries, it is **automatically
  rolled back** to its previous version.
- Every **seed / promote / rollback** is written to history, success or failure.

### Clean, swappable interfaces

| Concern      | Interface            | Production impl      | Local/dev impl        |
|--------------|----------------------|----------------------|-----------------------|
| Deploy       | `deployer.Deployer`  | `KubectlDeployer`    | `LogDeployer` (no-op) |
| Health check | `health.Checker`     | `HTTPChecker`        | `AlwaysHealthy`       |
| Persistence  | `store.Store`        | `Postgres`           | `Memory`              |

The `KubectlDeployer` shells out to `kubectl` (`set image` + `rollout status`),
authenticating in-cluster via the pod's ServiceAccount. It keeps the binary and
dependency tree small while getting battle-tested rollout semantics.

**Concurrency & reliability.**
- Operations on the same application are serialized by a lock obtained from the
  `Store`. The Postgres store uses a **session advisory lock**, so serialization
  holds **across replicas** — an accidental scale-up cannot run two concurrent
  promotions on the same app. (The in-memory store's lock is process-local, which
  is fine because it is single-process by nature.)
- A seed/promote/rollback runs under a context **detached from the HTTP request**
  and bounded by `operation_timeout`. A client disconnect or load-balancer idle
  timeout can therefore never abort an in-flight deploy or, critically, its
  automatic rollback.
- The stored state is updated as soon as a deploy lands, so it never lags the
  cluster even if a subsequent health check and rollback both fail.

`replicas: 1` is still the recommended default (simplest reasoning), but the
advisory lock means correctness no longer silently depends on it when using
Postgres.

---

## Project layout

```
cmd/ringpromoter/        main: config load, wiring, HTTP server, graceful shutdown
internal/
  ring/                  ordered ring pipeline (the ONE place to add a ring)
  config/                YAML + env config loading and validation
  store/                 Store interface, Memory + Postgres impls, schema.sql
  deployer/              Deployer interface, KubectlDeployer, LogDeployer
  health/                Checker interface, HTTPChecker, AlwaysHealthy
  promoter/              promotion rules (seed/promote/rollback) + unit tests
  api/                   REST handlers, bearer-token auth, request logging
  web/                   embedded single-page UI (vanilla JS)
deploy/k8s/              Namespace, RBAC, ConfigMap, Secret, Deployment, Service, Ingress
Dockerfile               small multi-stage image (distroless + kubectl)
config.yaml              local-development config (2 sample apps)
```

---

## Run locally

No cluster or database needed — the defaults use the no-op deployer, an
always-healthy checker and the in-memory store.

```bash
go run ./cmd/ringpromoter --config config.yaml
# -> http://localhost:8080  (UI + API), token: local-dev-token
```

Open the UI at <http://localhost:8080>, paste the token (`local-dev-token`) into
the token box, pick an app, and use the Seed / Promote / Rollback buttons.

Drive it from the CLI:

```bash
TOKEN=local-dev-token
BASE=http://localhost:8080

curl -s -H "Authorization: Bearer $TOKEN" $BASE/api/apps | jq

# seed ring0, then promote it up the pipeline
curl -s -H "Authorization: Bearer $TOKEN" -X POST \
  -d '{"ring":"ring0","version":"1.4.2"}' $BASE/api/apps/web-frontend/seed | jq
curl -s -H "Authorization: Bearer $TOKEN" -X POST \
  -d '{"from_ring":"ring0"}' $BASE/api/apps/web-frontend/promote | jq

curl -s -H "Authorization: Bearer $TOKEN" $BASE/api/apps/web-frontend/rings   | jq
curl -s -H "Authorization: Bearer $TOKEN" $BASE/api/apps/web-frontend/history | jq
```

To exercise the real backends locally, set `deployer: kubectl`, `health: http`
and `database.driver: postgres` (with a DSN) in the config or via env vars.

---

## REST API

All `/api` routes require `Authorization: Bearer <token>`. `/healthz` and the UI
are unauthenticated.

| Method & path                    | Body                  | Description                               |
|----------------------------------|-----------------------|-------------------------------------------|
| `GET  /healthz`                  | –                     | Service liveness (no auth).               |
| `GET  /api/apps`                 | –                     | List apps and the ring pipeline.          |
| `GET  /api/apps/{app}/rings`     | –                     | Per-ring state + live version & health.   |
| `GET  /api/apps/{app}/history`   | –                     | History, newest first.                    |
| `POST /api/apps/{app}/seed`      | `{"ring","version"}`  | Set an initial version for a ring.        |
| `POST /api/apps/{app}/promote`   | `{"from_ring"}`       | Promote to the next ring.                 |
| `POST /api/apps/{app}/rollback`  | `{"ring"}`            | Roll a ring back to its previous version. |

**Status codes.** `seed`/`promote`/`rollback` return **200** when the operation
succeeded (deployed and healthy), **422** when it ran but failed (e.g. health
check failed and the target was rolled back — details in the JSON body), and
**4xx** for precondition errors: `404` (unknown app/ring), `400` (empty version
/ promoting past the last ring), `409` (nothing to promote/roll back), `401`
(bad token). This lets CI treat a non-2xx response as "promotion failed".

Every mutating call returns a `Result` object:

```json
{
  "app": "web-frontend",
  "action": "promote",
  "ring": "ring1",
  "from_ring": "ring0",
  "version": "1.4.2",
  "success": true,
  "rolled_back": false,
  "message": "promoted 1.4.2 from ring0 to ring1 and healthy",
  "state": { "current_version": "1.4.2", "previous_version": "", "healthy": true }
}
```

---

## Add an application (config only)

No code change, no rebuild. Add an entry under `apps:` — locally in `config.yaml`,
in production in the `ring-promoter-config` ConfigMap:

```yaml
apps:
  - name: billing-worker
    rings:
      ring0:
        namespace: ring0
        deployment: billing-worker      # k8s Deployment to update
        container: worker               # container whose image tag is set
        image: registry.example.com/billing-worker
        health_url: http://billing-worker.ring0.svc.cluster.local/health
      ring1:
        namespace: ring1
        deployment: billing-worker
        container: worker
        image: registry.example.com/billing-worker
        health_url: http://billing-worker.ring1.svc.cluster.local/health
      # ...ring2, ring3
```

Apply and roll the pod:

```bash
kubectl apply -f deploy/k8s/configmap.yaml
kubectl rollout restart deploy/ring-promoter -n ring-system
```

An app does not need to define every ring — only the ones it lives in.

---

## Add a ring (single-place change)

The ring pipeline is the single source of truth in
[`internal/ring/ring.go`](internal/ring/ring.go). Add one line, in order:

```go
var ordered = []Ring{
    {Name: "ring0", Label: "Dev"},
    {Name: "ring1", Label: "Integration"},
    {Name: "ring2", Label: "Acceptance"},
    {Name: "ring2.5", Label: "Canary"},   // <-- new ring
    {Name: "ring3", Label: "Production"},
}
```

Everything derived from this — promotion order, API responses and the UI —
updates automatically. Then, operationally: create the ring's namespace and a
matching `RoleBinding` (see `deploy/k8s/`), and add the ring to each app's config
where it applies.

---

## Deploy on k3s

1. **Build and push the image** (multi-arch friendly):

   ```bash
   docker build -t registry.example.com/ring-promoter:1.0.0 .
   docker push registry.example.com/ring-promoter:1.0.0
   ```

2. **Provide a Postgres** reachable from the cluster and put its DSN in the
   Secret (`deploy/k8s/secret.yaml`). Set a strong `RP_API_TOKEN` there too.

3. **Edit the manifests**: set the real image in `deployment.yaml`, and your app
   registry in `configmap.yaml`.

4. **Apply**, in order:

   ```bash
   kubectl apply -f deploy/k8s/namespace.yaml
   kubectl apply -f deploy/k8s/rbac.yaml
   kubectl apply -f deploy/k8s/secret.yaml
   kubectl apply -f deploy/k8s/configmap.yaml
   kubectl apply -f deploy/k8s/deployment.yaml
   kubectl apply -f deploy/k8s/service.yaml
   kubectl apply -f deploy/k8s/ingress.yaml   # optional (Traefik)
   ```

5. **Reach it**: via the Ingress host (`ring-promoter.local`) or a port-forward:

   ```bash
   kubectl -n ring-system port-forward svc/ring-promoter 8080:80
   ```

The `KubectlDeployer` uses the `ring-promoter` ServiceAccount; the RBAC grants it
`patch`/`update` on Deployments (and read on ReplicaSets/Pods for rollout status)
in each ring namespace. It runs as a single non-root replica with a read-only
root filesystem. State/history live in Postgres, so restarts are safe.

---

## How an app's CI calls the API

After an application's CI builds and pushes a new image tag, it seeds ring0 and
lets the platform (or a human, via the UI) promote it onward. Example
(GitHub Actions style):

```bash
RP=https://ring-promoter.example.com
APP=web-frontend
TOKEN=${RING_PROMOTER_TOKEN}   # from CI secrets
VERSION=$GITHUB_SHA            # the image tag you just pushed

# Seed the new version into Dev. --fail makes CI fail on a non-2xx response.
curl --fail -sS -X POST "$RP/api/apps/$APP/seed" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"ring\":\"ring0\",\"version\":\"$VERSION\"}"

# (Optional) auto-promote to Integration once Dev is healthy.
curl --fail -sS -X POST "$RP/api/apps/$APP/promote" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"from_ring":"ring0"}'
```

Because a failed promotion returns a non-2xx status (with the target
auto-rolled-back), `curl --fail` surfaces it as a failed CI step. Higher rings
(Acceptance → Production) are typically promoted deliberately from the UI or a
separate approved job using the same `promote` call.

---

## Postgres schema

Applied automatically on start-up (idempotent). Source of truth:
[`internal/store/schema.sql`](internal/store/schema.sql).

```sql
CREATE TABLE IF NOT EXISTS ring_state (
    app              TEXT        NOT NULL,
    ring             TEXT        NOT NULL,
    current_version  TEXT        NOT NULL DEFAULT '',
    previous_version TEXT        NOT NULL DEFAULT '',
    healthy          BOOLEAN     NOT NULL DEFAULT FALSE,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (app, ring)
);

CREATE TABLE IF NOT EXISTS history (
    id           BIGSERIAL   PRIMARY KEY,
    app          TEXT        NOT NULL,
    ring         TEXT        NOT NULL,
    action       TEXT        NOT NULL,   -- seed | promote | rollback
    from_version TEXT        NOT NULL DEFAULT '',
    to_version   TEXT        NOT NULL DEFAULT '',
    result       TEXT        NOT NULL,   -- success | failure
    message      TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_history_app_created ON history (app, created_at DESC, id DESC);
```

---

## Configuration reference

Config comes from a YAML file; any value can be overridden by an environment
variable (env wins). Secrets should always come from the environment / a Secret.

| Env var           | Config key          | Default        | Notes                                  |
|-------------------|---------------------|----------------|----------------------------------------|
| `RP_LISTEN_ADDR`  | `listen_addr`       | `:8080`        | HTTP bind address.                     |
| `RP_API_TOKEN`    | `api_token`         | – (required)   | Bearer token for `/api`.               |
| `RP_DEPLOYER`     | `deployer`          | `log`          | `kubectl` or `log`.                    |
| `RP_HEALTH`       | `health`            | `always`       | `http` or `always`.                    |
| `RP_DB_DRIVER`    | `database.driver`   | `memory`       | `postgres` or `memory`.                |
| `RP_DB_DSN`       | `database.dsn`      | –              | Required for `postgres`.               |
| `RP_RETRY_COUNT`  | `retry.count`       | `3`            | Health retries after the first check. `0` = one check, no retries. |
| `RP_RETRY_DELAY`  | `retry.delay`       | `5s`           | Wait between retries.                   |
| `RP_OP_TIMEOUT`   | `operation_timeout` | `10m`          | Max time for one seed/promote/rollback (deploy + health + rollback). |
| `RP_CONFIG_FILE`  | – (flag `--config`) | `config.yaml`  | Path to the config file.               |

The application registry (`apps:`) lives in the file only.

---

## Testing

```bash
go test ./...          # all tests
go test -race ./...    # with the race detector
```

The promotion rules — including source-health gating, the retry loop, automatic
rollback, "never skip a ring", and concurrency safety — are covered in
[`internal/promoter/promoter_test.go`](internal/promoter/promoter_test.go).
