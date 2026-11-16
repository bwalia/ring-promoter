import SwiftUI

/// The navigation column, built to match `web/src/components/sidebar.tsx`.
///
/// Structurally the same three things the web console has, in the same order:
/// a brand row that returns you to the overview, a filter box, and then
/// labelled groups of rows — the destinations first, the live application list
/// under them. The filter drives `OverviewStore.searchText`, so one box
/// narrows both this list and the cards in the main pane, exactly as the web's
/// does.
///
/// It is hand-built rather than a `List`. AppKit's source list draws its
/// selection in the system accent colour and its group headers in AppKit's own
/// style; the console's emphasis is a neutral `--sidebar-accent` fill under an
/// 11px uppercase label, and there is no way to reach that through `List`.
struct MacSidebar: View {
    let store: OverviewStore?
    /// Raised by the connection footer. The sheet itself is presented by
    /// `MacRootView`, from the window's content — see `ConnectionFooter`.
    @Binding var isAddingInstance: Bool
    @Binding var isManagingInstances: Bool
    @Environment(AppSession.self) private var session
    @Environment(Router.self) private var router
    @Environment(ConsoleFocus.self) private var focus
    @FocusState private var filterFocused: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            brand
            filterBox
            ScrollView {
                MacSidebarSections(store: store)
            }
            .scrollContentBackground(.hidden)
            ConnectionFooter(
                isAddingInstance: $isAddingInstance,
                isManagingInstances: $isManagingInstances
            )
        }
        .frame(maxHeight: .infinity, alignment: .top)
        .background(Color.rpSidebar)
        .onChange(of: focus.filterRequests) { filterFocused = true }
    }

    // MARK: - Brand

    private var brand: some View {
        Button {
            router.tab = .overview
            router.overviewPath = []
        } label: {
            HStack(spacing: 10) {
                Image(systemName: "smallcircle.filled.circle")
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundStyle(Color.rpPrimaryForeground)
                    .frame(width: 26, height: 26)
                    .background(Color.rpPrimary, in: .rect(cornerRadius: RPRadius.md))
                Text("Ring Promoter")
                    .displayHeading(RPFont.bodySize)
                    .foregroundStyle(Color.rpForeground)
                Spacer(minLength: 0)
            }
            .contentShape(.rect)
        }
        .buttonStyle(.plain)
        .padding(.horizontal, 12)
        .padding(.top, 12)
        .padding(.bottom, 12)
        .accessibilityLabel("Ring Promoter, back to the application list")
    }

    private var filterBox: some View {
        ConsoleFilterField(
            prompt: "Filter applications…",
            text: Binding(
                get: { store?.searchText ?? "" },
                set: { store?.searchText = $0 }
            ),
            shortcut: "⌘F",
            isFocused: $filterFocused
        )
        .disabled(store == nil)
        .padding(.horizontal, 10)
        .padding(.bottom, 12)
    }

}

/// The sidebar's groups of rows.
///
/// Split out of `MacSidebar` so `ConsoleSnapshot` can render them: AppKit's
/// `ImageRenderer` draws a `ScrollView` as an empty box, and these are exactly
/// the rows worth looking at.
struct MacSidebarSections: View {
    let store: OverviewStore?
    @Environment(Router.self) private var router

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            destinations
            applications
        }
        .padding(.horizontal, 8)
        .padding(.top, 2)
        .padding(.bottom, 14)
    }

    private var destinations: some View {
        VStack(alignment: .leading, spacing: 2) {
            ConsoleSectionLabel("Navigation")
            ForEach(Router.Tab.allCases) { tab in
                ConsoleNavRow(
                    title: tab.sidebarLabel,
                    isSelected: router.tab == tab && !isShowingAnApp,
                    action: {
                        router.tab = tab
                        if tab == .overview { router.overviewPath = [] }
                    },
                    leading: {
                        Image(systemName: tab.systemImage)
                            .font(.system(size: 13))
                            .frame(width: 16)
                            // Same correction the iPhone tab bar makes: the
                            // filled Rings glyph closes up its inner circle.
                            .environment(\.symbolVariants, tab == .rings ? .none : .fill)
                    }
                )
            }
        }
    }

    @ViewBuilder
    private var applications: some View {
        if let store, !store.summaries.isEmpty {
            let shown = store.orderedSummaries
            VStack(alignment: .leading, spacing: 2) {
                ConsoleSectionLabel("Applications") {
                    Text("\(shown.count)")
                        .font(RPFont.sectionLabel)
                        .monospacedDigit()
                        .foregroundStyle(Color.rpMutedForeground)
                }
                if shown.isEmpty {
                    Text("No matches for “\(store.searchText)”.")
                        .font(.system(size: 12))
                        .foregroundStyle(Color.rpMutedForeground)
                        .padding(.horizontal, 8)
                        .padding(.vertical, 4)
                } else {
                    ForEach(shown) { summary in
                        ConsoleNavRow(
                            title: summary.title,
                            isSelected: isShowing(app: summary.name),
                            isEmphasised: true,
                            action: { router.show(app: summary.name) },
                            leading: {
                                Circle()
                                    .fill(summary.consoleTint)
                                    .frame(width: 7, height: 7)
                                    .frame(width: 16)
                            },
                            trailing: {
                                if summary.isBusy {
                                    Image(systemName: "arrow.triangle.2.circlepath")
                                        .font(.system(size: 10, weight: .semibold))
                                        .foregroundStyle(Color.rpInFlight)
                                        .symbolEffect(.pulse, isActive: true)
                                }
                            }
                        )
                        .accessibilityLabel("\(summary.title), \(summary.consoleStateLabel)")
                    }
                }
            }
        }
    }

    private var isShowingAnApp: Bool {
        router.tab == .overview && !router.overviewPath.isEmpty
    }

    private func isShowing(app: String) -> Bool {
        router.tab == .overview && router.overviewPath.first == .app(app)
    }
}

