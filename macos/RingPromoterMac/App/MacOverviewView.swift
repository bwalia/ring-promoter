import SwiftUI

/// The application list, as cards on the console surface.
///
/// A Mac-only view of the *same* `OverviewStore` the iPhone uses: not one line
/// of loading, sorting, filtering or trouble-detection logic is duplicated
/// here, and `PipelineStrip`, `StatusBadge`, `ErrorRow`, `CalmEmptyState` and
/// `RelativeTimestamp` are the shared components.
///
/// What differs is the container. `Features/Overview/OverviewView.swift` draws
/// a grouped `List`, which is right on a phone and is what produced the flat,
/// hairline-separated rows this app shipped with on the Mac — AppKit renders a
/// grouped list as a plain table. The web console draws panels: `rounded-lg
/// border bg-card`, stacked with air between them. On a 1180pt window that is
/// both the better layout and the consistent one, so this pane follows the web.
struct MacOverviewView: View {
    @Bindable var store: OverviewStore
    @Environment(AppSession.self) private var session
    @Environment(Router.self) private var router

    var body: some View {
        @Bindable var router = router

        NavigationStack(path: $router.overviewPath) {
            ScrollView {
                OverviewCardStack(store: store)
            }
            .navigationTitle("Applications")
            .toolbar {
                ToolbarItem(placement: .navigation) {
                    InstanceSwitcher()
                        // AppKit renders a bare `Menu` as a bordered pop-up
                        // button, which boxes the instance banner in a second
                        // frame. Borderless lets the banner be the control.
                        .menuStyle(.borderlessButton)
                        .fixedSize()
                }
                ToolbarItem(placement: .primaryAction) {
                    if !store.groups.isEmpty { MacGroupFilterMenu(store: store) }
                }
                ToolbarItem(placement: .primaryAction) {
                    Button {
                        Task { await store.refresh() }
                    } label: {
                        Image(systemName: "arrow.clockwise")
                            .symbolEffect(.pulse, isActive: store.isLoading)
                    }
                    .help("Refresh now")
                    .accessibilityLabel("Refresh now")
                }
            }
            .navigationDestination(for: Route.self) { route in
                destination(for: route).consoleSurface()
            }
        }
        .consoleSurface()
    }

    @ViewBuilder
    private func destination(for route: Route) -> some View {
        switch route {
        case .app(let name): MacAppDetailView(app: name)
        case .job(let app, let id): JobView(app: app, jobID: id)
        case .history(let app): HistoryView(app: app)
        case .maintenance(let app): MaintenanceView(app: app)
        }
    }
}

/// The cards themselves, split out of the screen above so the offscreen
/// renderer in `ConsoleSnapshot` can draw them without a window.
struct OverviewCardStack: View {
    @Bindable var store: OverviewStore
    @Environment(AppSession.self) private var session

    var body: some View {
        // Not a LazyVStack: a control plane has tens of applications, not
        // thousands, so laziness buys nothing — and a lazy container draws
        // nothing at all under `ImageRenderer`, which is how this pane is
        // checked.
        VStack(alignment: .leading, spacing: 18) {
            if store.summaries.isEmpty, store.error == nil, store.lastUpdated == nil {
                loading
            } else if !store.summaries.isEmpty {
                summary
            }

            if let error = store.error, store.summaries.isEmpty {
                ConsoleCard(isHighlighted: true) {
                    ErrorRow(error: error) { Task { await store.refresh() } }
                }
            }

            if store.isStale {
                staleBanner
            }

            if !store.troubled.isEmpty {
                group(
                    label: "Needs attention",
                    systemImage: "exclamationmark.triangle.fill",
                    tint: .rpUnhealthy,
                    summaries: store.troubled
                )
            }

            if store.isAllClear, store.searchText.isEmpty {
                ConsoleCard {
                    CalmEmptyState(
                        title: "Everything is healthy",
                        message: "All rings are passing their health checks."
                    )
                }
            }

            if !store.calm.isEmpty {
                group(
                    label: store.troubled.isEmpty ? "Applications" : "Everything else",
                    systemImage: nil,
                    tint: .rpMutedForeground,
                    summaries: store.calm
                )
            }

            if store.orderedSummaries.isEmpty, store.error == nil, !store.isLoading {
                ConsoleCard {
                    CalmEmptyState(
                        title: store.searchText.isEmpty ? "No applications" : "No matches",
                        message: store.searchText.isEmpty
                            ? "This cluster has no applications configured."
                            : "Nothing matches \u{201C}\(store.searchText)\u{201D}.",
                        systemImage: "magnifyingglass",
                        tint: .rpNeutral
                    )
                }
            }

            if let lastUpdated = store.lastUpdated {
                RelativeTimestamp(date: lastUpdated, prefix: "Updated")
                    .frame(maxWidth: .infinity, alignment: .center)
                    .padding(.top, 4)
            }
        }
        .padding(.horizontal, 22)
        .padding(.vertical, 20)
        .frame(maxWidth: 980, alignment: .leading)
        .frame(maxWidth: .infinity)
    }

    /// The fleet in three figures, before any scrolling. The same shape the
    /// web console puts across the top of a dashboard.
    private var summary: some View {
        HStack(alignment: .top, spacing: 44) {
            ConsoleStat(label: "Applications") {
                ConsoleStatValue(text: "\(store.summaries.count)", isMonospaced: true)
            }
            ConsoleStat(label: "Needing attention") {
                ConsoleStatValue(
                    text: store.troubled.isEmpty ? "None" : "\(store.troubled.count)",
                    tint: store.troubled.isEmpty ? .rpHealthy : .rpUnhealthy,
                    isMonospaced: !store.troubled.isEmpty,
                    dot: store.troubled.isEmpty ? .rpHealthy : .rpUnhealthy
                )
            }
            if let deploying = busyCount {
                ConsoleStat(label: "In flight") {
                    ConsoleStatValue(
                        text: "\(deploying)", tint: .rpInFlight,
                        isMonospaced: true, dot: .rpInFlight
                    )
                }
            }
            Spacer(minLength: 0)
        }
        .padding(.bottom, 2)
    }

