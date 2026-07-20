# operator

> **Ring Promoter feature focus:** the **`github` deployer** (operators are
> released **by git tag**, and Ring Promoter dispatches the app's release
> workflow with `version_as_ref: true`), **CRD / operator lifecycle &
> promotion**, and a **`maintenance_window` promotion gate on prod**. The one
> sample app with **no user-facing Ingress** — a deliberate contrast.

A minimal Kubernetes **operator** in Go. It manages a `Greeting` custom resource
and, for each one, reconciles a same-named `ConfigMap` carrying the greeting's
message. Like every training app it is a Go service + Dockerfile + Helm chart +
CI + [`architecture.md`](./architecture.md) — but this one is a *controller*, not
a web app, so it exposes only `/healthz` + `/metrics` and **no Ingress**.

## The custom resource

`Greeting` — group `training.ringpromoter.io`, version `v1`, **namespaced**:

```yaml
apiVersion: training.ringpromoter.io/v1
kind: Greeting
metadata:
  name: hello
spec:
  message: "Hello from the Ring Promoter training operator 👋"
```

For each `Greeting` the operator ensures a `ConfigMap` of the **same name** in the
**same namespace** whose `data.message` mirrors `spec.message`. It reconciles on a
**poll loop** (default every 15s) — level-triggered, so it only writes when the
ConfigMap is missing or has drifted.

## Endpoints

| Path       | Purpose                                                     |
|------------|-------------------------------------------------------------|
| `/healthz` | `{"status":"ok","version":"..."}` — liveness + version      |
| `/metrics` | Prometheus text (`operator_reconcile_total`, `operator_configmaps_created_total`, `operator_greetings`, build info) |

Served on the health port (default `:8080`). The version comes from `RP_VERSION`
(set by the Helm chart / Ring Promoter to the deployed tag), falling back to the
`-X main.version` build ldflag, then `dev`. Because `/healthz` echoes the version,
a ring with `health_version_field: version` only passes once the promoted build
is genuinely live.

## Implementation note — no client-go

The controller is **standard-library only**. Instead of controller-runtime /
client-go it talks to the Kubernetes API over `net/http` using the **in-cluster
REST config**: the API server from `KUBERNETES_SERVICE_HOST/PORT`, the CA bundle
and the projected ServiceAccount token from
`/var/run/secrets/kubernetes.io/serviceaccount`. This keeps the image tiny and
the build hermetic. Run outside a cluster (e.g. `go run ./cmd` on a laptop) and
it serves health/metrics but disables the reconcile loop.

## Run it locally

```bash
go run ./cmd            # health/metrics on http://localhost:8080 (reconcile disabled off-cluster)
curl -s localhost:8080/healthz     # {"status":"ok","version":"dev"}
curl -s localhost:8080/metrics
```

## Build the image

```bash
docker build --build-arg VERSION=v0.1.0 -t ghcr.io/bwalia/rp-training-operator:v0.1.0 .
```

## Install the CRD + operator

The chart installs the `Greeting` CRD by default (`crd.install=true`). Deploy:

```bash
helm upgrade --install operator ./chart \
  --namespace operator-int --create-namespace \
  --set image.tag=v0.1.0

kubectl apply -f examples/greeting.yaml
kubectl get greeting hello
kubectl get configmap hello -o jsonpath='{.data.message}'; echo
```

Prefer to manage the CRD out-of-band? Set `crd.install=false` and apply it once:

```bash
kubectl apply -f api/crd.yaml
helm upgrade --install operator ./chart --set crd.install=false --set image.tag=v0.1.0
```

### What the chart ships (no Ingress)

- **Deployment** — one replica (an operator is a singleton controller).
- **ServiceAccount** + **ClusterRole** + **ClusterRoleBinding** — grants
  `get/list/watch` on `greetings` (`training.ringpromoter.io`) and
  `get/list/create/update` on `configmaps`.
