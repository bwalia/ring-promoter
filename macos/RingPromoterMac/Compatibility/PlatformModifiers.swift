import SwiftUI

// Shims for the SwiftUI spellings the shared iOS views use that AppKit has no
// equivalent for.
//
// The point of this file is that none of the shared view files needed an
// `#if os(macOS)` added to them. Each shim is a no-op that type-checks against
// the real call site, so the iOS source stays the single definition of every
// screen and this target carries the platform difference instead.
//
// Deliberately NOT shimmed, because the macOS 14 SDK already provides them and
// a stub makes every call site ambiguous rather than portable:
//
//   - `textContentType`         — .name, .URL and .password all exist on macOS,
//                                 and drive Keychain autofill.
//   - `presentationDetents`     — exists; a Mac sheet simply ignores it.
//   - `interactiveDismissDisabled` — exists on macOS 13+.
//
// Each of those was tried as a shim first and the compiler rejected it as an
// ambiguous use. If one reappears here, that is the error to expect.

// MARK: - Navigation bar

/// `.navigationBarTitleDisplayMode(.inline)` describes a UINavigationBar. A Mac
/// window title has no large/inline distinction to set.
enum RPNavigationBarTitleDisplayMode: Sendable {
    case automatic, inline, large
}

extension View {
    func navigationBarTitleDisplayMode(
        _ mode: RPNavigationBarTitleDisplayMode
    ) -> some View {
        self
    }
}

// MARK: - Text input

/// `.textInputAutocapitalization(.never)` — there is no software keyboard to
/// shift, so there is nothing to suppress. Hardware keyboard input is already
/// literal.
struct RPTextInputAutocapitalization: Sendable {
    static let never = RPTextInputAutocapitalization()
    static let words = RPTextInputAutocapitalization()
    static let sentences = RPTextInputAutocapitalization()
    static let characters = RPTextInputAutocapitalization()
}

/// `.keyboardType(.URL)` — same reason. Note this is distinct from
/// `textContentType`, which macOS does support and which is what actually
/// drives autofill.
struct RPKeyboardType: Sendable {
    static let `default` = RPKeyboardType()
    static let URL = RPKeyboardType()
    static let emailAddress = RPKeyboardType()
    static let numberPad = RPKeyboardType()
}

extension View {
    func textInputAutocapitalization(
        _ autocapitalization: RPTextInputAutocapitalization?
    ) -> some View {
        self
    }

    func keyboardType(_ type: RPKeyboardType) -> some View {
        self
    }
}

// MARK: - Toolbar and search placement

// `.topBarLeading`, `.topBarTrailing` and `.navigationBarDrawer` name positions
// on a UINavigationBar and are marked unavailable on macOS. The shared views use
// them nine times, so they are mapped here once rather than guarded at every
// call site.
//
// `.navigation` and `.primaryAction` are the leading and trailing ends of a Mac
// window toolbar, which is what the iOS spellings mean.
extension ToolbarItemPlacement {
    static var topBarLeading: ToolbarItemPlacement { .navigation }
    static var topBarTrailing: ToolbarItemPlacement { .primaryAction }
}

/// `.searchable(placement: .navigationBarDrawer(displayMode: .always))` in
/// VersionPickerView. A Mac search field lives in the toolbar and is always
/// visible, so the display mode has nothing left to express.
extension SearchFieldPlacement {
    enum RPNavigationBarDrawerDisplayMode: Sendable {
        case automatic, always
    }

    static func navigationBarDrawer(
        displayMode: RPNavigationBarDrawerDisplayMode
    ) -> SearchFieldPlacement {
        .toolbar
    }
}

// MARK: - Picker style

/// `.pickerStyle(.navigationLink)` pushes the options onto a navigation stack,
/// which is an iPhone idiom for a list too long to inline. The one use is the
/// instance-tint picker in AddInstanceView, and on a Mac a form row like that
/// is a popup button.
extension PickerStyle where Self == MenuPickerStyle {
    static var navigationLink: MenuPickerStyle { .menu }
}

// MARK: - Toolbar surface

/// `for: .navigationBar` on `toolbarBackground` / `toolbarColorScheme`. The Mac
/// equivalent surface is the window toolbar.
///
/// This one is not cosmetic: the Rings of Applications screen paints its
/// toolbar with `DescentPalette.space` so the near-black console reads as one
/// surface instead of a dark canvas under a light title bar. Dropping it would
/// visibly break that screen's design on Mac.
extension ToolbarPlacement {
    static var navigationBar: ToolbarPlacement { .windowToolbar }
}
