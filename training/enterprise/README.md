# Enterprise path (~1 day)

For platform teams standardising release promotion across many apps and teams.

Topics:
- **Governance** — promotion policies as code; who can promote to prod; the
  production password; auditability via history.
- **RBAC** — least-privilege deployer RBAC per namespace (vs the training
  cluster-wide grant); the k8sjob `ring-deploy-job` identity separation.
- **High availability & DR** — single-writer control plane + Postgres advisory
  locks; backup/restore of the Postgres state; multi-cluster considerations.
- **Scaling** — many apps × rings; groups; naming conventions.
- **Onboarding your own apps** — copy a [sample-app](../sample-apps) shape, add
  a `promotion_policy`, register in the [training config](../config).
- **Running your own instance** — [deployment runbook](../../deploy/instances/fictionally/README.md).

Delivered as the [2-day / enterprise workshop](../workshops); pair with your own
cluster and app inventory.