    private var busyCount: Int? {
        let count = store.summaries.filter(\.isBusy).count
        return count == 0 ? nil : count
    }

    /// Shaped like the cards that are coming, so nothing jumps when they
    /// arrive.
    private var loading: some View {
        VStack(alignment: .leading, spacing: 18) {
            HStack(spacing: 44) {
                ForEach(0..<2, id: \.self) { _ in
                    VStack(alignment: .leading, spacing: 7) {
                        ConsoleSkeleton(width: 92, height: 9)
                        ConsoleSkeleton(width: 70, height: 20)
                    }
                }
                Spacer(minLength: 0)
            }
            ConsoleSkeleton(width: 84, height: 9)
            ForEach(0..<3, id: \.self) { _ in
                ConsoleCard {
                    VStack(alignment: .leading, spacing: 12) {
                        ConsoleSkeleton(width: 150, height: 16)
                        ConsoleSkeleton(height: 42, cornerRadius: RPRadius.md)
                    }
                }
            }
        }
        .accessibilityLabel("Loading applications")
    }

    private var staleBanner: some View {
        ConsoleCard(padding: 12) {
            Label(
                "Showing the last data this app was able to load.",
                systemImage: "wifi.exclamationmark"
            )
            .font(.system(size: 13))
            .foregroundStyle(Color.rpGate)
        }
    }

    @ViewBuilder
    private func group(
        label: String, systemImage: String?, tint: Color, summaries: [AppSummary]
    ) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 5) {
                if let systemImage {
                    Image(systemName: systemImage)
                        .font(.system(size: 10, weight: .semibold))
                        .foregroundStyle(tint)
                }
                Text(label.uppercased())
                    .font(RPFont.sectionLabel)
                    .tracking(0.6)
                    .foregroundStyle(systemImage == nil ? Color.rpMutedForeground : tint)
            }
            .accessibilityAddTraits(.isHeader)
            .padding(.leading, 2)

            ForEach(summaries) { summary in
                AppCard(summary: summary)
            }
        }
    }

}

/// One application as a panel: title, the shared pipeline strip, and the
/// trouble line if it has one.
private struct AppCard: View {
    let summary: AppSummary
    @Environment(AppSession.self) private var session
    @State private var isHovering = false

    var body: some View {
        NavigationLink(value: Route.app(summary.name)) {
            VStack(alignment: .leading, spacing: 10) {
                HStack(spacing: 8) {
                    Circle()
                        .fill(summary.consoleTint)
                        .frame(width: 8, height: 8)
                    Text(summary.title)
                        .font(RPFont.panelHeading)
                        .foregroundStyle(Color.rpForeground)
                        .lineLimit(1)
                    Spacer(minLength: 6)
                    if let action = summary.busyAction {
                        StatusBadge(
                            text: action, systemImage: "arrow.triangle.2.circlepath",
                            tint: .rpInFlight
                        )
                    }
                    Image(systemName: "chevron.right")
                        .font(.system(size: 10, weight: .semibold))
                        .foregroundStyle(Color.rpMutedForeground)
                        .opacity(isHovering ? 1 : 0.35)
                }
                PipelineStrip(rings: summary.rings, pipeline: session.pipeline)
                if let trouble = summary.troubleSummary {
                    Label(trouble, systemImage: "exclamationmark.circle.fill")
                        .font(.system(size: 12))
                        .foregroundStyle(Color.rpUnhealthy)
                        .lineLimit(2)
                }
            }
            .padding(14)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Color.rpCard, in: .rect(cornerRadius: RPRadius.lg))
            .overlay {
                RoundedRectangle(cornerRadius: RPRadius.lg)
                    .strokeBorder(
                        summary.isInTrouble
                            ? Color.rpUnhealthy.opacity(isHovering ? 0.6 : 0.4)
                            : (isHovering ? Color.rpMutedForeground.opacity(0.4) : Color.rpBorder)
                    )
            }
            .contentShape(.rect(cornerRadius: RPRadius.lg))
        }
        .buttonStyle(.plain)
        .onHover { isHovering = $0 }
        .accessibilityHint("Opens the pipeline for \(summary.title)")
    }
}

/// Filter the pane by a server-side group. Same menu the iPhone shows; the
/// `private` original lives in the shared `OverviewView.swift` and cannot be
/// reached from here.
private struct MacGroupFilterMenu: View {
    @Bindable var store: OverviewStore

    var body: some View {
        Menu {
            Picker("Group", selection: $store.selectedGroupID) {
                Text("All applications").tag(String?.none)
                ForEach(store.groups) { group in
                    Text(group.name).tag(String?.some(group.id))
                }
            }
            .pickerStyle(.inline)
        } label: {
            Label(
                store.selectedGroup?.name ?? "All",
                systemImage: store.selectedGroupID == nil
                    ? "line.3.horizontal.decrease.circle"
                    : "line.3.horizontal.decrease.circle.fill"
            )
        }
        .menuStyle(.borderlessButton)
        .fixedSize()
        .help("Filter by group")
        .accessibilityLabel("Filter by group")
    }
}
