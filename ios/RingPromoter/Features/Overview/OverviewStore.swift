import Foundation
import SwiftUI

/// One application as the Overview shows it.
struct AppSummary: Identifiable, Hashable, Sendable {
    let name: String
    let title: String
    let rings: [RingStatus]
    /// The newest job for this app, from the shared `GET /api/jobs` feed.
    let latestJob: Job?
    /// Rings could not be loaded; the strip shows what was cached, if anything.
    let loadError: String?

    var id: String { name }

    var unhealthyRings: [RingStatus] { rings.filter(\.needsAttention) }

    var isInTrouble: Bool {
        !unhealthyRings.isEmpty || latestJob?.outcome == .failed
            || latestJob?.outcome == .failedAndRolledBack || loadError != nil
    }

    /// An action is in flight for this application right now.
    ///
    /// Which *ring* it is deploying into cannot be shown: a running job carries
    /// no `result`, and therefore no target ring, until it finishes. The row
    /// says "promoting" for the whole application instead of guessing at a ring
    /// and animating the wrong one. See `docs/API-GAPS.md`.
    var isBusy: Bool {
        guard let job = latestJob else { return false }
        return !job.isFinished
    }

    /// The action currently running, for the badge on the row.
    var busyAction: String? {
        guard isBusy, let job = latestJob else { return nil }
        return job.promotionAction?.title ?? job.action.capitalized
    }

    /// The single line that explains why this app is pinned to the top.
    var troubleSummary: String? {
        if let loadError { return loadError }
        if let job = latestJob, job.outcome == .failedAndRolledBack {
            return job.summaryMessage ?? "Last deploy failed and was rolled back."
        }
        if let job = latestJob, job.outcome == .failed {
            return job.summaryMessage ?? "Last deploy failed."
        }
        let unhealthy = unhealthyRings
        guard !unhealthy.isEmpty else { return nil }
        if unhealthy.count == 1, let ring = unhealthy.first {
            return ring.liveHealthError.map { "\(ring.ring.label): \($0)" }
                ?? "\(ring.ring.label) is unhealthy."
        }
        return "\(unhealthy.count) rings are unhealthy."
    }
}

/// Loads and holds everything the Overview draws.
///
/// Ring requests run one after another on purpose. Nested `async let` /
/// `withTaskGroup` on this `@Observable` store SIGSEGV'd inside
/// `swift::TaskGroup::offer` on real iPhones (TestFlight 1.0 and 1.1) while
/// Observation notified SwiftUI — see TestFlight crash feedback from iPhone17,2.
@MainActor
@Observable
final class OverviewStore {
    private(set) var summaries: [AppSummary] = []
    private(set) var groups: [AppGroup] = []
    private(set) var isLoading = false
    /// When the data on screen was fetched. Nil before the first load.
    private(set) var lastUpdated: Date?
    /// The cached snapshot is being shown because the last refresh failed.
    private(set) var isStale = false
    private(set) var error: APIError?

    var searchText = ""
    var selectedGroupID: String?

    private let session: AppSession
    /// Guards against two refreshes racing (pull-to-refresh during a timer tick).
    private var refreshInFlight = false

    init(session: AppSession) {
        self.session = session
    }

    // MARK: - Derived

    /// Apps in trouble first, then the rest alphabetically by display title.
    /// Sorting here rather than in the view keeps "surface trouble first"
    /// testable.
    var orderedSummaries: [AppSummary] {
        filtered.sorted { lhs, rhs in
            if lhs.isInTrouble != rhs.isInTrouble { return lhs.isInTrouble }
            return lhs.title.localizedCaseInsensitiveCompare(rhs.title) == .orderedAscending
        }
    }

    var troubled: [AppSummary] { orderedSummaries.filter(\.isInTrouble) }
    var calm: [AppSummary] { orderedSummaries.filter { !$0.isInTrouble } }

    var selectedGroup: AppGroup? {
        guard let selectedGroupID else { return nil }
        return groups.first { $0.id == selectedGroupID }
    }