- **Service** (ClusterIP) — exposes `/healthz` + `/metrics` for Prometheus only.
- **CustomResourceDefinition** — optional (`crd.install`).
- **No Ingress.** An operator has no public HTTP surface; this is the deliberate
  contrast with the user-facing sample apps.

## How Ring Promoter manages it — the `github` deployer, released by tag

The kubectl-deployed apps in this academy hand Ring Promoter an image tag and RP
runs the Helm upgrade. An **operator ships differently**: it is **released by git
tag** through its own release workflow, and Ring Promoter uses the per-app
**`github` deployer** to *dispatch that workflow* rather than deploying directly.

The RP app-config shape (the orchestrator wires the real values into
`training/config` / the control-plane ConfigMap; shown here for reference):

```yaml
apps:
  - name: operator
    deployer: github                 # override the global deployer
    github:
      owner: bwalia
      repo: rp-training-operator     # the operator's own repository
      workflow: release.yml          # the release workflow to dispatch
      version_as_ref: true           # dispatch ON the version's git TAG, not a fixed branch
      token_env: RP_GITHUB_TOKEN     # env holding the API token (from a Secret)
    promotion_policy:
      maintenance_window:            # prod may only take a version during a window
        rings: [prod]
        recurring:
          - days: [Sat, Sun]
            start: "02:00"
            end:   "04:00"
            timezone: Europe/London
    rings:
      int:  { target_env: int,  health_url: "http://operator.operator-int.svc/healthz",  health_version_field: version }
      test: { target_env: test, health_url: "http://operator.operator-test.svc/healthz", health_version_field: version }
      acc:  { target_env: acc,  health_url: "http://operator.operator-acc.svc/healthz",  health_version_field: version }
      prod: { target_env: prod, health_url: "http://operator.operator-prod.svc/healthz", health_version_field: version }
```

What `version_as_ref: true` does: a Ring Promoter "version" for this app is a git
**tag** (e.g. `v1.4.0`). Instead of dispatching the workflow on a fixed `ref`
(a branch) and passing the version as an input, the deployer dispatches the
workflow **on the version's tag itself** — `workflow_dispatch` against
`refs/tags/v1.4.0`. The release workflow then builds and pushes exactly the ref
it runs from, so the tag *is* the release. (The workflow file must therefore
exist on every deployable tag.) You `seed` the tag into `int` and `promote` the
same tag onward; each ring's `health_version_field: version` makes a promotion
"stick" only once `/healthz` reports that exact tag.

### CRD / operator lifecycle & promotion

Promoting an operator is promoting **both** the controller image **and**,
implicitly, the CRD schema it depends on. The chart installs the CRD with a
`helm.sh/resource-policy: keep` annotation so a rolling promotion never deletes
the CRD (and every `Greeting`) out from under a running ring. Walk a version from
`int` → `test` → `acc` → `prod`, watching `operator_greetings` and the reconcile
counters on `/metrics` confirm the new controller is actually reconciling.

### The `maintenance_window` gate on prod

The `promotion_policy.maintenance_window` block guards `prod`: a version may only
enter the prod ring while a window is open — either a **recurring** window
(configured above) or an **ad-hoc** one an operator opens from the RP UI/API. A
promotion attempted outside the window is refused *before* any dispatch. This
models the real-world rule that cluster-wide controllers change only inside an
agreed change window.

## Files

| Path                     | Purpose                                             |
|--------------------------|-----------------------------------------------------|
| `api/crd.yaml`           | The `Greeting` CustomResourceDefinition             |
| `cmd/main.go`            | Binary entrypoint (thin)                            |
| `internal/controller/`   | Reconcile loop + stdlib Kubernetes REST client      |
| `examples/greeting.yaml` | A sample `Greeting` CR                              |
| `chart/`                 | Helm chart (Deployment, SA, RBAC, Service, CRD)     |
| `.github/workflows/release.yml` | Release-by-tag CI dispatched by the github deployer |

See [`architecture.md`](./architecture.md) for the reconcile loop and the
tag-dispatch promotion diagrams.
