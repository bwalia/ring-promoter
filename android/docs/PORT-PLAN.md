# Ring Promoter for Android — port plan

A native Kotlin/Compose app with the same behaviour as the iOS app in `../ios`,
built against the same backend (`internal/api`) and the same captured fixtures.

The iOS app is the specification. It is ~11,200 lines of Swift across 64 source
files (plus 3,200 lines of tests), and every non-obvious rule in it carries a
comment explaining *why*. Those comments are the port's real source material —
the rules are the product, not the Swift.

## Non-negotiables inherited from iOS

These are behaviours the iOS app went out of its way to guarantee. They are the
reason the app is trustworthy at 2am, and the port keeps all of them:

1. **The UI never offers an illegal action.** All enable/disable decisions come
   from one place (`PromotionRules`), stated once and unit-tested once.
2. **Switching control plane rebuilds every screen.** No response in flight for
   the previous server may land on the new one. iOS does this with a
   `generation` counter plus `.id(connectionKey)` on the tab tree.
3. **Demo mode never leaks into a live session.** Before connecting, the client
   is `UnconfiguredClient`, which refuses everything, rather than a demo client
   that would invent pipelines while claiming to be connected to production.
4. **Rollback is the least gated action in the app.** No password, no CR code,
   no window. Incident response is never the thing to slow down.
5. **Tokens never touch ordinary storage, logs, or the widget snapshot.**
6. **Colour is never the only signal.** Every status also carries an icon.
7. **Mutating calls always take the async path** (`?async=1`) and poll a job. A
   phone on a flaky network must never hold a long request open.

## Stack

Chosen to be boring. Every line of this app should be obvious to whoever is
paged at 3am.

| Concern | iOS | Android | Why this one |
|---|---|---|---|
| UI | SwiftUI | Jetpack Compose + Material 3 | Declarative, maps to SwiftUI almost screen for screen |
| State holders | `@Observable` classes | `ViewModel` + `StateFlow` | Same lifetime as the iOS stores, survives rotation |
| Concurrency | `async`/`await`, actors | coroutines + `StateFlow` | Direct translation |
| HTTP | `URLSession` | OkHttp | The Android standard; already under everything else |
| JSON | `Codable` + explicit `CodingKeys` | kotlinx.serialization + explicit `@SerialName` | Keeps the mapping auditable against the Go source, which is why iOS declared keys by hand |
| Navigation | `NavigationStack` + `Router` | navigation-compose + the same `Router` | Deep links are declarative |
| Token storage | Keychain | Android Keystore AES-GCM (hand-rolled, ~50 lines) | See below |
| Preferences | `UserDefaults` | `SharedPreferences` | 1:1 |
| Biometrics | LocalAuthentication | androidx.biometric | `DEVICE_CREDENTIAL` fallback gives the same "never lock out an on-call engineer" semantics |
| Widget | WidgetKit | Glance | The only Compose-native widget API |
| Dependency injection | Environment objects | Manual container + `CompositionLocal` | No Hilt. One less thing to understand |

**Deliberately skipped, with the reason:**

- **Push notifications / FCM.** The iOS app registers with APNs and then tells
  the user plainly that nothing can arrive because the control plane has no push
  sender. Shipping that on Android would mean a Firebase dependency, a
  `google-services.json`, and a Data-safety disclosure, all for a feature that
  cannot work. Add it when the control plane grows a sender.
- **Live Activities.** No Android equivalent. Already disabled on iOS.
- **Siri/Shortcuts equivalent** (App Actions). Read-only extras on iOS, not core.
- **Room / any database.** Nothing is persisted but instance records and
  settings.
- **Retrofit.** The API client uses explicit paths and hand-built bodies so that
  a nil optional is *omitted* rather than sent as null (the server decodes with
  `DisallowUnknownFields`). An interface-generation layer buys nothing here.

### Why the token store is hand-rolled

`androidx.security-crypto` (`EncryptedSharedPreferences`) is still at an alpha
version and is no longer the recommended path. Writing Keystore AES-GCM directly
is about 50 lines and gives exactly the iOS Keychain semantics that matter:

