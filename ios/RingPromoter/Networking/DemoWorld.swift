import Foundation

/// The mutable state behind demo mode.
///
/// Seeded from the bundled fixtures — real captured API responses — with
/// timestamps rebased onto "now" so relative times read sensibly during a demo
/// rather than showing a date from whenever the fixtures were recorded.
struct DemoWorld: Sendable {
    var apps: AppsResponse
    var ringsByApp: [String: [RingStatus]]
    var history: [HistoryEntry]
    var groups: [AppGroup]
    var signoffs: [Signoff]
    var windowsByApp: [String: [MaintenanceWindow]]
    var recurringByApp: [String: [RecurringWindow]]
    var gatedRingsByApp: [String: [String]]
    var versions: [String: VersionsResponse]
    var serverVersion: ServerVersion

    private var nextHistoryID: Int64
    private var groupSequence = 0
    private var windowSequence = 0

    var pipeline: RingPipeline { apps.pipeline }

    // MARK: - Lookups

    func rings(for app: String) -> [RingStatus] { ringsByApp[app] ?? [] }

    func ring(app: String, ring: String) -> RingStatus? {
        ringsByApp[app]?.first { $0.ring.name == ring }
    }

    func maintenance(for app: String, now: Date) -> MaintenanceStatus {
        let gatedRings = gatedRingsByApp[app] ?? []
        let windows = windowsByApp[app] ?? []
        let recurring = recurringByApp[app] ?? []
        var open: [String: Bool] = [:]
        for ring in gatedRings {
            open[ring] = windows.contains { $0.covers(ring: ring) && $0.isActive(at: now) }
        }
        return MaintenanceStatus(
            gated: !gatedRings.isEmpty,
            gatedRings: gatedRings,
            recurring: recurring,
            windows: windows.sorted { $0.createdAt > $1.createdAt },
            openRings: open
        )
    }

    // MARK: - Mutations

    mutating func setAutoPromote(app: String, ring: String, enabled: Bool) {
        updateRing(app: app, ring: ring) { $0.settingAutoPromote(enabled) }
    }

    /// Commit a finished operation: move versions, record health, write history.
    mutating func apply(result: ActionResult, at instant: Date) {
        updateRing(app: result.app, ring: result.ring) { current in
            current.applying(result: result, at: instant)
        }
        appendHistory(for: result, at: instant)
    }

    mutating func addWindow(app: String, from new: NewMaintenanceWindow) -> MaintenanceWindow {
        windowSequence += 1
        let window = MaintenanceWindow(
            id: "mw-demo\(windowSequence)", app: app, ring: new.ring,
            startsAt: new.startsAt, endsAt: new.endsAt,
            reason: new.reason, createdBy: new.createdBy, createdAt: Date()
        )
        windowsByApp[app, default: []].insert(window, at: 0)
        return window
    }

    mutating func removeWindow(app: String, id: String) -> Bool {
        guard let index = windowsByApp[app]?.firstIndex(where: { $0.id == id }) else {
            return false
        }
        windowsByApp[app]?.remove(at: index)
        return true
    }

    mutating func upsertSignoff(app: String, from new: NewSignoff) -> Signoff {
        let signoff = Signoff(
            app: app, ring: new.ring, version: new.version, decision: new.decision,
            engineer: new.engineer, qaStatus: new.qaStatus,
            note: new.note.isEmpty ? nil : new.note, updatedAt: Date()
        )
        signoffs.removeAll {
            $0.app == app && $0.ring == new.ring && $0.version == new.version
        }
        signoffs.insert(signoff, at: 0)
        return signoff
    }

    mutating func createGroup(name: String, apps members: [String]) -> AppGroup {
        groupSequence += 1
        let group = AppGroup(
            id: "g-demo\(groupSequence)", name: name, apps: members, updatedAt: Date()
        )
        groups.append(group)
        return group
    }

    mutating func updateGroup(id: String, name: String, apps members: [String]) -> AppGroup? {
        guard let index = groups.firstIndex(where: { $0.id == id }) else { return nil }
        let updated = AppGroup(id: id, name: name, apps: members, updatedAt: Date())
        groups[index] = updated
        return updated
    }

    mutating func deleteGroup(id: String) -> Bool {
        let before = groups.count
        groups.removeAll { $0.id == id }
        return groups.count != before
    }

