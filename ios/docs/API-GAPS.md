# API gaps

Things the iOS app needs that `bwalia/ring-promoter` does not provide today.
Each entry states what the app does *instead*, and the smallest Go-side change
that would remove the workaround. **Nothing here has been implemented** — the
backend is untouched, as required.

Ordered by how much they cost the app.

---

## 1. Push notifications: no sender, no device registry

**What is missing.** The service has no way to record a device token and nothing
that would call APNs when a job reaches a terminal state. An operator who starts
a promotion and locks their phone finds out it failed only by opening the app.

**What the app does now.** `Stores/PushRegistration.swift` implements the whole
client half — permission, APNs registration, token capture, and turning a
payload into a route — and stops at the boundary. `registerWithBackend` is a
single unimplemented function that trips an assertion rather than pretending.
`PushRegistration.route(from:)` already parses the payload proposed below, and
is unit-tested, so the client is ready the day the server can send one.

**Proposed minimal change.**

```
POST   /api/devices     {"token": "<hex>", "platform": "ios"}   → 204
DELETE /api/devices/{token}                                      → 204
```

- A `devices` table (token, platform, created_at) alongside the existing store
  interface; the in-memory implementation is a map.
- One hook where jobs already finish — `Job.finish` in `internal/api/jobs.go` —
  posting to APNs with the payload:

  ```json
  {
    "aps": { "alert": { "title": "payments-api", "body": "promote failed — rolled back" }, "sound": "default" },
    "app": "payments-api",
    "job_id": "job-12",
    "outcome": "rolled_back"
  }
  ```

- APNs credentials from the environment, and the feature off unless they are
  set — same pattern as `RP_OLLAMA_JWT_SECRET` gating AI diagnosis, and
  surfaced on `GET /api/apps` as `push_enabled` so the app can hide the toggle.

**Effort:** small. The job lifecycle already has exactly one terminal point.

---

## 2. Streaming logs: polling is the only option

**What is missing.** `GET /api/apps/{app}/jobs/{id}` returns a full snapshot.
There is no SSE or WebSocket stream, so the live job view polls — about once a
second while a job runs.

**What the app does now.** `Features/Job/JobPoller.swift` polls with a
deliberate cadence: ~1s while the job is young, stretching towards 8s as it runs
on, with separate exponential backoff after consecutive failures, and it stops
the moment the job is terminal or the view disappears. It works, but a five
minute deploy is roughly 100 requests that mostly return bytes the app already
has.

**Proposed minimal change.**

```
GET /api/apps/{app}/jobs/{id}/events        (text/event-stream)
```

Emitting one event per `Reporter` call — the `StartStep` / `Log` / `FinishStep`
methods in `internal/api/jobs.go` already funnel every change through one place:

```
event: step
data: {"id":"health","title":"Health check test","status":"running"}

event: log
data: {"step":"health","line":"attempt 1/3 failed: unhealthy: status 503"}

event: done
data: {"status":"failed","result":{…}}
```

Polling stays as the fallback, which matters: an SSE connection through a
corporate proxy on cellular is not reliable enough to be the only path.

**Effort:** medium — needs a per-job subscriber list and care around the
detached operation context.

---

## 3. No cross-app rings endpoint: the Overview is N+1

**What is missing.** Ring state is only available per application. Drawing the
Overview for an *n*-app control plane costs `1 + n` requests, and each one runs
a **live health check** server-side.

**What the app does now.** `Features/Overview/OverviewStore.swift` fans the ring
requests out concurrently with a task group, so wall-clock is one round trip
rather than *n*. On a three-app demo this is invisible; on a fifty-app control
plane it is fifty concurrent health checks every refresh, triggered by every
phone with the app open.

**Proposed minimal change.**

```
GET /api/rings                        → {"apps": {"web-frontend": [RingView…], …}}
GET /api/rings?live=false             → skip the live health check, serve stored state
```

`promoter.Rings` already builds the per-app view; this is a loop over
`p.Apps()`. The `live=false` variant matters more than the batching: an overview
does not need a fresh probe of every endpoint on every pull-to-refresh, and the
app would use it for background refreshes and reserve the live check for the app
detail screen.

**Effort:** small.

---

## 4. Sign-off lookup is list-only

**What is missing.** To decide whether the QA gate is satisfied for one exact
version, the app must fetch *every* sign-off for the app and filter client-side.

**What the app does now.** `GET /api/apps/{app}/signoffs` on the app detail
screen, then `Collection<Signoff>.signoff(ring:version:)` matches the exact
(ring, version) pair — never ring alone, so a GO for `v1` can never appear to
authorise `v2`. Fine at current sizes; unbounded over time.

