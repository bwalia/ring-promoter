import SwiftUI

/// One application's pipeline, laid out for a wide window.
///
/// A Mac-only *container* around the shared `AppDetailStore` and the shared
/// `RingCard`. Not one promotion rule, legality check or action call is
/// restated here — `RingCard` is the same type the iPhone draws, so what the
/// three clients allow an operator to do cannot drift.
///
/// What differs is the arrangement. `Features/AppDetail/AppDetailView.swift`
/// stacks the rings in a grouped `List`, which is right on a phone. macOS
/// renders that as a plain full-bleed table: the rows run to the window edges
/// with no gutter, and "Promote to test" — a full-width button, correct at
/// 343pt — becomes a 900pt capsule.
///
/// The web console already solved this. `web/src/components/dashboard/
/// pipeline.tsx` switches to `xl:flex-row` on a wide screen and puts the rings
/// side by side as `rounded-xl border` panels, each with its actions stacked
/// at the foot. This does the same, and falls back to the vertical stack when
/// the window is too narrow to hold them.
struct MacAppDetailView: View {
    let app: String

    @Environment(AppSession.self) private var session
    @Environment(Router.self) private var router
    @State private var store: AppDetailStore?
    @State private var sheet: ActionSheetRequest?

    var body: some View {
        Group {
            if let store {
                ScrollView {
                    AppDetailPipeline(store: store) { sheet = $0 }
                }
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .navigationTitle(store?.title ?? app)
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                Button {
                    Task { await store?.refresh() }
                } label: {
                    Image(systemName: "arrow.clockwise")
                        .symbolEffect(.pulse, isActive: store?.isLoading == true)
                }
                .help("Refresh now")
                .accessibilityLabel("Refresh now")
            }
            ToolbarItem(placement: .primaryAction) {
                Menu {
                    NavigationLink(value: Route.history(app: app)) {
                        Label("History", systemImage: "clock.arrow.circlepath")
                    }
                    NavigationLink(value: Route.maintenance(app: app)) {
                        Label("Maintenance windows", systemImage: "wrench.and.screwdriver")
                    }
                } label: {
                    Label("More", systemImage: "ellipsis.circle")
                }
                .menuStyle(.borderlessButton)
                .fixedSize()
            }
        }
        .task {
            if store == nil { store = AppDetailStore(app: app, session: session) }
            await store?.refresh()
        }
        .sheet(item: $sheet) { request in
            if let store {
                ActionSheetView(store: store, request: request) { jobID in
                    router.showJob(app: app, id: jobID)
                }
            }
        }
    }
}

/// The panels themselves, split out of the screen above so `ConsoleSnapshot`
/// can render them: `ImageRenderer` draws a `ScrollView` as an empty box.
struct AppDetailPipeline: View {
    @Bindable var store: AppDetailStore
    /// Only `ConsoleSnapshot` passes one — see `AppActivityPanel`.
    var preloadedActivity: AppActivityLoader?
    let onAction: (ActionSheetRequest) -> Void

    /// Below this a ring panel stops being readable. Chosen so the four-ring
    /// pipeline stays on a single row in a default 1180pt window — at that
    /// size each panel is ~219pt, which still holds "Promote to test".
    private let minimumPanelWidth: CGFloat = 212

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            if store.rings.isEmpty, store.error == nil {
                loading
            } else {
                summary
            }

            if let error = store.error, store.rings.isEmpty {
                ConsoleCard(isHighlighted: true) {
                    ErrorRow(error: error) { Task { await store.refresh() } }
                }
            }

            if !store.unhealthyRings.isEmpty {
                ConsoleCard(isHighlighted: true) {
                    VStack(alignment: .leading, spacing: 12) {
                        ForEach(store.unhealthyRings) { ring in
                            UnhealthyRingCallout(ring: ring)
                        }
                    }
                }
            }

            if !store.rings.isEmpty {
                ConsoleSectionLabel("Pipeline")
                rings
            }

            if !store.rings.isEmpty {
                AppActivityPanel(app: store.app, preloaded: preloadedActivity)
                    .padding(.top, 2)
            }