    mutating func setHistoryDiagnosis(id: Int64, text: String) {
        guard let index = history.firstIndex(where: { $0.id == id }) else { return }
        let entry = history[index]
        history[index] = HistoryEntry(
            id: entry.id, app: entry.app, ring: entry.ring, action: entry.action,
            fromVersion: entry.fromVersion, toVersion: entry.toVersion, result: entry.result,
            message: entry.message, diagnosis: text, createdAt: entry.createdAt
        )
    }

    private mutating func updateRing(
        app: String, ring: String, _ transform: (RingStatus) -> RingStatus
    ) {
        guard var rings = ringsByApp[app],
              let index = rings.firstIndex(where: { $0.ring.name == ring })
        else { return }
        rings[index] = transform(rings[index])
        ringsByApp[app] = rings
        recomputePromotability(app: app)
    }

    /// `canPromoteFrom` is derived server-side; keep the demo consistent with
    /// the same rule so the UI's enablement stays honest.
    private mutating func recomputePromotability(app: String) {
        guard var rings = ringsByApp[app] else { return }
        for index in rings.indices {
            let hasNext = pipeline.next(after: rings[index].ring.name) != nil
            rings[index] = rings[index].settingCanPromoteFrom(
                rings[index].configured && hasNext && !rings[index].currentVersion.isEmpty
                    && rings[index].liveHealthy
            )
        }
        ringsByApp[app] = rings
    }

    private mutating func appendHistory(for result: ActionResult, at instant: Date) {
        let entry = HistoryEntry(
            id: nextHistoryID, app: result.app, ring: result.ring, action: result.action,
            fromVersion: result.state.previousVersion, toVersion: result.version,
            result: result.success ? "success" : "failure",
            message: result.message, diagnosis: nil, createdAt: instant
        )
        nextHistoryID += 1
        history.insert(entry, at: 0)
    }

    static let sampleDiagnosis = """
        **What happened**

        The deploy to `test` completed, but every health check that followed came \
        back `503`. After three attempts Ring Promoter rolled the ring back to \
        `v9.0.0` — and that version answered `503` as well, so the ring is still \
        unhealthy.

        **Most likely cause**

        Because both the new and the previous version fail identically, this is \
        very unlikely to be the release itself. The pattern points at something \
        the ring depends on: a database or upstream service that is refusing \
        connections for every pod in that namespace.

        **What to check first**

        1. `kubectl -n test get pods` — are the pods running, or crash-looping?
        2. The dependency the readiness probe touches. A `503` on both versions \
           usually means the probe cannot reach it.
        3. Whether anything else changed in `test` around the same time — config, \
           secrets, or a network policy.

        **Note**

        This diagnosis is generated text shown for the demo. On a real server it \
        comes from the configured model, using the job's own step logs.
        """
}

// MARK: - Loading from fixtures

extension DemoWorld {
    /// Build the demo world from the bundled fixtures, falling back to a small
    /// built-in world if a fixture is missing — demo mode must never open to a
    /// blank screen.
    static func load(now: Date = Date()) -> DemoWorld {
        (try? loadThrowing(now: now)) ?? .fallback(now: now)
    }

