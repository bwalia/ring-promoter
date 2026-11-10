# App Store release runbook

Everything App Review looks at for Ring Promoter iOS: what the repository
already handles, what still needs a human, and the specific rejection risks
that remain. Written for whoever submits the build.

Run the automated half first — it checks most of this page:

```bash
cd ios
./scripts/verify-app-store-readiness.sh --online
```

`fastlane ios release` runs the same script and refuses to upload if it fails.

---

## 0.1. One-time Developer Portal capabilities

Two capabilities must be switched on for the App IDs, by hand, once each:

| App ID | Capability |
| --- | --- |
| `org.fictionally.ringpromoter` | Push Notifications, App Groups |
| `org.fictionally.ringpromoter.widget` | App Groups |

The App Group `group.org.fictionally.ringpromoter` must also be assigned to
both identifiers.

**CI cannot do this.** `fastlane ios prepare_signing` checks the capabilities
and fails with instructions if one is missing, but it cannot turn one on: the
spaceship call behind it (`patch_bundle_id_capability`) puts
`provisioning_request_client.team_id` in the request body, and resolving that
hits the legacy `/olympus/v1/session` endpoint, which only a username/password
Apple ID session can authenticate. An App Store Connect API key gets
`Unauthenticated` back and fastlane dies parsing it as JSON.

So it needs a browser and an account with **Developer Portal access** —
Account Holder, or an Admin whose role includes Certificates, Identifiers &
Profiles. Note that App Store Connect access alone is *not* enough: a user can
have full App Store Connect rights and still land on a registration page at
`developer.apple.com/account`.

1. <https://developer.apple.com/account/resources/identifiers/list/bundleId>
2. Open the identifier, tick the capability, Save.

Once ticked it stays ticked, so this is a per-App-ID setup step, not a
per-release one. A rebrand onto new bundle identifiers has to repeat it.

## 1. Before anything else: the publisher details

Nothing about the publisher is hard-coded. The legal pages and the store
listing are both built from environment variables, so releasing this app under
a different company needs no code change — only different values.

They live in **Vault**, in the flat KV v2 secret
`secret/ring-promoter/config` — the same Vault the iOS signing material
comes from, so the publisher identity has one source of truth rather than a
copy in each CI provider's settings, and the privacy policy and the App Store
listing can never name different companies.

`vault kv patch` adds or changes one key; `vault kv put` replaces the whole
secret, so use patch unless you mean to wipe the rest.

```bash
vault kv patch secret/ring-promoter/config \
  PUBLISHER="Example Ltd" \
  CONTACT_EMAIL="support@example.com" \
  JURISDICTION="England and Wales" \
  POSTAL_ADDRESS="1 Example Street|London|EC1A 1AA" \
  REVIEW_FIRST_NAME="Ada" REVIEW_LAST_NAME="Lovelace" \
  REVIEW_PHONE="+44 20 7946 0000"
```

**Keep this separate from `secret/ring-promoter/ios`.** That neighbouring
secret holds the App Store Connect API key, and `ios/ci/load-ios-vault-secrets.sh`
requires `ASC_KEY_ID`, `ASC_ISSUER_ID` and `ASC_PRIVATE_KEY_B64` from it.
Writing branding into it once already replaced those and left the release
unable to sign — KV v2 keeps versions, so
`vault kv get -version=<n> secret/ring-promoter/ios` recovers them if it
happens again.

The split is not bureaucracy: the branding is not iOS-specific — the website
build reads the same values for the legal pages — so merging the two would
mean a Pages build reading Apple distribution credentials in order to render
a privacy policy.

| Secret | Holds | Read by |
|---|---|---|
| `secret/ring-promoter/config` | Publisher branding | Pages build, iOS release |
| `secret/ring-promoter/ios` | App Store Connect API key, team id | iOS release only |

