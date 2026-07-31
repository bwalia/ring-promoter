# Ring Promoter for iOS

A native iPhone client for [Ring Promoter](https://github.com/bwalia/ring-promoter),
the deployment control plane that promotes application versions through an
ordered pipeline of rings:

```
int (Integration) → test (Test) → acc (Acceptance) → prod (Production)
```

The backend is unchanged. This app talks to its existing JSON REST API.

It is an **operator tool**, built for a release engineer on a phone, on call, at
2am. That shapes every decision in it:

- **Situational awareness first.** The Overview pins whatever is broken above
  whatever is fine, and a healthy system looks calm rather than empty.
- **Safe action.** Nothing illegal is ever offered. A production deploy needs
  the password, a typed confirmation *and* Face ID. A rollback needs one tap.
- **Live feedback.** Every action runs asynchronously and pushes a live job
  screen with per-step logs.

---

## Contents

- [Requirements](#requirements)
- [Getting started](#getting-started)
- [Pointing the app at an instance](#pointing-the-app-at-an-instance)
- [Demo mode](#demo-mode)
- [Architecture](#architecture)
- [How the promotion rules are enforced](#how-the-promotion-rules-are-enforced)
- [Testing](#testing)
- [Security review](#security-review)
- [Screenshots](#screenshots)
- [Known gaps](#known-gaps)

---

## Requirements

| | |
|---|---|
| Xcode | 26.3 or newer |
| Swift | 6, strict concurrency (`SWIFT_STRICT_CONCURRENCY=complete`) |
| Deployment target | iOS 17.0 |
| Devices | iPhone and iPad (adaptive SwiftUI; no separate iPad build) |
| Third-party dependencies | **none** |

There are no package dependencies, on purpose. Everything the app needs —
networking, Keychain, biometrics, widgets, Live Activities — is in the SDK, and
an operator tool that can deploy to production is the wrong place to add supply
chain surface for convenience.

---

## Getting started

```bash
cd ios
open RingPromoter.xcodeproj
```

Pick the **RingPromoter** scheme and run. On first launch the app offers two
paths: connect to a real control plane, or explore in demo mode with no server
at all.

From the command line:

```bash
xcodebuild -project RingPromoter.xcodeproj -scheme RingPromoter \
  -destination 'platform=iOS Simulator,name=iPhone 16 Pro' build

xcodebuild -project RingPromoter.xcodeproj -scheme RingPromoter \
  -destination 'platform=iOS Simulator,name=iPhone 16 Pro' test
```

### Bundle identifiers

The project ships with placeholder identifiers. Change these three together
before running on a device:

| Setting | Value | Where |
|---|---|---|
| App bundle id | `com.example.RingPromoter` | target build settings |
| Widget bundle id | `com.example.RingPromoter.widget` | target build settings |
| App Group | `group.com.example.RingPromoter` | `Config/*.entitlements` **and** `SharedWidget/WidgetSnapshot.swift` |

The App Group is what lets the widget read its cached snapshot. If it does not
match in all three places the widget silently shows its empty state.

---

## Pointing the app at an instance

You need a **server URL** and an **API token**.

### Getting a token

The token is whatever the server was started with — `RP_API_TOKEN`, or
`api_token` in `config.yaml`. For a locally-run backend that is `local-dev-token`:

```bash
# in the backend repo
go run ./cmd/ringpromoter --config config.yaml
# → http://localhost:8080, token: local-dev-token
```

In production it comes from the Kubernetes Secret the deployment reads
`RP_API_TOKEN` from. Ask whoever runs the control plane; this app has no way to
mint one.

### Connecting

**Settings → Add a control plane**, then:

1. **Name** — how you will recognise it ("Production", "Staging").
2. **URL** — `https://ring-promoter.example.com`. A bare hostname gets
   `https://` added; a trailing slash is trimmed.
3. **Token** — pasted; it goes to the Keychain only after the server accepts it.
4. **Colour tag** — the dot shown on every screen that can change something.

Connecting validates in two steps so you know *which* thing is wrong:

- `GET /healthz` — is there a Ring Promoter at this URL at all?
- `GET /api/apps` — does this token work?

An unreachable host, a TLS failure, a server that is not Ring Promoter, and a
rejected token are four different messages.

### Several control planes

Saved instances live side by side, each with its own token and colour. The
active one is named and colour-coded on the Overview, the Activity feed and
every action sheet — because "which cluster am I on?" is the question behind
most 2am mistakes. Switch from Settings; deleting an instance deletes its token
too.

---

## Demo mode

Demo mode drives the **entire app** from bundled fixtures, with no server and no
network. Use it for screenshots, App Store review, and demos on a plane.

Tap **Explore in demo mode**, or **Settings → Switch to demo mode**.

What makes it worth having is that it is a real simulation, not a stub:

- seeding, promoting and rolling back mutate state, write history, and produce a
  job whose steps advance over time;
- the rules that say **no** are all enforced — a shut maintenance window, a
  missing QA sign-off, a wrong production password, a config-managed
  auto-promote switch and an unhealthy source ring all refuse exactly as the
  server would;
- the failure path is reachable: seed any version containing **`bad`** and the
  health check fails and the ring is rolled back, so the rolled-back state and
  "Diagnose with AI" can be demonstrated.

Demo credentials, shown so a reviewer can walk the gated path:

| | |
|---|---|
| Production password | `demo` |
| Change-request code | `test` (the backend always accepts this one too) |

### Where the fixtures came from

`RingPromoter/Resources/Fixtures/*.json` are **real captured responses** from a
locally-run backend, not hand-written approximations — including the whole error
corpus (401, 403, 404, 409 for each gate, 400 for each bad input) and a genuine
failed-and-rolled-back job produced by pointing a ring's health check at an
endpoint that returns 503. That is what makes the decoding tests meaningful: if
the Go models change shape, they fail.

---

## Architecture

```
ios/
├── RingPromoter/
│   ├── App/            RingPromoterApp, RootView, Router, App Intents
│   ├── Models/         Codable mirrors of the Go types + the promotion rules
│   ├── Networking/     APIError, the client protocol, the live actor, demo mode
│   ├── Stores/         Keychain, instances, session, settings, biometrics
│   ├── Features/       one folder per screen: view + view model together
│   ├── DesignSystem/   ring chips, badges, colours, haptics, preview data
│   └── Resources/      captured fixtures, assets
├── SharedWidget/       compiled into BOTH the app and the widget extension
├── RingPromoterWidget/ WidgetKit widgets + the Live Activity presentation
├── RingPromoterTests/  Swift Testing — 111 tests
├── RingPromoterUITests/XCTest — the critical paths
├── Config/             Info.plists and entitlements
├── scripts/            screenshot capture
└── docs/               API-GAPS.md, screenshots
```

### The client

One actor, `RingPromoterClient`, owns the base URL, the token, the `URLSession`,
JSON coding, and the status-code → error mapping. **No view or view model ever
sees an HTTP status code.**

Everything is behind `RingPromoterAPI`, so previews, unit tests and demo mode
run against `DemoClient` with no network. There are three implementations:
`RingPromoterClient` (live), `DemoClient` (fixtures), and a scripted fake in the
tests.

Two decisions worth knowing about:

- **Every mutating call takes the async path** (`?async=1`). The server answers
  `202` with a job id and the app polls. A phone on a flaky network must never
  hold a long synchronous request open — and the poll survives the app being
  backgrounded, whereas an open request would not.
- **Optionals are omitted, not sent as null.** The server decodes with
  `DisallowUnknownFields` and reads `""` as a *wrong* password, so an untouched
  field is left out of the body entirely.

### Error mapping

`APIError` has one case per meaning, and they never collapse into "something
went wrong":

| Status | Case | What the app does |
|---|---|---|
| 400 | `.badRequest` | shows the server's message against the field |
| 401 | `.unauthorized` | sends the operator to the token screen |
| 403 | `.productionPasswordRequired` | re-opens the password field |
| 404 | `.notFound` | explains what is missing |
| 409 | `.conflict` | explains the gate |
| 422 | *not an error* | decoded as a `Result` — "it ran and failed" |
| 501 | `.notImplemented` | hides the AI feature |
| 5xx | `.server` | offers a retry |

`422` is deliberately not in the error type. A deploy that ran, failed its
health check and was rolled back is a **first-class outcome** with a message to
show, not an error toast.

### Dates

Go's `time.Time` marshals to RFC3339 with fractional seconds *only when the
value has them* — `…:52Z` and `…:47.905917Z` both appear in one response — so
the decoder tries both. A never-touched ring carries Go's zero time
(`0001-01-01T00:00:00Z`), which is recognised explicitly and shown as "never"
rather than as a date in the year 1.

One more sharp edge: the config-defined recurring maintenance windows serialise
with **PascalCase** keys (`Days`, `Start`, `End`, `Timezone`) because the Go
struct has no JSON tags, while everything else is snake_case. Every model
therefore declares explicit `CodingKeys` rather than using a key strategy.

### Offline behaviour

The last successful snapshot stays on screen with a clear "showing the last data
this app was able to load" marker. A failed refresh never blanks the operator's
only view of the system.

---

## How the promotion rules are enforced

The server enforces the rules. `Models/PromotionLegality.swift` exists so the
**UI never invites an operator to do something illegal** — every enable/disable
decision comes from one tested place rather than being re-derived in each view.

| Rule | How the app respects it |
|---|---|
| One ring at a time, never skip | Promote always targets `pipeline.next(after:)`; there is no way to express anything else |
| Source must be healthy | Promote is disabled when the server says `can_promote_from` is false, and says why |
| Auto-rollback on failure | The job view shows **failed — rolled back** as its own terminal state, distinct from plain failure |
| Gates guard sensitive rings | The promote sheet collects a CR code, shows window status with an option to open one, and shows the sign-off for the **exact version**, inline |
| Production password | Required whenever the action targets the last ring *and* the server reports `prod_protected` |
| Rollback is never gated | No password, no window, no sign-off, no CR code, no typed confirmation — incident response is not blocked |
| Auto-promote can be config-owned | Rendered disabled with "managed by config", and the `409` is handled too |
| The pipeline comes from the server | Ring names and order are read from `GET /api/apps`; nothing is hard-coded, so adding a ring server-side needs no app release |

**Production actions need three things**, all of them deliberate: the production
password, typing the application's name, and a fresh Face ID / Touch ID check.
The biometric check uses a new `LAContext` every time, so an unlock earlier in
the session cannot silently authorise a deploy.

---

## Testing

```bash
# unit tests — no network, no simulator UI
xcodebuild -project RingPromoter.xcodeproj -scheme RingPromoter \
  -destination 'platform=iOS Simulator,name=iPhone 16 Pro' \
  -only-testing:RingPromoterTests test

# UI tests — the critical paths, against demo mode
xcodebuild -project RingPromoter.xcodeproj -scheme RingPromoter \
  -destination 'platform=iOS Simulator,name=iPhone 16 Pro' \
  -only-testing:RingPromoterUITests test
```

**111 unit tests** (Swift Testing), covering:

- **Decoding** against the captured fixtures — gates, the zero timestamp, the
  PascalCase recurring windows, `rolled_back` being omitted when false, null
  arrays, both timestamp formats.
- **Every status code → state mapping**, asserted against the real captured
  error bodies, including that the server's own words survive the trip.
- **Promotion legality** — the rules table above, one test per row, plus the
  ones that are easy to get wrong: gates come from the *target* ring, rollback
  stays ungated even for an unhealthy production ring, "last ring" is derived
  rather than assumed to be called `prod`.
- **Polling** — stops on terminal, stops on cancel, backs off, keeps the last
  good snapshot through a transient failure. Runs against an immediate clock, so
  it takes milliseconds.
- **Demo mode** enforcing the same refusals the server does.
- **Deep links**, URL normalisation, Overview ordering, and an assertion that
  the widget snapshot contains no token, password or URL.

UI tests walk connect → view an app → open the promote sheet → watch a job reach
a terminal state → open the rollback sheet, and assert the production guard
keeps its submit button disabled until both the password and the typed
confirmation are supplied.

### Previews

Every screen has a `#Preview` backed by `DesignSystem/PreviewData.swift`, whose
values are shaped like real API responses (several lifted straight from the
fixtures). Previews cover the healthy, troubled and config-managed pipelines.

---

## Security review

### What is stored, and where

| Item | Where | Why |
|---|---|---|
| API token | Keychain, `kSecAttrAccessibleWhenUnlockedThisDeviceOnly` | see below |
| Server URL, name, colour | `UserDefaults` | not a secret; keeping it out of the Keychain makes the boundary obvious |
| Production password | **nowhere** — in memory for one request | it is per-action by design |
| Widget snapshot | App Group container | app names, versions, health — nothing more |

`kSecAttrAccessibleWhenUnlockedThisDeviceOnly` means the token is unreadable
while the device is locked, and is excluded from encrypted backups and from
device-to-device migration. The trade-off is deliberate: an operator re-pastes a
token after restoring a device, which is a much better outcome than a
control-plane token travelling inside a backup.

Deleting a saved instance deletes its token. **Settings → Forget every control
plane** wipes every record, every token and the widget snapshot in one action,
for handing a device on.

### What leaves the device

Only requests to the control-plane URL the operator entered. There is no
analytics, no crash reporting, no telemetry, and no third-party SDK that could
add any. The only other network activity is what iOS itself does.

### What is logged

Nothing. The app has no logging of request bodies, headers or responses. The
token and the production password appear in no log, no error message and no
crash report. Error text shown to the user is the server's `{"error": …}` string,
truncated to 300 characters if a proxy returns something else.

### TLS

**No App Transport Security exceptions.** `Config/RingPromoter-Info.plist`
contains no `NSAppTransportSecurity` dictionary at all, so every HTTPS
connection gets the platform default: TLS 1.2+, forward secrecy, and full
certificate validation. A certificate failure is reported as a certificate
failure, never silently retried.

`http://` URLs are accepted — a local `http://localhost:8080` is a legitimate
development target — but flagged with a warning in the add-instance sheet
("your token will cross the network in clear text") and a warning icon in the
instance list. There is no way to disable certificate validation for an HTTPS
URL.

Certificate pinning is **not** implemented. It would be actively harmful here:
operators point this at their own control planes with their own certificates,
often rotated by cert-manager, and a pin the app cannot update turns a routine
renewal into a broken app.

### The widget

The widget never touches the network and never reads a credential. It reads one
file from the App Group containing app names, versions, health flags and the
instance's display name. A unit test asserts the encoded snapshot contains no
`token`, `password`, `Bearer` or URL — because the App Group container is
outside the app's own sandbox.

The Keychain access group is declared in `SharedContainer` but deliberately
unused: an extension cannot prompt for a password or handle a `401`, so giving
it credentials would add risk with nothing to show for it.

### Biometrics

Two independent settings, because they solve different problems:

- **Lock the app when it closes** — re-locks the moment the app leaves the
  foreground, so the app switcher never shows a live pipeline on a device handed
  to someone else.
- **Confirm production actions** — a fresh authentication before anything that
  deploys into the last ring.

Both fall back to the device passcode rather than biometrics alone, so a failed
Face ID scan at 2am does not lock an on-call engineer out of a rollback. With no
passcode set at all, `authenticate` returns true and the typed-name confirmation
remains as the real guard — refusing would make the app unusable rather than
safer.

### Shortcuts and Siri

App Intents are **read-only**. "Show me the pipeline for X" opens the app. The
one destructive shortcut, "Prepare a rollback", opens the rollback sheet rather
than firing it. A voice command that could promote to production — no gate
sheet, no password field, no confirmation — is exactly the affordance this app
exists to avoid.

---

## Screenshots

`docs/screenshots/light/` holds the committed set. Regenerate it — and produce
the matching dark-mode set, which the script also captures — with:

```bash
./scripts/capture-screenshots.sh              # picks an available iPhone simulator
./scripts/capture-screenshots.sh "iPhone 16 Pro"
```

The screenshots are **taken by the UI tests**, one at each interesting point in
the critical paths, and extracted from the result bundle. That means they cannot
drift from the app: if a screen changes, the test that photographs it changes
with it, and a screen that stops working takes its screenshot with it.

---

## Known gaps

`docs/API-GAPS.md` lists everything the app needs that the backend does not
provide, each with a proposed minimal Go-side change. The short version:

- **Push notifications** have no sender or device registry. The client half is
  implemented and tested; `registerWithBackend` is deliberately unimplemented
  rather than faked.
- **Log streaming** does not exist, so the live job view polls with backoff.
- **No cross-app rings endpoint**, so the Overview is `1 + n` requests — each
  running a live health check server-side.
- **Jobs are in-memory and capped at 200**, so an old deep link can 404.

Nothing in the backend was changed to accommodate this app.
