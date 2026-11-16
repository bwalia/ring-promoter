import SwiftUI

// The building blocks the web console is made of, rebuilt in SwiftUI so the
// Mac app reads as the same product rather than as a stock macOS document app.
// Each one names the class list it is a transcription of, so the two can be
// checked against each other later.

/// `rounded-lg border bg-card` — the panel everything on a page sits in.
struct ConsoleCard<Content: View>: View {
    var padding: CGFloat = 14
    var isHighlighted = false
    @ViewBuilder var content: Content

    var body: some View {
        content
            .padding(padding)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Color.rpCard, in: .rect(cornerRadius: RPRadius.lg))
            .overlay {
                RoundedRectangle(cornerRadius: RPRadius.lg)
                    .strokeBorder(
                        isHighlighted ? Color.rpUnhealthy.opacity(0.45) : Color.rpBorder
                    )
            }
    }
}

/// `text-[11px] font-semibold uppercase tracking-wider text-muted-foreground`
/// — the label above each group in the sidebar.
struct ConsoleSectionLabel: View {
    let text: String
    var trailing: AnyView?

    init(_ text: String) {
        self.text = text
        self.trailing = nil
    }

    init<T: View>(_ text: String, @ViewBuilder trailing: () -> T) {
        self.text = text
        self.trailing = AnyView(trailing())
    }

    var body: some View {
        HStack(spacing: 4) {
            Text(text.uppercased())
                .font(RPFont.sectionLabel)
                .tracking(0.6)
                .foregroundStyle(Color.rpMutedForeground)
            Spacer(minLength: 0)
            trailing
        }
        .padding(.horizontal, 4)
        .padding(.vertical, 2)
        .accessibilityAddTraits(.isHeader)
    }
}

/// One clickable row in the sidebar.
///
/// `rounded-md px-2 py-2 text-sm`, filling with `--sidebar-accent` when
/// selected and with 60% of it on hover — which is the whole reason this is
/// hand-built rather than a `List` row: AppKit's source list paints its
/// selection in the system accent colour, and the console has no accent
/// colour. Its emphasis is a neutral fill.
struct ConsoleNavRow<Leading: View, Trailing: View>: View {
    let title: String
    var isSelected = false
    var isEmphasised = false
    let action: () -> Void
    @ViewBuilder var leading: Leading
    @ViewBuilder var trailing: Trailing

    @State private var isHovering = false

    var body: some View {
        Button(action: action) {
            HStack(spacing: 8) {
                leading
                    .foregroundStyle(
                        isSelected ? Color.rpForeground : Color.rpForeground.opacity(0.8)
                    )
                Text(title)
                    .font(.system(size: 13, weight: isEmphasised || isSelected ? .semibold : .regular))
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .foregroundStyle(
                        isSelected ? Color.rpForeground : Color.rpForeground.opacity(0.8)
                    )
                Spacer(minLength: 4)
                trailing
            }
            .padding(.horizontal, 8)
            .padding(.vertical, 6)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background {
                RoundedRectangle(cornerRadius: RPRadius.md)
                    .fill(
                        isSelected
                            ? Color.rpAccent
                            : (isHovering ? Color.rpAccent.opacity(0.6) : Color.clear)
                    )
            }
            .contentShape(.rect(cornerRadius: RPRadius.md))
        }
        .buttonStyle(.plain)
        .onHover { isHovering = $0 }
        .accessibilityAddTraits(isSelected ? [.isButton, .isSelected] : .isButton)
    }
}

extension ConsoleNavRow where Trailing == EmptyView {
    init(
        title: String,
        isSelected: Bool = false,
        isEmphasised: Bool = false,
        action: @escaping () -> Void,
        @ViewBuilder leading: () -> Leading
    ) {
        self.init(
            title: title, isSelected: isSelected, isEmphasised: isEmphasised,
            action: action, leading: leading, trailing: { EmptyView() }
        )
    }
}

/// The sidebar's filter box: `h-9 pl-8` with a leading magnifier and a key cap
/// on the right, over `--input`.
struct ConsoleFilterField: View {
    let prompt: String
    @Binding var text: String
    var shortcut: String?
    @FocusState.Binding var isFocused: Bool