`ci/load-branding-from-vault.sh` reads the branding one and exports the
values; both workflows run it before they need them. Nothing here is a GitHub secret or a
repository variable. None of it is secret in the first place — every value
ends up on a published page or the public listing — but keeping it beside the
signing material means one place to change for a rebrand.

Both workflows that need it now run on the self-hosted **mac-studio** runner,
because that is where the Vault token file lives. For the Pages deploy that is
a change: its build used to be GitHub-hosted to avoid queueing behind cluster
deploys.

| Variable | Used for | Required |
|---|---|---|
| `PUBLISHER` | Named on all three legal pages, and the listing copyright | yes |
| `CONTACT_EMAIL` | Contact on the legal pages; App Review contact by default | yes |
| `JURISDICTION` | Governing law in the terms | yes |
| `POSTAL_ADDRESS` | EULA clause 8 and EU trader details. `\|`-separated lines | yes |
| `REVIEW_FIRST_NAME` | App Review contact | yes |
| `REVIEW_LAST_NAME` | App Review contact | yes |
| `REVIEW_PHONE` | App Review contact | yes |
| `REVIEW_EMAIL` | App Review contact, if different from `CONTACT_EMAIL` | no |
| `COPYRIGHT` | Pins a first-publication year; defaults to `<year> <publisher>` | no |
| `APP_NAME`, `BUNDLE_ID`, `GITHUB_URL`, `SITE_URL` | White-label overrides; default to this project | no |
| `LEGAL_LAST_UPDATED` | "Last updated" date on the legal pages | no |

`web/.env.example` is the template; copy it to `web/.env.local` for local work.

Three constraints on the values:

- **The support mailbox must actually receive mail.** App Review writes to it,
  and an address that bounces is a Guideline 1.5 rejection.
- **The postal address is not optional.** Clause 8 of Apple's Minimum Terms
  for a developer-supplied EULA requires it, and the App Store also requires
  trader details for EU storefronts (see §4).
- **The publisher must match everywhere.** It does automatically, because the
  website and fastlane read the same variable — a privacy policy and a store
  listing naming different companies is exactly the mismatch App Review
  notices.

Missing values fail loudly rather than publishing a blank: `next build` throws
and names the variable, and `verify-app-store-readiness.sh` fails before
fastlane uploads anything.

Once they are set, redeploy the site — the legal pages publish from `web/`,
through `.github/workflows/deploy-pages.yml`, on a push to `main` that touches
`web/**`. The three URLs must return 200 **before** you submit.

### Releasing under a different brand

1. Change the values in `secret/ring-promoter/config` in Vault.
2. Edit `ios/fastlane/metadata/en-GB/` — the listing name, subtitle,
   description, keywords and URLs are per-brand *content*, not configuration,
   so they stay as reviewable files rather than becoming environment
   variables.
3. Change `PRODUCT_BUNDLE_IDENTIFIER`, the app group and the Apple team in the
   Xcode project and `fastlane/Fastfile`, and recapture screenshots.

---

## 2. What the repository now handles

| Item | Where | Why it matters |
|---|---|---|
| Privacy manifest | `RingPromoter/PrivacyInfo.xcprivacy` | Without it an upload that touches a required-reason API is answered with **ITMS-91053 Missing API declaration**. The app uses `UserDefaults`, declared with reason `CA92.1`. |
| Export compliance | `ITSAppUsesNonExemptEncryption = false` in `Config/RingPromoter-Info.plist` | Unanswered, the build sits at *Missing Compliance* and cannot be submitted. `false` is accurate: only iOS-provided TLS and Keychain are used. |
| Push entitlement | `aps-environment` in `Config/RingPromoter.entitlements`, driven by `APS_ENVIRONMENT` | The app calls `registerForRemoteNotifications()`. Without the entitlement that fails and Settings prints the raw error — a feature a reviewer can tap into a broken state (Guideline 2.1). `fastlane ios prepare_signing` asserts the capability is on the App ID and that the profile carries it. It cannot switch it on — see §0.1. |
| Privacy policy, terms, support pages | `web/src/app/{privacy,terms,support}/` | Privacy Policy URL and Support URL are required fields. The terms include Apple's Minimum Terms for a developer-supplied EULA in full. |
| Store listing text | `fastlane/metadata/` | Reviewable in a pull request instead of retyped into App Store Connect. Every character limit is checked. |
| Review notes | `fastlane/metadata/review_information/notes.txt` | The single highest-value field for this app — see §3. |
| Screenshots | `fastlane/screenshots/en-GB/` | Apple accepts a fixed list of pixel sizes and rejects anything else at upload. |
| App icon | 1024×1024, RGB, no alpha | An alpha channel in the icon is an upload rejection. |

