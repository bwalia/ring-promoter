# operator — architecture

A minimal Kubernetes operator: it watches one custom resource (`Greeting`) and
keeps a same-named `ConfigMap` in sync with each Greeting's `spec.message`. It
demonstrates the parts of Ring Promoter that only a controller exercises — the
`github` deployer (release by git tag), CRD/operator lifecycle, and a prod
maintenance-window gate. Unlike every other sample app it has **no Ingress**.

## Runtime shape — reconcile loop

The operator polls the Kubernetes API on an interval (default 15s), lists all
`Greeting` resources, and reconciles each toward its desired ConfigMap. It is
**level-triggered**: safe to run repeatedly, writes only when something drifted.

```mermaid
flowchart TD
  subgraph OP["operator Pod (singleton)"]
    T["ticker (RECONCILE_INTERVAL)"] --> L["list Greetings<br/>(in-cluster REST + SA token)"]
    L --> E{"for each Greeting"}
    E --> G["GET ConfigMap of same name"]
    G --> D{"exists?"}
    D -->|"no"| C["CREATE ConfigMap<br/>data.message = spec.message"]
    D -->|"yes, drifted"| U["UPDATE ConfigMap"]
    D -->|"yes, in sync"| N["no-op"]
    H["/healthz + /metrics :8080"]
  end
  API["kube-apiserver"]
  L -. "get/list/watch greetings" .-> API
  G -. "get configmaps" .-> API
  C -. "create configmaps" .-> API
  U -. "update configmaps" .-> API
  P["Prometheus"] -->|"scrape /metrics"| H
```

RBAC: a ClusterRole grants `get/list/watch` on `greetings`
(`training.ringpromoter.io`) and `get/list/create/update` on `configmaps`, bound
to the operator's ServiceAccount. There is no Service exposed to users — only a
ClusterIP Service so Prometheus can reach `/metrics`.

## Promotion — the `github` deployer dispatching a tagged release

An operator is **released by git tag**. Ring Promoter's `github` deployer with
`version_as_ref: true` dispatches the app's `release.yml` **on the version's tag
itself**, and that workflow builds/pushes exactly the ref it runs from. The tag
*is* the release; the same tag is then promoted ring to ring.

```mermaid
sequenceDiagram
  participant Dev as Maintainer
  participant GH as GitHub (operator repo)
  participant RP as Ring Promoter
  participant CI as release.yml (Actions)
  participant K as k3s (int/test/acc/prod)
  Dev->>GH: git tag v1.4.0 && push
  Note over RP: seed v1.4.0 -> int
  RP->>GH: workflow_dispatch release.yml ON refs/tags/v1.4.0<br/>(version_as_ref: true — no version input)
  GH->>CI: run release.yml @ v1.4.0
  CI->>CI: build & push image :v1.4.0
  CI-->>RP: run conclusion = success
  RP->>K: (ring's deploy) roll out operator :v1.4.0
  RP->>K: GET /healthz — must report version v1.4.0
  K-->>RP: {"status":"ok","version":"v1.4.0"}
  RP-->>RP: healthy ✓ — promote int→test→acc→prod
  Note over RP,K: prod promotion is gated by maintenance_window:<br/>refused unless a window (recurring OR ad-hoc) is open
```

## Why release-by-tag matters here

For a controller, a "version" is a git **tag**, not just an image tag handed to a
kubectl deployer. `version_as_ref: true` makes the workflow run *from* that tag,
so the CRD schema and the controller code that depend on each other are always
released together as one immutable ref. Combined with `health_version_field:
version` — `/healthz` echoing `RP_VERSION` — a ring only passes once the exact
promoted tag is genuinely reconciling, and the `maintenance_window` gate ensures
prod only changes inside an agreed change window.
