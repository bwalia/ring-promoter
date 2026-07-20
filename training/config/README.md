# Training instance configuration

[`apps.training.yaml`](./apps.training.yaml) is the full Ring Promoter config
for the training instance at **https://ring-promoter.fictionally.org**. It
registers all seven sample apps so the whole feature surface can be demonstrated
live (see the [feature matrix](../sample-apps/README.md#feature-coverage-matrix)).

## Conventions it assumes

- Each app is deployed **per ring** into namespace `<app>-<ring>` (e.g.
  `bank-api-acc`), with the Helm release name `<app>` and
  `--set fullnameOverride=<app>`, so the Deployment/Service is exactly `<app>`
  and the config's `deployment:` / `health_url:` values line up:

  ```bash
  helm upgrade --install bank-api ./training/sample-apps/bank-api/chart \
    --namespace bank-api-acc --create-namespace \
    --set fullnameOverride=bank-api \
    --set image.tag=v1.4.2 \
    --set ingress.host=bank-api-acc.fictionally.org
  ```

- Health checks target in-cluster Service DNS
  (`http://<app>.<app>-<ring>.svc.cluster.local/...`), reachable from the Ring
  Promoter control-plane pod.

## Secrets (never in this file)

| Env var | Purpose |
|---------|---------|
| `RP_API_TOKEN` | Bearer token for `/api` |
| `RP_DB_DSN` | Postgres DSN for Ring Promoter's own state |
| `RP_PROD_PASSWORD` | Extra password for production deploys |
| `RP_GITHUB_TOKEN` | `github` deployer (operator) |
| `RP_JIRA_TOKEN` | change-request (JIRA) gate (bank-api) |
| `RP_OLLAMA_JWT_SECRET` | enables AI failure diagnosis |

## Validate it locally

```bash
RP_API_TOKEN=dev RP_DB_DRIVER=memory RP_HEALTH=always \
RP_GITHUB_TOKEN=dummy RP_JIRA_TOKEN=dummy \
go run ./cmd/ringpromoter -config training/config/apps.training.yaml
# starts, validates the config (all 7 apps + gates), serves :8080 — Ctrl-C to stop
```

The training instance mounts this file from a ConfigMap — see the
[deployment runbook](../../deploy/instances/fictionally/README.md).
