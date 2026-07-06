# Deploying Ring Promoter to k3s1 (CI/CD)

Ring Promoter deploys itself to the **k3s1** cluster automatically when a PR is
merged to `main`, via `.github/workflows/deploy-k3s1.yml`. `main` is the source
of truth (GitOps): the workflow applies the manifests in `deploy/k8s/` (and the
ingress) and rolls the Deployment to an immutable image tag.

## How the pipeline works

Trigger: **push to `main`** (i.e. a merged PR) or manual `workflow_dispatch`
(with an optional `image_tag` to redeploy a specific build). It runs on the
self-hosted **Mac Studio** runner (`runs-on: [self-hosted, mac-studio]`).

1. **test** — `go vet ./...` + `go test ./...`. A failing test blocks the deploy.
2. **deploy**
   - Build + push `docker.io/bwalia/ring-promoter` (`linux/amd64`) with tags
     `sha-<short>` (immutable) and `latest` (on `main`), injecting
     `VERSION/GIT_COMMIT/BUILD_TIME` (shown at `/version` and in the UI footer).
   - Load the k3s1 kubeconfig from the `KUBE_CONFIG_DATA_K3S1` secret.
   - **Preflight**: fail fast if `secret/ring-promoter` is missing (CI never
     creates secrets).
   - **Apply** `namespace.yaml`, `rbac.yaml`, `configmap.yaml`, `service.yaml`,
     and `kubernetes/ingress/ring-promoter.yaml`. **`secret.yaml` is never
     applied** (it holds placeholders and would clobber the real Secret).
   - Roll the Deployment to `…:sha-<short>` and wait for `rollout status`.
   - Health-check `http://ring-promoter.diytaxreturn.co.uk/healthz`.

> **GitOps caveat:** the ConfigMap (the app registry) is applied from the repo
> every run — make app-registry changes in `deploy/k8s/configmap.yaml`, not by
> editing the live ConfigMap on the cluster (edits there are overwritten).

## One-time bootstrap (before the first run)

Do this once by hand; CI takes over afterwards.

1. **Postgres on k3s1** — ensure a Postgres is reachable from the cluster (e.g.
   the Zalando operator as on k3s0, or a managed instance) and note its DSN.

2. **Create the real Secret** (never committed):
   ```bash
   kubectl --kubeconfig ~/.kube/k3s1.yaml -n ring-system create namespace ring-system 2>/dev/null || true
   kubectl --kubeconfig ~/.kube/k3s1.yaml -n ring-system create secret generic ring-promoter \
     --from-literal=RP_API_TOKEN='<a-strong-random-token>' \
     --from-literal=RP_DB_DSN='postgres://<user>:<pass>@<host>:5432/ringpromoter?sslmode=require' \
     --from-literal=RP_GITHUB_TOKEN='<github-dispatch-token>'
   ```
   (`RP_GITHUB_TOKEN` needs `actions:write` + `contents:read` on the repos of any
   `deployer: github` app — e.g. `bwalia/wslproxy`, `bwalia/diy-tax-return-uk`.)

3. **GitHub repo secrets** (Settings → Secrets and variables → Actions):
   - `DOCKER_USER`, `DOCKER_PASSWD` — Docker Hub push creds (org already uses these).
   - `KUBE_CONFIG_DATA_K3S1` — base64 of the public-endpoint kubeconfig:
     ```bash
     base64 -i ~/.kube/k3s1.yaml | pbcopy   # macOS; paste as the secret value
     ```
     (Its server must be `https://k3s1api.diytaxreturn.co.uk:6443`, reachable from
     the runner.)

4. **Docker Hub** — create the repo `bwalia/ring-promoter` and mark it **public**
   (so k3s1 pulls anonymously; no imagePullSecret needed).

5. **DNS** — `ring-promoter.diytaxreturn.co.uk` → the k3s1 Traefik LoadBalancer
   (currently `18.133.126.242`). Verify with `dig +short ring-promoter.diytaxreturn.co.uk`.

6. **Self-hosted runner** — register the Mac Studio as a repo runner with labels
   `self-hosted,mac-studio`. Ensure Docker Desktop (with buildx) is running and
   `kubectl` is on `PATH`.

## Manual deploy / rollback

- Redeploy the current `main`: **Actions → Deploy to k3s1 → Run workflow**.
- Redeploy or roll back to a specific build: run the workflow with
  `image_tag = sha-<short>` of that commit (images are immutable per-SHA).

## Verify

```bash
kubectl --kubeconfig ~/.kube/k3s1.yaml -n ring-system get deploy,pods
curl -fsS http://ring-promoter.diytaxreturn.co.uk/healthz     # 200
curl -s  http://ring-promoter.diytaxreturn.co.uk/version      # shows the deployed git sha
```

## Notes

- **Node arch**: the workflow builds `linux/amd64` (matches the image running on
  k3s0). If k3s1 nodes are arm64, change `platforms:` to `linux/arm64` (or build
  multi-arch). The preflight logs `kubectl get nodes -o wide` so you can confirm.
- **Single runner**: if the Mac Studio runner is offline, deploys queue until it
  returns — acceptable for this control-plane service.
- **k3s0 is out of scope** (LAN-only); this pipeline targets k3s1 only.
