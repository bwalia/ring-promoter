# Ring Promoter

A small, production-grade control plane that promotes versions of **multiple
applications** through an ordered set of deployment **rings**:

```
int (Integration) → test (Test) → acc (Acceptance) → prod (Production)
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
- [Deploy a VM/CI app (e.g. wslproxy)](#deploy-a-vmci-app-eg-wslproxy)
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

| Concern      | Interface            | Production impls                          | Local/dev impl        |
|--------------|----------------------|-------------------------------------------|-----------------------|
| Deploy       | `deployer.Deployer`  | `KubectlDeployer`, `GitHubActionsDeployer`| `LogDeployer` (no-op) |
| Health check | `health.Checker`     | `HTTPChecker`                             | `AlwaysHealthy`       |
| Persistence  | `store.Store`        | `Postgres`                                | `Memory`              |

The `KubectlDeployer` shells out to `kubectl` (`set image` + `rollout status`),
authenticating in-cluster via the pod's ServiceAccount. It keeps the binary and
dependency tree small while getting battle-tested rollout semantics.

The `GitHubActionsDeployer` targets applications that run on **VMs** (not
Kubernetes) but already have a CI/CD pipeline — it triggers that pipeline via
the GitHub Actions **workflow-dispatch** API and waits for the run to conclude,
returning an error unless it succeeds (so the same health-check + auto-rollback
logic applies). See [Deploy a VM/CI app](#deploy-a-vmci-app-e.g-wslproxy).

The deployer is selected **per application** (via an optional `deployer:` field
in the app's config), so a single control plane can promote Kubernetes apps and
VM/CI apps side by side. Apps without the field use the global `deployer`.

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
deploy/k8s/              Namespace, RBAC, ConfigMap, Secret, Deployment, Service
kubernetes/ingress/      public Ingress (wslproxy) for ring-promoter.diytaxreturn.co.uk
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

# seed int, then promote it up the pipeline
curl -s -H "Authorization: Bearer $TOKEN" -X POST \
  -d '{"ring":"int","version":"1.4.2"}' $BASE/api/apps/web-frontend/seed | jq
curl -s -H "Authorization: Bearer $TOKEN" -X POST \
  -d '{"from_ring":"int"}' $BASE/api/apps/web-frontend/promote | jq

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
| `GET  /api/apps/{app}/jobs/{id}` | –                     | Live job status (steps, logs, result).    |
| `POST /api/apps/{app}/seed`      | `{"ring","version"}`  | Set an initial version for a ring.        |
| `POST /api/apps/{app}/promote`   | `{"from_ring"}`       | Promote to the next ring.                 |
| `POST /api/apps/{app}/rollback`  | `{"ring"}`            | Roll a ring back to its previous version. |

**Sync vs async.** `seed`/`promote`/`rollback` run **synchronously** by default and
return the final `Result` (200 success / 422 ran-but-failed) — ideal for CI
(`curl --fail`). Add `?async=1` to run in the background: the call returns **202**
with `{"job_id": "..."}` immediately, and you poll `GET /api/apps/{app}/jobs/{id}`
for live step-by-step progress (per-step status + logs and the final result). The
web UI uses the async path to render its live deployment view.

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
  "ring": "test",
  "from_ring": "int",
  "version": "1.4.2",
  "success": true,
  "rolled_back": false,
  "message": "promoted 1.4.2 from int to test and healthy",
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
      int:
        namespace: int
        deployment: billing-worker      # k8s Deployment to update
        container: worker               # container whose image tag is set
        image: registry.example.com/billing-worker
        health_url: http://billing-worker.int.svc.cluster.local/health
      test:
        namespace: test
        deployment: billing-worker
        container: worker
        image: registry.example.com/billing-worker
        health_url: http://billing-worker.test.svc.cluster.local/health
      # ...acc, prod
```

Apply and roll the pod:

```bash
kubectl apply -f deploy/k8s/configmap.yaml
kubectl rollout restart deploy/ring-promoter -n ring-system
```

An app does not need to define every ring — only the ones it lives in.

---

## Deploy a VM/CI app (e.g. wslproxy)

Some applications don't run on Kubernetes — they run on **VMs** and are deployed
by their own CI/CD. `wslproxy` is one: an OpenResty/Lua app rolled out to VM
hosts (pipeline `int → test → prod`) by GitHub Actions that builds the requested
ref and reloads OpenResty on each host.

Such an app opts into the **`github` deployer** with a per-app `deployer:` field
and a `github:` block. No code change — it is pure configuration:

```yaml
apps:
  - name: wslproxy
    deployer: github                      # override the global deployer
    github:
      owner: bwalia
      repo: wslproxy
      workflow: deploy-single-environment.yml   # per-env deploy, no cascade
      ref: main                           # branch hosting the workflow (default)
      deploy_mode: full                   # value sent as the DEPLOY_MODE input
      token_env: RP_GITHUB_TOKEN          # env holding the API token (Secret)
      poll_interval: 20s                  # how often the run is polled
      run_lookup_timeout: 90s             # wait for the dispatched run to appear
      # Dispatch input names default to ENV / DEPLOY_BRANCH / DEPLOY_MODE;
      # override with env_input / version_input / mode_input for other workflows.
    rings:                                # target_env is sent as the ENV input
      int:  { target_env: int,  health_url: "https://int-our.wslproxy.com/healthz" }
      test: { target_env: test, health_url: "https://test.wslproxy.com/healthz" }
      acc:  { target_env: acc,  health_url: "https://prod-our-v1.wslproxy.com/healthz" }  # pop0
      prod: { target_env: prod, health_url: "https://pop1.diytaxreturn.co.uk/healthz" }   # pop1
```

