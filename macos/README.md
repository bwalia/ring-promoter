# Ring Promoter for Mac

A native macOS app — not Catalyst, and not a rewrite. It shares the iOS app's
source in place: the same models, networking, stores, screens and design
system. Only the window shell is Mac-specific.

```
macos/
  project.yml                 the Xcode project, as a spec
  RingPromoterMac/
    App/
      RingPromoterMacApp.swift   @main, window sizing, ⌘1…⌘4, ⌘F
      MacRootView.swift          sidebar in place of the tab bar
      MacSidebar.swift           the console's navigation column
      MacOverviewView.swift      the application list, as cards
      MacAppDetailView.swift     one pipeline, rings side by side
    DesignSystem/
      ConsoleTheme.swift         the web client's tokens, in SwiftUI
      ConsoleComponents.swift    card, nav row, section label, filter box
                                 and PanelGrid, the wrapping panel layout
      ConsoleSnapshot.swift      offscreen renderer (DEBUG only)
      ConsoleAppIcon.swift       the icon, drawn (DEBUG only)
      ConsoleDiagnostics.swift   Keychain/network probe (DEBUG only)
    Compatibility/
      PlatformModifiers.swift    SwiftUI spellings AppKit lacks
      PlatformServices.swift     UIKit services AppKit lacks
    Resources/
      Assets.xcassets            AccentColor only
```

## Building

The `.xcodeproj` is **generated and git-ignored**. Build it with:

```sh
cd macos
xcodegen generate
open RingPromoterMac.xcodeproj
```

Or from the command line:

```sh
xcodebuild -project RingPromoterMac.xcodeproj -scheme RingPromoterMac \
  -configuration Debug -destination 'platform=macOS' build
```

Add or remove a shared file and re-run `xcodegen generate` — nothing needs
editing by hand.

**The iOS project is not generated.** `ios/RingPromoter.xcodeproj` is
hand-maintained, and regenerating it would discard code signing, the widget
extension and the Live Activity configuration. This spec only reads from it.

## What is shared, and what is not

65 Swift files make up the iOS app. 61 of them compile here untouched. Four are
excluded and replaced:

| Excluded | Replaced by | Why |
|---|---|---|
| `App/RingPromoterApp.swift` | `RingPromoterMacApp.swift` | `@main` may exist once; `@UIApplicationDelegateAdaptor` has no macOS form |
| `App/AppDelegate.swift` | — | `UIApplicationDelegate`. Only ever referenced from the file above |
| `DesignSystem/Haptics.swift` | `PlatformServices.swift` | UIKit feedback generators → `NSHapticFeedbackManager` |
| `Stores/LiveActivityController.swift` | `PlatformServices.swift` | ActivityKit is iPhone-only; replaced by a no-op with an identical surface |

`DesignSystem/ScreenPreviews.swift` is also excluded — SwiftUI previews that
name iOS preview devices.

Three shared files carry a Mac-driven change rather than being forked. All
three are inert on iOS, and the full 147-test suite is run against them:

- `DesignSystem/HealthPresentation.swift` — the `Color(light:dark:)` token
  initialiser, `UIColor`'s trait provider on iOS and `NSColor`'s appearance
  provider here. **Every `rp*` colour depends on this**, so it is the one place
  the design system had to learn about a second platform.
- `Stores/PushRegistration.swift` — `import UIKit`, for `UIApplication` alone.
- `Features/AppDetail/AppDetailView.swift` — `RingCard` and
  `UnhealthyRingCallout` are internal rather than private, so the Mac can lay
  the same components out differently without restating a single promotion
  rule; and the ring label no longer wraps, because one word can only
  hyphenate ("Produc-tion" in a narrow panel). Both are inert at phone width.

Everything else bends in `Compatibility/`, which exists so that no screen file
needed an `#if os(macOS)` added to it. The iOS source stays the single
definition of every screen.

## Matching the web console

The first build of this target used stock AppKit chrome — a source list with
system-accent selection, and the shared grouped `List`, which macOS renders as
a plain hairline-separated table. It worked and it looked like nothing else
the product ships.

`DesignSystem/ConsoleTheme.swift` is a transcription of the web client's design
tokens from `web/src/app/globals.css`: `--background`, `--card`, `--sidebar`,
`--accent`, `--border`, `--muted-foreground`, the `--radius` scale and the
`--rp-font-*` type scale. Those tokens are under an explicit theme lock — see
`web/AGENTS.md` — so this is a transcription and not a new palette. Every token
is achromatic, which makes the oklch → sRGB conversion exact and lands the ramp
on Tailwind's `neutral` scale (`#0A0A0A`, `#171717`, `#262626`, `#E5E5E5`,
`#FAFAFA`).

Health colours are deliberately **not** redefined. `rpHealthy`, `rpUnhealthy`,
`rpGate`, `rpInFlight` and `rpProduction` stay in the shared
`DesignSystem/HealthPresentation.swift`, so a ring that is amber on the iPhone
is amber here. The web supplies the *surfaces*; iOS supplies the *states*.