    static func loadThrowing(now: Date = Date()) throws -> DemoWorld {
        var apps = try FixtureLoader.decode(AppsResponse.self, from: FixtureLoader.Name.apps)
        // The fixtures were captured from a server without an AI backend; demo
        // mode turns the feature on so "Diagnose with AI" is demonstrable.
        apps = AppsResponse(
            apps: apps.apps, titles: apps.titles, rings: apps.rings,
            prodProtected: apps.prodProtected, aiEnabled: true
        )

        let webRings = try FixtureLoader
            .decode(RingsResponse.self, from: FixtureLoader.Name.rings).rings
        let gatedRings = try FixtureLoader
            .decode(RingsResponse.self, from: FixtureLoader.Name.ringsGated).rings
        let managedRings = try FixtureLoader
            .decode(RingsResponse.self, from: FixtureLoader.Name.ringsManaged).rings
        let unhealthyRings = try FixtureLoader
            .decode(RingsResponse.self, from: FixtureLoader.Name.ringsUnhealthy).rings

        // payments-api gets the unhealthy `test` ring from the failure capture
        // so the Overview has something genuinely wrong to pin to the top.
        let paymentsRings = gatedRings.map { ring -> RingStatus in
            guard let unhealthy = unhealthyRings.first(where: { $0.ring.name == ring.ring.name }),
                  ring.ring.name == "test"
            else { return ring.rebased(to: now) }
            return unhealthy.withGates(ring.gates).rebased(to: now)
        }

        let history = try (
            FixtureLoader.decode(HistoryResponse.self, from: FixtureLoader.Name.history).history
                + FixtureLoader.decode(
                    HistoryResponse.self, from: FixtureLoader.Name.historyWithFailures
                ).history
        )
        .enumerated()
        .map { index, entry in entry.rebased(to: now.addingTimeInterval(-Double(index) * 2_400)) }

        let groups = try FixtureLoader
            .decode(GroupsResponse.self, from: FixtureLoader.Name.groups).groups
            .map { $0.rebased(to: now) }
        let signoffs = try FixtureLoader
            .decode(SignoffsResponse.self, from: FixtureLoader.Name.signoffs).signoffs
            .map { $0.rebased(to: now) }
        let gatedMaintenance = try FixtureLoader
            .decode(MaintenanceStatus.self, from: FixtureLoader.Name.maintenanceGated)

        return DemoWorld(
            apps: apps,
            ringsByApp: [
                "web-frontend": webRings.map { $0.rebased(to: now) },
                "payments-api": paymentsRings,
                "batch-worker": managedRings.map { $0.rebased(to: now) },
            ],
            history: history,
            groups: groups,
            signoffs: signoffs,
            // Rebase the captured ad-hoc window so it is open *now*, which is
            // what makes the gated promote path walkable in a demo.
            windowsByApp: [
                "payments-api": gatedMaintenance.windows.map {
                    $0.rebased(startingAt: now.addingTimeInterval(-1_800), duration: 7_200)
                }
            ],
            recurringByApp: ["payments-api": gatedMaintenance.recurring],
            gatedRingsByApp: ["payments-api": gatedMaintenance.gatedRings],
            versions: ["web-frontend": .demoVersions],
            serverVersion: ServerVersion(
                version: "demo", commit: "demo0000", builtAt: "—", startedAt: now.addingTimeInterval(-86_400)
            ),
            nextHistoryID: (history.map(\.id).max() ?? 0) + 1
        )
    }

    /// A minimal world used only if the bundled fixtures cannot be read.
    static func fallback(now: Date) -> DemoWorld {
        let rings = [
            Ring(name: "int", label: "Integration"),
            Ring(name: "test", label: "Test"),
            Ring(name: "acc", label: "Acceptance"),
            Ring(name: "prod", label: "Production"),
        ]
        let apps = AppsResponse(
            apps: ["web-frontend"], titles: ["web-frontend": "Web Frontend"], rings: rings,
            prodProtected: true, aiEnabled: true
        )
        let states = rings.enumerated().map { index, ring in
            RingStatus(
                ring: ring, configured: true,
                currentVersion: index < 2 ? "2.7.1" : "",
                previousVersion: "", liveVersion: index < 2 ? "2.7.1" : "",
                healthy: index < 2, liveHealthy: index < 2, liveHealthError: nil,
                autoPromote: false, autoPromoteManaged: false,
                updatedAt: index < 2 ? now : .distantPast,
                canPromoteFrom: index < 2, gates: .none
            )
        }
        return DemoWorld(
            apps: apps, ringsByApp: ["web-frontend": states], history: [], groups: [],
            signoffs: [], windowsByApp: [:], recurringByApp: [:], gatedRingsByApp: [:],
            versions: ["web-frontend": .demoVersions],
            serverVersion: ServerVersion(
                version: "demo", commit: "demo0000", builtAt: "—", startedAt: now
            ),
            nextHistoryID: 1
        )
    }
}

// MARK: - Small model conveniences used by the simulation
//
// These types get their memberwise initialisers synthesised, so only the
// derived helpers the simulation needs are declared here.

