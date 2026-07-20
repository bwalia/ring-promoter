# hello-world — architecture

The smallest realistic Ring Promoter workload: one stateless HTTP service that
reports its own version. It exists to demonstrate the full loop — build → seed →
promote → health-check → rollback — with nothing else in the way.

## Runtime shape

```mermaid
flowchart LR
  U["User / curl"] -->|"https://hello-world.fictionally.org"| CF["Cloudflare DNS<br/>CNAME → pop0.wslproxy.com"]
  CF --> POP["wslproxy POP (pop0)"]
  POP --> TR["Traefik ingress (k3s1)<br/>ingressClassName: traefik"]
  TR --> SVC["Service :80"]
  SVC --> P1["Pod: hello-world<br/>/healthz reports version"]
  SVC --> P2["Pod: hello-world"]
```

## Promotion loop

```mermaid
sequenceDiagram
  participant CI as App CI (GitHub Actions)
  participant RP as Ring Promoter
  participant K as k3s1 (traefik)
  CI->>CI: build image :sha-abc123
  CI->>RP: POST /api/apps/hello-world/seed {ring:int, version:sha-abc123}
  RP->>K: deploy sha-abc123 to int
  RP->>K: GET /healthz — must report version sha-abc123
  K-->>RP: {"status":"ok","version":"sha-abc123"}
  RP-->>CI: healthy ✓
  Note over RP,K: promote int→test→acc→prod (auto_promote / manual),<br/>auto-rollback if a ring stays unhealthy
```

## Why the version endpoint matters

`/healthz` echoes `RP_VERSION`. The Ring Promoter ring config sets
`health_version_field: version`, so a ring only passes once the endpoint is
actually serving the promoted build — a stale replica answering `200 OK` fails
the check and is rolled back. This is the single most important idea the
training academy teaches, and hello-world isolates it.
