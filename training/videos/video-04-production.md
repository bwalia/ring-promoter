# Video 04 — Production approval & the prod password

**Length:** ~5 min · **Maps to:** [Lab 04](../labs/lab-04-production-approval.md)

## 0:00 — Hook
> "Three rings deploy on a token. The fourth — production — asks for one more
> thing: a human password. Let's cross the last ring the right way."

Show: the dashboard, `hello-world`, `acc` holding `v0.1.0`, `prod` empty with a
red "Production" badge.

## 0:30 — The rule
Narration: "When the instance sets `RP_PROD_PASSWORD`, *any* operation that deploys
to the last ring must carry it — promoting into prod, seeding prod directly, and
enabling auto-promote into prod. Rollbacks are exempt, so incident response is
never blocked."

Show: a slide listing the three guarded paths into prod + the one exemption
(rollback).

## 1:00 — Try without the password
On screen:
```bash
curl -s -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/hello-world/promote?async=1" \
  -d '{"from_ring":"acc"}'
# 403   (production password required)
```
Narration: "403 — refused before any deploy. The token got you authenticated; the
password is a *second*, deliberate checkpoint."

On screen (UI): the Promote-to-Production dialog shows a **Production password**
field and a red "This deploys to Production" warning.

## 2:00 — Promote with the password
Narration: "The `rp.sh` helper prompts nothing extra, so for prod we send the raw
call with `password`. In the UI you type it into the dialog and confirm."

On screen:
```bash
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/hello-world/promote?async=1" \
  -d '{"from_ring":"acc","password":"<prod-password>"}'
# {"job_id":"job-..."}
```
Expected: job `success`; `prod` shows `v0.1.0`, healthy. `v0.1.0` is now live
across all four rings.

## 2:45 — Where the password lives
Narration: "The password isn't in the config file — it's an instance Secret,
`RP_PROD_PASSWORD`, set alongside the API token when you stand the instance up.
Regulated apps like `bank-api` combine it with the promotion gates we'll cover in
video 6 — window, sign-off, and change-request — all on top of this same
password."

Show: the Secret keys from the instance runbook (RP_API_TOKEN, RP_PROD_PASSWORD,
RP_JIRA_TOKEN, …), the value redacted.

## 3:30 — The auto-promote trap
Narration: "Auto-promote is the sneaky path. If a version could auto-flow into
prod without a password, the gate would be pointless. So enabling **auto-promote
into prod** *also* requires the password — consent is checked at the moment you
flip the switch, not later when it fires."

Show: the auto-promote toggle on the `prod` ring asking for the password to enable.

## 4:00 — Why it matters
Show: a slide — token = "who you are", prod password = "yes, to production,
right now". "One extra human checkpoint on one ring, guarding every path in — and
nothing in the way of getting back out."

## 4:30 — Outro
> "Next video: progressive delivery — canary and blue/green — with shopping-cart
> and kuard." Subscribe.

---
**YouTube description:**
Cross the last ring safely. See the production-password gate that guards every
path into prod — promote, seed, and auto-promote — while rollbacks stay exempt.
Watch a 403 without the password and a clean promotion with it. Part 4 of the Ring
Promoter training series. Repo: github.com/bwalia/ring-promoter (training/).
Chapters: 0:00 Intro · 0:30 The rule · 1:00 403 without it · 2:00 Promote with it ·
2:45 Where it lives · 3:30 Auto-promote trap · 4:00 Why · 4:30 Next.