extension RingStatus {
    private func copy(
        currentVersion: String? = nil, previousVersion: String? = nil, liveVersion: String? = nil,
        healthy: Bool? = nil, liveHealthy: Bool? = nil, liveHealthError: String?? = nil,
        autoPromote: Bool? = nil, updatedAt: Date? = nil, canPromoteFrom: Bool? = nil,
        gates: RingGates? = nil
    ) -> RingStatus {
        RingStatus(
            ring: ring, configured: configured,
            currentVersion: currentVersion ?? self.currentVersion,
            previousVersion: previousVersion ?? self.previousVersion,
            liveVersion: liveVersion ?? self.liveVersion,
            healthy: healthy ?? self.healthy,
            liveHealthy: liveHealthy ?? self.liveHealthy,
            liveHealthError: liveHealthError ?? self.liveHealthError,
            latencyMs: latencyMs,
            ttfbMs: ttfbMs,
            autoPromote: autoPromote ?? self.autoPromote,
            autoPromoteManaged: autoPromoteManaged,
            updatedAt: updatedAt ?? self.updatedAt,
            canPromoteFrom: canPromoteFrom ?? self.canPromoteFrom,
            gates: gates ?? self.gates
        )
    }

    func settingAutoPromote(_ enabled: Bool) -> RingStatus { copy(autoPromote: enabled) }
    func settingCanPromoteFrom(_ value: Bool) -> RingStatus { copy(canPromoteFrom: value) }
    func withGates(_ gates: RingGates) -> RingStatus { copy(gates: gates) }

    /// Move a never-deployed ring's zero timestamp aside and pull real ones
    /// close to `now`, so demo mode shows "12 minutes ago", not a stale date.
    func rebased(to now: Date) -> RingStatus {
        guard hasBeenDeployedTo else { return self }
        return copy(updatedAt: now.addingTimeInterval(-Double.random(in: 300...7_200)))
    }

    /// Apply a finished operation's effect to this ring.
    func applying(result: ActionResult, at instant: Date) -> RingStatus {
        if result.success {
            return copy(
                currentVersion: result.version,
                previousVersion: currentVersion,
                liveVersion: result.version,
                healthy: true, liveHealthy: true, liveHealthError: .some(nil),
                updatedAt: instant
            )
        }
        if result.rolledBack {
            // The target was returned to what it was running before.
            return copy(
                liveVersion: currentVersion,
                healthy: false, liveHealthy: false,
                liveHealthError: .some("unhealthy: status 503"),
                updatedAt: instant
            )
        }
        return copy(
            healthy: false, liveHealthy: false,
            liveHealthError: .some("unhealthy: status 503"), updatedAt: instant
        )
    }
}

extension HistoryEntry {
    func rebased(to instant: Date) -> HistoryEntry {
        HistoryEntry(
            id: id, app: app, ring: ring, action: action, fromVersion: fromVersion,
            toVersion: toVersion, result: result, message: message, diagnosis: diagnosis,
            createdAt: instant
        )
    }
}

extension AppGroup {
    func rebased(to instant: Date) -> AppGroup {
        AppGroup(id: id, name: name, apps: apps, updatedAt: instant)
    }
}

extension Signoff {
    func rebased(to instant: Date) -> Signoff {
        Signoff(
            app: app, ring: ring, version: version, decision: decision, engineer: engineer,
            qaStatus: qaStatus, note: note, updatedAt: instant
        )
    }
}

extension MaintenanceWindow {
    func rebased(startingAt start: Date, duration: TimeInterval) -> MaintenanceWindow {
        MaintenanceWindow(
            id: id, app: app, ring: ring, startsAt: start,
            endsAt: start.addingTimeInterval(duration), reason: reason,
            createdBy: createdBy, createdAt: start
        )
    }
}

extension VersionsResponse {
    /// A version list for demo mode, so the Seed sheet's picker path is
    /// reachable. Real servers only supply this for github-deployed apps.
    static let demoVersions = VersionsResponse(
        supported: true,
        versions: [
            AppVersion(name: "main", type: "branch"),
            AppVersion(name: "release/2.8", type: "branch"),
            AppVersion(name: "hotfix/checkout-timeout", type: "branch"),
            AppVersion(name: "2.8.0", type: "tag"),
            AppVersion(name: "2.7.2", type: "tag"),
            AppVersion(name: "2.7.1", type: "tag"),
        ]
    )
}

