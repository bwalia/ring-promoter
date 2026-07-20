# Video 02 — Promotions, ring by ring

**Length:** ~6 min · **Maps to:** [Lab 02](../labs/lab-02-promote-dev-to-test.md),
[Lab 03](../labs/lab-03-promote-to-acc.md)

## 0:00 — Hook
> "You seeded a version into `int`. Now let's walk it toward production — one ring
> at a time — and watch Ring Promoter refuse to skip a step or accept a stale
> build."

Show: the dashboard, the `hello-world` app, `int` green with `v0.1.0`, the other
three rings empty.

## 0:30 — Set up the shell
On screen:
```bash
export RP_URL=https://ring-promoter.fictionally.org RP_TOKEN=<token>
```
Narration: "Every call is `Authorization: Bearer $RP_TOKEN`. The `rp.sh` helper
in `training/scripts` wraps the API — we'll use it and the raw curl side by side."

## 1:00 — Promote int → test
Narration: "Promote copies the source ring's current version to the next ring. No
version argument — the source *is* the version."

On screen (UI): on the `int` card click **Promote to Test**.

On screen (curl):
```bash
curl -fsS -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -X POST "$RP_URL/api/apps/hello-world/promote?async=1" \
  -d '{"from_ring":"int"}'
# {"job_id":"job-2"}
```

## 2:00 — Watch the three hops
Narration: "A promotion is three steps: health-check the *source* first, deploy to
the target, then version-aware health-check the target."

On screen:
```bash
curl -s -H "Authorization: Bearer $RP_TOKEN" \
  "$RP_URL/api/apps/hello-world/jobs/job-2" | jq '.status, .steps[].title'
# "success"
# "source-health ..." , "deploy ..." , "health ..."
```
Expected: job `success`; `test` now shows `v0.1.0`, healthy; `acc` and `prod`
untouched.

## 2:45 — One ring at a time
Narration: "Try to promote `int` again — the card's Promote is disabled. `test`
already holds `v0.1.0`; there's nothing new to move. Rings advance only when the
source holds something the next ring doesn't. Strictly ordered: `int → test → acc
→ prod`, no skipping."

Show: the disabled Promote button on the `int` card.

## 3:15 — Promote test → acc, the easy way
On screen:
```bash
./scripts/rp.sh promote hello-world test      # or the UI "Promote to Acceptance"
```

## 3:45 — The health-check retry loop
Narration: "After deploying to `acc`, Ring Promoter polls the ring's health URL up
to `retry.count + 1` times, waiting `retry.delay` between attempts — training uses
`count: 5, delay: 10s`. It passes on the first attempt that reports the deployed
version. If every attempt fails, it auto-rolls-back."

On screen:
```bash
JOB=$(curl -s -H "Authorization: Bearer $RP_TOKEN" "$RP_URL/api/jobs" \
  | jq -r '.jobs[] | select(.app=="hello-world") | .id')
curl -s -H "Authorization: Bearer $RP_TOKEN" \
  "$RP_URL/api/apps/hello-world/jobs/$JOB" \
  | jq '.steps[] | select(.id=="health") | .logs'
# ["attempt 1/6: healthy"]   (or several "attempt N failed" lines before success)
```

## 4:30 — Status-only vs version-aware (the kuard contrast)
Narration: "hello-world reports its version, so the check is version-aware. Some
apps don't. `kuard` — the KUARD demo — serves `200 OK` from `/healthy` with no
version field, so its ring is status-only and the *image tag* is the version."

On screen:
```bash
./scripts/rp.sh promote kuard int
curl -s -o /dev/null -w '%{http_code}\n' https://kuard-test.fictionally.org/healthy
# 200
```
Show: kuard's ring cards advancing; note it has `auto_promote` enabled, so int/test
chain on their own.

## 5:15 — Why it matters
Show: a slide — "still rolling out" (retry, then pass) vs "genuinely broken" (all
attempts fail → auto-rollback). "Retries give the new version time to come up
*without* masking a broken deploy — because the check is version-aware, a stale
replica never counts as success."

## 5:45 — Outro
> "Next video: when a deploy goes wrong — rollbacks, and fixing a failed
> version-mismatch deploy." Subscribe.

---
**YouTube description:**
Promote a version ring by ring with Ring Promoter — `int → test → acc` — and see
the source-health check, the version-aware retry loop, and status-only health on
the KUARD demo. Ring Promoter never skips a ring and never accepts a stale build.
Part 2 of the Ring Promoter training series. Repo:
github.com/bwalia/ring-promoter (training/).
Chapters: 0:00 Intro · 0:30 Shell · 1:00 Promote to test · 2:00 The three hops ·
2:45 One ring at a time · 3:15 Promote to acc · 3:45 Retry loop · 4:30 Status-only
(kuard) · 5:15 Why · 5:45 Next.
