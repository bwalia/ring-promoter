# kuard — architecture

The canonical Ring Promoter training workload: the upstream
`gcr.io/kuar-demo/kuard-amd64` image promoted **blue → green** by image tag. It
demonstrates promoting a **third-party image** with a **STATUS-ONLY** health
check — `/healthy` returns `200` and reports no version, so the deployed tag is
the only "version" that exists.

## Runtime shape

```mermaid
flowchart LR
  U["User / curl"] -->|"https://kuard.fictionally.org"| CF["Cloudflare DNS<br/>CNAME → pop0.wslproxy.com"]
  CF --> POP["wslproxy POP (pop0)"]
  POP --> TR["Traefik ingress (k3s1)<br/>ingressClassName: traefik"]
  TR --> SVC["Service :80 → :8080"]
  SVC --> P1["Pod: kuard-amd64:blue<br/>/healthy → 200 (no version)"]
  SVC --> P2["Pod: kuard-amd64:blue"]
```

## Promotion loop (blue ↔ green)

```mermaid
sequenceDiagram
  participant CI as App CI (GitHub Actions)
  participant RP as Ring Promoter
  participant K as k3s1 (traefik)
  Note over CI: No build — kuard is a third-party image.
  CI->>RP: POST /api/apps/kuard/seed {ring:int, version:green}
  RP->>K: kubectl set image ...=kuard-amd64:green (int)
  RP->>K: GET /healthy — status only, expect 200
  K-->>RP: 200 OK (no version field)
  RP-->>CI: healthy ✓
  Note over RP,K: auto_promote int→test→acc→prod,<br/>auto-rollback to blue if a ring stays unhealthy
```

## Why a STATUS-ONLY health check

kuard's `/healthy` endpoint answers `200 OK` with no version in the body, so
Ring Promoter's ring config sets **no** `health_version_field` — it can only
assert "a pod is up and serving". The promoted **image tag** (`blue`/`green`) is
therefore the source of truth for which build is live. This is the deliberate
contrast with `hello-world`: same promotion machinery, but here the app can't
prove its own version, so the health check is status-only and the tag carries
the identity.
