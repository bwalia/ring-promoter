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
    DesignSystem/
      ConsoleTheme.swift         the web client's tokens, in SwiftUI
      ConsoleComponents.swift    card, nav row, section label, filter box
      ConsoleSnapshot.swift      offscreen renderer (DEBUG only)
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

Two shared files carry a conditional rather than being forked, because each had
exactly one platform-specific line:

- `DesignSystem/HealthPresentation.swift` — the `Color(light:dark:)` token
  initialiser, `UIColor`'s trait provider on iOS and `NSColor`'s appearance
  provider here. **Every `rp*` colour depends on this**, so it is the one place
  the design system had to learn about a second platform.
- `Stores/PushRegistration.swift` — `import UIKit`, for `UIApplication` alone.

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
- **An 880×560 minimum**, below which the sidebar and a ring strip cannot
  both stay readable.

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
- **An app icon.** The catalogue holds `AccentColor` only. The iOS
  `AppIcon.appiconset` is excluded because macOS needs its own sizes, including
  512×512 and 1024×1024.
- **The web's typefaces**, as above.
- **Code signing for distribution.** `CODE_SIGN_IDENTITY` is `-`, which is
  ad-hoc: fine to build and run locally, not notarised and not distributable.
