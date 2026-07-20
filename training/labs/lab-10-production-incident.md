# Lab 10 — Production incident (end-to-end)

**Goal:** run a realistic incident from alert to resolution, using everything the
academy taught.

**Time:** ~30 min · **App:** [bank-api](../sample-apps/bank-api) ·
**Feature:** detection → rollback → diagnosis → fix → re-promotion

## Scenario

`bank-api` `v1.4.0` was promoted to `prod` last night. This morning `/readyz` is
flapping and error rate is up. You are on call.

## Step 1 — confirm the blast radius

```bash
./scripts/rp.sh rings bank-api | jq '.rings[] | {ring:.ring.name,
  version:.current_version, healthy:.live_healthy, err:.live_health_error}'
```

Identify which rings run `v1.4.0` and whether `prod` is unhealthy live.

## Step 2 — stop the bleeding: roll back prod

Rollback is **ungated** (no password, no gates) precisely for this moment:

```bash
./scripts/rp.sh rollback bank-api prod
```

Confirm `prod` is back on the previous version and healthy. The incident's
customer impact is now contained.

## Step 3 — diagnose

Open the failed/old job → expand the failing step. If Ollama is enabled, click
**Diagnose with AI**. Check the app's `/metrics` and pod logs. Suppose the cause
is a bad DB migration in `v1.4.0`.

## Step 4 — fix forward under change control

Prepare `v1.4.1` (fixed migration). Because this is a real prod change, the gates
apply again (this is a feature, not friction):

1. Open a maintenance window (or use the recurring one).
2. Record a **GO** sign-off for `bank-api` / `prod` / `v1.4.1` with the QA result.
3. Promote up the pipeline with a change-request code, ending at prod with the
   prod password — as in [Lab 08](./lab-08-promotion-policies.md).

```bash
./scripts/rp.sh seed bank-api int v1.4.1
./scripts/rp.sh promote bank-api int
./scripts/rp.sh promote bank-api test test        # acc, cr_code=test
# ...record prod sign-off + open window...
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/bank-api/promote?async=1" \
  -d '{"from_ring":"acc","cr_code":"test","password":"<prod-password>"}'
```

## Step 5 — write it up

Ring Promoter's **history** is your audit trail: every seed/promote/rollback,
who, when, from/to which version, and the outcome. Export it for the post-mortem:

```bash
curl -s -H "Authorization: Bearer $RP_TOKEN" \
  "$RP_URL/api/apps/bank-api/history" | jq '.history[:10]'
```

## What you learned

- **Detect** with live health + metrics, **contain** with an ungated rollback,
  **diagnose** with logs + AI, **fix forward** under the same gates that protect
  every prod change, and **audit** with history.
- The gates that slow you down on a normal day are exactly what make a fix-forward
  safe during an incident.

**You've completed the academy.** Revisit the
[feature matrix](../sample-apps/README.md#feature-coverage-matrix) to see how far
you've come, or deploy your own instance with the
[runbook](../../deploy/instances/fictionally/README.md).
