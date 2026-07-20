# Lab 09 — Canary & blue/green

**Goal:** run the two most common progressive-delivery patterns and see where
Ring Promoter fits.

**Time:** ~20 min · **Apps:** [kuard](../sample-apps/kuard) (blue/green),
[shopping-cart](../sample-apps/shopping-cart) (canary) ·
**Feature:** progressive delivery

## How Ring Promoter relates to these

Ring Promoter promotes a **version into a ring** and verifies it. *Within* a
ring, canary and blue/green are a deployment-shape concern you express in the
Helm chart + Traefik; Ring Promoter is what decides the version is good enough to
widen or to promote onward.

## Part A — blue/green (kuard)

kuard ships two image tags, `blue` and `green`. Blue/green = run the new colour
alongside the old, then flip the Service selector.

1. `green` is live in `test`. Deploy `blue` as a second Deployment:
   ```bash
   helm upgrade --install kuard-blue ./sample-apps/kuard/chart \
     --namespace kuard-test --set image.tag=blue --set fullnameOverride=kuard-blue
   ```
2. **Flip**: point the `kuard` Service selector at the blue pods (edit the
   Service selector, or `kubectl patch svc kuard -n kuard-test`).
3. Seed the new colour into Ring Promoter so it tracks the live version:
   ```bash
   ./scripts/rp.sh seed kuard test blue
   ```
4. **Roll back** instantly by flipping the selector back to green — no re-deploy.

## Part B — canary (shopping-cart)

Canary = send a small slice of traffic to the new version first.

1. Deploy the new backend as a second Deployment (`shopping-cart-canary`) with
   few replicas, same Service labels, so it receives a fraction of traffic —
   or use Traefik weighting between two Services (see
   [shopping-cart/architecture.md](../sample-apps/shopping-cart/architecture.md)).
2. Watch the canary's `/metrics` (error rate, latency).
3. Happy? Scale the canary up / shift the weight to 100%, then **promote** the
   version in Ring Promoter so the next ring gets it:
   ```bash
   ./scripts/rp.sh promote shopping-cart test
   ```
4. Unhappy? Scale the canary to zero — no promotion happened, so nothing to undo
   in Ring Promoter.

## What you learned

- Blue/green swaps a Service selector between two Deployments (instant rollback).
- Canary shifts a slice of traffic before committing.
- Ring Promoter is the **gate and record**: it verifies the version and carries
  it to the next ring once the in-ring rollout is trusted.

**Next:** [Lab 10 — Production incident](./lab-10-production-incident.md).
