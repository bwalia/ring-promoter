# Prompt: restore ring-promoter.fictionally.org (and check the other fictionally.org hosts)

Run this from `~/projects/ring-promoter` (this repo owns `fictionally.org` —
see `deploy/instances/fictionally/` and
`deploy/argocd/ring-promoter-fictionally.yaml`).

Everything under "Verified" was checked on 2026-08-05 against the live gateway
and against `bwalia/diy-tax-return-uk` @ `main`. Re-check anything that looks
stale before acting on it.

---

## The task

`https://ring-promoter.fictionally.org/` returns **503**. Restore it, then check
whether the other 19 `fictionally.org` hosts were affected by the same event.

## Verified (2026-08-05)

```
ring-promoter.fictionally.org   -> 503
fictionally.org                 -> 200   (so the gateway and DNS are fine)
```

503 response headers — the request **is** reaching wslproxy and being routed;
the backend is what is failing:

```
HTTP/2 503
server: wslproxy/1.0.1
x-wsl-rule:     aws-k3s1-ingress-13.42.188.136
x-wsl-backend:  stable, ingress, wslproxy, nodeport, kube-vip
x-debug-origin-port: 8888
```

**Note the discrepancy** — this is the most useful clue:

| | Rule | Backend |
|---|---|---|
| **Live** (from headers) | `aws-k3s1-ingress-13.42.188.136` | port 8888 |
| **`diy-tax-return-uk` git copy** | `8f161403-8592-1111-6294-9c57974505b0` | `http://193.237.176.232:8888` |

The live rule name and the committed rule id disagree, while the port matches.
So the record has been rewritten at least once, and one of those two rules is
pointing at a backend that no longer answers.

## Likely cause — please verify rather than assume

The `diy-tax-return-uk` repo **also carried a copy of this record** at
`.github/wslproxy/data/servers/prod/host:ring-promoter.fictionally.org.json`,
and its `wslproxy-register-domains` workflow pushed the whole prod profile to
the shared gateway.

- That repo's copy was last updated by commit `b994989` (2026-08-01,
  *"sync wslproxy live prod state … server config drift"*) — i.e. a snapshot of
  the gateway as it looked on 1 Aug.
- On **2026-08-05 ~21:40 UTC** a prod push from that repo ran and re-PUT **158
  server records**, including 57 log lines touching `fictionally.org` hosts.

`PUT /api/servers/<id>` overwrites the live record with the pushed body. So any
change made to a `fictionally.org` record between 1 Aug and 5 Aug — from this
repo or the admin UI — was reverted to the 1 Aug snapshot by that push.

**That is a hypothesis, not a conclusion.** Confirm before acting:

1. Did `ring-promoter.fictionally.org` work on 5 Aug before ~21:40 UTC?
2. Does the live record now match `diy-tax-return-uk`'s 1 Aug snapshot
   (rule `8f161403-…`, backend `http://193.237.176.232:8888`), or the
   `aws-k3s1-ingress-13.42.188.136` rule the headers name?

If it predates that push, the cause is elsewhere — most likely the backend
itself. A 503 with a valid rule match usually means the upstream is down, not
misrouted.

## This will not happen again from that repo

`diy-tax-return-uk` PR #899 (merged 2026-08-05) added an ownership gate: its
register workflow now pushes **only `*.diytaxreturn.co.uk`** and skips the 118
foreign records it carries, `fictionally.org` included. Those records remain in
its git history as inert copies, but nothing pushes them.

Worth deciding separately whether they should be deleted from that repo
entirely, once you have confirmed this repo holds the authoritative copies.

## Suggested order of work

1. **Check the backend first.** `193.237.176.232:8888` — is it serving, and does
   it have a route for `Host: ring-promoter.fictionally.org`? A 503 through a
   matched rule is an upstream problem far more often than a routing one.
2. **Compare live vs your repo's committed record**, and reconcile whichever is
   stale. Note the live rule (`aws-k3s1-ingress-13.42.188.136`) is not the one
   the diy-tax-return-uk snapshot names.
3. **Sweep the other 19 `fictionally.org` hosts** for the same symptom — if the
   push is the cause, it will not have hit only one. Known hosts include
   `kubepilot.`, `shop.`, `fullstackapp.`, `netscaler-grafana.`,
   `stripe-payment-test.`, `dev-api-opsapi-remote.`, `dev-api-opsapi-ps.`
4. **Re-push from the owning repo** once the records are right, so live and git
   agree and the next snapshot is accurate.

## Two gateway-wide issues worth knowing

Both affect every project on this gateway, not just yours.

**Redirect loops.** TLS terminates at wslproxy, which proxies to backends as
plain HTTP on port 80 (`x-debug-origin-https: http`). **Zero of 154** vhost
configs set `X-Forwarded-Proto`, so a backend with an HTTP→HTTPS redirect sends
the client back to HTTPS, wslproxy forwards as HTTP again, and it loops. Seen on
`test.sysops247.com`. The fix needs both halves: wslproxy sending
`X-Forwarded-Proto: https` / `X-Forwarded-Port: 443`, **and** the backend
trusting them (for Traefik:
`--entrypoints.web.forwardedHeaders.trustedIPs=<wslproxy IP>`).

**Double-base64 `config` blobs.** Some records store `config` as
`base64(base64(nginx))`. The register workflow decodes exactly once, so nginx
receives a line of base64 and `nginx -t` fails with
`unexpected end of file … .conf:1`. Records stay serving on an older good conf
until something forces regeneration, then break. This took
`acc-spectoncr.diytaxreturn.co.uk` from 401 to 404 on 2026-08-04.

Check yours:

```python
import base64, json
d = json.load(open("host:<name>.json"))
once = base64.b64decode(d["config"] + "=" * (-len(d["config"]) % 4))
print("OK" if once.lstrip()[:6] in (b"server", b"#") or
      once.lstrip().startswith((b"server", b"#")) else "DOUBLE-ENCODED")
```

`diy-tax-return-uk` has a validator for this and four related failure modes at
`.github/wslproxy/validate.py` — worth copying rather than rewriting.

## Done when

- `https://ring-promoter.fictionally.org/` returns a normal response, not 503.
- The other `fictionally.org` hosts are confirmed working or separately ticketed.
- The live gateway state and this repo's committed state agree.
