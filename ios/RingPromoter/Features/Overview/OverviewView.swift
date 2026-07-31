import SwiftUI

/// The root screen: every application, with trouble at the top.
struct OverviewView: View {
    @Environment(AppSession.self) private var session
    @Environment(Router.self) private var router
    @Environment(\.scenePhase) private var scenePhase
    @State private var store: OverviewStore?

    var body: some View {
        @Bindable var router = router

        NavigationStack(path: $router.overviewPath) {
            Group {
                if let store {
                    OverviewList(store: store)
                } else {
                    ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
                }
            }
            .navigationTitle("Applications")
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    InstanceBanner(
                        name: session.instanceName, tint: session.instanceTint,
                        isDemo: session.isDemo
                    )
                }
                ToolbarItem(placement: .topBarTrailing) {
                    if let store, !store.groups.isEmpty {
                        GroupFilterMenu(store: store)
                    }
                }
            }
            .navigationDestination(for: Route.self) { route in
                destination(for: route)
            }
        }
        .task {
            if store == nil { store = OverviewStore(session: session) }
            await store?.refresh()
        }
        .task(id: session.settings.refreshInterval) {
            await autoRefreshLoop()
        }
        .onChange(of: scenePhase) { _, phase in
            // Coming back to the app is the moment an operator most wants the
            // truth, so refresh immediately rather than waiting for the timer.
            if phase == .active { Task { await store?.refresh() } }
        }
    }

    /// Periodic refresh while the Overview is on screen. Cancelled
    /// automatically when the view goes away, because `.task` owns it.
    private func autoRefreshLoop() async {
        let interval = session.settings.refreshInterval
        guard interval > 0 else { return }
        while !Task.isCancelled {
            try? await Task.sleep(for: .seconds(interval))
            guard !Task.isCancelled else { return }
            await store?.refresh()
        }
    }

    @ViewBuilder
    private func destination(for route: Route) -> some View {
        switch route {
        case .app(let name):
            AppDetailView(app: name)
        case .job(let app, let id):
            JobView(app: app, jobID: id)
        case .history(let app):
            HistoryView(app: app)
        case .maintenance(let app):
            MaintenanceView(app: app)
        }
    }
}

/// The list itself, split out so the loading branch above stays readable.
private struct OverviewList: View {
    @Bindable var store: OverviewStore
    @Environment(Router.self) private var router

    var body: some View {
        List {
            if let error = store.error, store.summaries.isEmpty {
                Section { ErrorRow(error: error) { Task { await store.refresh() } } }
            }

            if store.isStale {
                Section {
                    Label(
                        "Showing the last data this app was able to load.",
                        systemImage: "wifi.exclamationmark"
                    )
                    .font(.subheadline)
                    .foregroundStyle(Color.rpGate)
                }
            }

            if !store.troubled.isEmpty {
                Section {
                    ForEach(store.troubled) { summary in
                        AppRow(summary: summary)
                    }
                } header: {
                    Label("Needs attention", systemImage: "exclamationmark.triangle.fill")
                        .foregroundStyle(Color.rpUnhealthy)
                }
            }

            if store.isAllClear, store.searchText.isEmpty {
                Section {
                    CalmEmptyState(
                        title: "Everything is healthy",
                        message: "All rings are passing their health checks."
                    )
                    .listRowBackground(Color.clear)
                }
            }

            if !store.calm.isEmpty {
                Section(store.troubled.isEmpty ? "Applications" : "Everything else") {
                    ForEach(store.calm) { summary in
                        AppRow(summary: summary)
                    }
                }
            }

            if store.orderedSummaries.isEmpty, store.error == nil, !store.isLoading {
                Section {
                    CalmEmptyState(
                        title: store.searchText.isEmpty
                            ? "No applications" : "No matches",
                        message: store.searchText.isEmpty
                            ? "This control plane has no applications configured."
                            : "Nothing matches “\(store.searchText)”.",
                        systemImage: "magnifyingglass",
                        tint: .rpNeutral
                    )
                    .listRowBackground(Color.clear)
                }
            }

            if let lastUpdated = store.lastUpdated {
                Section {
                    RelativeTimestamp(date: lastUpdated, prefix: "Updated")
                        .frame(maxWidth: .infinity, alignment: .center)
                        .listRowBackground(Color.clear)
                }
            }
        }
        .searchable(text: $store.searchText, prompt: "Search applications")
        .refreshable { await store.refresh() }
    }
}

/// One application: title, pipeline strip, and its trouble line if it has one.
private struct AppRow: View {
    let summary: AppSummary
    @Environment(AppSession.self) private var session

    var body: some View {
        NavigationLink(value: Route.app(summary.name)) {
            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 8) {
                    Text(summary.title)
                        .font(.headline)
                        .lineLimit(1)
                    Spacer(minLength: 4)
                    if let action = summary.busyAction {
                        StatusBadge(
                            text: action, systemImage: "arrow.triangle.2.circlepath",
                            tint: .rpInFlight
                        )
                    }
                }
                PipelineStrip(rings: summary.rings, pipeline: session.pipeline)
                if let trouble = summary.troubleSummary {
                    Label(trouble, systemImage: "exclamationmark.circle.fill")
                        .font(.caption)
                        .foregroundStyle(Color.rpUnhealthy)
                        .lineLimit(2)
                }
            }
            .padding(.vertical, 4)
        }
        .accessibilityHint("Opens the pipeline for \(summary.title)")
    }
}

/// Filter the list by a server-side group.
private struct GroupFilterMenu: View {
    @Bindable var store: OverviewStore

    var body: some View {
        Menu {
            Picker("Group", selection: $store.selectedGroupID) {
                Text("All applications").tag(String?.none)
                ForEach(store.groups) { group in
                    Text(group.name).tag(String?.some(group.id))
                }
            }
        } label: {
            Label(
                store.selectedGroup?.name ?? "All",
                systemImage: store.selectedGroupID == nil
                    ? "line.3.horizontal.decrease.circle"
                    : "line.3.horizontal.decrease.circle.fill"
            )
        }
        .accessibilityLabel("Filter by group")
    }
}

/// A dismissible failure row with a retry, used wherever a load can fail.
struct ErrorRow: View {
    let error: APIError
    var retry: (() -> Void)?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label(error.title, systemImage: "exclamationmark.triangle.fill")
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(Color.rpUnhealthy)
            Text(error.userMessage)
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            if let retry, error.isRetryable {
                Button("Try again", action: retry)
                    .buttonStyle(.bordered)
                    .controlSize(.small)
            }
        }
        .padding(.vertical, 4)
        .accessibilityElement(children: .combine)
    }
}
