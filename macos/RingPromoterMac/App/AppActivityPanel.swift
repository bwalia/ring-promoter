import SwiftUI

/// What actually happened to this application, under its pipeline.
///
/// The web console puts a history panel on the dashboard; the phone puts it
/// one tap away behind the toolbar menu, which is right on a phone and wasteful
/// on a window with a thousand spare points below the rings. So the ledger is
/// inline here: who promoted what, where it went, and what the server said
/// when it failed.
///
/// `HistoryRow` is the shared component — the same rows the History screen and
/// the Activity feed draw, including the expandable detail and the AI
/// diagnosis. Only the container is Mac-specific.
struct AppActivityPanel: View {
    let app: String
    /// A loader that has already fetched. Only `ConsoleSnapshot` passes one:
    /// `ImageRenderer` never runs `.task`, so without it the panel could only
    /// ever be photographed mid-skeleton and its rows never checked.
    var preloaded: AppActivityLoader?

    @Environment(AppSession.self) private var session
    @State private var loader: AppActivityLoader?

    /// Seeded from `preloaded` so the very first synchronous render has it:
    /// `ImageRenderer` captures one pass and never waits for `.task`, so
    /// without this the panel can only be photographed empty. The `.task`
    /// below still rebuilds the loader whenever `app` changes, which is what
    /// stops one application's data showing under another's title.
    init(app: String, preloaded: AppActivityLoader? = nil) {
        self.app = app
        self.preloaded = preloaded
        _loader = State(initialValue: preloaded)
    }
    @State private var expanded: Set<Int64> = []


    /// Enough to answer "what just happened"; the rest is behind "Full
    /// history", which is the screen built for reading it.
    private let inlineLimit = 6

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            header
            ConsoleCard(padding: 0) {
                VStack(spacing: 0) {
                    if let loader {
                        content(loader)
                    } else {
                        rowPadding { ConsoleSkeleton(height: 34) }
                    }
                }
            }
        }
        .task(id: app) {
            if let preloaded {
                loader = preloaded
                return
            }
            // Keyed on `app`, and rebuilt rather than reused. The sidebar
            // jumps straight from one application to another at the same depth
            // in the navigation stack, and SwiftUI reuses this view when it
            // does — so creating the store only when it was nil left the
            // previous application's data on screen under the new title.
            let created = AppActivityLoader(app: app, session: session)
            loader = created
            await created.refresh()
        }
    }

    private var header: some View {
        HStack(spacing: 8) {
            ConsoleSectionLabel("Recent activity")
            NavigationLink(value: Route.history(app: app)) {
                Text("Full history")
                    .font(.system(size: 11, weight: .medium))
                    .foregroundStyle(Color.rpMutedForeground)
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Open the full history for this application")
        }
    }

    @ViewBuilder
    private func content(_ loader: AppActivityLoader) -> some View {
        if let error = loader.error, loader.entries.isEmpty {
            rowPadding {
                ErrorRow(error: error) { Task { await loader.refresh() } }
            }
        } else if loader.entries.isEmpty {
            rowPadding {
                if loader.isLoading {
                    ConsoleSkeleton(height: 34)
                } else {
                    Label(
                        "Nothing yet. Seeds, promotions and rollbacks appear here as they "
                            + "happen.",
                        systemImage: "clock"
                    )
                    .font(.system(size: 12))
                    .foregroundStyle(Color.rpMutedForeground)
                }
            }
        } else {
            let shown = Array(loader.entries.prefix(inlineLimit))
            ForEach(Array(shown.enumerated()), id: \.element.id) { index, entry in
                if index > 0 {
                    Rectangle().fill(Color.rpBorder).frame(height: 1)
                }
                rowPadding {
                    HistoryRow(
                        app: app,
                        entry: entry,
                        isExpanded: expanded.contains(entry.id),
                        onToggle: { toggle(entry.id) }
                    )
                }
            }
        }
    }

    private func rowPadding<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        content()
            .padding(.horizontal, 14)
            .padding(.vertical, 11)
            .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func toggle(_ id: Int64) {
        if expanded.contains(id) { expanded.remove(id) } else { expanded.insert(id) }
    }
}

/// Loads one application's ledger.
///
/// Mac-only and separate from `AppDetailStore` on purpose: the phone does not
/// show history beside the pipeline, so putting this in the shared store would
/// make every iPhone refresh pay for a request it never draws.
@MainActor
@Observable
final class AppActivityLoader {
    private(set) var entries: [HistoryEntry] = []
    private(set) var isLoading = false
    private(set) var error: APIError?

    private let app: String
    private let session: AppSession

    init(app: String, session: AppSession) {
        self.app = app
        self.session = session
    }

    func refresh() async {
        isLoading = true
        defer { isLoading = false }
        do throws(APIError) {
            entries = try await session.api.history(app: app)
            error = nil
        } catch {
            // Keep whatever is on screen: a flaky history request must not
            // blank the ledger an operator is reading.
            self.error = error
            session.note(error)
        }
    }
}


/// The readiness scorecard, under the pipeline.
///
/// `HopScoreCard` is the shared component — the phone draws the same cards in
/// a `Section`. Only the panel around them is Mac-specific.
struct DeploymentControlsPanel: View {
    let app: String
    /// See `AppActivityPanel.preloaded`.
    var preloaded: ControlsLoader?

