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
    var isBusy: Bool {
        guard let job = latestJob else { return false }
        return !job.isFinished
    }

    /// Best-effort: which ring the running job is currently working on, so that
    /// ring animates.
    ///
    /// The API does not say. A running job carries no `result`, and therefore no
    /// target ring, until it finishes (see `docs/API-GAPS.md`). The step titles
    /// do name it — "Deploy v9.1.0 to **test**", "Health check **test**" — so
    /// this matches the app's own ring names against the newest step's words.
    ///
    /// Deliberately used for **animation only**. Matching on prose is not a
    /// contract: a wrong guess costs a pulse on the wrong chip, never a wrong
    /// action, and no match simply means nothing animates. Nothing in
    /// `PromotionLegality` is allowed to consult this.
    var busyRing: String? {
        guard let job = latestJob, !job.isFinished else { return nil }
        guard let title = job.steps.last?.title else { return nil }
        // Whole words only, so a ring called "int" is not found inside "point"
        // and "acc" is not found inside "Acceptance".
        let words = Set(
            title.lowercased()
                .split { !$0.isLetter && !$0.isNumber }
                .map(String.init)
        )
        return rings.map(\.ring.name).first { words.contains($0.lowercased()) }
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
/// The app list, each app's rings, the shared job feed and the groups are
/// fetched concurrently — there are as many ring requests as applications, and
/// doing them in series would make a 20-app control plane feel broken.
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
    private var refreshTask: Task<Void, Never>?

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

    /// One section of the Overview.
    struct Section: Identifiable, Hashable, Sendable {
        let id: String
        let title: String
        let apps: [AppSummary]
    }

    /// The healthy applications, organised by their server-side group.
    ///
    /// Groups are what teams actually own, so they are the natural shape for
    /// this list — better than hiding them behind a filter menu that has to be
    /// discovered. An app in more than one group appears in each; an app in none
    /// falls into "Ungrouped", which is omitted entirely when every app is
    /// grouped.
    ///
    /// A broken application appears **both** in the pinned attention section and
    /// in its own group. That is deliberate: the pinned copy is the shortcut an
    /// operator needs at 2am, and the copy in the group is what stops a team
    /// scanning their own section from concluding everything is fine.
    var groupedSections: [Section] {
        let all = orderedSummaries
        guard !groups.isEmpty else {
            return all.isEmpty ? [] : [Section(id: "all", title: "Applications", apps: all)]
        }

        var sections: [Section] = []
        var grouped: Set<String> = []
        for group in groups.sorted(by: { $0.name < $1.name }) {
            let members = all.filter { group.contains($0.name) }
            grouped.formUnion(group.apps)
            guard !members.isEmpty else { continue }
            sections.append(Section(id: group.id, title: group.name, apps: members))
        }
        let ungrouped = all.filter { !grouped.contains($0.name) }
        if !ungrouped.isEmpty {
            sections.append(Section(id: "ungrouped", title: "Ungrouped", apps: ungrouped))
        }
        return sections
    }

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
    /// `onAppear` at once: a second call cancels nothing and simply awaits the
    /// first.
    func refresh() async {
        if let refreshTask {
            await refreshTask.value
            return
        }
        let task = Task { await performRefresh() }
        refreshTask = task
        await task.value
        refreshTask = nil
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
        // The shared job feed and the groups are one request each and are not
        // worth failing the whole refresh for.
        async let jobsResult = try? api.recentJobs()
        async let groupsResult = try? api.groups()

        // One rings request per application, run concurrently: in series, a
        // 20-app control plane would take 20 round trips to draw.
        let results = await withTaskGroup(
            of: (String, Result<[RingStatus], APIError>).self,
            returning: [String: Result<[RingStatus], APIError>].self
        ) { group in
            for app in capabilities.apps {
                group.addTask {
                    // `throws(APIError)` does not survive the task group's
                    // closure type, so the typed error is restored here.
                    do throws(APIError) {
                        return (app, .success(try await api.rings(app: app)))
                    } catch {
                        return (app, .failure(error))
                    }
                }
            }
            var collected: [String: Result<[RingStatus], APIError>] = [:]
            for await (app, result) in group { collected[app] = result }
            return collected
        }

        let jobs = await jobsResult ?? []
        let jobsByApp = Dictionary(
            jobs.map { ($0.app, $0) }, uniquingKeysWith: { first, _ in first }
        )

        // Preserve the server's app ordering; the view re-sorts by trouble.
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
        groups = await groupsResult ?? groups
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
