# Lab 04 — Production approval (the prod password)

**Goal:** promote into production and see the production-password gate that
guards the last ring.

**Time:** ~10 min · **App:** [hello-world](../sample-apps/hello-world) ·
**Feature:** production password

## Background

When the instance sets `RP_PROD_PASSWORD`, any operation that deploys to the
**last ring** (`prod`) must carry it: promoting into prod, seeding prod
directly, and enabling auto-promote into prod. Rollbacks are exempt so incident
response is never blocked.

## Step 1 — try without the password

```bash
curl -s -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/hello-world/promote?async=1" \
  -d '{"from_ring":"acc"}'
# 403   (production password required)
```

**UI:** the Promote-to-Production dialog shows a **Production password** field
and a red "This deploys to Production" warning.

## Step 2 — promote with the password

```bash
./scripts/rp.sh promote hello-world acc     # UI: enter the password, confirm
# or raw:
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/hello-world/promote?async=1" \
  -d '{"from_ring":"acc","password":"<prod-password>"}'
```

**Expected:** job `success`; `prod` shows `v0.1.0`, healthy. `v0.1.0` is now live
across all four rings.

## Step 3 — the auto-promote trap (and why it's guarded)

Enabling **auto-promote into prod** also requires the password — otherwise it
would be a way around the gate. Consent is checked when the switch is *enabled*.

## What you learned

- The prod password is an extra human checkpoint on the last ring only.
- It guards every path into prod (promote, seed, auto-promote), not just one.

**Next:** [Lab 05 — Rollback](./lab-05-rollback.md).
