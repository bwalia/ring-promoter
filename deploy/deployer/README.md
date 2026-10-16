# rp-deployer — helm-deploy runner for the k8sjob deployer

`docker.io/bwalia/rp-deployer:<tag>` runs one Ring Promoter deploy as a Kubernetes
Job (see `internal/executor/k8sjob`). It clones a chart from a git repo at the
promoted commit, runs `helm upgrade --install --wait=legacy` into the ring's
namespace, waits for the Deployment rollout, and checks the app's health URL
reports the new version. No GitHub Actions runner is involved.

Tools are pinned: Helm v4.2.2 (the cluster's releases were written by Helm 4),
kubectl v1.36.3, git, openssh, jq. Runs as uid 10001. GitHub's SSH host keys are
pinned from `api.github.com/meta` (`github_known_hosts`).

Inputs are environment variables; see the header of `rp-helm-deploy.sh`.
`HELM_DRY_RUN=1` runs `helm upgrade --dry-run=server` only, as a safe end-to-end
check of the image, deploy key and RBAC.

First user: `workstation-shop` (deploy/k8s/configmap.yaml), ServiceAccount
`rp-deploy-workstation-shop` and its deploy-key ExternalSecret in
`deploy/k8s/rbac.yaml`.

Build and push (multi-arch; bump the tag and the configmap together):

    docker buildx build --platform linux/amd64,linux/arm64 \
      -t docker.io/bwalia/rp-deployer:0.1.0 --push deploy/deployer
