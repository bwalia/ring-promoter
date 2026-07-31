import Foundation

/// Demo mode: the whole app driven from the bundled fixtures, with no server.
///
/// Used for screenshots, App Store review and offline demos. It is a real
/// simulation rather than a stub — seeding, promoting and rolling back mutate
/// state, write history, and produce a job whose steps advance over time — so
/// the live job view, the gates and the failure paths can all be exercised.
///
/// Rules that matter are simulated faithfully, including the ones that say
/// "no": a shut maintenance window, a missing sign-off and a wrong production
/// password all fail here exactly as the server would fail them.
actor DemoClient: RingPromoterAPI {
    /// The production password demo mode accepts. Shown on the demo banner so
    /// the gated path is reachable during review.
    static let productionPassword = "demo"
    /// The change-request code the backend always accepts, whatever the
    /// provider — the real server does the same.
    static let demoChangeRequestCode = "test"

    private var world: DemoWorld
    private var jobs: [String: DemoJob] = [:]
    private var jobSequence = 0

    init(world: DemoWorld = .load()) {
        self.world = world
        if let ambient = Self.deployInFlight(in: world, id: "job-1") {
            jobs[ambient.id] = ambient
            jobSequence = 1
        }
    }

    /// Start the demo with one promotion already running.
    ///
    /// Without this, nothing on the Overview moves until you press something,
    /// and a real deploy takes minutes while a simulated one takes seconds — so
    /// the in-flight animation would be gone before anyone looked at it. The
    /// job is placed on an application inside a **multi-application group**, so
    /// what you see is one ring working inside a group of several apps, which
    /// is what the pipeline looks like in real life.
    private static func deployInFlight(in world: DemoWorld, id: String) -> DemoJob? {
        // Pick an app that shares a group with at least one other, so the
        // animation is seen in context rather than on a lone row.
        let candidates = world.groups.filter { $0.apps.count > 1 }.flatMap(\.apps)
        guard
            let app = candidates.first(where: {
                world.ring(app: $0, ring: "int")?.isEmpty == false
            }),
            let source = world.ring(app: app, ring: "int"),
            let target = world.pipeline.next(after: "int")
        else { return nil }

        return DemoJob(
            id: id, app: app, action: .promote,
            targetRing: target.name, sourceRing: "int", version: source.currentVersion,
            previousVersion: world.ring(app: app, ring: target.name)?.currentVersion ?? "",
            // Started a moment ago, and paced so it stays visibly in flight for
            // about a minute rather than the few seconds an operator-triggered
            // demo job takes.
            startedAt: Date().addingTimeInterval(-4),
            fails: false,
            stepDuration: 20
        )
    }

    // MARK: - Unauthenticated

    func checkHealth() async throws(APIError) {}

    func serverVersion() async throws(APIError) -> ServerVersion {
        world.serverVersion
    }

    // MARK: - Reads

    func apps() async throws(APIError) -> AppsResponse { world.apps }

    func rings(app: String) async throws(APIError) -> [RingStatus] {
        try requireApp(app)
        return world.rings(for: app)
    }

    func history(app: String) async throws(APIError) -> [HistoryEntry] {
        try requireApp(app)
        return world.history.filter { $0.app == app }.sorted { $0.createdAt > $1.createdAt }
    }

    func versions(app: String) async throws(APIError) -> VersionsResponse {
        try requireApp(app)
        return world.versions[app] ?? .unsupported
    }

    func job(app: String, id: String) async throws(APIError) -> Job {
        guard let demo = jobs[id], demo.app == app else {
            throw .notFound("job not found")
        }
        // Advance on read: the poller sees steps appear as time passes, without
        // any background task to leak.
        let advanced = demo.snapshot(at: Date())
        applyCompletion(of: demo, snapshot: advanced)
        return advanced
    }

    func recentJobs() async throws(APIError) -> [Job] {
        let now = Date()
        var newestPerApp: [String: Job] = [:]
        for demo in jobs.values {
            let snapshot = demo.snapshot(at: now)
            if let existing = newestPerApp[demo.app], existing.startedAt >= snapshot.startedAt {
                continue
            }
            newestPerApp[demo.app] = snapshot
        }
        return newestPerApp.values.sorted { $0.startedAt > $1.startedAt }
    }

    // MARK: - Actions

    func seed(
        app: String, ring: String, version: String, crCode: String?, password: String?
    ) async throws(APIError) -> String {
        try requireApp(app)
        let trimmed = version.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { throw .badRequest("version must not be empty") }
        guard let target = world.ring(app: app, ring: ring), target.configured else {
            throw .notFound("ring not configured for application")
        }
        try checkProductionPassword(app: app, targetRing: ring, password: password)
        try checkGates(app: app, targetRing: ring, version: trimmed, crCode: crCode)
        return startJob(app: app, action: .seed, target: ring, from: nil, version: trimmed)
    }

    func promote(
        app: String, fromRing: String, crCode: String?, password: String?
    ) async throws(APIError) -> String {
        try requireApp(app)
        guard let source = world.ring(app: app, ring: fromRing), source.configured else {
            throw .notFound("ring not configured for application")
        }
        guard let next = world.pipeline.next(after: fromRing) else {
            throw .badRequest("no next ring: already at the last ring")
        }
        guard !source.currentVersion.isEmpty else {
            throw .conflict("source ring has no version to promote")
        }
        guard source.liveHealthy else {
            throw .conflict("source ring \(fromRing) is unhealthy, promotion aborted")
        }
        try checkProductionPassword(app: app, targetRing: next.name, password: password)
        try checkGates(
            app: app, targetRing: next.name, version: source.currentVersion, crCode: crCode
        )
        return startJob(
            app: app, action: .promote, target: next.name, from: fromRing,
            version: source.currentVersion
        )
    }

    func rollback(app: String, ring: String) async throws(APIError) -> String {
        try requireApp(app)
        guard let state = world.ring(app: app, ring: ring), state.configured else {
            throw .notFound("ring not configured for application")
        }
        guard !state.previousVersion.isEmpty else {
            throw .conflict("ring has no previous version to roll back to")
        }
        // Deliberately no password and no gate check: rollback is exempt on the
        // real server too, so incident response is never blocked.
        return startJob(
            app: app, action: .rollback, target: ring, from: nil, version: state.previousVersion
        )
    }

    func setAutoPromote(
        app: String, ring: String, enabled: Bool, password: String?
    ) async throws(APIError) {
        try requireApp(app)
        guard let state = world.ring(app: app, ring: ring) else {
            throw .notFound("ring not configured for application")
        }
        guard !state.autoPromoteManaged else {
            throw .conflict(
                "auto-promote is managed by config: auto-promote for \(app)/\(ring) "
                    + "is declared in config — change it there"
            )
        }
        if enabled, let next = world.pipeline.next(after: ring),
           world.pipeline.isProduction(next.name) {
            try checkPassword(password)
        }
        world.setAutoPromote(app: app, ring: ring, enabled: enabled)
    }

    // MARK: - Gates

    func maintenance(app: String) async throws(APIError) -> MaintenanceStatus {
        try requireApp(app)
        return world.maintenance(for: app, now: Date())
    }

    func openMaintenanceWindow(
        app: String, window: NewMaintenanceWindow
    ) async throws(APIError) -> MaintenanceWindow {
        try requireApp(app)
        guard window.endsAt > window.startsAt else {
            throw .badRequest("invalid maintenance window: end must be after start")
        }
        return world.addWindow(app: app, from: window)
    }

    func closeMaintenanceWindow(app: String, id: String) async throws(APIError) {
        try requireApp(app)
        guard world.removeWindow(app: app, id: id) else {
            throw .notFound("maintenance window not found")
        }
    }

    func signoffs(app: String) async throws(APIError) -> [Signoff] {
        try requireApp(app)
        return world.signoffs.filter { $0.app == app }.sorted { $0.updatedAt > $1.updatedAt }
    }

    func recordSignoff(app: String, signoff: NewSignoff) async throws(APIError) -> Signoff {
        try requireApp(app)
        guard !signoff.version.trimmingCharacters(in: .whitespaces).isEmpty else {
            throw .badRequest("invalid sign-off: version is required")
        }
        guard !signoff.engineer.trimmingCharacters(in: .whitespaces).isEmpty else {
            throw .badRequest("invalid sign-off: the release engineer's name is required")
        }
        return world.upsertSignoff(app: app, from: signoff)
    }

    // MARK: - Groups

    func groups() async throws(APIError) -> [AppGroup] {
        world.groups.sorted { $0.name < $1.name }
    }

    func createGroup(name: String, apps: [String]) async throws(APIError) -> AppGroup {
        let trimmed = name.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty else { throw .badRequest("group name must not be empty") }
        for app in apps where !world.apps.apps.contains(app) {
            throw .badRequest("unknown application: \"\(app)\"")
        }
        return world.createGroup(name: trimmed, apps: apps)
    }

    func updateGroup(id: String, name: String, apps: [String]) async throws(APIError) -> AppGroup {
        guard let updated = world.updateGroup(id: id, name: name, apps: apps) else {
            throw .notFound("group not found")
        }
        return updated
    }

    func deleteGroup(id: String) async throws(APIError) {
        guard world.deleteGroup(id: id) else { throw .notFound("group not found") }
    }

    // MARK: - AI diagnosis

    func diagnoseJob(app: String, id: String) async throws(APIError) -> DiagnosisResponse {
        guard world.apps.aiEnabled else {
            throw .notImplemented("AI diagnosis is not configured on this server")
        }
        guard var demo = jobs[id], demo.app == app else { throw .notFound("job not found") }
        guard demo.snapshot(at: Date()).status == .failed else {
            throw .conflict("only failed jobs can be diagnosed")
        }
        demo.diagnosis = DemoWorld.sampleDiagnosis
        jobs[id] = demo
        return DiagnosisResponse(diagnosisStatus: .running, diagnosis: nil)
    }

    func diagnoseHistoryEntry(app: String, id: Int64) async throws(APIError) -> DiagnosisResponse {
        guard world.apps.aiEnabled else {
            throw .notImplemented("AI diagnosis is not configured on this server")
        }
        guard let entry = world.history.first(where: { $0.id == id && $0.app == app }) else {
            throw .notFound("history entry not found")
        }
        guard !entry.succeeded else { throw .conflict("only failed entries can be diagnosed") }
        world.setHistoryDiagnosis(id: id, text: DemoWorld.sampleDiagnosis)
        return DiagnosisResponse(diagnosisStatus: .done, diagnosis: DemoWorld.sampleDiagnosis)
    }

    func historyDiagnosis(app: String, id: Int64) async throws(APIError) -> DiagnosisResponse {
        guard let entry = world.history.first(where: { $0.id == id && $0.app == app }) else {
            throw .notFound("history entry not found")
        }
        guard let diagnosis = entry.diagnosis, !diagnosis.isEmpty else {
            return DiagnosisResponse(diagnosisStatus: .none, diagnosis: nil)
        }
        return DiagnosisResponse(diagnosisStatus: .done, diagnosis: diagnosis)
    }

    // MARK: - Rule checks (mirroring the server's)

    private func requireApp(_ app: String) throws(APIError) {
        guard world.apps.apps.contains(app) else { throw .notFound("application not found") }
    }

    private func checkProductionPassword(
        app: String, targetRing: String, password: String?
    ) throws(APIError) {
        guard world.apps.prodProtected, world.pipeline.isProduction(targetRing) else { return }
        try checkPassword(password)
    }

    private func checkPassword(_ password: String?) throws(APIError) {
        let supplied = password?.trimmingCharacters(in: .whitespaces) ?? ""
        if supplied.isEmpty {
            throw .productionPasswordRequired("production password required")
        }
        if supplied != Self.productionPassword {
            throw .productionPasswordRequired("incorrect production password")
        }
    }

    private func checkGates(
        app: String, targetRing: String, version: String, crCode: String?
    ) throws(APIError) {
        let gates = world.ring(app: app, ring: targetRing)?.gates ?? .none
        if gates.changeRequest {
            let code = crCode?.trimmingCharacters(in: .whitespaces) ?? ""
            guard !code.isEmpty else {
                throw .badRequest(
                    "change-request code required: promotion to \(targetRing) requires a valid change-request code"
                )
            }
            guard code == Self.demoChangeRequestCode else {
                throw .badRequest(
                    "change-request code invalid: invalid change request code\n"
                        + "no change-request system configured; "
                        + "only the demo code \"test\" is accepted"
                )
            }
        }
        if gates.maintenanceWindow,
           !world.maintenance(for: app, now: Date()).isOpen(for: targetRing) {
            throw .conflict(
                "maintenance window closed: no active maintenance window for \(targetRing) "
                    + "(open one, or wait for a scheduled window)"
            )
        }
        if gates.qaSignoff {
            let signoff = world.signoffs
                .filter { $0.app == app }
                .signoff(ring: targetRing, version: version)
            guard let signoff else {
                throw .conflict(
                    "qa/release sign-off required: \(version) needs a release-engineer "
                        + "sign-off for \(targetRing) before it can be promoted"
                )
            }
            guard signoff.isGo else {
                throw .conflict(
                    "qa/release sign-off is NO-GO: \(version) was signed off NO-GO for \(targetRing)"
                )
            }
        }
    }

    // MARK: - Job simulation

    private func startJob(
        app: String, action: PromotionAction, target: String, from: String?, version: String
    ) -> String {
        jobSequence += 1
        let id = "job-\(jobSequence)"
        // Demo mode deliberately fails one specific case so the failure and
        // rollback paths are reachable without breaking a real cluster: any
        // version containing "bad".
        let shouldFail = version.localizedCaseInsensitiveContains("bad")
        let demo = DemoJob(
            id: id, app: app, action: action, targetRing: target, sourceRing: from,
            version: version,
            previousVersion: world.ring(app: app, ring: target)?.currentVersion ?? "",
            startedAt: Date(), fails: shouldFail
        )
        jobs[id] = demo
        return id
    }

    /// When a simulated job reaches its terminal state, commit its effect to
    /// the world exactly once so rings and history reflect it afterwards.
    private func applyCompletion(of demo: DemoJob, snapshot: Job) {
        guard snapshot.isFinished, !demo.applied, let result = snapshot.result else { return }
        var updated = demo
        updated.applied = true
        jobs[demo.id] = updated
        world.apply(result: result, at: snapshot.finishedAt ?? Date())
    }
}
