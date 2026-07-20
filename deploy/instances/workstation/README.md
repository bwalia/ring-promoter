# Ring Promoter instance — rp.workstation.co.uk

A second Ring Promoter instance, separate from the training instance
(`ring-promoter.fictionally.org`), for **real workstation projects**
(spectoncr, kubepilot, and more later). It runs in its own namespace with its
own Postgres database, and is published through the same wslproxy edge.

| | |
|---|---|
| Namespace | `workstation-ring-promoter` |
| Public URL | `https://rp.workstation.co.uk` |
| In-cluster ingress | Traefik (`ingressClassName: traefik`) |
| DNS | Cloudflare **CNAME → pop0.wslproxy.com** (already published) |
| Edge | wslproxy vhost on pop0 — see [`wslproxy-server.json`](./wslproxy-server.json) |
| Store | Postgres, own DB `ringpromoter_ws` on the shared `ring-promoter-db` |

## Reachability — Cloudflare + wslproxy (same pattern as fictionally)

k3s1's Traefik LoadBalancer IPs are private, so the Ingress alone is not
internet-reachable. Public traffic arrives via the **wslproxy edge (pop0)**,
which needs two things outside the cluster:

1. **DNS** — a Cloudflare **CNAME** `rp.workstation.co.uk → pop0.wslproxy.com`
   (already resolves; external-dns publishes it from the Ingress annotations).
2. **A wslproxy vhost (tenant)** for the host on pop0, attaching routing rule
   `8f161403-8592-1111-6294-9c57974505b0` (the shared rule that forwards into
   the k3s1 Traefik backend). Until this exists, the edge answers
   **"Host not configured"** — a plain 200 with that body means the vhost is
   still missing.

## Register the edge vhost — via GitHub Actions

Both steps are automated by the reusable workflow
[`.github/workflows/register-edge-vhost.yml`](../../../.github/workflows/register-edge-vhost.yml),
which reuses the shared `WSLPROXY_USER` / `WSLPROXY_PASSWORD` /
`WSLPROXY_GATEWAY_URL` (and optional `CF_API_TOKEN`) repo secrets. Its inputs
default to this instance:

```bash
gh workflow run register-edge-vhost.yml \
  -f host=rp.workstation.co.uk \
  -f zone=workstation.co.uk \
  -f server_spec=deploy/instances/workstation/wslproxy-server.json
```

pop0 picks up the new vhost within ~1 minute via its config-sync cron; the
workflow then polls `https://rp.workstation.co.uk/healthz` until it returns a
real `200` (not the "Host not configured" page).

## Verify

```bash
dig +short rp.workstation.co.uk                                   # -> pop0.wslproxy.com.
curl -sS -o /dev/null -w '%{http_code}\n' https://rp.workstation.co.uk/healthz   # 200 {"status":"ok"}
```

## Apps

The instance currently ships a placeholder config (`deployer: log`, one inert
`sample` app) so nothing crash-loops before real projects are onboarded. Add
workstation projects (spectoncr, kubepilot, …) to its config the same way the
training apps are defined in `training/config/README.md`, each with its own
per-ring namespaces, deployer, and health check.
