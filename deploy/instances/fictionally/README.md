# Deploy the training Ring Promoter instance — ring-promoter.fictionally.org

A **new, separate** Ring Promoter instance on k3s1 that manages the seven
training apps. It does not touch the workstation instance
(`workstation-ring-promoter`) or the diytaxreturn instance (`ring-system`).

## Automated (recommended): push to main

`.github/workflows/deploy-training-k3s1.yml` does everything on every push to
`main` (self-hosted `mac-studio` runner): test + validate the training config →
build/push the image → **`helm upgrade --install`** the
[`deploy/helm/ring-promoter`](../../helm/ring-promoter) chart (Deployment,
Service, Traefik Ingress, RBAC, and the app-registry ConfigMap injected from the
training config via `--set-file` — single source of truth) → ensure the
Cloudflare **CNAME → pop0.wslproxy.com** → register the **wslproxy** vhost via
its admin API → verify in-cluster and at the public URL.

The manifests live in a **Helm chart**, so this instance can equally be deployed
by **ArgoCD** — apply [`deploy/argocd/ring-promoter-fictionally.yaml`](../../argocd/ring-promoter-fictionally.yaml).

Deploy by hand with the same chart:

```bash
helm upgrade --install ring-promoter deploy/helm/ring-promoter \
  -f deploy/helm/ring-promoter/values-fictionally.yaml \
  --set-file config=training/config/apps.training.yaml \
  --set image.tag=<tag> -n fictionally-ring-promoter --create-namespace
```

**Repo secrets** (`gh secret set … -R bwalia/ring-promoter`):

| Secret | Set? | Purpose |
|--------|------|---------|
| `KUBE_CONFIG_DATA_K3S1` | ✅ set | base64 kubeconfig for k3s1 (shared) |
| `DOCKER_USER` / `DOCKER_PASSWD` | ✅ set | Docker Hub push |
| `CF_API_TOKEN` | optional | Cloudflare DNS:Edit on `fictionally.org`. **Not required** — external-dns already maintains the CNAME from the Ingress annotations; the workflow soft-skips without it. |
| `WSLPROXY_USER` / `WSLPROXY_PASSWORD` | for edge | wslproxy admin login (email + password). The workflow logs in and upserts the vhost for the host via the wslproxy admin API (`/api/user/login` → `/api/servers`), mirroring `wslproxy/api-scripts`. Without them the step warns and you register the vhost manually ([wslproxy-vhost.md](./wslproxy-vhost.md)). |
| `WSLPROXY_GATEWAY_URL` | optional | wslproxy admin gateway base URL (defaults to `https://pop0.wslproxy.com`). |

**One-time bootstrap the workflow can't do** (CI never creates secrets): create
the Postgres DB/role `ringpromoter_training` and the `ring-promoter` Secret in
the namespace (steps 1–2 below). After that, merging to `main` deploys.

> DNS note: `ring-promoter.fictionally.org` already resolves to
> `pop0.wslproxy.com` (verified). The remaining gate is the wslproxy vhost — the
> public health check fails with "Host not configured" until it's registered.

---

## Manual runbook (for the first bootstrap, or a cluster without the workflow)

| | |
|---|---|
| Namespace | `fictionally-ring-promoter` (+ `ring-exec` for k8sjob Jobs) |
| Public URL | `https://ring-promoter.fictionally.org` |
| In-cluster ingress | Traefik (`ingressClassName: traefik`) |
| DNS | Cloudflare **CNAME → pop0.wslproxy.com** (via external-dns) |
| Edge | wslproxy vhost on pop0 (see [`wslproxy-vhost.md`](./wslproxy-vhost.md)) |
| Store | Postgres (own DB `ringpromoter_training`) |

> These are the artifacts + runbook. Applying them to the live cluster,
> creating the real Secret, publishing DNS, and registering the wslproxy vhost
> are steps **you** run — they need cluster credentials and are live prod
> changes.

## 1. Namespaces + RBAC

```bash
kubectl apply -f deploy/instances/fictionally/namespace.yaml
kubectl apply -f deploy/instances/fictionally/rbac.yaml
```

## 2. Secret (do NOT apply secret.yaml as-is — it holds placeholders)

Create the real Secret by hand (or via a sealed-secret / external secret
manager):