**Proposed minimal change.**

```
GET /api/apps/{app}/signoffs?ring=acc&version=v5.0.0   → 200 Signoff | 404
```

`store.GetSignoff` already exists and takes exactly these three arguments — this
is a query-parameter branch in `handleListSignoffs`.

**Effort:** trivial.

---

## 5. Job history is in-memory and capped at 200

**What is missing.** `JobManager` keeps the most recent 200 jobs in memory, and
they are gone on restart. `GET /api/apps/{app}/jobs/{id}` for an older job
returns 404, so a deep link from a notification or a Live Activity can land on
"job not found" through no fault of the operator.

**What the app does now.** Treats the 404 as what it is and offers the app's
history instead. History *is* persisted, so the outcome is recoverable — but the
step-by-step logs are not, except for the newest `store.KeepFailureLogs` (3)
failures per app.

**Proposed minimal change.** Either

- persist finished jobs through the existing `Store` interface, keyed by id, so
  `GET .../jobs/{id}` keeps working after a restart; or
- add `GET /api/apps/{app}/history/{id}` returning the entry **with** its stored
  `Logs`, and have the app fall back to it when a job id 404s. The field already
  exists on `store.HistoryEntry` and is deliberately not serialised (`json:"-"`).

The second is much cheaper and covers the case that matters — looking at a
failure after the fact.

**Effort:** small (option 2).

---

## 6. `/api/jobs` returns only the newest job per app

**What is missing.** The cross-app activity feed shows one job per application.
Two promotions of the same app half an hour apart collapse into one row, so the
feed cannot answer "what has this team been doing today?".

**What the app does now.** Renders exactly what the endpoint returns, splitting
running from finished. Per-app history fills the gap when someone digs in.

**Proposed minimal change.**

```
GET /api/jobs?limit=50&all=true       → the newest N jobs across all apps
```

`JobManager` already keeps them in insertion order; this is a different slice of
`m.order`, with the current behaviour as the default so no existing client
changes.

**Effort:** trivial.

---

## 7. A running job does not say what it is targeting

**What is missing.** `Job` carries `app`, `action` and `steps`, but the target
ring and version live inside `result` — which is only populated once the job has
**finished**. While a promotion is running there is no structured way to know
which ring it is deploying into.

**What the app does now.** The Overview marks the whole application as busy
("Promoting") rather than animating a specific ring, because guessing would mean
pulsing the wrong one. Step titles do contain the answer (`"Deploy v9.1.0 to
test"`), but parsing prose is not a contract worth depending on.

**Proposed minimal change.** Lift the fields onto the job itself, set when it is
created:

```json
{ "id": "job-12", "app": "payments-api", "action": "promote",
  "ring": "test", "from_ring": "int", "version": "v9.1.0", "status": "running", … }
```

`JobManager.run` is already called with everything needed — `handlePromote` has
the source ring, and `ring.Next` gives the target. Purely additive; `result`
keeps its current shape.

This also improves the Live Activity, which currently has to wait for the job to
finish before it can name the ring on the Lock Screen.

**Effort:** trivial.

---

## 8. No way to cancel a running job

**What is missing.** Once a seed/promote/rollback starts there is no
`DELETE /api/apps/{app}/jobs/{id}`. An operator who realises mid-deploy that
they picked the wrong version can only wait, then roll back.

**What the app does now.** Nothing — no cancel affordance is shown, because
offering a button that cannot work would be worse than not having one.

**Proposed minimal change.** This one is **not** obviously safe and is listed
for discussion rather than as a request. The operation context is deliberately
detached from the request (`context.WithoutCancel`) precisely so a disconnect
cannot abort an in-flight deploy or its auto-rollback. A cancel endpoint would
have to define what "cancel" means at each step — before the deploy, mid-deploy,
during the health check, during the rollback — and getting that wrong leaves a
ring in an unknown state. The safe subset is cancelling a job that has not
started its deploy step yet.

**Effort:** medium, and mostly design rather than code.

---

## Not gaps

Worth recording, so nobody re-litigates them:

- **Async execution.** `?async=1` is exactly what a phone needs. The app uses it
  for every mutating call.
- **The 422 contract.** "It ran and failed" being distinct from "it was refused"
  is what lets the app show a rolled-back deploy as its own outcome rather than
  an error. This is unusually well done and the app leans on it.
- **`prod_protected` and `ai_enabled` on `GET /api/apps`.** Capability flags
  mean the client never has to guess. More endpoints could follow this pattern
  (see `push_enabled` above).
- **Gates on the ring view.** `gates` arriving with each ring is what lets the
  promote sheet ask for the right things before submitting, rather than
  discovering them from a 409.
