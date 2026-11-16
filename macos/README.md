# Ring Promoter for Mac

A native macOS app — not Catalyst, and not a rewrite. It shares the iOS app's
source in place: the same models, networking, stores, screens and design
system. Only the window shell is Mac-specific.

```
macos/
  project.yml                 the Xcode project, as a spec
  RingPromoterMac/
    App/
      RingPromoterMacApp.swift   @main, window sizing, ⌘1…⌘4
      MacRootView.swift          sidebar in place of the tab bar
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

## The one design decision

iOS puts the four destinations in a tab bar. This target puts the same four in a
sidebar, read from the same `Router.Tab` — so adding a tab on iOS adds a row
here, and the two cannot drift.

A `TabView` does compile on macOS, so sharing `RootView` unchanged was possible.
It was not taken: it strands an iPhone tab strip across a 1180pt window and
leaves the sidebar — where a Mac operator looks for navigation — empty.

Added for the Mac, with nothing on iOS to mirror:

- **⌘1…⌘4** for the four destinations, disabled while disconnected.
- **A connection footer** pinned under the sidebar, naming the control plane or
  showing "Demo mode". On iOS this is one tap away in Settings; a Mac window
  stays open for hours across several planes, so "which server am I about to
  promote on" belongs permanently on screen.
- **A 860×560 minimum**, below which a ring table stops being readable.

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
- **Code signing for distribution.** `CODE_SIGN_IDENTITY` is `-`, which is
  ad-hoc: fine to build and run locally, not notarised and not distributable.