One honest gap: the web loads Poppins, Unbounded and JetBrains Mono from Google
Fonts. None is on macOS, and the only copies in this workspace are `.woff2`,
which CoreText cannot register — converting them needs `fonttools` + `brotli`,
which this machine does not have. The faces here are San Francisco. The *scale*
is the web's: 15pt body, 21pt bold headings, the -0.02em display tracking.

### Seeing it

A screenshot of the running window needs macOS Screen Recording permission,
which a terminal running the build does not have — so a UI change could be
built, launched, and never actually looked at. `ConsoleSnapshot` renders the
same views offscreen instead, in both appearances, from the bundled demo
fixtures:

```sh
# The target is sandboxed, so it can only write inside its own container.
SHOTS=~/Library/Containers/org.fictionally.ringpromoter/Data/tmp/shots
mkdir -p "$SHOTS"
"$(xcodebuild -project RingPromoterMac.xcodeproj -scheme RingPromoterMac \
    -configuration Debug -showBuildSettings \
    | awk -F' = ' '/ BUILT_PRODUCTS_DIR/{d=$2} / FULL_PRODUCT_NAME/{n=$2} END{print d"/"n}'
  )/Contents/MacOS/RingPromoterMac" --snapshot "$SHOTS"
open "$SHOTS"
```

`ImageRenderer` draws neither a `ScrollView` nor a `NavigationStack`, which is
why `MacSidebarSections` and `OverviewCardStack` are separate views: the parts
worth looking at sit outside those wrappers and can be rendered on their own.
It is `#if DEBUG` and unreachable without the argument.

It cannot draw a `Form`, a `List` or a `Canvas` at all — they come out as an
empty rectangle. So the sheets, the Settings tab, the Activity feed and the
Rings stage have to be checked by eye in the running app. Rendering them was
tried and the files came back blank; there is no point adding them back.

## The design decisions

iOS puts the four destinations in a tab bar. This target puts the same four in a
sidebar, read from the same `Router.Tab` — so adding a tab on iOS adds a row
here, and the two cannot drift.

A `TabView` does compile on macOS, so sharing `RootView` unchanged was possible.
It was not taken: it strands an iPhone tab strip across a 1180pt window and
leaves the sidebar — where a Mac operator looks for navigation — empty.

The sidebar also carries the live application list, which is what the web
console's sidebar is mostly made of. `OverviewStore` moved up into
`MacRootView` so that list stays current on every tab and so one filter box
narrows both it and the cards in the main pane — one store, two views of it,
no second copy of the loading or sorting logic.

The application list itself is `MacOverviewView`, a Mac-only *container* around
the same store: the web draws panels (`rounded-lg border bg-card`) where the
phone draws a grouped `List`. `PipelineStrip`, `StatusBadge`, `ErrorRow`,
`CalmEmptyState` and `RelativeTimestamp` inside those panels are the shared
components, unchanged.

Rows are hand-built rather than `List` rows for one concrete reason: AppKit
paints source-list selection in the system accent colour, and this console has
no accent colour — its emphasis is a neutral `--sidebar-accent` fill under an
11px uppercase label.

Added for the Mac, with nothing on iOS to mirror:

- **⌘1…⌘4** for the four destinations, disabled while disconnected.
- **⌘F** to focus the sidebar filter. The web focuses its filter on `/`; a Mac
  expects ⌘F, and the key cap drawn on the field says so.
- **A connection footer** pinned under the sidebar, naming the control plane or
  showing "Demo mode". On iOS this is one tap away in Settings; a Mac window
  stays open for hours across several planes, so "which server am I about to
  promote on" belongs permanently on screen.
- **An 880×560 minimum**, below which the sidebar and a ring panel cannot
  both stay readable.

### The app icon

Drawn, not painted. `ConsoleAppIcon` is a SwiftUI view on a 1024pt canvas, and
`--snapshot` renders it at all ten sizes macOS asks for plus the `Contents.json`
that maps them — so every size is the same artwork and none can drift.

The mark is the product's own metaphor rather than decoration: three rings with
production lit at the centre — int, test, acc, prod — and a promotion in flight
on the outermost one, its head leading a fading trail. Strokes are heavier than
they need to be at 512pt because one drawing has to survive being shrunk to 32,
where only the lit core really reads.

The PNGs are committed, because the generator is `#if DEBUG` and a release
build has to find them already there. To change the icon, edit the view and
re-run `--snapshot`, then copy from the snapshot directory into
`Resources/Assets.xcassets/AppIcon.appiconset/`.

### The detail screen

`AppDetailView`'s grouped `List` is right on a phone and wrong on a Mac: macOS
renders it as a plain full-bleed table, so rows ran to the window edges and
"Promote to test" — a full-width button, correct at 343pt — became a 900pt
capsule.

`MacAppDetailView` is a container around the *same* `AppDetailStore` and the
*same* `RingCard`. Not one legality check, action call or gate rule is restated,
which matters more here than anywhere else in the app: what an operator is
allowed to promote has exactly one implementation across all three clients.

The arrangement follows `web/src/components/dashboard/pipeline.tsx`, which
switches to `xl:flex-row` and puts the rings side by side as panels. `PanelGrid`
is a custom `Layout` because:

