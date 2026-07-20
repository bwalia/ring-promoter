# Lab 05 — Rollback

**Goal:** deploy a second version, then roll a ring back to its previous version.

**Time:** ~10 min · **App:** [hello-world](../sample-apps/hello-world) ·
**Feature:** `rollback`, previous version

## Prerequisites

A ring with two versions in its history. Seed a second version into `test`:

```bash
# build & push v0.2.0 (or reuse any second tag), then:
./scripts/rp.sh seed hello-world test v0.2.0
```

Now `test` shows current `v0.2.0`, previous `v0.1.0`.

## Step 1 — roll back

**UI:** open the `test` card → details → **Roll back to v0.1.0**.

**curl:**

```bash
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/hello-world/rollback?async=1" \
  -d '{"ring":"test"}'
```

## Step 2 — observe the swap

```bash
./scripts/rp.sh rings hello-world | jq '.rings[] | select(.ring.name=="test")
  | {current:.current_version, previous:.previous_version}'
# { "current": "v0.1.0", "previous": "v0.2.0" }
```

**Expected:** `test` is back on `v0.1.0`; the version it rolled *off* becomes the
new "previous", so you can roll forward again if needed.

## Step 3 — rollbacks are ungated

Note rollback needed **no** password and is exempt from promotion gates — even
on `prod`. Incident response must never be blocked.

## What you learned

- **Rollback** returns a ring to its previous version and health-checks it.
- The current/previous pair lets you roll back and forward freely.
- Rollbacks bypass the prod password and all promotion gates by design.

**Next:** [Lab 06 — Fix a failed deployment](./lab-06-fix-failed-deployment.md).