`deliver` uploads metadata and screenshots from this repository
(`skip_metadata` and `skip_screenshots` are both `false`), so **what is
committed wins** over whatever is currently in App Store Connect.

### Locale

Metadata lives in `fastlane/metadata/en-GB/` and screenshots in
`fastlane/screenshots/en-GB/`. That must match the **primary language** of the
app record in App Store Connect. If the record says English (U.S.), rename both
directories to `en-US` and pass `LOCALE=en-US` to the scripts — otherwise
`deliver` fails with *locale not available*.

---

## 3. Why this app needs good review notes

Ring Promoter is a client for a control plane the operator runs themselves.
A reviewer who opens it with no server to point it at sees an onboarding screen
asking for a URL and a token — and an app that appears not to work is a
Guideline 2.1 rejection.

The app answers this properly: **demo mode** drives every screen from bundled
data with no network at all, including the gates that refuse an illegal
promotion and a genuine failed-and-rolled-back deploy.
`review_information/notes.txt` walks the reviewer through it and gives the two
demo-mode credentials (production password `demo`, change-request code `test`).

Read that file before submitting. If the app changes, it changes.

In App Store Connect, **Sign-in required: No** — there is no account to create,
and so no account-deletion obligation under Guideline 5.1.1(v) either.

---

## 4. App Store Connect, field by field

Nothing below is in the repository; all of it is set once in the web UI.

**App Information**

| Field | Answer |
|---|---|
| Primary category | Developer Tools |
| Secondary category | Utilities |
| Age rating | 4+ — answer *None* to every content question |
| Content rights | Contains no third-party content |

**Pricing and Availability** — set the price (free unless you decide
otherwise) and the territories.

**Trader status (EU Digital Services Act).** Under *Business* → *Trader
Status*, declare whether you are a trader and supply the name, address, phone
and email. Apple verifies it, and an app without verified trader details is
removed from EU storefronts. Use the same address as `POSTAL_ADDRESS`.

**App Privacy** — the nutrition label. Answer:

| Question | Answer |
|---|---|
| Do you collect data from this app? | **No** |
| Does the app use data to track you? | **No** |
| Third-party partners with access | **None** |

That is accurate and matches the bundled `PrivacyInfo.xcprivacy`: the app has
no analytics, no crash reporting, no telemetry and no third-party SDK at all.
Nothing is transmitted anywhere except to the control plane the operator
entered. On-device storage — the Keychain token, the saved server addresses —
is not "collection" under Apple's definition.

**Version Information** — description, keywords, screenshots, support URL and
privacy policy URL all arrive from `deliver`. Set **Version Release** to
*Manually release this version* so a passed review does not go live
unattended; `automatic_release: false` in the Fastfile already asks for this.

**App Review Information** — arrives from `deliver` once §1 is filled in.

---

## 5. Remaining rejection risks, most likely first

Ranked by how often Apple actually acts on them for an app of this shape.

### iPad layout — Guideline 4.0 / 2.4.1

The app ships for iPhone **and** iPad (`TARGETED_DEVICE_FAMILY = "1,2"`) and
has no iPad-specific layout: no size-class branching, no `NavigationSplitView`.
It is a `TabView` of `NavigationStack`s, which SwiftUI renders correctly at
iPad width but not distinctively — Apple does reject apps that read as a
stretched phone app.

