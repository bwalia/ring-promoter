# Captured API fixtures

These are **real responses** from a locally-run Ring Promoter backend, not
hand-written approximations. That is the point: the decoding tests in
`RingPromoterTests/ModelDecodingTests.swift` run against them, so a change to
the Go models breaks the build rather than the app.

They also back **demo mode** (`Networking/DemoWorld.swift`), which rebases their
timestamps onto "now" so relative times read sensibly during a demo.

## How they were captured

A backend was run with an in-memory store, the no-op deployer, a production
password, and a config declaring three applications — one plain, one with all
three promotion gates, one with a config-managed `auto_promote` ring:

```bash
RP_PROD_PASSWORD=prod-pass ./ringpromoter --config fixture-config.yaml
```

Then every endpoint was exercised, including each way it can refuse.

The failure fixtures (`job-failed-rolledback.json`, `result-422-failed.json`,
`rings-unhealthy.json`, `history-with-failures.json`) needed a genuinely failing
health check. Two local HTTP servers were used — one always healthy for `int`,
one flippable for `test`/`acc`/`prod` — so a version could be seeded while
healthy and then promoted after the target endpoint started returning `503`.
That produces the real four-step job: source health → deploy → health check
failed → auto-rollback, which is the state the app must render as
**failed — rolled back** rather than as a plain error.

## What each one covers

| Fixture | Covers |
|---|---|
| `apps.json` | app list, ring pipeline, `prod_protected`, `ai_enabled`, display titles |
| `rings.json` | a plain four-ring pipeline with previous versions |
| `rings-gated.json` | per-ring `gates`, the CR provider, Go's zero timestamp on a never-deployed ring |
| `rings-managed.json` | `auto_promote_managed` — the switch config owns |
| `rings-unhealthy.json` | `live_health_error`, stored vs live health disagreeing |
| `job-running.json` | a job mid-flight, with no result yet |
| `job-success.json` | a finished job; note `rolled_back` is **omitted**, not `false` |
| `job-failed-rolledback.json` | the four-step failure with retry logs and `rolled_back: true` |
| `jobs.json` | the newest job per app |
| `history.json`, `history-with-failures.json` | history newest-first, successes and failures |
| `maintenance-gated.json` | **PascalCase** recurring windows (`Days`, `Start`…), an open ad-hoc window |
| `maintenance-ungated.json` | `gated_rings` and `recurring` arriving as JSON `null` |
| `signoffs.json` | GO and NO-GO for different exact versions |
| `groups.json` | server-side groups and their members |
| `versions-unsupported.json` | `supported: false`, the free-form-input path |
| `result-422-failed.json` | "it ran and failed" — a `Result`, not an error |
| `error-*.json` | the complete refusal corpus, one file per status and reason |
| `autopromote-*.json` | the auto-promote toggle's success, 403 and 409 |

Only the response **bodies** are stored here. The HTTP status each one arrived
with was recorded at capture time and is pinned in `APIErrorMappingTests`
(`capturedCorpus`), which pairs every fixture with the code the server actually
returned and asserts it maps to the right `APIError` case — so the status
contract is under test, not just the JSON shape.

## Re-capturing

The capture scripts are not committed — they depend on a throwaway config and
two disposable health servers. If the API changes, re-run the backend with a
gated config and re-record; the table above lists what each file has to keep
demonstrating. `FixtureCorpus.all` in the tests enumerates every filename, so a
fixture that goes missing fails a test rather than disappearing quietly.

## What is *not* here

No real hostnames, tokens, or customer data. The applications are invented
(`web-frontend`, `payments-api`, `batch-worker`) and every URL points at
`example.com`.