- `setUnlockedDeviceRequired(true)` mirrors
  `kSecAttrAccessibleWhenUnlockedThisDeviceOnly` — unreadable while locked, so a
  background refresh cannot leak a token from a locked phone.
- Keystore keys are non-exportable and never leave the device, which mirrors
  "excluded from encrypted backups and device-to-device migration". Paired with
  `android:allowBackup="false"`, a restored backup on someone else's phone
  carries no usable credential.

The trade-off is the same one iOS documented and accepted: an operator re-pastes
a token after restoring a device, which beats a control-plane token travelling
inside a backup.

## Project shape

```
android/
  settings.gradle.kts            Gradle 9.1, version catalog
  gradle/libs.versions.toml
  app/
    src/main/
      AndroidManifest.xml
      assets/fixtures/*.json     copied verbatim from ios/.../Resources/Fixtures
      kotlin/org/fictionally/ringpromoter/
        model/        Ring, RingStatus, Job, PromotionLegality, …
        net/          RingPromoterApi, HttpClient, ApiError, Json
        demo/         DemoClient, DemoWorld, DemoJob, Fixtures
        store/        Instance, InstanceStore, TokenStore, Settings, Session, BiometricGate
        ui/           theme/, overview/, detail/, job/, rings/, activity/,
                      history/, maintenance/, groups/, settings/, onboarding/
        widget/       Glance pipeline widget
    src/test/         JVM unit tests (ports of RingPromoterTests)
    src/androidTest/  one critical-path Compose test
```

`applicationId` = `org.fictionally.ringpromoter`, matching the iOS bundle ID.
**This is permanent on Play and must be confirmed before the first upload.**

`minSdk 26` / `compileSdk 36` / `targetSdk 36`. versionName starts at `1.0`,
versionCode `1` — this is a fresh Play listing, not a continuation of iOS 1.10.

## Fixtures are copied, not rewritten

`ios/RingPromoter/Resources/Fixtures/` holds 50 **real captured responses** from
a locally-run backend, not hand-written approximations. They back both the iOS
decoding tests and demo mode. They are copied into `assets/fixtures/` byte for
byte, so:

- the Android decoding tests fail if the Go models change, exactly as the iOS
  ones do;
- demo mode shows the same data a reviewer already saw on iOS;
- nobody has to invent a second, divergent set of demo data.

## Phases

Each phase ends with something runnable and one check that fails if the phase's
logic breaks. No phase is "done" without its check.

**Status: phases 0–4 are complete and green.** 106 JVM unit tests pass, and the
whole critical path — demo mode, Overview, app detail, action sheet, live job —
was walked on an emulator and screenshotted. Phases 5–8 remain.

### Phase 0 — it builds and runs ✅
Gradle skeleton, version catalog, manifest, Compose scaffold, theme ported from
the iOS design system (health colours, instance tints). Installs on the running
emulator and shows a placeholder.
**Check:** `assembleDebug` succeeds; app launches on `emulator-5554`.

### Phase 1 — the domain ✅
All of `model/`: `Ring`/`RingPipeline`, `RingStatus`/`RingGates`, `Job` and its
status enums, `AppVersion`, `AppsResponse`, `AppGroup`, `HistoryEntry`,
`Maintenance`, `Signoff`, `TestKit`, `ActionResult`, and `PromotionLegality` +
`PromotionRules`. Unknown enum values decode to `unknown` rather than throwing,
as on iOS. Go's zero timestamp (`0001-01-01T00:00:00Z`) maps to a distant past
sentinel, and RFC3339 is accepted with *and* without fractional seconds —
both forms occur in a single response.
**Check:** port `ModelDecodingTests` and `PromotionRulesTests` against the copied
fixtures. These are the two highest-value test files in the iOS suite.

### Phase 2 — networking and demo mode ✅
`RingPromoterApi` interface, the OkHttp client, the status-code → `ApiError`
mapping (401 → token screen, 403 → password field, 409 → gate explanation — never
collapsed into "something went wrong"), `UnconfiguredClient`, then
`DemoWorld`/`DemoClient`/`DemoJob` with fixture timestamps rebased onto now.
**Check:** port `ClientTests` (status mapping, token header, omitted-vs-null
bodies) and `DemoModeTests`.