    /// Everything loaded fine and nothing is unhealthy.
    var isAllClear: Bool {
        !summaries.isEmpty && summaries.allSatisfy { !$0.isInTrouble }
    }

    private var filtered: [AppSummary] {
        var result = summaries
        if let group = selectedGroup {
            result = result.filter { group.contains($0.name) }
        }
        let query = searchText.trimmingCharacters(in: .whitespaces)
        if !query.isEmpty {
            result = result.filter {
                $0.title.localizedCaseInsensitiveContains(query)
                    || $0.name.localizedCaseInsensitiveContains(query)
            }
        }
        return result
    }

    // MARK: - Loading

    /// Refresh everything. Safe to call from pull-to-refresh, a timer, and
    /// `onAppear` at once: a second call while one is in flight is ignored
    /// (callers that need fresh data after a mutation should await the first).
    func refresh() async {
        guard !refreshInFlight else { return }
        refreshInFlight = true
        defer { refreshInFlight = false }
        await performRefresh()
    }

    private func performRefresh() async {
        isLoading = true
        defer { isLoading = false }

        let capabilities: AppsResponse
        do {
            capabilities = try await session.loadCapabilities()
        } catch {
            // Keep whatever is on screen and mark it stale, so a flaky network
            // never blanks the operator's only view of the system.
            self.error = error
            isStale = !summaries.isEmpty
            session.note(error)
            return
        }

        let api = session.api
        // Sequential on purpose — no `async let` / `withTaskGroup`. Those
        // nested TaskGroups crashed TestFlight iPhone builds inside
        // `TaskGroup::offer` while @Observable mutations hit SwiftUI.
        let jobs = (try? await api.recentJobs()) ?? []
        let fetchedGroups = (try? await api.groups()) ?? groups

        var results: [String: Result<[RingStatus], APIError>] = [:]
        for app in capabilities.apps {
            do throws(APIError) {
                results[app] = .success(try await api.rings(app: app))
            } catch {
                results[app] = .failure(error)
            }
        }

        let jobsByApp = Dictionary(
            jobs.map { ($0.app, $0) }, uniquingKeysWith: { first, _ in first }
        )

        // Preserve the server's app ordering; the view re-sorts by trouble.
        // Mutate Observable state only after every await has finished.
        summaries = capabilities.apps.map { app in
            let result = results[app]
            var loadError: String?
            if case .failure(let failure) = result { loadError = failure.userMessage }
            return AppSummary(
                name: app,
                title: capabilities.title(for: app),
                rings: (try? result?.get()) ?? cachedRings(for: app),
                latestJob: jobsByApp[app],
                loadError: loadError
            )
        }
        groups = fetchedGroups
        // A group that has been deleted server-side must not keep filtering.
        if let selectedGroupID, !groups.contains(where: { $0.id == selectedGroupID }) {
            self.selectedGroupID = nil
        }
        error = nil
        isStale = false
        lastUpdated = .now
        publishWidgetSnapshot()
    }

    private func cachedRings(for app: String) -> [RingStatus] {
        summaries.first { $0.name == app }?.rings ?? []
    }

    /// Hand the widget a fresh snapshot after every successful refresh.
    private func publishWidgetSnapshot() {
        let snapshot = WidgetSnapshot(
            instanceName: session.instanceName,
            instanceTint: session.instanceTint.rawValue,
            apps: summaries.map { summary in
                WidgetSnapshot.AppCell(
                    name: summary.name,
                    title: summary.title,
                    rings: summary.rings.map { ring in
                        WidgetSnapshot.RingCell(
                            name: ring.ring.name, label: ring.ring.label,
                            version: ring.currentVersion, healthy: ring.isHealthy,
                            configured: ring.configured, empty: ring.isEmpty
                        )
                    }
                )
            },
            capturedAt: .now,
            isDemo: session.isDemo
        )
        WidgetSnapshotStore.write(snapshot)
        WidgetRefresher.reloadAll()
    }
}
