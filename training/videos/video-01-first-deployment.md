# Video 01 — Your first deployment

**Length:** ~5 min · **Maps to:** [Lab 01](../labs/lab-01-first-deployment.md)

## 0:00 — Hook
> "In five minutes we'll ship a version through Ring Promoter and watch it prove
> the new build is actually live — not just answering 200 OK."

Show: the dashboard at `ring-promoter.fictionally.org`, the `hello-world` app,
four ring cards.

## 0:30 — Build a version
On screen:
```bash
cd training/sample-apps/hello-world
docker build --build-arg VERSION=v0.1.0 -t ghcr.io/bwalia/rp-training-hello-world:v0.1.0 .
```
Narration: "One immutable image, self-describing via `RP_VERSION`."

## 1:30 — Seed into int
On screen (UI): click **Seed** on the `int` card → enter `v0.1.0`.
Expected: a live job appears with `deploy` then `health` steps.

## 2:30 — The version-aware health check
Narration: "Ring Promoter doesn't just check for 200 — it requires `/healthz` to
report the version it just deployed."
```bash
curl -s https://hello-world-int.fictionally.org/healthz
# {"status":"ok","version":"v0.1.0"}
```

## 3:30 — Why it matters
Show: a slide contrasting "200 OK from a stale replica" (fails) vs "reports
v0.1.0" (passes). "A promotion only sticks when the build is genuinely live."

## 4:30 — Outro
> "Next video: promoting int → test → acc, one ring at a time." Subscribe.

---
**YouTube description:**
Ship your first version with Ring Promoter — build, seed, and watch a
version-aware health check confirm the deploy. Part 1 of the Ring Promoter
training series. Repo: github.com/bwalia/ring-promoter (training/).
Chapters: 0:00 Intro · 0:30 Build · 1:30 Seed · 2:30 Health check · 3:30 Why ·
4:30 Next.
