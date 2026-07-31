import Foundation
import SwiftUI

/// Everything one application's detail screen needs, and the actions it can
/// start.
///
/// The rules live in `PromotionRules`; this store only asks. That is what keeps
/// "what may I offer?" in one tested place instead of scattered through views.
@MainActor
@Observable
final class AppDetailStore {
    let app: String

    private(set) var title: String
    private(set) var rings: [RingStatus] = []
    private(set) var versions: VersionsResponse = .unsupported
    private(set) var maintenance: MaintenanceStatus = .ungated
    private(set) var signoffs: [Signoff] = []
    private(set) var isLoading = false
    private(set) var lastUpdated: Date?
    private(set) var error: APIError?
    /// The job started by the most recent action, so the screen can offer
    /// "watch it" after a sheet closes.
    private(set) var runningJobID: String?

    private let session: AppSession
    private var refreshTask: Task<Void, Never>?

    init(app: String, session: AppSession) {
        self.app = app
        self.session = session
        self.title = session.capabilities?.title(for: app) ?? app
    }

    var rules: PromotionRules { session.rules }
    var pipeline: RingPipeline { session.pipeline }
    var prodProtected: Bool { session.prodProtected }

    /// Rings the app actually declares. An unconfigured ring is still shown,
    /// but greyed out, so the pipeline's shape is never misleading.
    var configuredRings: [RingStatus] { rings.filter(\.configured) }

    var unhealthyRings: [RingStatus] { rings.filter(\.needsAttention) }

    func ring(named name: String) -> RingStatus? {
        rings.first { $0.ring.name == name }
    }

    /// The sign-off recorded for one exact (ring, version), which is the only
    /// match the QA gate accepts.
    func signoff(ring: String, version: String) -> Signoff? {
        signoffs.signoff(ring: ring, version: version)
    }

    // MARK: - Loading

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
        let api = session.api

        // Rings are the only request worth failing the screen for. The rest
        // enrich the action sheets and degrade quietly.
        async let versionsResult = try? api.versions(app: app)
        async let maintenanceResult = try? api.maintenance(app: app)
        async let signoffsResult = try? api.signoffs(app: app)

        do {
            rings = try await api.rings(app: app)
            error = nil
            lastUpdated = .now
        } catch {
            self.error = error
            session.note(error)
        }

        versions = await versionsResult ?? versions
        maintenance = await maintenanceResult ?? maintenance
        signoffs = await signoffsResult ?? signoffs
        if let capabilities = session.capabilities {
            title = capabilities.title(for: app)
        }
    }

    /// Reload only the parts an action sheet depends on, without the flicker of
    /// a full refresh.
    func refreshGates() async {
        let api = session.api
        async let maintenanceResult = try? api.maintenance(app: app)
        async let signoffsResult = try? api.signoffs(app: app)
        maintenance = await maintenanceResult ?? maintenance
        signoffs = await signoffsResult ?? signoffs
        // Ring gates carry `maintenance_window_open`, so they must be refreshed
        // too or the badge and the sheet would disagree.
        if let refreshed = try? await api.rings(app: app) { rings = refreshed }
    }

    // MARK: - Actions

    /// Every action returns the id of the job the server started, which the
    /// caller pushes as a live view.
    func promote(from ring: String, crCode: String?, password: String?) async throws(APIError) -> String {
        let id = try await session.api.promote(
            app: app, fromRing: ring, crCode: crCode, password: password
        )
        runningJobID = id
        return id
    }

    func seed(
        ring: String, version: String, crCode: String?, password: String?
    ) async throws(APIError) -> String {
        let id = try await session.api.seed(
            app: app, ring: ring, version: version, crCode: crCode, password: password
        )
        runningJobID = id
        return id
    }

    func rollback(ring: String) async throws(APIError) -> String {
        let id = try await session.api.rollback(app: app, ring: ring)
        runningJobID = id
        return id
    }

    func setAutoPromote(
        ring: String, enabled: Bool, password: String?
    ) async throws(APIError) {
        try await session.api.setAutoPromote(
            app: app, ring: ring, enabled: enabled, password: password
        )
        // Re-read rather than assume: config may own the switch, and the server
        // is the only authority on what it now is.
        if let refreshed = try? await session.api.rings(app: app) { rings = refreshed }
    }

    func openMaintenanceWindow(
        ring: String, from start: Date, to end: Date, reason: String, createdBy: String
    ) async throws(APIError) {
        _ = try await session.api.openMaintenanceWindow(
            app: app,
            window: NewMaintenanceWindow(
                ring: ring, startsAt: start, endsAt: end, reason: reason, createdBy: createdBy
            )
        )
        await refreshGates()
    }

    func closeMaintenanceWindow(id: String) async throws(APIError) {
        try await session.api.closeMaintenanceWindow(app: app, id: id)
        await refreshGates()
    }

    func recordSignoff(
        ring: String, version: String, decision: Signoff.Decision, engineer: String,
        qaStatus: String, note: String
    ) async throws(APIError) {
        _ = try await session.api.recordSignoff(
            app: app,
            signoff: NewSignoff(
                ring: ring, version: version, decision: decision, engineer: engineer,
                qaStatus: qaStatus, note: note
            )
        )
        await refreshGates()
    }
}
