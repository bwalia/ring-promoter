# Lab 03 — Promote test → staging (acc)

**Goal:** promote to acceptance and understand the health-check retry loop that
decides success or rollback.

**Time:** ~10 min · **App:** [hello-world](../sample-apps/hello-world) ·
**Feature:** health check + retries

## Prerequisites

- [Lab 02](./lab-02-promote-dev-to-test.md) done: `test` holds `v0.1.0`.
- The `acc` workload exists (namespace `hello-world-acc`).

## Step 1 — promote test → acc

```bash
./scripts/rp.sh promote hello-world test      # or the UI "Promote to Acceptance"
```

## Step 2 — understand the retry loop

After deploying to `acc`, Ring Promoter polls the ring's `health_url` up to
`retry.count + 1` times, waiting `retry.delay` between attempts (see the
instance config — training uses `count: 5, delay: 10s`). It passes on the first
attempt that reports the deployed version; if all attempts fail, it
**auto-rolls-back**.

Watch the attempts in the job's `health` step logs:

```bash
JOB=$(curl -s -H "Authorization: Bearer $RP_TOKEN" "$RP_URL/api/jobs" \
  | jq -r '.jobs[] | select(.app=="hello-world") | .id')
curl -s -H "Authorization: Bearer $RP_TOKEN" \
  "$RP_URL/api/apps/hello-world/jobs/$JOB" | jq '.steps[] | select(.id=="health") | .logs'
# ["attempt 1/6: healthy"]  (or several "attempt N failed" lines before success)
```

**Expected:** `acc` shows `v0.1.0`, healthy.

## Step 3 — why retries matter

A rollout takes a few seconds; the first health poll may catch the old replica
still serving. Retries give the new version time to come up **without** masking
a genuinely broken deploy — because the check is version-aware, a stale replica
never counts as success.

## What you learned

- Health checks retry with a delay before giving up.
- A version-aware check + retries distinguishes "still rolling out" from "broken".

**Next:** [Lab 04 — Production approval](./lab-04-production-approval.md).
