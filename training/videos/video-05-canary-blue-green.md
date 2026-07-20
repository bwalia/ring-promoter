# Video 05 — Canary & blue/green

**Length:** ~7 min · **Maps to:** [Lab 09](../labs/lab-09-canary-blue-green.md)

## 0:00 — Hook
> "Canary and blue/green happen *inside* a ring. Ring Promoter is what decides the
> version is good enough to widen — or to carry onward. Let's see exactly where
> the line is."

Show: the dashboard with `shopping-cart` and `kuard`; a slide of the two patterns.

## 0:30 — The mental model
Narration: "Ring Promoter promotes a *version into a ring* and verifies it. The
*shape* of the rollout within a ring — one colour beside another, or a slice of
traffic to a canary — is a Helm-chart-plus-Traefik concern. Ring Promoter is the
gate and the record: it verifies the version and moves it on once the in-ring
rollout is trusted."

## 1:15 — Part A: blue/green with kuard
Narration: "kuard ships two image tags, `blue` and `green`. Blue/green means run
the new colour alongside the old, then flip the Service selector. `green` is live
in `test`; we deploy `blue` as a second Deployment."

On screen:
```bash
helm upgrade --install kuard-blue ./sample-apps/kuard/chart \
  --namespace kuard-test --set image.tag=blue --set fullnameOverride=kuard-blue
```

## 2:15 — Flip the selector
On screen:
```bash
kubectl patch svc kuard -n kuard-test \
  -p '{"spec":{"selector":{"app.kubernetes.io/instance":"kuard-blue"}}}'
```
Narration: "Traffic moves to blue the instant the selector changes — no re-deploy,
no rollout wait."

## 2:45 — Tell Ring Promoter the live version
Narration: "Now record it, so Ring Promoter tracks what's actually serving. kuard
is status-only, so the *tag* is the version — we seed the colour."

On screen:
```bash
./scripts/rp.sh seed kuard test blue
curl -s -o /dev/null -w '%{http_code}\n' https://kuard-test.fictionally.org/healthy
# 200
```
Narration: "Roll back instantly by flipping the selector back to green — again, no
re-deploy. Ring Promoter's health check for kuard verifies liveness only, because
there's no version field to match."

## 3:45 — Part B: canary with shopping-cart
Narration: "Canary sends a small slice of traffic to the new version first.
shopping-cart's backend and frontend share one image tag, so they move together.
Deploy the new backend as a second Deployment with a few replicas and the same
Service labels — or weight two Services in Traefik."

On screen:
```bash
helm upgrade --install shopping-cart-canary ./sample-apps/shopping-cart/chart \
  --namespace shopping-cart-test \
  --set image.tag=v0.2.0 --set replicaCount=1 \
  --set fullnameOverride=shopping-cart-canary
```
Reference: [shopping-cart/architecture.md](../sample-apps/shopping-cart/architecture.md)
for the Traefik-weighting variant.

## 4:45 — Watch the canary
On screen:
```bash
curl -s https://shopping-cart-test.fictionally.org/metrics \
  | grep -E 'error|latency'
```
Narration: "Watch error rate and latency on the canary's `/metrics`. This is the
signal that decides go or no-go — and it's *your* signal, not Ring Promoter's."

## 5:15 — Happy path: promote onward
Narration: "Happy? Scale the canary up or shift the weight to 100%, then promote
the version in Ring Promoter so the next ring gets it. shopping-cart is a
version-field-health app, so the promotion only sticks once the target reports the
new version."

On screen:
```bash
./scripts/rp.sh promote shopping-cart test
```

## 5:45 — Unhappy path
On screen:
```bash
kubectl scale deploy/shopping-cart-canary -n shopping-cart-test --replicas=0
```
Narration: "Unhappy? Scale the canary to zero. No promotion happened, so there's
nothing to undo in Ring Promoter — the ring still holds the old, trusted version."

## 6:15 — Why it matters
Show: a slide — "in-ring rollout shape" (chart + Traefik) vs "the version is good"
(Ring Promoter). "Blue/green gives you instant rollback by selector; canary gives
you a graduated blast radius. Ring Promoter sits above both: it verifies the
version and carries it to the next ring once you trust the in-ring rollout."

## 6:45 — Outro
> "Next video: the three promotion gates — maintenance window, Go-No-Go sign-off,
> and the change-request code — on bank-api." Subscribe.

---
**YouTube description:**
Run the two progressive-delivery patterns and see exactly where Ring Promoter
fits. Blue/green with the KUARD demo (flip a Service selector, seed the live tag);
canary with shopping-cart (weight a slice of traffic, watch /metrics, then
promote). Ring Promoter is the gate and record above the in-ring rollout. Part 5
of the Ring Promoter training series. Repo: github.com/bwalia/ring-promoter
(training/).
Chapters: 0:00 Intro · 0:30 Mental model · 1:15 Blue/green (kuard) · 2:15 Flip ·
2:45 Record the version · 3:45 Canary (shopping-cart) · 4:45 Watch metrics · 5:15
Promote · 5:45 Abort · 6:15 Why · 6:45 Next.