Review the committed `ipad13-*` screenshots with that in mind before
submitting. If any screen looks like a phone layout centred in a lot of empty
space, the cheapest fix is to ship iPhone-only for this version:

```
TARGETED_DEVICE_FAMILY = "1"      # in both app-target configs
```

That also removes the 13" iPad screenshot requirement. iPad can return as a
later version once a split-view layout is actually built.

### Minimum functionality — Guideline 4.2

A niche operator tool occasionally draws this. The mitigation is already in
place and should stay visible in the screenshots: the app is not a thin web
wrapper, it is a native client with widgets, biometric gating, App Intents and
a working offline mode.

### A dead-end feature a reviewer can reach — Guideline 2.1

Settings offers notification permission, and after this change registration
succeeds — but **no notification can ever arrive**, because the control plane
has no push sender. The screen says so in its footer, which is why this is a
risk and not a defect.

If a reviewer flags it, the honest answer is in the footer text and in
`docs/API-GAPS.md`. The alternative, if you would rather not argue it, is to
hide the Notifications section until the backend can send one.

### Broken links in metadata — Guideline 2.1

The description links nothing, but the support and privacy URLs are fetched
during review. `verify-app-store-readiness.sh --online` checks all three return
200. Run it *after* the Pages deploy, not before.

---

## 6. Submission sequence

```bash
cd ios

# 1. Fill in §1, then prove it.
./scripts/verify-app-store-readiness.sh

# 2. Deploy the website so the legal URLs resolve, then re-check online.
#    (push to main touching web/** → deploy-pages.yml)
./scripts/verify-app-store-readiness.sh --online

# 3. Recapture screenshots if any screen has changed.
./scripts/capture-app-store-screenshots.sh
```

Then, in GitHub Actions:

1. Run **iOS Release** with `target: testflight`. Install the build and walk
   the critical paths on a real device — including Face ID and the widget,
   neither of which the simulator exercises honestly.
2. Confirm in App Store Connect that the build shows no *Missing Compliance*
   warning and that no ITMS-91053 mail arrived.
3. Set the §4 fields that are not in the repository.
4. Run **iOS Release** with `target: app_store`. The workflow runs the verifier
   with `--online` before it builds, then `deliver` uploads the listing and
   submits for review.
5. Review is typically 24–48 hours. Because `automatic_release` is false, a
   pass waits for you to release it.

---

## 7. What was deliberately not done

- **No widget privacy manifest.** The widget extension calls no
  required-reason API — it reads one JSON file from the App Group container.
  Add one the moment it touches `UserDefaults`; `@AppStorage` counts, and the
  verifier's API scan covers both targets.
- **Live Activities stay disabled.** `NSSupportsLiveActivities` is `false`, the
  widget bundle omits `PromotionLiveActivity`, and `LiveActivityController`
  is hard-off after an unexplained crash. All three agree, which is the point.
  Nothing in the store listing or the screenshots claims the feature.
- **No certificate pinning.** Operators point this at their own control planes
  with their own certificates, often rotated by cert-manager; a pin the app
  cannot update turns a routine renewal into a broken app.
- **No analytics.** Which is what makes the "Data Not Collected" declaration in
  §4 true, and worth keeping true.
- **No in-app links to the privacy policy or terms.** Apple requires the
  Privacy Policy *URL* on the listing, which is covered, and an in-app link is
  only mandatory for apps with accounts or subscriptions — this app has
  neither. It is still a reasonable addition to Settings, and the pages are
  already live; it was left out here because changing a screen invalidates
  every committed screenshot, and that trade belongs to whoever owns the UI.
- **Only 10 of the 15 test screenshots ship.** `capture-app-store-screenshots.sh`
  holds the curated list and the order shoppers see. The full 15 stay in
  `docs/screenshots/` as documentation.
