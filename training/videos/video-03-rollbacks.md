# Video 03 — Rollbacks & failed deploys

**Length:** ~6 min · **Maps to:** [Lab 05](../labs/lab-05-rollback.md),
[Lab 06](../labs/lab-06-fix-failed-deployment.md)

## 0:00 — Hook
> "Every deploy tool can move forward. The ones you trust can move *back* — fast,
> and without asking permission. Let's roll a ring back, then break a deploy on
> purpose and watch Ring Promoter catch it."

Show: the dashboard, `hello-world`, `test` holding `v0.1.0`.

## 0:30 — Seed a second version
Narration: "Rollback needs a ring with two versions in its history. Seed `v0.2.0`
into `test`."

On screen:
```bash
./scripts/rp.sh seed hello-world test v0.2.0
```
Expected: `test` now shows current `v0.2.0`, previous `v0.1.0`.

## 1:00 — Roll back
On screen (UI): open the `test` card → details → **Roll back to v0.1.0**.

On screen (curl):
```bash
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/hello-world/rollback?async=1" \
  -d '{"ring":"test"}'
```
Narration: "Rollback takes only a ring — it returns that ring to its previous
version and health-checks it."

## 1:45 — Observe the swap
On screen:
```bash
./scripts/rp.sh rings hello-world | jq '.rings[] | select(.ring.name=="test")
  | {current:.current_version, previous:.previous_version}'
# { "current": "v0.1.0", "previous": "v0.2.0" }
```
Narration: "The version it rolled *off* becomes the new 'previous', so you can roll
forward again if you need to. Current and previous just swap."

## 2:15 — Rollbacks are ungated
Narration: "No password. No promotion gates. Rollback is exempt even on `prod` —
incident response must never be blocked. That's a design rule, not a config
setting."

Show: the `prod` card's rollback control, no password field.

## 2:45 — Break a deploy on purpose
Narration: "Now the core lesson: did the new version *really* go live? Seed a tag
whose image exists but reports the wrong version string. We deploy `v9.9.9` while
the binary still reports `v0.1.0`."

On screen:
```bash
# deploy an image tagged v9.9.9 whose binary reports v0.1.0 (mismatch), then:
./scripts/rp.sh seed hello-world int v9.9.9
```

## 3:30 — Watch it fail
On screen:
```bash
JOB=$(curl -s -H "Authorization: Bearer $RP_TOKEN" "$RP_URL/api/jobs" \
  | jq -r '.jobs[] | select(.app=="hello-world") | .id')
curl -s -H "Authorization: Bearer $RP_TOKEN" \
  "$RP_URL/api/apps/hello-world/jobs/$JOB" \
  | jq '.status, (.steps[] | {t:.title, s:.status})'
```
Expected: the `health` step fails with something like
`endpoint reports "v0.1.0", want "v9.9.9"`; the job ends `failed`.

Narration: "A plain `200 OK` would have accepted this. A version-aware check never
accepts the wrong build. On a *promote* this failure triggers an auto-rollback; on
a *seed* there's no baseline to roll back to, so the state records the failure and
keeps the logs."

## 4:15 — Diagnose
On screen (UI): open the failed job → expand the `health` step. If Ollama is
enabled, click **Diagnose with AI** for a plain-language explanation.
Narration: "The failure logs stay attached to the history entry, so you can
diagnose it now or later."

## 4:45 — Fix and re-deploy
Narration: "The fix is never to loosen the check — it's to make the running build
actually report the promoted version. Rebuild so the tag and the reported version
match, then re-seed."

On screen:
```bash
docker build --build-arg VERSION=v0.2.0 -t ghcr.io/bwalia/rp-training-hello-world:v0.2.0 .
docker push ghcr.io/bwalia/rp-training-hello-world:v0.2.0
./scripts/rp.sh seed hello-world int v0.2.0     # now healthy
```
Expected: job `success`; `int` reports `v0.2.0`.

## 5:15 — Why it matters
Show: a slide — "wrong build live, answering 200" (a mismatch a plain check would
miss) vs "reports the promoted version" (passes). "Version-aware health turns a
silent bad deploy into a loud, logged, auto-reverted one."

## 5:45 — Outro
> "Next video: the last ring. Production approval and the prod password."
> Subscribe.

---
**YouTube description:**
Roll a ring back to its previous version, then break a deploy on purpose and watch
Ring Promoter's version-aware health check catch a wrong-build-live mismatch,
auto-roll-back, and keep the logs for diagnosis. Rollbacks bypass every gate by
design. Part 3 of the Ring Promoter training series. Repo:
github.com/bwalia/ring-promoter (training/).
Chapters: 0:00 Intro · 0:30 Seed v0.2.0 · 1:00 Roll back · 1:45 The swap · 2:15
Ungated · 2:45 Break it · 3:30 Watch it fail · 4:15 Diagnose · 4:45 Fix · 5:15 Why
· 5:45 Next.
