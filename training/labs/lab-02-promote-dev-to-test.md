# Lab 02 — Promote dev → test

**Goal:** promote the version you seeded into `int` onward to `test` — one ring
at a time — and see the source-health check gate it.

**Time:** ~10 min · **App:** [hello-world](../sample-apps/hello-world) ·
**Feature:** `promote`, one-ring-at-a-time, source-health check

## Prerequisites

- [Lab 01](./lab-01-first-deployment.md) done: `hello-world` `int` holds
  `v0.1.0` and is healthy.
- The `test` ring's workload exists (deploy it once, as in Lab 01 step 2, into
  namespace `hello-world-test`).

```bash
export RP_URL=https://ring-promoter.fictionally.org RP_TOKEN=<token>
```

## Step 1 — promote int → test

**UI:** on the `int` card click **Promote to Test**.

**curl:**

```bash
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/hello-world/promote?async=1" \
  -d '{"from_ring":"int"}'
# {"job_id":"job-2"}
```

## Step 2 — watch the hops

Ring Promoter (1) health-checks the **source** (`int`) first, then (2) deploys
`v0.1.0` to `test`, then (3) health-checks `test` (version-aware).

```bash
curl -s -H "Authorization: Bearer $RP_TOKEN" \
  "$RP_URL/api/apps/hello-world/jobs/job-2" | jq '.status, .steps[].title'
# "source-health ..." , "deploy ..." , "health ..."
```

**Expected:** job `success`; `test` now shows `v0.1.0`, healthy; `acc`/`prod`
untouched (promotion never skips a ring).

## Step 3 — prove one-ring-at-a-time

Try to promote `int` again — the card's Promote is disabled because `test`
already has `v0.1.0` (nothing new to move). Rings advance only when the source
holds something the next ring doesn't.

## What you learned

- **Promote** copies the source ring's current version to the next ring, checks
  the source is healthy first, deploys, then health-checks the target.
- Promotion is strictly ordered — `int → test → acc → prod`, no skipping.

**Next:** [Lab 03 — Promote test → staging](./lab-03-promote-to-acc.md).
