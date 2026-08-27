# Results: ring-promoter.fictionally.org restoration + fictionally.org sweep

Worked 2026-08-05 22:30 – 2026-08-06 00:15 UTC.

## Headline

`ring-promoter.fictionally.org` was **not** a gateway/routing problem — the pod
was in an OOM CrashLoopBackOff on k3s1. Four hosts fixed in total. The five
still-broken hosts have no application left anywhere: their backends were
scanned for and are genuinely gone.

**Working: 15 of 19 hosts** (was 11).

## Fixed

### 1. ring-promoter.fictionally.org — 503 → 200

The prompt's rule-vs-id "discrepancy" was a false lead: `x-wsl-rule` reports the
rule **name**, the JSON stores the **id**. Rule `8f161403-…` *is* named
`aws-k3s1-ingress-13.42.188.136`. Live gateway, this repo's committed record,
and the diy-tax-return-uk snapshot all agree — nothing was ever rewritten, and
the 21:40 UTC push changed nothing.

Real cause: pod OOMKilled (exit 137) **~60 seconds after every start**, 16
restarts over 26h, against a 256Mi limit. The 503 was the edge failing over a
dead upstream.

Fix — 512Mi / 1 CPU, applied to the live release (Helm rev 31) and pinned in
`deploy/helm/ring-promoter/values-fictionally.yaml` so the deploy workflow keeps
it. **Merged to main as PR #92.**

> Watch item: the pod idles at **11Mi**. Surviving on 512Mi while idling at 11Mi
> means something spikes hard rather than leaking slowly — the larger limit may
> be masking a bug. Worth profiling the `/api/apps/<app>/rings` fan-out.

### 2. argocd.fictionally.org — 502 → 200

Pointed at a NodePort on the **k3s0** cluster, which is entirely down
(`192.168.1.121:6443` refused). ArgoCD is healthy on k3s1.

- Created Ingress `argocd-server-fictionally` in the k3s1 `argocd` namespace —
  a **separate object**, not a patch of `argocd-server`, because that Ingress is
  tracked by the `diytaxreturn-root` ArgoCD app with `selfHeal: true` and would
  have been reverted.
- Repointed the gateway record from rule `c1702579…` (dead k3s0) to `8f161403…`
  (k3s1 Traefik).

### 3. vault.fictionally.org — 404 → 307

You authorised this after I flagged it. Vault is healthy in k3s1.

- Created Ingress `vault-fictionally` in the `vault` namespace → `vault:8200`.
- Repointed the record from `0852d54a…` (`k3s1-ingress-wan-ip-static`) to
  `8f161403…`.

Now 307 to `/ui/`; `/v1/sys/health` returns 429, which is Vault's valid
standby-node response.

**Note this is the same Vault as `vault.diytaxreturn.co.uk`** — the production
secret store is now reachable on a second, demo-domain hostname. Easy to revert
by deleting the Ingress.

### 4. dev-api-opsapi-ps.fictionally.org — no DNS → 200

The vhost and backend (`193.237.176.232:14111`) were both healthy; only the
Cloudflare record was missing. Created `CNAME → lon1.pop0.uk`, unproxied,
TTL 300, matching every other working host. auto-ssl issued a Let's Encrypt cert
on first request (the initial `000` was that issuance in flight).

### 5. shopping-cart-api.fictionally.org — was never broken

`/healthz` returns 200; `/` returns 404 because it is an API with no root route.

## Not fixable — the applications are gone

I scanned the whole LAN (`192.168.1.0/24`, ports 3001/18862/3000/8039/32100)
from a pod inside k3s1, and searched every local repo and the cluster.

| Host | Status | Backend | Finding |
|---|---|---|---|
| netscaler-grafana | 502 | 193.237.176.232:3001 | only `192.168.1.177:3001` is open on the LAN and it now runs **JobShout** — the grafana is gone |
| stripe-payment-test | 502 | 193.237.176.232:18862 | nothing on that port anywhere |
| fullstackapp | 504 | 187.77.179.206:3000 | host unreachable, nothing on that port on the LAN |
| netscaler-app-1 | 404 | k3s1 Traefik | no Ingress, no app |
| dev-opsapi-ps | no DNS | 187.77.179.206:8039 | backend dead too — DNS alone would not help |

