# Lab 06 — Fix a failed deployment

**Goal:** cause a version-mismatch failure, watch the automatic rollback, then
fix it — the core "did the new version really go live?" lesson.

**Time:** ~15 min · **App:** [hello-world](../sample-apps/hello-world) ·
**Feature:** version-aware health → auto-rollback → recovery

## Step 1 — break the deploy on purpose

Seed a version whose **image tag exists but serves the wrong version string**.
The easiest way: point the ring at an image tag that runs an older build (or set
`RP_VERSION` in the chart to a value that won't match the tag being promoted).
Deploy `v9.9.9` while the actual image still reports `v0.1.0`:

```bash
# deploy an image tagged v9.9.9 whose binary reports v0.1.0 (mismatch), then:
./scripts/rp.sh seed hello-world int v9.9.9
```

## Step 2 — watch it fail and roll back

```bash
JOB=$(curl -s -H "Authorization: Bearer $RP_TOKEN" "$RP_URL/api/jobs" \
  | jq -r '.jobs[] | select(.app=="hello-world") | .id')
curl -s -H "Authorization: Bearer $RP_TOKEN" \
  "$RP_URL/api/apps/hello-world/jobs/$JOB" | jq '.status, .steps[] | {t:.title, s:.status}'
```

**Expected:** the `health` step fails with something like
`endpoint reports "v0.1.0", want "v9.9.9"`; because a version-aware check never
accepts the wrong build, the job ends `failed`. (On a *promote* this triggers an
auto-rollback; on a *seed* there's no baseline to roll back to, so the state
records the failure.)

## Step 3 — diagnose

Open the failed job → expand the `health` step. If Ollama is enabled, click
**Diagnose with AI** for a plain-language explanation. The failure logs are kept
with the history entry so you can diagnose it later too.

## Step 4 — fix and re-deploy

Rebuild the image so its reported version matches the tag, then re-seed:

```bash
docker build --build-arg VERSION=v0.2.0 -t ghcr.io/bwalia/rp-training-hello-world:v0.2.0 .
docker push ghcr.io/bwalia/rp-training-hello-world:v0.2.0
./scripts/rp.sh seed hello-world int v0.2.0     # now healthy
```

## What you learned

- A version-aware health check catches "wrong build live" that a plain `200 OK`
  would miss.
- Failed promotes auto-roll-back; failures keep their logs for diagnosis.
- The fix is to make the running build actually report the promoted version.

**Next:** [Lab 07 — Application groups](./lab-07-groups.md).
