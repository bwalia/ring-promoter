# Video 07 — The three deployers (kubectl / github / k8sjob)

**Length:** ~7 min · **Maps to:** [sample-apps](../sample-apps/README.md) —
[hello-world](../sample-apps/hello-world), [operator](../sample-apps/operator),
[image-proc](../sample-apps/image-proc)

## 0:00 — Hook
> "Seed and promote look the same in the UI no matter what the app is. Underneath,
> Ring Promoter can push a version three very different ways. Same verb, three
> engines — let's open the hood."

Show: the feature matrix row for deployers — kubectl, github, k8sjob.

## 0:30 — Deployer 1: kubectl (hello-world)
Narration: "The default. Ring Promoter runs the Helm upgrade itself, in its own
process, patching the Deployment in the `<app>-<ring>` namespace. Most apps use
this — hello-world, kuard, shopping-cart, bank-api, ai-chat."

On screen (what RP runs per ring):
```bash
helm upgrade --install hello-world ./chart \
  --namespace hello-world-int --create-namespace \
  --set image.tag=v0.1.0 \
  --set ingress.host=hello-world.fictionally.org
```
Narration: "You hand it an image tag; it does the rollout, then the version-aware
health check on `/healthz`."

## 1:45 — Deployer 2: github (operator, version_as_ref)
Narration: "An operator isn't shipped as a loose image tag — it's *released by git
tag* through its own release workflow. So Ring Promoter uses the `github` deployer:
instead of deploying directly, it dispatches that workflow."

On screen (the app-config shape):
```yaml
apps:
  - name: operator
    deployer: github
    github:
      owner: bwalia
      repo: rp-training-operator     # the operator's own repository
      workflow: release.yml          # the release workflow to dispatch
      version_as_ref: true           # dispatch ON the version's git TAG
      token_env: RP_GITHUB_TOKEN     # API token, from a Secret
```

## 3:00 — What version_as_ref actually does
Narration: "A 'version' for this app *is* a git tag — `v1.4.0`. With
`version_as_ref: true`, Ring Promoter doesn't dispatch on a fixed branch and pass
the version as an input. It dispatches the workflow *on the tag itself* —
`workflow_dispatch` against `refs/tags/v1.4.0`. The workflow builds and pushes
exactly the ref it runs from, so the tag *is* the release."

Show: a slice of the flow — seed the tag into `int`, promote the same tag onward;
each ring's `health_version_field: version` makes it stick only once `/healthz`
reports that exact tag. Note: the workflow file must exist on every deployable tag.
The operator has *no Ingress* — a deliberate contrast; health/metrics are
ClusterIP-only.

## 4:15 — Deployer 3: k8sjob (image-proc)
Narration: "The `k8sjob` deployer doesn't run Helm inside Ring Promoter's process
at all. Each deploy runs as a short-lived Kubernetes Job in the `ring-exec`
namespace — isolating deploy credentials and tooling in a throwaway pod."

On screen (the app-config shape):
```yaml
apps:
  - name: image-proc
    deployer: k8sjob
    health_version_header: X-App-Version   # version from a RESPONSE HEADER
    k8sjob:
      namespace: ring-exec
      image: ghcr.io/bwalia/rp-training-deploy:latest   # image with helm+kubectl
      command:
        - /bin/sh
        - -c
        - |
          helm upgrade --install image-proc ./chart \
            --namespace image-proc-$RING --create-namespace \
            --set image.tag=$VERSION \
            --set ingress.host=imageproc.fictionally.org
```

## 5:15 — Three ways to read the version, too
Narration: "Notice image-proc's health is different as well. hello-world reports
its version in a JSON *field*. image-proc puts it in a *response header*."

On screen:
```bash
curl -si https://imageproc.fictionally.org/healthz
# HTTP/1.1 200 OK
# X-App-Version: v1.2.3
# Content-Type: application/json
# {"status":"ok","version":"v1.2.3"}
```
Narration: "`health_version_header: X-App-Version` reads that header; kuard and the
operator are status-only. Whatever the source, a stale replica answering `200 OK`
with the old version fails the check."

## 6:00 — Same verb, isolated by RBAC
Narration: "The training instance's RBAC matches the deployers: a cluster-wide
Deployment-patch role for the kubectl apps, and a *namespace-scoped* Jobs role in
`ring-exec` for k8sjob — the deploy Jobs run as their own `ring-deploy-job`
service account. The github deployer needs no cluster rights at all; it just needs
the `RP_GITHUB_TOKEN` Secret to call the Actions API."

Show: the three deployer rows against their RBAC — cluster patch / ring-exec Jobs /
GitHub token only.

## 6:30 — Why it matters
Show: a slide — one Promote button, three engines (in-process Helm / dispatch a
release workflow / run a Job), three ways to prove the version (field / header /
status). "Pick the deployer that fits how each app actually ships. The workflow
above it — seed, promote, gate, roll back — never changes."

## 7:00 — Outro
> "Last video: enterprise — groups, RBAC, HA and DR, and running your own Ring
> Promoter instance." Subscribe.

---
**YouTube description:**
One Promote button, three engines. See how Ring Promoter pushes a version three
ways: the kubectl deployer (in-process Helm) on hello-world, the github deployer
with version_as_ref (dispatch a release workflow on the version's git tag) on the
operator, and the k8sjob deployer (each deploy as a Kubernetes Job in ring-exec) on
image-proc — plus field / header / status-only health and the RBAC that isolates
each. Part 7 of the Ring Promoter training series. Repo:
github.com/bwalia/ring-promoter (training/).
Chapters: 0:00 Intro · 0:30 kubectl · 1:45 github · 3:00 version_as_ref · 4:15
k8sjob · 5:15 Header health · 6:00 RBAC · 6:30 Why · 7:00 Next.
