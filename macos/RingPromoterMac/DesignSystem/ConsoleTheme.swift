import SwiftUI

/// The console palette, transcribed from the web client's design tokens in
/// `web/src/app/globals.css`.
///
/// Those tokens are under an explicit theme lock — `web/AGENTS.md` says the
/// near-black neutral console look is the repo owner's deliberate choice and a
/// blue-violet redesign was reverted in PR #31. So this is a transcription,
/// not a new palette: every `oklch(L 0 0)` token converted to its sRGB
/// equivalent. The chroma is zero throughout, which makes the conversion exact
/// and lands the ramp precisely on Tailwind's `neutral` scale:
///
///     oklch(1     0 0) → #FFFFFF    oklch(0.269 0 0) → #262626
///     oklch(0.985 0 0) → #FAFAFA    oklch(0.205 0 0) → #171717
///     oklch(0.97  0 0) → #F5F5F5    oklch(0.145 0 0) → #0A0A0A
///     oklch(0.922 0 0) → #E5E5E5    oklch(0.708 0 0) → #A1A1A1
///                                   oklch(0.556 0 0) → #737373
///
/// Health colours are deliberately NOT redefined here. `rpHealthy`,
/// `rpUnhealthy`, `rpGate`, `rpInFlight` and `rpProduction` live in the shared
/// `DesignSystem/HealthPresentation.swift` and are what the iPhone draws; a
/// ring that is amber on the phone has to be amber here. Only the *surfaces*
/// come from the web.
extension Color {
    /// Page background. The window's canvas.
    static let rpBackground = Color(light: Color(rpHex: 0xFFFFFF), dark: Color(rpHex: 0x0A0A0A))
    /// Default text.
    static let rpForeground = Color(light: Color(rpHex: 0x0A0A0A), dark: Color(rpHex: 0xFAFAFA))
    /// Panels that sit on the background: `--card`.
    static let rpCard = Color(light: Color(rpHex: 0xFFFFFF), dark: Color(rpHex: 0x171717))
    /// The navigation column: `--sidebar`.
    static let rpSidebar = Color(light: Color(rpHex: 0xFAFAFA), dark: Color(rpHex: 0x171717))
    /// Hover and selection fill: `--accent` / `--muted` / `--secondary`, which
    /// the web defines to the same value.
    static let rpAccent = Color(light: Color(rpHex: 0xF5F5F5), dark: Color(rpHex: 0x262626))
    /// Secondary text: `--muted-foreground`.
    static let rpMutedForeground = Color(light: Color(rpHex: 0x737373), dark: Color(rpHex: 0xA1A1A1))
    /// Hairlines: `--border`. Translucent in dark, exactly as the web has it,
    /// so a border over a card and a border over the page are not the same
    /// flat grey.
    static let rpBorder = Color(light: Color(rpHex: 0xE5E5E5), dark: Color(white: 1, opacity: 0.10))
    /// Control outlines: `--input`, a touch stronger than `--border`.
    static let rpInput = Color(light: Color(rpHex: 0xE5E5E5), dark: Color(white: 1, opacity: 0.15))
    /// Filled emphasis: `--primary`. Inverted against the background, which is
    /// why it is near-black in light mode and near-white in dark.
    static let rpPrimary = Color(light: Color(rpHex: 0x171717), dark: Color(rpHex: 0xE5E5E5))
    /// Text and glyphs drawn on `rpPrimary`.
    static let rpPrimaryForeground = Color(light: Color(rpHex: 0xFAFAFA), dark: Color(rpHex: 0x171717))

    /// 0xRRGGBB, in the sRGB space the web tokens are specified in.
    init(rpHex: UInt32) {
        self.init(
            .sRGB,
            red: Double((rpHex >> 16) & 0xFF) / 255,
            green: Double((rpHex >> 8) & 0xFF) / 255,
            blue: Double(rpHex & 0xFF) / 255
        )
    }
}

/// Corner radii, from the web's `--radius: 0.625rem` and the `calc()`
/// derivations beside it.
enum RPRadius {
    /// `--radius-sm`: chips and the smallest controls.
    static let sm: CGFloat = 6
    /// `--radius-md`: nav rows, buttons, fields.
    static let md: CGFloat = 8
    /// `--radius-lg`: cards and panels.
    static let lg: CGFloat = 10
    /// `--radius-xl`: the largest containers.
    static let xl: CGFloat = 14
}

/// The web's type scale, from the `--rp-font-*` custom properties.
///
/// One honest gap: the web loads Poppins (body), Unbounded (display) and
/// JetBrains Mono (numerals) from Google Fonts. None of the three is on macOS
/// and the repo only carries them as `.woff2`, which CoreText cannot register —
/// converting them needs tooling this machine does not have. So the *faces*
/// here are San Francisco. Everything else about the scale is the web's: the
/// 15pt body, the heading sizes, the 700 display weight, and the -0.02em
/// tracking that gives those headings their tight set.
enum RPFont {
    /// `--rp-font-body`, 0.9375rem.
    static let bodySize: CGFloat = 15

    /// Body copy.
    static var body: Font { .system(size: bodySize) }
    /// `.rp-heading-panel`: a panel's own title.
    static var panelHeading: Font { .system(size: bodySize, weight: .semibold) }
    /// `.rp-heading-toolbar` / `--rp-font-h3`, at its desktop maximum.
    static var toolbarHeading: Font { .system(size: 21, weight: .bold) }
    /// `--rp-font-h2`, clamped to something a window pane can carry.
    static var sectionHeading: Font { .system(size: 24, weight: .bold) }
    /// `.rp-stat`: the number in a KPI tile.
    static var stat: Font { .system(size: 21, weight: .bold) }
    /// The 11px uppercase label above each sidebar section.
    static var sectionLabel: Font { .system(size: 11, weight: .semibold) }
    /// Versions and other data that has to line up column-wise.
    static var mono: Font { .system(size: 12, weight: .medium, design: .monospaced) }

    /// The web sets `letter-spacing: -0.02em` on every display heading.
    static func displayTracking(_ size: CGFloat) -> CGFloat { size * -0.02 }
}
