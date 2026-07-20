# Docs

Reference material for the academy. Start with [architecture](./architecture.md).

| Doc | Covers |
|-----|--------|
| [architecture.md](./architecture.md) | Components, promotion decision flow, core ideas |
| Promotion engine | Rules: one-ring-at-a-time, source-health, auto-rollback (see `internal/promoter` + [repo README](../../README.md)) |
| Deployers | kubectl / github / k8sjob — [sample-apps](../sample-apps) demonstrate each |
| Promotion gates | Maintenance windows, QA/release sign-off, change-request — [repo README](../../README.md#promotion-gates-promotion_policy) |
| Canary & blue/green | [shopping-cart architecture](../sample-apps/shopping-cart) |
| GitOps & release trains | This training config + per-app CI |

Deeper design rationale lives in the main [repository README](../../README.md)
and `docs/` at the repo root (e.g. the Kubernetes executor design).
