# Lab 01 — Your first deployment

**Goal:** build the `hello-world` image, seed it into the `int` ring, and watch
Ring Promoter confirm the running version with a version-aware health check.

**Time:** ~15 minutes · **App:** [hello-world](../sample-apps/hello-world) ·
**Feature:** seed + `health_version_field`

## Prerequisites

```bash
export RP_URL=https://ring-promoter.fictionally.org
export RP_TOKEN=<your-token>
```

## Step 1 — build and push a version

```bash
cd training/sample-apps/hello-world
VERSION=v0.1.0
docker build --build-arg VERSION=$VERSION \
  -t ghcr.io/bwalia/rp-training-hello-world:$VERSION .
docker push ghcr.io/bwalia/rp-training-hello-world:$VERSION
```

> No registry access? Skip the push and use any existing tag — the point is the
> **flow**, not the image.

## Step 2 — deploy the `int` ring's workload (one-time)

Ring Promoter updates an existing Deployment's image; create it once per ring:

```bash
helm upgrade --install hello-world ./chart \
  --namespace hello-world-int --create-namespace \
  --set fullnameOverride=hello-world \
  --set image.tag=$VERSION \
  --set ingress.host=hello-world-int.fictionally.org
```

## Step 3 — seed the version into `int`

**UI:** open `$RP_URL`, pick **Hello World**, click **Seed** on the `int` ring,
enter `v0.1.0`.

**curl:**

```bash
curl --fail -sS -X POST "$RP_URL/api/apps/hello-world/seed?async=1" \
  -H "Authorization: Bearer $RP_TOKEN" -H "Content-Type: application/json" \
  -d '{"ring":"int","version":"v0.1.0"}'
# {"job_id":"job-1"}
```

## Step 4 — watch the health check

Ring Promoter deploys `v0.1.0`, then polls `/healthz` and requires it to report
`version: v0.1.0` (the ring sets `health_version_field: version`). Watch the job:

```bash
curl -s -H "Authorization: Bearer $RP_TOKEN" \
  "$RP_URL/api/apps/hello-world/jobs/job-1" | jq '.status, .steps[].title'
```

**Expected:** the job ends `success`; the `int` card shows `v0.1.0`, healthy.

## Step 5 — prove the version-awareness

Confirm the endpoint really reports the deployed build:

```bash
curl -s https://hello-world-int.fictionally.org/healthz
# {"status":"ok","version":"v0.1.0"}
```

If a stale replica were still serving an old version, this check would **fail**
and Ring Promoter would not mark the ring healthy — the core guarantee.

## What you learned

- **Seed** sets a version into a ring and health-checks it.
- Version-aware health (`health_version_field`) means "200 OK" isn't enough —
  the endpoint must actually be running the promoted build.
- The same image is immutable but self-describing via `RP_VERSION`.

**Next:** [Lab 02 — Promote dev → test](./README.md).