    @Environment(AppSession.self) private var session
    @State private var loader: ControlsLoader?

    /// Seeded from `preloaded` so the very first synchronous render has it:
    /// `ImageRenderer` captures one pass and never waits for `.task`, so
    /// without this the panel can only be photographed empty. The `.task`
    /// below still rebuilds the loader whenever `app` changes, which is what
    /// stops one application's data showing under another's title.
    init(app: String, preloaded: ControlsLoader? = nil) {
        self.app = app
        self.preloaded = preloaded
        _loader = State(initialValue: preloaded)
    }
    @State private var expanded: Set<String> = []


    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                ConsoleSectionLabel("Deployment controls")
                Text("scored for the hop in front of you")
                    .font(.system(size: 11))
                    .foregroundStyle(Color.rpMutedForeground)
            }
            ConsoleCard(padding: 0) {
                VStack(spacing: 0) {
                    if let loader {
                        content(loader)
                    } else {
                        row { ConsoleSkeleton(height: 48) }
                    }
                }
            }
        }
        .task(id: app) {
            if let preloaded {
                loader = preloaded
                return
            }
            // Keyed on `app`, and rebuilt rather than reused. The sidebar
            // jumps straight from one application to another at the same depth
            // in the navigation stack, and SwiftUI reuses this view when it
            // does — so creating the store only when it was nil left the
            // previous application's data on screen under the new title.
            let created = ControlsLoader(app: app, session: session)
            loader = created
            await created.refresh()
        }
    }

    @ViewBuilder
    private func content(_ loader: ControlsLoader) -> some View {
        if let error = loader.error, loader.view.hops.isEmpty {
            row { ErrorRow(error: error) { Task { await loader.refresh() } } }
        } else if loader.view.hops.isEmpty {
            row {
                if loader.isLoading {
                    ConsoleSkeleton(height: 48)
                } else {
                    Label(
                        "No promotions to score — this application has a single ring.",
                        systemImage: "checkmark.circle"
                    )
                    .font(.system(size: 12))
                    .foregroundStyle(Color.rpMutedForeground)
                }
            }
        } else {
            ForEach(Array(loader.view.hops.enumerated()), id: \.element.id) { index, hop in
                if index > 0 {
                    Rectangle().fill(Color.rpBorder).frame(height: 1)
                }
                row {
                    HopScoreCard(
                        hop: hop,
                        isExpanded: expanded.contains(hop.id),
                        onToggle: { toggle(hop.id) }
                    )
                }
            }
        }
    }

    private func row<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        content()
            .padding(.horizontal, 14)
            .padding(.vertical, 11)
            .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func toggle(_ id: String) {
        if expanded.contains(id) { expanded.remove(id) } else { expanded.insert(id) }
    }
}


/// Scan posture per ring, with the policy it is judged against.
///
/// `SecurityPostureCard` is the shared component — the phone stacks the same
/// cards in a `Section`. Laid across a `PanelGrid` here for the same reason
/// the rings are: one card per ring reads as a row, not a list.
struct SecurityPanel: View {
    let app: String
    /// See `AppActivityPanel.preloaded`.
    var preloaded: SecurityLoader?

    @Environment(AppSession.self) private var session
    @State private var loader: SecurityLoader?

    /// Seeded from `preloaded` so the very first synchronous render has it:
    /// `ImageRenderer` captures one pass and never waits for `.task`, so
    /// without this the panel can only be photographed empty. The `.task`
    /// below still rebuilds the loader whenever `app` changes, which is what
    /// stops one application's data showing under another's title.
    init(app: String, preloaded: SecurityLoader? = nil) {
        self.app = app
        self.preloaded = preloaded
        _loader = State(initialValue: preloaded)
    }

    var body: some View {
        Group {
            if let loader, !loader.security.isSilent || loader.error != nil {
                VStack(alignment: .leading, spacing: 8) {
                    HStack(spacing: 8) {
                        ConsoleSectionLabel("Security & compliance")
                        Text(loader.security.policy.summary)
                            .font(.system(size: 11))
                            .foregroundStyle(Color.rpMutedForeground)
                            .lineLimit(1)
                    }
                    ConsoleCard {
                        content(loader)
                    }
                }
            }
        }
        .task(id: app) {
            if let preloaded {
                loader = preloaded
                return
            }
            // Keyed on `app` and rebuilt, like every other per-application
            // view here.
            let created = SecurityLoader(app: app, session: session)
            loader = created
            await created.refresh()
        }
    }

    @ViewBuilder
    private func content(_ loader: SecurityLoader) -> some View {
        if let error = loader.error, loader.security.posture.isEmpty {
            ErrorRow(error: error) { Task { await loader.refresh() } }
        } else {
            VStack(alignment: .leading, spacing: 14) {
                PanelGrid(minimumItemWidth: 196, spacing: 12) {
                    ForEach(loader.security.posture) { evaluation in
                        SecurityPostureCard(evaluation: evaluation)
                            .padding(12)
                            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                            .background(
                                Color.rpBackground.opacity(0.5),
                                in: .rect(cornerRadius: RPRadius.md)
                            )
                            .overlay {
                                RoundedRectangle(cornerRadius: RPRadius.md)
                                    .strokeBorder(Color.rpBorder)
                            }
                    }
                }
                if loader.security.posture.allSatisfy(\.isUnscanned) {
                    SecurityScanHint(app: app)
                }
            }
        }
    }
}
