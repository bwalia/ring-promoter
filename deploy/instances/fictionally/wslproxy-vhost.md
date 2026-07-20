# wslproxy edge registration — ring-promoter.fictionally.org

The k3s1 Traefik LoadBalancer addresses are private, so the Ingress alone does
not make `ring-promoter.fictionally.org` reachable from the internet. Public
traffic arrives through the **wslproxy edge (pop0)**, which terminates the
request and proxies it to the k3s1 Traefik addresses. Two things are needed
outside the cluster:

1. **DNS** — a Cloudflare **CNAME** `ring-promoter.fictionally.org → pop0.wslproxy.com`.
   external-dns publishes this automatically from the Ingress annotations (see
   `ingress.yaml`); confirm the `fictionally.org` zone is managed by external-dns
   / Cloudflare with a token that can edit it. Manual fallback:

   ```
   Type: CNAME   Name: ring-promoter   Target: pop0.wslproxy.com   Proxied: off
   ```

2. **A wslproxy vhost (tenant)** for the host on the pop0 edge, backing onto the
   k3s1 Traefik addresses — the same pattern used for `rp.workstation.co.uk`.

## Register the vhost — automated by the deploy workflow

The deploy workflow registers the vhost via the **wslproxy admin API** using the
`WSLPROXY_USER` / `WSLPROXY_PASSWORD` repo secrets (with the server spec
[`wslproxy-server.json`](./wslproxy-server.json), modeled on the prod RP
template). It mirrors `wslproxy/api-scripts`:

1. **Login** — `POST $GW/api/user/login {email,password}` → `.data.accessToken`.
2. **Upsert** — `GET $GW/api/servers`; if no server matches
   `ring-promoter.fictionally.org`, `POST $GW/api/servers` with the spec.

`$GW` defaults to `https://pop0.wslproxy.com` (override with
`WSLPROXY_GATEWAY_URL`).

### Manual equivalent (from the wslproxy repo)

```bash
cd wslproxy/api-scripts
ADMIN_EMAIL="$WSLPROXY_USER" ADMIN_PASSWORD="$WSLPROXY_PASSWORD" GATEWAY_URL="$GW" \
  ./auth/login.sh
./servers/create-server.sh /path/to/wslproxy-server.json
```

If the host needs binding into a routing rule/upstream (as the other RP hosts
are), attach it with `api-scripts/servers/attach-rule.sh` — the deploy step logs
a reminder when it creates a fresh vhost.

## Verify

```bash
dig +short ring-promoter.fictionally.org        # -> pop0.wslproxy.com.
curl -sS -o /dev/null -w '%{http_code}\n' https://ring-promoter.fictionally.org/healthz
# 200, and the body is {"status":"ok"} — a plain 200 with a "Host not configured"
# body means the vhost is not registered yet.
```