How it maps onto Ring Promoter:

- **A "version"** is a git **branch, tag or commit SHA** — passed as the
  workflow's `DEPLOY_BRANCH` input (handed to `actions/checkout`). You `seed` it
  into `int` and `promote` the exact same value onward.
- **Each ring's `target_env`** becomes the workflow's `ENV` input, so `int/1/2`
  deploy to `int/test/prod` respectively. `prod` is left undefined for this app.
  (Rings are consecutive so "never skip a ring" holds.)
- **The workflow deploys exactly one environment** (`ENV=prod` fans out to all
  three prod hosts). This independence matters: a plain `int` deploy must not
  cascade to prod, and a prod auto-rollback must not revert `int`/`test`. See the
  note below on why a dedicated single-environment workflow is required.
- **Deploy** = dispatch the workflow, then poll the resulting run. A non-`success`
  conclusion is an error, so a failed run (or a failed post-deploy health check)
  triggers the usual **auto-rollback** — which re-dispatches the workflow for the
  previous version, against that ring's environment only.
- **Health** uses the standard `HTTPChecker` against each ring's `health_url`
  (OpenResty serves `/health` via `api/ping.lua`, returning `200 {"status":"healthy"}`).

> **Use a single-environment workflow, not the full delivery pipeline.**
> wslproxy's `deploy-wslproxy-delivery-pipeline.yml` is a cumulative promotion
> cascade: `deploy-int` runs on every dispatch and each later stage `needs:` the
> previous, gated by `TARGET_HOST` (its `TARGET_ENV` input is cosmetic). Pointing
> the deployer at it would make a single-ring deploy run the whole chain to
> production. The companion `deploy-single-environment.yml`
> ([wslproxy PR #1121](https://github.com/bwalia/wslproxy/pull/1121)) deploys one
> `ENV` in isolation, reusing the same per-host settings — that is what this
> config targets.

**Token.** The `github` deployer authenticates with the token in the env var
named by `token_env` (default `RP_GITHUB_TOKEN`), injected from the Secret. It
needs `actions:write` (dispatch) and `contents:read` on the repo — a fine-grained
PAT or a GitHub App token. It is never stored in the ConfigMap.

**Drive it** exactly like any other app — the mechanism is transparent to the API:

```bash
RP=https://ring-promoter.example.com; APP=wslproxy; TOKEN=$RING_PROMOTER_TOKEN
# Seed a ref into int, then promote int -> test -> prod.
curl --fail -sS -X POST "$RP/api/apps/$APP/seed" \
  -H "Authorization: Bearer $TOKEN" -d '{"ring":"int","version":"release-1.0.10"}'
curl --fail -sS -X POST "$RP/api/apps/$APP/promote" \
  -H "Authorization: Bearer $TOKEN" -d '{"from_ring":"int"}'   # -> test
curl --fail -sS -X POST "$RP/api/apps/$APP/promote" \
  -H "Authorization: Bearer $TOKEN" -d '{"from_ring":"test"}'   # -> prod (all hosts)
```

> One-instance caveat: matching the dispatched run relies on it being the newest
> `workflow_dispatch` run for that workflow created after Ring Promoter fired it.
> Ring Promoter serializes operations per app, so it never races itself; a human
> manually dispatching the *same* workflow at the *same instant* is the only way
> to confuse the match. The matched run's URL is logged so it can be verified.

---

## Add a ring (single-place change)

The ring pipeline is the single source of truth in
[`internal/ring/ring.go`](internal/ring/ring.go). Add one line, in order:

```go
var ordered = []Ring{
    {Name: "int", Label: "Integration"},
    {Name: "test", Label: "Test"},
    {Name: "acc", Label: "Acceptance"},
    {Name: "canary", Label: "Canary"},   // <-- new ring
    {Name: "prod", Label: "Production"},
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
   kubectl apply -f kubernetes/ingress/ring-promoter.yaml   # public host via the wslproxy ingress
   ```

5. **Reach it**: via the Ingress host (`http://ring-promoter.diytaxreturn.co.uk`)
   or a port-forward:

   ```bash
   kubectl -n ring-system port-forward svc/ring-promoter 8080:80
   ```

The `KubectlDeployer` uses the `ring-promoter` ServiceAccount; the RBAC grants it
`patch`/`update` on Deployments (and read on ReplicaSets/Pods for rollout status)
in each ring namespace. It runs as a single non-root replica with a read-only
root filesystem. State/history live in Postgres, so restarts are safe.

---

## How an app's CI calls the API

After an application's CI builds and pushes a new image tag, it seeds int and
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
  -d "{\"ring\":\"int\",\"version\":\"$VERSION\"}"

# (Optional) auto-promote to Integration once Dev is healthy.
curl --fail -sS -X POST "$RP/api/apps/$APP/promote" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"from_ring":"int"}'
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
| `RP_DEPLOYER`     | `deployer`          | `log`          | Global default: `kubectl`, `log` or `github`. Overridable per app via `deployer:`. |
| `RP_GITHUB_TOKEN` | – (per-app `token_env`) | –          | API token for apps using the `github` deployer (needs `actions:write` + `contents:read`). From a Secret. |
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
