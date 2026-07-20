# Getting started

## 1. Prerequisites

- A Kubernetes cluster you can reach with `kubectl` (the academy targets
  **k3s1**; Traefik is the ingress controller).
- `helm` (v3+), `go` (1.26+), `docker`.
- Access to a Ring Promoter instance. Use the shared training instance at
  **https://ring-promoter.fictionally.org**, or deploy your own with the
  [instance runbook](../../deploy/instances/fictionally/README.md).

## 2. Get an API token

Every `/api` call needs a bearer token (`RP_API_TOKEN` on the server). Ask your
instance admin, then export it:

```bash
export RP_URL=https://ring-promoter.fictionally.org
export RP_TOKEN=<your-token>
```

## 3. Confirm you can talk to it

```bash
curl -s "$RP_URL/healthz"                                   # {"status":"ok"}
curl -s -H "Authorization: Bearer $RP_TOKEN" "$RP_URL/api/apps" | jq '.apps'
# ["hello-world","kuard","shopping-cart","bank-api","image-proc","ai-chat","operator"]
```

Open `$RP_URL` in a browser to see the dashboard: every app, its four rings, and
the current version + health of each.

## 4. Core vocabulary

| Term | Meaning |
|------|---------|
| **Ring** | An ordered environment: `int → test → acc → prod`. |
| **Seed** | Set a version into a ring directly (usually `int`), then health-check it. |
| **Promote** | Copy the current version from one ring to the next, health-check, auto-rollback on failure. |
| **Rollback** | Return a ring to its previous version. |
| **Health check** | After a deploy, RP polls the ring's `health_url`; version-aware rings also require the endpoint to report the deployed version. |
| **Gate** | A precondition on entering a ring: maintenance window, QA/release sign-off, or a change-request code. |

## 5. Next

Do [Lab 01 — your first deployment](../labs/lab-01-first-deployment.md).
