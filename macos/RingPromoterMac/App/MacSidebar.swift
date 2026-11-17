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
                // The same artwork the Dock shows. It used to be a flat SF
                // Symbol on a white square, which made the app appear to have
                // two unrelated identities.
                ConsoleBrandMark(size: 28)
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
    /// Collapsed groups, by id. Local to the window: which groups an operator
    /// has folded away is not worth syncing to a phone.
    @State private var collapsed: Set<String> = []

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

    /// Server-side groups, each collapsible, then everything.
    ///
    /// The web console's sidebar is grouped this way and it is not decoration:
    /// twenty-one applications in one alphabetical list is a list you read
    /// rather than navigate. `OverviewStore` already loads the groups for the
    /// Overview's filter menu, so this costs no extra request.
    @ViewBuilder
    private var applications: some View {
        if let store, !store.summaries.isEmpty {
            let shown = store.orderedSummaries
            let grouped = groups(in: store, shown: shown)

            ForEach(grouped, id: \.group.id) { entry in
                VStack(alignment: .leading, spacing: 2) {
                    ConsoleSectionLabel(entry.group.name) {
                        HStack(spacing: 6) {
                            Text("\(entry.summaries.count)")
                                .font(RPFont.sectionLabel)
                                .monospacedDigit()
                                .foregroundStyle(Color.rpMutedForeground)
                            Image(systemName: "chevron.right")
                                .font(.system(size: 8, weight: .bold))
                                .foregroundStyle(Color.rpMutedForeground)
                                .rotationEffect(.degrees(collapsed.contains(entry.group.id) ? 0 : 90))
                        }
                    }
                    .contentShape(.rect)
                    .onTapGesture { toggle(entry.group.id) }
                    if !collapsed.contains(entry.group.id) {
                        ForEach(entry.summaries) { summary in
                            applicationRow(summary)
                        }
                    }
                }
            }

            VStack(alignment: .leading, spacing: 2) {
                ConsoleSectionLabel(grouped.isEmpty ? "Applications" : "All applications") {
                    Text("\(shown.count)")
                        .font(RPFont.sectionLabel)
                        .monospacedDigit()
                        .foregroundStyle(Color.rpMutedForeground)
                }
                if shown.isEmpty {
                    Text("No matches for \u{201C}\(store.searchText)\u{201D}.")
                        .font(.system(size: 12))
                        .foregroundStyle(Color.rpMutedForeground)
                        .padding(.horizontal, 8)
                        .padding(.vertical, 4)
                } else {
                    ForEach(shown) { summary in
                        applicationRow(summary)
                    }
                }
            }
        }
    }

    private func applicationRow(_ summary: AppSummary) -> some View {
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

    /// Groups that still have a visible member. A group whose applications are
    /// all filtered out disappears rather than leaving an empty heading.
    private func groups(
        in store: OverviewStore, shown: [AppSummary]
    ) -> [(group: AppGroup, summaries: [AppSummary])] {
        store.groups.compactMap { group in
            let members = shown.filter { group.apps.contains($0.name) }
            return members.isEmpty ? nil : (group, members)
        }
    }

    private func toggle(_ id: String) {
        if collapsed.contains(id) { collapsed.remove(id) } else { collapsed.insert(id) }
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
/// about to promote on" belongs permanently on screen. It names the host as
/// well as the label, because two clusters called "Production" is a thing that
/// happens.
///
/// A `Button` and a `popover` rather than a `Menu`. AppKit lays a borderless
/// menu's label out on its own terms — it centred the text and put the
/// indicator in front of it, ignoring the padding this had — and a popover is
/// the only way to get a row that looks like the rest of the sidebar.
///
/// The sheets are raised through bindings rather than presented here. The
/// shared `InstanceSwitcher` carries its own, and on this target it is hosted
/// inside the window toolbar, a separate AppKit view tree where a sheet
/// attached to a toolbar item is not reliably presented.
struct ConnectionFooter: View {
    @Binding var isAddingInstance: Bool
    @Binding var isManagingInstances: Bool
    @Environment(AppSession.self) private var session
    @State private var isShowingSwitcher = false
    @State private var isHovering = false

    private var dotColor: Color {
        session.isDemo ? .rpGate : session.instanceTint.color
    }

    var body: some View {
        VStack(spacing: 0) {
            Rectangle()
                .fill(Color.rpBorder)
                .frame(height: 1)
            Button {
                isShowingSwitcher = true
            } label: {
                HStack(spacing: 9) {
                    Circle()
                        .fill(dotColor)
                        .frame(width: 8, height: 8)
                        .overlay { Circle().strokeBorder(Color.white.opacity(0.2)) }
                    VStack(alignment: .leading, spacing: 1) {
                        Text(session.isDemo ? "Demo mode" : session.instanceName)
                            .font(.system(size: 12, weight: .medium))
                            .foregroundStyle(Color.rpForeground)
                            .lineLimit(1)
                            .truncationMode(.middle)
                        Text(subtitle)
                            .font(.system(size: 10))
                            .foregroundStyle(Color.rpMutedForeground)
                            .lineLimit(1)
                            .truncationMode(.middle)
                    }
                    Spacer(minLength: 4)
                    Image(systemName: "chevron.up.chevron.down")
                        .font(.system(size: 9, weight: .semibold))
                        .foregroundStyle(Color.rpMutedForeground)
                }
                .padding(.horizontal, 10)
                .padding(.vertical, 8)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background {
                    RoundedRectangle(cornerRadius: RPRadius.md)
                        .fill(isHovering ? Color.rpAccent.opacity(0.7) : Color.clear)
                }
                .contentShape(.rect(cornerRadius: RPRadius.md))
            }
            .buttonStyle(.plain)
            .onHover { isHovering = $0 }
            .padding(8)
            .popover(isPresented: $isShowingSwitcher, arrowEdge: .trailing) {
                switcher
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(
            session.isDemo
                ? "Demo mode, no server connected. Change cluster."
                : "Connected to \(session.instanceName). Change cluster."
        )
    }

    private var subtitle: String {
        if session.isDemo { return "No server — bundled sample data" }
        return session.activeInstance?.displayHost ?? "Connected"
    }

    private var switcher: some View {
        VStack(alignment: .leading, spacing: 2) {
            ConsoleSectionLabel("Clusters")
                .padding(.horizontal, 6)
                .padding(.bottom, 2)
            ForEach(session.instances.instances) { instance in
                ConsoleNavRow(
                    title: instance.name,
                    isSelected: instance.id == session.activeInstance?.id,
                    action: {
                        isShowingSwitcher = false
                        if instance.id != session.activeInstance?.id {
                            _ = session.activate(instance)
                        }
                    },
                    leading: {
                        Circle()
                            .fill(instance.tint.color)
                            .frame(width: 8, height: 8)
                            .frame(width: 16)
                    },
                    trailing: {
                        Text(instance.displayHost)
                            .font(.system(size: 10))
                            .foregroundStyle(Color.rpMutedForeground)
                            .lineLimit(1)
                    }
                )
            }
            Divider().padding(.vertical, 5)
            ConsoleNavRow(
                title: "Add a cluster…",
                action: { isShowingSwitcher = false; isAddingInstance = true },
                leading: { Image(systemName: "plus.circle").frame(width: 16) }
            )
            ConsoleNavRow(
                title: "Manage clusters…",
                action: { isShowingSwitcher = false; isManagingInstances = true },
                leading: { Image(systemName: "server.rack").frame(width: 16) }
            )
            if !session.isDemo {
                ConsoleNavRow(
                    title: "Switch to demo mode",
                    action: { isShowingSwitcher = false; session.enterDemoMode() },
                    leading: { Image(systemName: "theatermasks").frame(width: 16) }
                )
            }
        }
        .padding(8)
        .frame(width: 290)
        .background(Color.rpSidebar)
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
