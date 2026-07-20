# Lab 08 — Promotion policies (gates)

**Goal:** promote `bank-api` into a gated ring and satisfy all three gates — a
maintenance window, a QA/release sign-off, and a change-request code.

**Time:** ~20 min · **App:** [bank-api](../sample-apps/bank-api) ·
**Feature:** maintenance windows + Go-No-Go sign-off + change-request (JIRA)

## Background

`bank-api`'s `promotion_policy` guards `acc`/`prod`. A promotion into a guarded
ring is refused — **before any deploy** — until every gate is satisfied. The
dashboard shows a 🔒 on gated rings and a checklist in the Promote dialog.

Get a version up to `test` first (it's ungated):

```bash
./scripts/rp.sh seed hello-world int v1.0.0   # analogous; for bank-api:
./scripts/rp.sh seed bank-api int v1.0.0
./scripts/rp.sh promote bank-api int          # int -> test
```

## Step 1 — try to promote test → acc (watch it get blocked)

```bash
curl -s -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/bank-api/promote?async=1" -d '{"from_ring":"test"}'
# 409 {"error":"qa/release sign-off required: v1.0.0 needs a release-engineer
#      sign-off for acc before it can be promoted"}
```

Gates are checked in order — you'll clear them one at a time.

## Step 2 — gate 1: maintenance window

`acc` isn't window-gated in the training config (only `prod` is), but `prod`
is — so we'll open one before the prod hop. Open an ad-hoc window now:

```bash
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
END=$(date -u -v+2H +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '+2 hours' +%Y-%m-%dT%H:%M:%SZ)
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/bank-api/maintenance-windows" \
  -d "{\"ring\":\"prod\",\"starts_at\":\"$NOW\",\"ends_at\":\"$END\",\"reason\":\"release\",\"created_by\":\"you\"}"
```

**UI:** in the Promote dialog, the **Maintenance window** row flips to *open now*;
"Open window" opens one inline.

## Step 3 — gate 2: QA/release Go-No-Go sign-off

Record a **GO** for the exact version and target ring:

```bash
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/bank-api/signoffs" \
  -d '{"ring":"acc","version":"v1.0.0","decision":"go","engineer":"J. Patel","qa_status":"passed"}'
```

A sign-off is version-specific: a GO for `v1.0.0` does **not** authorise
`v1.0.1`. The engineer name is required.

## Step 4 — gate 3: change-request code

`acc`/`prod` require a valid CR code, validated against JIRA. The demo code
`test` always passes:

```bash
./scripts/rp.sh promote bank-api test test          # cr_code = "test"
# or with a real JIRA code:
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/bank-api/promote?async=1" \
  -d '{"from_ring":"test","cr_code":"CR-1234"}'
```

**Expected:** with the sign-off present and a valid CR code, the promotion into
`acc` succeeds. (A missing code → `400 change-request code required`; an invalid
one → `400 change-request code invalid`.)

## Step 5 — the prod hop

Promoting `acc → prod` needs: the open maintenance window (step 2), a GO sign-off
for `prod` + `v1.0.0`, a valid CR code, **and** the production password. Record a
`prod` sign-off, then:

```bash
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/bank-api/promote?async=1" \
  -d '{"from_ring":"acc","cr_code":"test","password":"<prod-password>"}'
```

## What you learned

- Three independent gates guard sensitive rings, enforced **before any deploy**.
- Sign-offs are version-specific; windows come from config **or** ad-hoc; CR codes
  validate against JIRA (with `test` as the universal demo code).
- Auto-promote into a change-request-gated ring fails closed (no interactive
  code) — those rings are always promoted into explicitly.

**Next:** [Lab 09 — Canary & blue/green](./lab-09-canary-blue-green.md).
