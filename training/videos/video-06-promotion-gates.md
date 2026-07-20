# Video 06 — Promotion gates (windows, sign-off, CR code)

**Length:** ~8 min · **Maps to:** [Lab 08](../labs/lab-08-promotion-policies.md)

## 0:00 — Hook
> "Some rings shouldn't take a version just because it's healthy. `bank-api` guards
> `acc` and `prod` with three independent gates — a maintenance window, a Go-No-Go
> sign-off, and a change-request code — all checked *before any deploy*. Let's
> clear them one at a time."

Show: the dashboard, `bank-api`, a 🔒 on the `acc` and `prod` rings.

## 0:30 — Get a version to test (ungated)
Narration: "`test` is ungated, so land a version there first."

On screen:
```bash
./scripts/rp.sh seed bank-api int v1.0.0
./scripts/rp.sh promote bank-api int          # int -> test
```

## 1:15 — Watch the gate block you
On screen:
```bash
curl -s -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/bank-api/promote?async=1" -d '{"from_ring":"test"}'
# 409 {"error":"qa/release sign-off required: v1.0.0 needs a release-engineer
#      sign-off for acc before it can be promoted"}
```
Narration: "409 — refused before any deploy. Gates are checked in order; we clear
them one at a time. The dashboard shows the same checklist in the Promote dialog."

## 2:00 — Gate 1: maintenance window
Narration: "`acc` isn't window-gated in the training config — only `prod` is — so
we open the window we'll need for the prod hop. Windows come from config
(recurring) *or* ad-hoc, opened from the UI or API. Here's an ad-hoc one, now
through two hours from now."

On screen:
```bash
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
END=$(date -u -v+2H +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -d '+2 hours' +%Y-%m-%dT%H:%M:%SZ)
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/bank-api/maintenance-windows" \
  -d "{\"ring\":\"prod\",\"starts_at\":\"$NOW\",\"ends_at\":\"$END\",\"reason\":\"release\",\"created_by\":\"you\"}"
```
On screen (UI): in the Promote dialog the **Maintenance window** row flips to *open
now*; "Open window" opens one inline.

## 3:15 — Gate 2: Go-No-Go sign-off
Narration: "Record a **GO** for the exact version and the exact target ring. A
sign-off is version-specific — a GO for `v1.0.0` does *not* authorise `v1.0.1` —
and the engineer name is required."

On screen:
```bash
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/bank-api/signoffs" \
  -d '{"ring":"acc","version":"v1.0.0","decision":"go","engineer":"J. Patel","qa_status":"passed"}'
```
Narration: "That's the QA/release Go-No-Go: a named engineer, a decision, and the
QA status, pinned to one build going to one ring."

## 4:30 — Gate 3: change-request code
Narration: "`acc` and `prod` require a valid change-request code, validated against
JIRA. The demo code `test` always passes. The `rp.sh` promote helper takes the CR
code as its third argument."

On screen:
```bash
./scripts/rp.sh promote bank-api test test          # cr_code = "test"
# or with a real JIRA code, raw:
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/bank-api/promote?async=1" \
  -d '{"from_ring":"test","cr_code":"CR-1234"}'
```
Expected: with the sign-off present and a valid CR code, the promotion into `acc`
succeeds. Narration: "A missing code returns `400 change-request code required`; an
invalid one, `400 change-request code invalid`."

## 5:45 — The prod hop: all gates plus the password
Narration: "Promoting `acc → prod` needs everything at once — the open maintenance
window, a GO sign-off for `prod` and `v1.0.0`, a valid CR code, *and* the
production password from video 4. Record the prod sign-off first."

On screen:
```bash
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/bank-api/signoffs" \
  -d '{"ring":"prod","version":"v1.0.0","decision":"go","engineer":"J. Patel","qa_status":"passed"}'

curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/bank-api/promote?async=1" \
  -d '{"from_ring":"acc","cr_code":"test","password":"<prod-password>"}'
```
Expected: job `success`; `prod` shows `v1.0.0`, healthy.

## 6:45 — Auto-promote fails closed
Narration: "One deliberate gap: auto-promote into a change-request-gated ring
*fails closed*. There's no interactive code to supply, so those rings are always
promoted into explicitly. A gate you could automate around isn't a gate."

## 7:15 — Why it matters
Show: a slide — three independent locks (window / sign-off / CR), all checked
before the first byte deploys, plus the prod password. "Independent gates, enforced
early. You never deploy a build that then gets vetoed — the veto happens first."

## 7:45 — Outro
> "Next video: the three deployers — how Ring Promoter actually pushes a version,
> from kubectl to a GitHub workflow to a Kubernetes Job." Subscribe.

---
**YouTube description:**
Clear all three of Ring Promoter's promotion gates on bank-api — a maintenance
window, a version-specific Go-No-Go sign-off, and a JIRA change-request code — then
make the prod hop with the production password on top. Every gate is enforced
before any deploy, and auto-promote into a gated ring fails closed. Part 6 of the
Ring Promoter training series. Repo: github.com/bwalia/ring-promoter (training/).
Chapters: 0:00 Intro · 0:30 Land in test · 1:15 Blocked (409) · 2:00 Window · 3:15
Sign-off · 4:30 CR code · 5:45 Prod hop · 6:45 Fails closed · 7:15 Why · 7:45 Next.