    var body: some View {
        HStack(spacing: 6) {
            Image(systemName: "magnifyingglass")
                .font(.system(size: 12, weight: .medium))
                .foregroundStyle(Color.rpMutedForeground)
            TextField(prompt, text: $text)
                .textFieldStyle(.plain)
                .font(.system(size: 13))
                .focused($isFocused)
            if !text.isEmpty {
                Button {
                    text = ""
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .font(.system(size: 12))
                        .foregroundStyle(Color.rpMutedForeground)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Clear the filter")
            } else if let shortcut {
                KeyCap(shortcut)
            }
        }
        .padding(.horizontal, 8)
        .frame(height: 30)
        .background(Color.rpAccent.opacity(0.4), in: .rect(cornerRadius: RPRadius.md))
        .overlay {
            RoundedRectangle(cornerRadius: RPRadius.md)
                .strokeBorder(isFocused ? Color.rpMutedForeground.opacity(0.6) : Color.rpInput)
        }
    }
}

/// `<kbd>` — a bordered key cap, as on the web's search affordance.
struct KeyCap: View {
    let text: String

    init(_ text: String) { self.text = text }

    var body: some View {
        Text(text)
            .font(.system(size: 10, weight: .medium, design: .monospaced))
            .foregroundStyle(Color.rpMutedForeground)
            .padding(.horizontal, 4)
            .padding(.vertical, 1)
            .background(Color.rpBackground.opacity(0.6), in: .rect(cornerRadius: 4))
            .overlay { RoundedRectangle(cornerRadius: 4).strokeBorder(Color.rpBorder) }
            .accessibilityHidden(true)
    }
}

/// `variant="outline" size="sm"` — a bordered button on the console surface.
struct ConsoleOutlineButtonStyle: ButtonStyle {
    @State private var isHovering = false

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: 12, weight: .medium))
            .padding(.horizontal, 10)
            .frame(height: 28)
            .background(
                isHovering || configuration.isPressed
                    ? Color.rpAccent : Color.rpAccent.opacity(0.4),
                in: .rect(cornerRadius: RPRadius.md)
            )
            .overlay { RoundedRectangle(cornerRadius: RPRadius.md).strokeBorder(Color.rpInput) }
            .foregroundStyle(Color.rpForeground)
            .opacity(configuration.isPressed ? 0.85 : 1)
            .onHover { isHovering = $0 }
    }
}

/// `variant="ghost" size="icon"` — a toolbar glyph with no chrome until hover.
struct ConsoleGhostButtonStyle: ButtonStyle {
    @State private var isHovering = false

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.system(size: 13, weight: .medium))
            .frame(width: 28, height: 28)
            .background(
                isHovering || configuration.isPressed ? Color.rpAccent : Color.clear,
                in: .rect(cornerRadius: RPRadius.md)
            )
            .foregroundStyle(Color.rpForeground.opacity(0.85))
            .opacity(configuration.isPressed ? 0.85 : 1)
            .onHover { isHovering = $0 }
    }
}

/// Puts a shared screen onto the console surface.
///
/// Applied from here rather than inside the screens so not one of the shared
/// iOS files needs a `#if os(macOS)`. It hides AppKit's own list and form
/// chrome, paints `--background` behind, and hands the window toolbar the
/// sidebar colour so the top of the window reads as one strip.
///
/// Deliberately NOT applied to `RingsUniverseView`: that screen already paints
/// its own toolbar with `DescentPalette.space` so the stage and the toolbar
/// read as a single black field, and a second `toolbarBackground` on the same
/// placement would fight it.
struct ConsoleSurface: ViewModifier {
    func body(content: Content) -> some View {
        content
            .scrollContentBackground(.hidden)
            .background(Color.rpBackground)
            .listStyle(.inset)
            .formStyle(.grouped)
            .toolbarBackground(Color.rpSidebar, for: .windowToolbar)
            .toolbarBackground(.visible, for: .windowToolbar)
    }
}

extension View {
    func consoleSurface() -> some View { modifier(ConsoleSurface()) }

    /// The web's display headings: a bold face set tight.
    func displayHeading(_ size: CGFloat) -> some View {
        font(.system(size: size, weight: .bold)).tracking(RPFont.displayTracking(size))
    }
}

/// Lets a menu-bar command move focus into a field several views away.
///
/// Mac-only, and deliberately not folded into the shared `Router`: ⌘F is a
/// desktop affordance, and `Router` is the one piece of navigation state the
/// phone, the widget, App Intents and push notifications all share.
@MainActor
@Observable
final class ConsoleFocus {
    /// Bumped by ⌘F. The sidebar watches the count rather than a flag, so two
    /// presses in a row both land.
    private(set) var filterRequests = 0

    func requestFilter() { filterRequests += 1 }
}
