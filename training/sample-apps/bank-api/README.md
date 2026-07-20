# bank-api

> **Ring Promoter feature focus:** the high-governance app. bank-api
> demonstrates **all three** promotion gates — `maintenance_window`,
> `qa_signoff`, and `change_request` (provider `jira`) — **plus** the
> production password, Kubernetes Secrets, and database migrations. Promoting a
> version into `acc`/`prod` requires an OPEN maintenance window **and** a
> release-engineer GO sign-off for the exact version **and** a valid
> change-request (CR) code — and, for `prod`, the production password.

A small Go REST API backed by PostgreSQL. It issues JWTs and reads account
balances, and it is deliberately resilient: **it never crashes when the database
is down** — `/healthz` stays green, `/readyz` reports not-ready, and balance
reads fall back to a demo value. It copies the shape of the
[hello-world reference app](../hello-world/) (Go service + Dockerfile + Helm
chart + CI + `architecture.md`) and adds a real dependency, migrations, and
secrets.

## Endpoints

| Path                          | Auth  | Purpose                                                       |
|-------------------------------|-------|--------------------------------------------------------------|
| `POST /login`                 | none  | Validates the demo user, returns an HS256 JWT (`JWT_SECRET`) |
| `GET /accounts/{id}/balance`  | JWT   | Balance from Postgres, or a demo balance if the DB is down   |
| `/healthz`                    | none  | `{"status":"ok","version":"..."}` — liveness + version       |
| `/readyz`                     | none  | `200` only when the database is reachable, else `503`        |
| `/metrics`                    | none  | Prometheus text (`bankapi_*`, `bankapi_database_up`, build info) |

The version comes from `RP_VERSION` (set by the Helm chart / Ring Promoter to
the deployed tag), falling back to the `-X main.version` build ldflag, then
`dev`. Because `/healthz` echoes it, the Ring Promoter ring config uses
`health_version_field: version` so a promotion only "sticks" once the new build
is genuinely live.

## Configuration (all from env / a Secret)

| Variable          | Default            | Notes                                                  |
|-------------------|--------------------|--------------------------------------------------------|
| `DATABASE_URL`    | *(empty)*          | lib/pq DSN. Empty/unreachable = degraded, never a crash |
| `JWT_SECRET`      | insecure dev default | HS256 signing key. **Set from a Secret in real rings** |
| `RUN_MIGRATIONS`  | `false`            | `true` applies embedded migrations on boot (best-effort) |
| `DEMO_USER`       | `demo`             | Login username for labs                                |
| `DEMO_PASSWORD`   | `demo`             | Login password for labs                                |
| `PORT`            | `8080`             | Listen port                                            |

`DATABASE_URL` and `JWT_SECRET` come from a Kubernetes **Secret**. The chart can
render a placeholder Secret (`change-me`) for int/test labs; **real secrets
NEVER live in git** — see [Secrets](#secrets-never-commit-real-ones).

## Run it locally

```bash
# No database — starts degraded (readyz not-ready), balances use the demo value.
JWT_SECRET=dev go run ./cmd            # http://localhost:8080
curl -s localhost:8080/healthz         # {"status":"ok","version":"dev"}

# Log in and read a balance:
TOKEN=$(curl -s -X POST localhost:8080/login \
  -d '{"username":"demo","password":"demo"}' | jq -r .token)
curl -s -H "Authorization: Bearer $TOKEN" localhost:8080/accounts/1/balance

# With a real database:
docker run -d --name pg -e POSTGRES_DB=bank -e POSTGRES_USER=bank \
  -e POSTGRES_PASSWORD=secret -p 5432:5432 postgres:16-alpine
export DATABASE_URL="postgres://bank:secret@localhost:5432/bank?sslmode=disable"
go run ./cmd migrate                   # apply migrations + seed
JWT_SECRET=dev go run ./cmd            # now /readyz is ready, balance is from the DB
```

## Database migrations

`db/migrations/0001_init.sql` creates the `accounts` table and seeds one row.
The migrations are **embedded into the binary** (`//go:embed`), so they travel
with the image. There are two ways to apply them:

1. **Migrate Job (canonical).** The chart renders a Kubernetes `Job` (a Helm
   `pre-install`/`pre-upgrade` hook) that runs the app image as
   `bank-api migrate`, applying the embedded migrations before the app rolls
   out. One image, one source of truth for the SQL.
2. **On boot (optional).** Set `RUN_MIGRATIONS=true` (`migrations.runOnBoot` in
   the chart) and the app applies them at startup, best-effort — it still boots
   if the DB is not yet up.

See [`architecture.md`](./architecture.md) for the full rationale.

## Secrets (never commit real ones)

`chart/templates/secret.yaml` renders a Secret with **placeholder** values
(`change-me`) and is gated by `secret.create`:

- **int/test labs:** `secret.create=true` (default) — placeholders are fine.
- **acc/prod:** `secret.create=false` and reference an existing Secret
  (`secret.name`) that you manage out-of-band — a **SealedSecret** or
  external-secrets — or inject real values from Ring Promoter's `RP_*`
  environment at deploy time. Real credentials are **never** committed to git.

## Ephemeral PostgreSQL (training only)

The chart deploys `postgres:16-alpine` as an ephemeral `Deployment`+`Service`
(`emptyDir`, data not persisted), gated by `postgres.enabled` (default `true`).
**In production set `postgres.enabled=false`** and point `DATABASE_URL` at a
managed database (RDS / Cloud SQL / Crunchy, ...).

## Build the image

```bash
docker build --build-arg VERSION=v0.1.0 -t ghcr.io/bwalia/rp-training-bank-api:v0.1.0 .
```

## Deploy via Helm (what Ring Promoter does per ring)

```bash
helm upgrade --install bank-api ./chart \
  --namespace bank-api-int --create-namespace \
  --set image.tag=v0.1.0 \
  --set ingress.host=bank-api.fictionally.org
```

The Ingress uses `ingressClassName: traefik` (k3s1) and annotates the host so
**external-dns publishes a Cloudflare CNAME → `pop0.wslproxy.com`** (the wslproxy
POP). See [`architecture.md`](./architecture.md).

## How Ring Promoter governs it

Registered in [`training/config/apps.training.yaml`](../../config/apps.training.yaml)
with four rings (int → test → acc → prod) and a `promotion_policy` that enables
**all three gates**. Promoting a version into a guarded ring is blocked until
every gate is satisfied:

| Gate                 | What it requires                                                            |
|----------------------|-----------------------------------------------------------------------------|
| `maintenance_window` | An **open** window for the ring (a recurring config window **or** an operator-opened ad-hoc one). |
| `qa_signoff`         | A recorded **GO sign-off for the exact version** by a release engineer.     |
| `change_request`     | A valid **CR code** (provider `jira`; the demo code `test` is always accepted). |
| production password  | For `prod` only: the operator must supply Ring Promoter's **production password**. |

So a promotion to `prod` needs, together: an open maintenance window **+** a
release-engineer GO sign-off for that exact version **+** a valid CR code **+**
the production password. This is the strictest workload in the academy — walk it
in the governance lab.
