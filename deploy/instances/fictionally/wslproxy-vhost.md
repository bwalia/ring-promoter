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

## Register the vhost (in the wslproxy repo)

This repo does not contain the wslproxy edge config; register there exactly as
`rp.workstation.co.uk` was:

1. Clone the wslproxy vhost template used for the Ring Promoter hosts (OpenResty
   auto-ssl, HTTP→HTTPS redirect) and add `ring-promoter.fictionally.org` as a
   new vhost whose upstream is the k3s1 Traefik LoadBalancer addresses.
2. Wire the new host into the pop0 routing rule alongside the other hosts.
3. Open a PR, merge, then run the **`wslproxy-register-domains`** workflow to
   push the host to the wslproxy **prod** edge and activate it. Dry-run first.

## Verify

```bash
dig +short ring-promoter.fictionally.org        # -> pop0.wslproxy.com.
curl -sS -o /dev/null -w '%{http_code}\n' https://ring-promoter.fictionally.org/healthz
# 200, and the body is {"status":"ok"} — a plain 200 with a "Host not configured"
# body means the vhost is not registered yet.
```
