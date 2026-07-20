# image-proc — architecture

An async, queue-backed Ring Promoter workload. An API accepts jobs and enqueues
them onto a Redis list; a horizontally-scaled worker drains the list and
processes them. It demonstrates three things the smaller hello-world app does
not: an async producer/consumer split, worker **autoscaling**, and the
**header** variant of version-aware health (`X-App-Version`).

## Runtime shape

```mermaid
flowchart LR
  U["User / curl"] -->|"https://imageproc.fictionally.org"| CF["Cloudflare DNS<br/>CNAME → pop0.wslproxy.com"]
  CF --> POP["wslproxy POP (pop0)"]
  POP --> TR["Traefik ingress (k3s1)<br/>ingressClassName: traefik"]
  TR --> SVC["API Service :80"]
  SVC --> A1["API pod<br/>POST /jobs, GET /jobs/id<br/>every response: X-App-Version"]
  SVC --> A2["API pod"]

  A1 -->|"LPUSH imageproc:jobs"| R[("Redis :6379<br/>list + job status<br/>ephemeral")]
  A2 -->|"LPUSH"| R

  R -->|"BRPOP"| W1["Worker pod<br/>process + mark done<br/>/healthz + /metrics side port"]
  R -->|"BRPOP"| W2["Worker pod"]
  R -->|"BRPOP"| W3["Worker pod (scaled out)"]

  HPA["HorizontalPodAutoscaler<br/>CPU stand-in → queue depth in prod"] -. scales .-> W1
  HPA -. scales .-> W3
```

The worker has **no ingress** — only a side HTTP port for `/healthz` (liveness)
and `/metrics`. Producers and consumers are decoupled entirely through the Redis
list `imageproc:jobs`; job status lives at `imageproc:job:<id>`.

## Worker scaling

The chart ships an `autoscaling/v2` HorizontalPodAutoscaler targeting the worker
Deployment. In training it scales on **CPU** because that needs nothing beyond
`metrics-server`. In **production** an async worker should scale on **queue
depth** — the backlog is the real signal, not CPU burn — by exporting
`imageproc_queue_depth` to a custom/external metric source (Prometheus Adapter,
or KEDA's Redis scaler) and using an `External` metric like ~30 queued jobs per
replica. The stand-in and the production form are both documented inline in
[`chart/templates/worker-hpa.yaml`](./chart/templates/worker-hpa.yaml).

## Version-aware health — the header variant

Every API response, including `/healthz`, carries `X-App-Version: <version>`.
The Ring Promoter ring config sets `health_version_header: X-App-Version`
(instead of hello-world's `health_version_field: version`), so a ring passes
only once the endpoint is actually serving the promoted build. `/healthz` stays
`ok` even when Redis is down — Redis being unreachable makes `POST /jobs` return
`503`, but the API process itself is healthy and still reports its version.

## Promotion via the k8sjob deployer

image-proc uses Ring Promoter's **k8sjob deployer**: rather than Ring Promoter
running `helm`/`kubectl` in-process, each deploy is executed as a **Kubernetes
Job in the `ring-exec` namespace** running a deploy script. Deploy credentials
and tooling live in a short-lived pod per deploy.

```mermaid
sequenceDiagram
  participant CI as App CI (GitHub Actions)
  participant RP as Ring Promoter
  participant K8s as k3s1 API
  participant Job as Deploy Job (ns: ring-exec)
  participant Ring as image-proc-<ring>
  CI->>CI: build api + worker images :vX
  CI->>RP: POST /api/apps/image-proc/seed {ring:int, version:vX}
  RP->>K8s: create Job (deployer: k8sjob) in ring-exec
  K8s->>Job: run deploy script
  Job->>Ring: helm upgrade --install (api, worker, redis) tag=vX
  Job-->>RP: Job succeeded
  RP->>Ring: GET /healthz — read X-App-Version header
  Ring-->>RP: X-App-Version: vX ✓
  RP-->>CI: healthy ✓
  Note over RP,Ring: promote int→test→acc→prod (auto_promote / manual),<br/>auto-rollback if the header != promoted version
```

## App-config shape (k8sjob)

The orchestrator wires the real Ring Promoter config; this is the shape it uses
(also in the [README](./README.md)):

```yaml
apps:
  - name: image-proc
    deployer: k8sjob
    health_version_header: X-App-Version
    k8sjob:
      namespace: ring-exec
      image: ghcr.io/bwalia/rp-training-deploy:latest
      command: ["/bin/sh", "-c", "helm upgrade --install image-proc ./chart --namespace image-proc-$RING --create-namespace --set image.tag=$VERSION"]
    rings:
      - { name: int,  auto_promote: true }
      - { name: test, auto_promote: true }
      - { name: acc,  auto_promote: false }
      - { name: prod, auto_promote: false }
```
