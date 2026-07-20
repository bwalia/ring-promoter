# Sample applications

Seven applications that, **between them, demonstrate every Ring Promoter
capability**. Each directory is shaped like the app's own repository (source +
Dockerfile + Helm chart + CI + docs) — in production each would live in its own
repo and Ring Promoter would promote it across the shared rings.

All apps share the same conventions (see [`hello-world`](./hello-world), the
reference template):

- A `/healthz` endpoint that reports the running version, so Ring Promoter's
  version-aware health checks can confirm the promoted build actually went live.
- A Helm chart exposing the app via `ingressClassName: traefik` (k3s1), with
  **external-dns publishing a Cloudflare CNAME → `pop0.wslproxy.com`** (the
  wslproxy POP) for each host under `fictionally.org`.
- Prometheus `/metrics`, readiness/liveness probes, a non-root read-only
  security context, and resource requests/limits.

## The apps

| App | Stack | Ring Promoter feature focus |
|-----|-------|-----------------------------|
| [hello-world](./hello-world) | Go (stdlib) | seed/promote/rollback, `health_version_field`, auto-promote — the template |
| [kuard](./kuard) | Upstream image | kubectl deployer with a 3rd-party image, **status-only** health, blue/green via tag |
| [shopping-cart](./shopping-cart) | Go + Redis + web | **groups**, canary + blue/green, version-field health |
| [bank-api](./bank-api) | Go + Postgres + JWT | **all three promotion gates** + production password + secrets + migrations |
| [image-proc](./image-proc) | Go API + worker + queue | **k8sjob deployer**, async worker scaling, `health_version_header` |
| [ai-chat](./ai-chat) | Go + Ollama | **ref-pinned ring**, AI failure diagnosis, optional GPU, version-field health |
| [operator](./operator) | Go operator + CRD | **github deployer** (`version_as_ref`), operator lifecycle, maintenance-window gate |

## Feature-coverage matrix

Proof that the set exercises everything Ring Promoter does. Registered in
[`../config/apps.training.yaml`](../config/apps.training.yaml).

| Ring Promoter capability | Demonstrated by |
|--------------------------|-----------------|
| Seed a version | all |
| Promote ring→ring (int→test→acc→prod) | all |
| Rollback | all |
| Auto-promote chains | hello-world, kuard |
| **kubectl** deployer | hello-world, kuard, shopping-cart, bank-api, ai-chat |
| **github** deployer (`version_as_ref`) | operator |
| **k8sjob** deployer (deploy as a Kubernetes Job) | image-proc |
| Health: version via JSON field (`health_version_field`) | hello-world, shopping-cart, bank-api, ai-chat |
| Health: version via response header (`health_version_header`) | image-proc |
| Health: status-only (no version) | kuard, operator |
| Health: custom expected status (`health_expect_status`) | documented example (see config comments) |
| Ref-pinned ring (`ref:`) | ai-chat (acc → `release`) |
| Promotion gate — maintenance windows (config + ad-hoc) | bank-api, operator |
| Promotion gate — QA/release Go-No-Go sign-off | bank-api |
| Promotion gate — change-request code (JIRA) | bank-api |
| Production password | bank-api / instance-wide (prod) |
| Application groups | shopping-cart (see Lab 07) |
| AI failure diagnosis (Ollama) | ai-chat ecosystem / instance-wide |
| Display names | all |
| Secrets & DB migrations | bank-api |

See the [labs](../labs) to exercise each of these hands-on.