### Phase 3 — session and storage ✅
`Instance` + URL normalisation, `InstanceStore`, the Keystore `TokenStore`,
`Settings`, `Session` with the generation counter, `BiometricGate`.
**Check:** port `MultiInstanceTests`, including that a stale response from the
previous control plane is dropped.

### Phase 4 — the critical path ✅
Onboarding → Overview → App detail → action sheets and gate sheets → version
picker → live job view with steps and logs. This is the path a reviewer and an
operator both actually walk.
**Check:** walked on a device and screenshotted at every step, driven by
`scripts/tap.sh`, which taps by *label* rather than by coordinate so a layout
change fails the walk loudly instead of silently tapping empty space. A Compose
instrumentation test still to be added in phase 5.

### Phase 5 — the rest of the screens
Activity feed, History, Maintenance windows, Groups, Settings + control-plane
management + instance switcher, Test kits, lock screen, re-authentication.
**Check:** `NavigationAndStoreTests` port.

### Phase 6 — Rings of Applications ("Descent")
The hardest screen: Earth at the centre, promotion rings as orbits (int
outermost, prod just above Earth), one straight spoke per app with a lit trail
showing how far its newest version has travelled. Falls back to "lanes" (one row
per app, one column per ring) on a narrow stage or a large fleet.

The geometry is already written three times — `ios/.../DescentLayout.swift`,
`ios/.../EarthGlobe.swift`, and the original `web/src/lib/descent-layout.ts`.
Port the maths as pure functions first and test it against the iOS test
expectations, then draw it with Compose `Canvas`. Honour
`Settings.Global.ANIMATOR_DURATION_SCALE == 0` as iOS honours Reduce Motion.
**Check:** `RingsUniverseTests` port — pure geometry, no rendering.

### Phase 7 — platform extras
Glance widget (pipeline snapshot, no token in the snapshot), `ringpromoter://`
deep links, biometric app lock, theme switching.
**Check:** widget snapshot round-trips; deep link opens the right app.

### Phase 8 — ship
App icon, feature graphic, phone screenshots from the emulator, signing key,
Play release lane in CI mirroring `ios-release.yml`. Feeds the Play Console
prompt already written.

## Toolchain notes

Settled by trial during phase 0, so nobody has to repeat it:

- **AGP 9.5.1 / Kotlin 2.4.21 / Gradle 9.9.1 / compileSdk 37**, targetSdk 36.
  The whole current AndroidX line requires compileSdk 37 and AGP ≥ 9.1, so
  staying on 36 would have frozen every future library upgrade on day one.
  `platforms;android-37.0` and `build-tools;37.0.0` were installed for this.
- **AGP 9 has built-in Kotlin support.** Applying `org.jetbrains.kotlin.android`
  alongside it is now a hard error; only the `compose` and `serialization`
  compiler plugins are applied.
- **OkHttp is pinned to 4.12.0.** OkHttp 5.x requires compileSdk 37 *and* a
  newer AGP than the one available when this was set up, and buys nothing here.
- The framework has no `Theme.DeviceDefault.DayNight.NoActionBar`, and AppCompat
  is not a dependency, so the pre-Compose window background is day/night split
  with a `values-night` colour instead.

Build and test with Android Studio's bundled JDK:

```sh
cd android
JAVA_HOME="/Applications/Android Studio.app/Contents/jbr/Contents/Home" ./gradlew test assembleDebug
```

## Order of work, and what I will ask about

Phases 0–3 need no decisions — they are a mechanical port of logic that already
has tests.

Two things to settle before they bite:

- **The `applicationId`**, before anything is uploaded to Play. Permanent, and
  currently set to `org.fictionally.ringpromoter` to match iOS.
- **Whether Phase 6 is wanted in v1.** The Descent screen is roughly a fifth of
  the UI work for a screen that is beautiful rather than load-bearing; the lanes
  fallback alone would get the same information on screen for a tenth of the
  effort. Shipping v1 without the orbital rendering is a defensible call, and
  the tab can arrive in 1.1.