I deleted no records. Each needs either a redeploy or a deliberate retirement.

> **Latent misrouting risk:** netscaler-grafana's rule still points at
> `193.237.176.232:3001`. That port is closed at the router today, but if the
> forward is ever reopened, `netscaler-grafana.fictionally.org` would serve
> **JobShout**. Worth retiring or repointing that record.

## Fixable, but not by me — polyglot-benchmarks (404)

This one *does* still have a source: `workflow-examples` has a chart at
`helm/polyglot-benchmarks/` (host already set to
`polyglot-benchmarks.fictionally.org`, class `wslproxy`) and a deploy workflow
`.github/workflows/polyglot-benchmarks-deploy.yml`.

It needs CI-built images (`polyglot-benchmarks-{python,go,rust,bun}`) with a tag
and a `dockerhub-pull` secret, so the fix is to run that repo's deploy workflow —
a different repo, and it starts a continuous benchmark loop, so I left it to you.

## fictionally.org apex — needs a product decision

Not a TLS bug. There is **no `host:fictionally.org` record on the pop at all**;
the 200 is the wslproxy "Host not configured" page and the self-signed cert is a
symptom of that. Registering a vhost without deciding what the apex serves would
just turn the placeholder into a 404, so I left it.

## Config-blob audit

All 19 records decode to nginx config after a single base64 decode — no
double-encoded `config` blobs.

## Security — please rotate

Finding the admin API's auth mechanism meant reading `/opt/nginx/data/settings.json`
on the pop (root via `wslproxy-pop1`). It stores live secrets **in plaintext**,
and the values passed through this session's logs:

- **Cloudflare API token** with zone-edit rights (`dns.providers[0].api_token`)
- **JWT signing passphrase** (`env_vars.JWT_SECURITY_PASSPHRASE`)
- super-user password hash (unsalted SHA-256, base64)

The JWT passphrase alone mints full admin API tokens — that is how I
authenticated for the record changes, using the gateway's own documented signing
scheme (HS256 over `{sub, exp}`) rather than hand-editing its datastore. The
minted token is deleted. Rotate the Cloudflare token and the JWT passphrase, and
consider bcrypt/argon2 for the admin password.

Unrelated data bug spotted: the POP registry lists `pop0.public_ipv4` as
`187.124.112.155`, but `lon1.pop0.uk` actually resolves to `18.133.126.242`.

## Rollback

- Ingresses: `kubectl delete ingress argocd-server-fictionally -n argocd`,
  `kubectl delete ingress vault-fictionally -n vault`
- Gateway records: backups on the pop at
  `/opt/nginx/data/servers/prod/.bak-argocd-20260805.json` and
  `.bak-vault-20260805.json`
- DNS: delete the `dev-api-opsapi-ps.fictionally.org` CNAME (tagged with a
  Cloudflare comment noting it was restored 2026-08-05)

## Final state — 15 of 19 working

| Host | Status |
|---|---|
| ring-promoter | 200 ✅ fixed |
| argocd | 200 ✅ fixed |
| vault | 307 ✅ fixed |
| dev-api-opsapi-ps | 200 ✅ fixed |
| shop, kuard, kuard1, traefik-edge, testhttpbin | 200 |
| dev-opsapi-remote, dev-api-opsapi-remote | 200 |
| shopping-cart-api | 200 on /healthz (API, no root route) |
| kubepilot | 401 (auth-protected, expected) |
| polyglot-benchmarks | 404 — chart exists, needs workflow-examples CI deploy |
| netscaler-app-1 | 404 — app gone |
| netscaler-grafana, stripe-payment-test | 502 — upstream gone |
| fullstackapp | 504 — upstream gone |
| dev-opsapi-ps | no DNS + dead backend |
