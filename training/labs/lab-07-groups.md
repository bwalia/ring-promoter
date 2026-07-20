# Lab 07 — Application groups

**Goal:** group related apps so a team sees and promotes them together.

**Time:** ~10 min · **Apps:** [shopping-cart](../sample-apps/shopping-cart) + friends ·
**Feature:** application groups (server-side, shared by all users)

## Background

A **group** is a named collection of apps, stored server-side and shared by
everyone using the control plane. Groups don't change promotion mechanics — they
organise the dashboard (e.g. one team's services on one screen).

## Step 1 — create a group

**UI:** sidebar → **New group** → name it `Storefront`, add `shopping-cart` (and
any others).

**curl:**

```bash
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/groups" \
  -d '{"name":"Storefront","apps":["shopping-cart","hello-world"]}'
# {"id":"g-...","name":"Storefront","apps":["shopping-cart","hello-world"], ...}
```

Members must be configured apps; duplicates are de-duplicated.

## Step 2 — view and promote across the group

Open the **Storefront** group page: every member's rings on one screen. Promote
each member from here — handy for a multi-service release (shopping-cart's
backend + frontend share one image tag, so they move together).

```bash
curl -s -H "Authorization: Bearer $RP_TOKEN" "$RP_URL/api/groups" | jq '.groups'
```

## Step 3 — update / delete

```bash
GID=$(curl -s -H "Authorization: Bearer $RP_TOKEN" "$RP_URL/api/groups" \
  | jq -r '.groups[] | select(.name=="Storefront") | .id')
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X PUT "$RP_URL/api/groups/$GID" -d '{"name":"Storefront","apps":["shopping-cart"]}'
# curl ... -X DELETE "$RP_URL/api/groups/$GID"   # to remove
```

## What you learned

- Groups are shared, server-side collections that organise apps for a team.
- They're a view concern — promotion rules are unchanged per app.

**Next:** [Lab 08 — Promotion policies](./lab-08-promotion-policies.md).