/// Which control plane the window is pointed at, pinned under the sidebar —
/// and the way to change it.
///
/// On iOS this lives in the Settings tab, one tap away. A Mac window stays
/// open for hours across several planes, so the answer to "which server am I
/// about to promote on" belongs permanently on screen.
///
/// It is also a button, and that is not only for convenience. The shared
/// `InstanceSwitcher` carries its own `.sheet` modifiers, and on this target
/// it is hosted inside the window toolbar — a separate AppKit view tree, where
/// a sheet attached to a toolbar item is not reliably presented. This route
/// raises a binding instead and lets `MacRootView` present from the window's
/// own content, which always works.
struct ConnectionFooter: View {
    @Binding var isAddingInstance: Bool
    @Binding var isManagingInstances: Bool
    @Environment(AppSession.self) private var session
    @State private var isHovering = false

    var body: some View {
        VStack(spacing: 0) {
            Rectangle()
                .fill(Color.rpBorder)
                .frame(height: 1)
            Menu {
                Section("Clusters") {
                    ForEach(session.instances.instances) { instance in
                        Button {
                            if instance.id != session.activeInstance?.id {
                                _ = session.activate(instance)
                            }
                        } label: {
                            if instance.id == session.activeInstance?.id {
                                Label(instance.name, systemImage: "checkmark")
                            } else {
                                Text(instance.name)
                            }
                        }
                    }
                }
                Button {
                    isAddingInstance = true
                } label: {
                    Label("Add a cluster…", systemImage: "plus.circle")
                }
                Button {
                    isManagingInstances = true
                } label: {
                    Label("Manage clusters…", systemImage: "server.rack")
                }
                if !session.isDemo {
                    Button {
                        session.enterDemoMode()
                    } label: {
                        Label("Switch to demo mode", systemImage: "theatermasks")
                    }
                }
            } label: {
                HStack(spacing: 7) {
                    Circle()
                        .fill(session.isDemo ? Color.rpGate : session.instanceTint.color)
                        .frame(width: 7, height: 7)
                    Text(session.isDemo ? "Demo mode" : session.instanceName)
                        .font(.system(size: 11, weight: .medium))
                        .foregroundStyle(Color.rpMutedForeground)
                        .lineLimit(1)
                        .truncationMode(.middle)
                    Spacer(minLength: 0)
                    Image(systemName: "chevron.up.chevron.down")
                        .font(.system(size: 8, weight: .semibold))
                        .foregroundStyle(Color.rpMutedForeground)
                }
                .padding(.horizontal, 10)
                .padding(.vertical, 7)
                .background {
                    RoundedRectangle(cornerRadius: RPRadius.md)
                        .fill(isHovering ? Color.rpAccent.opacity(0.6) : Color.clear)
                }
                .contentShape(.rect(cornerRadius: RPRadius.md))
            }
            .menuStyle(.borderlessButton)
            .menuIndicator(.hidden)
            .onHover { isHovering = $0 }
            .padding(.horizontal, 6)
            .padding(.vertical, 6)
            .accessibilityLabel(
                session.isDemo
                    ? "Demo mode, no server connected. Change cluster."
                    : "Connected to \(session.instanceName). Change cluster."
            )
        }
    }
}

extension Router.Tab {
    /// The sidebar wording.
    ///
    /// Matches the web console's nav, which spells the Rings destination out
    /// in full. `label` stays the short form the iPhone needs, where four
    /// labels have to share one tab bar row.
    var sidebarLabel: String {
        self == .rings ? "Rings of Applications" : label
    }
}

extension AppSummary {
    /// The dot beside an application in the sidebar, and the fill on its card.
    ///
    /// Reuses the shared health tokens rather than the web's `--status-*`
    /// palette: a ring that is amber on the iPhone has to be amber here.
    var consoleTint: Color {
        if isBusy { return .rpInFlight }
        if isInTrouble { return .rpUnhealthy }
        if rings.allSatisfy({ !$0.configured || $0.isEmpty }) { return .rpNeutral }
        return .rpHealthy
    }

    /// Spoken instead of the colour, which carries no meaning on its own.
    var consoleStateLabel: String {
        if isBusy { return busyAction ?? "deploying" }
        if let trouble = troubleSummary { return trouble }
        if rings.allSatisfy({ !$0.configured || $0.isEmpty }) { return "nothing deployed" }
        return "healthy"
    }
}