```bash
kubectl -n fictionally-ring-promoter create secret generic ring-promoter \
  --from-literal=RP_API_TOKEN="$(openssl rand -hex 32)" \
  --from-literal=RP_DB_DSN="postgres://ringpromoter_training:REAL@ring-promoter-db.ring-system.svc.cluster.local:5432/ringpromoter_training?sslmode=require" \
  --from-literal=RP_GITHUB_TOKEN="<pat actions:write on rp-training-operator>" \
  --from-literal=RP_JIRA_TOKEN="<jira api token>" \
  --from-literal=RP_PROD_PASSWORD="<prod password>"
```

Create the Postgres database/role `ringpromoter_training` on the shared
`ring-promoter-db` first (the schema is applied automatically on start-up).

## 3. Config (from the training config, no duplication)

```bash
kubectl -n fictionally-ring-promoter create configmap ring-promoter-config \
  --from-file=config.yaml=training/config/apps.training.yaml \
  --dry-run=client -o yaml | kubectl apply -f -
```

## 4. Control plane

```bash
kubectl apply -f deploy/instances/fictionally/service.yaml
kubectl apply -f deploy/instances/fictionally/deployment.yaml
kubectl apply -f deploy/instances/fictionally/ingress.yaml
kubectl -n fictionally-ring-promoter rollout status deploy/ring-promoter
```

## 5. DNS + edge

- **DNS**: external-dns publishes the Cloudflare CNAME from the Ingress
  annotations (`→ pop0.wslproxy.com`). Confirm the `fictionally.org` zone is
  managed by external-dns.
- **wslproxy vhost**: register `ring-promoter.fictionally.org` on the pop0 edge —
  see [`wslproxy-vhost.md`](./wslproxy-vhost.md).

## 6. Verify

```bash
kubectl -n fictionally-ring-promoter get deploy,po,svc,ingress
curl -sS https://ring-promoter.fictionally.org/healthz          # {"status":"ok"}
TOKEN=$(kubectl -n fictionally-ring-promoter get secret ring-promoter -o jsonpath='{.data.RP_API_TOKEN}' | base64 -d)
curl -s -H "Authorization: Bearer $TOKEN" \
  https://ring-promoter.fictionally.org/api/apps | jq '.apps'
# all 7 training apps
```

## 7. Deploy the sample apps

Each app is deployed per ring with its Helm chart — see
[`training/config/README.md`](../../../training/config/README.md) and
[Lab 01](../../../training/labs/lab-01-first-deployment.md).

## 8. Special-deployer prerequisites (image-proc + operator)

Five apps use the in-process kubectl deployer and need nothing extra. The two
"advanced deployer" apps each need a one-time bootstrap, without which they
cannot be seeded/promoted (they were originally excluded from the seed workflow
for exactly this reason):

- **image-proc (`k8sjob`)** — each deploy runs as a Job in `ring-exec` under the
  `ring-deploy-job` ServiceAccount. Apply that SA + RBAC:

  ```bash
  kubectl apply -f deploy/instances/fictionally/ring-exec-rbac.yaml
  ```

  The deploy image is public (`docker.io/dtzar/helm-kubectl`); no pull secret is
  required. If you swap in a private deploy image, add a pull secret to
  `ring-exec` and attach it to `ring-deploy-job` (see that file's header).

- **operator (`github`)** — the deployer dispatches `release.yml` in
  `bwalia/rp-training-operator` on the deployed version's tag (`version_as_ref`)
  and waits for the run to succeed. It requires:
  - the **repo to exist** with `release.yml` (declaring the `ENV`,
    `DEPLOY_BRANCH`, `DEPLOY_MODE` workflow_dispatch inputs) and a **`v1` tag**;
  - the repo **public** (or an Actions budget on the account — private-repo
    Actions consume billed minutes and fail closed when the budget is spent);
  - a real **`RP_GITHUB_TOKEN`** in the `ring-promoter` Secret with rights to
    resolve refs and dispatch Actions on that repo (the placeholder `ci` token
    only works for the in-memory CI smoke test).

  Prod is `maintenance_window`-gated: open a window before seeding/promoting to
  `prod` (`POST /api/apps/operator/maintenance-windows`).

## Rebuild the Ring Promoter image (if needed)

The image `docker.io/bwalia/ring-promoter:latest` is built from the repo root
`Dockerfile` (embeds the web UI). Pin a real tag in `deployment.yaml` for
production rather than `:latest`.