- `ViewThatFits` was tried first and is the wrong tool. It measures a
  candidate's *ideal* width, and a row of flexible panels has none — it decided
  the horizontal arrangement never fit and silently picked the vertical one at
  every pane width.
- Wrapping has to be balanced. A pipeline is a sequence, and four rings laid
  out 3 + 1 orphan production on a row of its own. `PanelGrid` works out the
  row count first and spreads panels evenly, so a narrow window gives 2 + 2.

The window's default width is 1280 — the web console's own `xl` breakpoint —
so the pipeline opens as one row and degrades to 2 x 2 as the window narrows.

Above the pipeline sit three figures, the same shape
`web/src/components/dashboard/overview-cards.tsx` puts across the top of its
dashboard: what is in production, whether the rings are healthy, and how many
report a live version other than the one deployed. Deliberately *not* "this
server needs a production password" — that is true of every application on the
server, so it belongs on the confirmation sheet where it changes what you do,
not repeated across every pipeline.

While any of that is loading, both panes draw `ConsoleSkeleton` blocks shaped
like the content that is coming, which is what the web does with `<Skeleton>`.
A spinner says only "wait"; a skeleton says what to expect and stops the pane
jumping when it arrives.

## Not yet here

Nothing below is stubbed out as if it works — each is genuinely absent:

- **Widgets.** `WidgetSnapshot.swift` is compiled in and the write path runs,
  but with no App Group entitlement `SharedContainer.containerURL` is nil and
  every write is a no-op by design. A Mac widget target would need that
  entitlement adding, with the `$(TeamIdentifierPrefix)` prefix macOS requires.
- **Push.** `PushRegistration` forwards to `NSApplication` correctly, but the
  target carries no push entitlement, and the Go backend still has no sender —
  the same boundary the iOS app documents.
- **Live Activities.** iPhone-only. The Mac equivalent would be a user
  notification, which is separate work and is not pretended at.
- **The web's typefaces**, as above.
- **Pipelines, the audit log and topology editing.** `GET /api/pipelines`,
  `/api/pipeline-runs/*`, `/api/audit` and `/api/topology/edges` all exist on
  the server and neither app has client code for them.
- **History search and filtering.** The web lists every deployment, searchable;
  the apps show the six most recent and a link to the full screen.
- **Activity, Settings and the Rings stage, verified by eye.** They are themed,
  but `ImageRenderer` cannot draw a `List`, a `Form` or a `Canvas`, so unlike
  the sidebar, the cards and the pipeline they have never been looked at.
## Submitting to the App Store

Run the preflight first; it reports what is actually missing rather than
what a build log will complain about later:

```sh
cd macos && ./scripts/verify-mac-release-readiness.sh
```

Two of its blockers need the Apple account and cannot be done from a script:
an **Apple Distribution** certificate and a **macOS provisioning profile**.
The app currently has neither — `security find-identity` shows only an Apple
Development certificate, which the App Store will not accept.

The shortest honest path for a *first* macOS submission is Xcode, because it
creates both interactively:

1. `cd macos && xcodegen generate && open RingPromoterMac.xcodeproj`
2. Xcode → Settings → Accounts → add the Apple ID, select the team.
3. Set the target's signing to **Automatic**; Xcode will create the
   distribution certificate and the profile.
4. Product → Archive, then **Distribute App → App Store Connect**.

Before that upload can be submitted, in App Store Connect:

- There must be a record for the **macOS platform** of
  `org.fictionally.ringpromoter`. A native Mac app is not automatically
  covered by the iOS record — check whether macOS can be added as a platform
  to the existing app or whether it needs its own.
- Any pending agreements must be accepted. Nobody can do that on your behalf.
- Mac screenshots and a macOS description are required.

**There is no `macos-release.yml`.** The iOS and Android lanes exist because
they were built against working credentials; writing a third one blind is how
`android-release.yml` came to be committed in a state that would have failed on
the first step of its first release. It is worth building once a distribution
certificate exists, so its first run can be a test rather than a guess.

## Signing

`CODE_SIGN_IDENTITY` is `Apple Development` against team `2DL868X4M5`, not
ad-hoc. That is not cosmetic: an ad-hoc binary has no team identifier, so its
code signing requirement is its own hash and changes on every rebuild — and the
legacy macOS keychain, which ties each item to the signature that wrote it,
then asks the operator for their login password after every build. See the
comment in `ios/RingPromoter/Stores/Keychain.swift`.

A clone without a certificate for that team will not build. Either set your own
team in `project.yml`, or pass it per build:

```sh
xcodebuild ... CODE_SIGN_IDENTITY=- CODE_SIGN_STYLE=Manual DEVELOPMENT_TEAM=
```

Two things this does **not** give:

- **The data-protection keychain.** It needs an `application-identifier`
  entitlement, which comes from a provisioning profile, which needs the Apple
  account added in Xcode. Until then `Keychain.swift` falls back to the legacy
  keychain — correctly, and `--diagnose` says which one is in use.
- **Distribution.** Running on another Mac needs *Developer ID Application*
  plus notarisation, which is a different certificate.

## Not yet here (continued)

- **Code signing for distribution**, as above.