            if let updated = store.lastUpdated {
                RelativeTimestamp(date: updated, prefix: "Updated")
                    .frame(maxWidth: .infinity, alignment: .center)
                    .padding(.top, 2)
            }
        }
        .padding(.horizontal, 22)
        .padding(.vertical, 20)
    }

    /// The three answers an operator wants before scrolling: what is in
    /// production, is it healthy, and how fresh is this. Taken straight from
    /// `web/src/components/dashboard/overview-cards.tsx`, which puts the same
    /// three across the top of its dashboard.
    private var summary: some View {
        HStack(alignment: .top, spacing: 44) {
            ConsoleStat(label: "Production") {
                if let version = productionVersion {
                    ConsoleStatValue(text: version, isMonospaced: true)
                } else {
                    ConsoleStatValue(text: "—", tint: .rpMutedForeground)
                }
            }
            ConsoleStat(label: "Ring health") {
                let (healthy, active) = healthCount
                if active == 0 {
                    ConsoleStatValue(text: "—", tint: .rpMutedForeground)
                } else {
                    ConsoleStatValue(
                        text: healthy == active ? "Healthy" : "\(healthy)/\(active) healthy",
                        tint: healthy == active ? .rpHealthy : .rpUnhealthy,
                        dot: healthy == active ? .rpHealthy : .rpUnhealthy
                    )
                }
            }
            // Deliberately not "this server needs a production password":
            // that is true of every application on it, so it belongs on the
            // confirmation sheet where it changes what you do, not repeated
            // across every pipeline. Drift is per-application and is the thing
            // worth noticing without scrolling.
            if driftingRings > 0 {
                ConsoleStat(label: "Version drift") {
                    ConsoleStatValue(
                        text: "\(driftingRings) ring\(driftingRings == 1 ? "" : "s")",
                        tint: .rpGate, dot: .rpGate
                    )
                }
                .accessibilityLabel(
                    "\(driftingRings) ring\(driftingRings == 1 ? "" : "s") "
                        + "reporting a different live version"
                )
            }
            Spacer(minLength: 0)
        }
        .padding(.bottom, 2)
    }

    /// Shaped like what is coming, so the pane does not jump when it arrives.
    /// A spinner would only say "wait".
    private var loading: some View {
        VStack(alignment: .leading, spacing: 18) {
            HStack(spacing: 44) {
                ForEach(0..<3, id: \.self) { _ in
                    VStack(alignment: .leading, spacing: 7) {
                        ConsoleSkeleton(width: 78, height: 9)
                        ConsoleSkeleton(width: 120, height: 20)
                    }
                }
                Spacer(minLength: 0)
            }
            ConsoleSkeleton(width: 70, height: 9)
            PanelGrid(minimumItemWidth: minimumPanelWidth, spacing: 12) {
                ForEach(0..<4, id: \.self) { _ in
                    ConsoleCard(fillsHeight: true) {
                        VStack(alignment: .leading, spacing: 12) {
                            ConsoleSkeleton(width: 110, height: 16)
                            ConsoleSkeleton(height: 12)
                            ConsoleSkeleton(width: 140, height: 12)
                            ConsoleSkeleton(height: 28, cornerRadius: 14)
                                .padding(.top, 18)
                        }
                    }
                }
            }
        }
        .accessibilityLabel("Loading the pipeline")
    }

    /// Rings whose endpoint reports a version other than the one deployed.
    private var driftingRings: Int {
        store.rings.filter { $0.configured && $0.versionIsDrifting }.count
    }

    private var productionVersion: String? {
        guard let prod = store.rings.first(where: { store.pipeline.isProduction($0.ring.name) }),
              prod.configured, !prod.isEmpty
        else { return nil }
        return prod.currentVersion
    }

    /// Only rings that are configured *and* carry a version can be healthy or
    /// not; an empty ring is neither, and counting it either way would be a
    /// lie on the one figure an operator reads first.
    private var healthCount: (healthy: Int, active: Int) {
        let active = store.rings.filter { $0.configured && !$0.isEmpty }
        return (active.filter(\.isHealthy).count, active.count)
    }

    /// Side by side when the window can hold every ring at a readable width,
    /// wrapping to further rows when it cannot — which is exactly what the web
    /// console's `xl:flex-row` does. `PanelGrid` explains why this is a custom
    /// layout rather than `ViewThatFits`.
    private var rings: some View {
        PanelGrid(minimumItemWidth: minimumPanelWidth, spacing: 12) {
            ForEach(store.rings) { ring in
                panel(for: ring)
            }
        }
    }

    private func panel(for ring: RingStatus) -> some View {
        ConsoleCard(isHighlighted: ring.needsAttention, fillsHeight: true) {
            RingCard(ring: ring, store: store, onAction: onAction)
        }
    }
}
