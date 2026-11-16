import SwiftUI

// The building blocks the web console is made of, rebuilt in SwiftUI so the
// Mac app reads as the same product rather than as a stock macOS document app.
// Each one names the class list it is a transcription of, so the two can be
// checked against each other later.

/// `rounded-lg border bg-card` — the panel everything on a page sits in.
struct ConsoleCard<Content: View>: View {
    var padding: CGFloat = 14
    var isHighlighted = false
    /// Stretch to the height offered rather than hugging the content. Set when
    /// cards sit side by side, so their feet line up.
    var fillsHeight = false
    @ViewBuilder var content: Content

    var body: some View {
        content
            .padding(padding)
            .frame(
                maxWidth: .infinity,
                maxHeight: fillsHeight ? .infinity : nil,
                alignment: .topLeading
            )
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


/// Lays panels out in rows: as many per row as fit at `minimumItemWidth`,
/// every panel in a row the same width, and every row as tall as its tallest
/// panel.
///
/// This is what `web/src/components/dashboard/pipeline.tsx` gets from
/// `flex flex-col gap-2 xl:flex-row`, and what SwiftUI has no stock container
/// for. `ViewThatFits` was tried first and is the wrong tool: it measures a
/// candidate's *ideal* width, and a row of flexible panels has no ideal width
/// to speak of — it reported that the horizontal arrangement did not fit at
/// any pane width and silently picked the vertical one every time.
struct PanelGrid: Layout {
    var minimumItemWidth: CGFloat = 236
    var spacing: CGFloat = 12

    func sizeThatFits(
        proposal: ProposedViewSize, subviews: Subviews, cache: inout ()
    ) -> CGSize {
        let width = proposal.replacingUnspecifiedDimensions(
            by: CGSize(width: minimumItemWidth, height: 0)
        ).width
        let heights = rowHeights(subviews, width: width)
        return CGSize(
            width: width,
            height: heights.reduce(0, +) + spacing * CGFloat(max(0, heights.count - 1))
        )
    }

    func placeSubviews(
        in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()
    ) {
        let columns = columnCount(forWidth: bounds.width, count: subviews.count)
        let itemWidth = itemWidth(in: bounds.width, columns: columns)
        var y = bounds.minY
        for (row, height) in rowHeights(subviews, width: bounds.width).enumerated() {
            var x = bounds.minX
            for index in indices(inRow: row, columns: columns, count: subviews.count) {
                subviews[index].place(
                    at: CGPoint(x: x, y: y),
                    anchor: .topLeading,
                    proposal: ProposedViewSize(width: itemWidth, height: height)
                )
                x += itemWidth + spacing
            }
            y += height + spacing
        }
    }

    /// How many panels go in a row.
    ///
    /// Not simply "as many as fit": a pipeline is a sequence, and four rings
    /// laid out 3 + 1 orphans production on a row of its own and breaks the
    /// left-to-right reading. So the row count is worked out first and the
    /// panels are then spread evenly over it — four rings in a window too
    /// narrow for one row become 2 + 2, never 3 + 1.
    private func columnCount(forWidth width: CGFloat, count: Int) -> Int {
        guard count > 0 else { return 1 }
        guard width > 0 else { return count }
        let fitting = max(1, Int((width + spacing) / (minimumItemWidth + spacing)))
        let rows = Int((Double(count) / Double(min(count, fitting))).rounded(.up))
        return max(1, Int((Double(count) / Double(max(rows, 1))).rounded(.up)))
    }

    private func itemWidth(in width: CGFloat, columns: Int) -> CGFloat {
        guard columns > 1 else { return width }
        return (width - spacing * CGFloat(columns - 1)) / CGFloat(columns)
    }

    private func indices(inRow row: Int, columns: Int, count: Int) -> Range<Int> {
        let start = row * columns
        return start..<min(start + columns, count)
    }

    /// One entry per row. A row is as tall as its tallest panel measured at
    /// the width that row will actually give it.
    private func rowHeights(_ subviews: Subviews, width: CGFloat) -> [CGFloat] {
        let columns = columnCount(forWidth: width, count: subviews.count)
        let itemWidth = itemWidth(in: width, columns: columns)
        guard columns > 0, subviews.count > 0 else { return [] }
        return stride(from: 0, to: subviews.count, by: columns).map { start in
            (start..<min(start + columns, subviews.count))
                .map {
                    subviews[$0].sizeThatFits(
                        ProposedViewSize(width: itemWidth, height: nil)
                    ).height
                }
                .max() ?? 0
        }
    }
}
