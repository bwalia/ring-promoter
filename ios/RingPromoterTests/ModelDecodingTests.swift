import Foundation
import Testing

@testable import RingPromoter

/// Decoding tests run against **real captured responses** from a locally-run
/// Ring Promoter, not hand-written JSON. That is the point: if the Go models
/// change shape, these fail.
@Suite("Model decoding against captured API responses")
struct ModelDecodingTests {

    @Test("Every bundled fixture is present and readable")
    func everyFixtureLoads() throws {
        for name in FixtureCorpus.all {
            let data = try FixtureLoader.data(name)
            #expect(!data.isEmpty, "\(name).json was empty")
        }
    }

    // MARK: - GET /api/apps

    @Test("apps carries the pipeline and both capability flags")
    func appsResponse() throws {
        let apps = try FixtureLoader.decode(AppsResponse.self, from: FixtureLoader.Name.apps)

        #expect(apps.apps == ["web-frontend", "payments-api", "batch-worker"])
        #expect(apps.rings.map(\.name) == ["int", "test", "acc", "prod"])
        #expect(apps.rings.first?.label == "Integration")
        // The capture was made against a server started with RP_PROD_PASSWORD
        // and no AI backend.
        #expect(apps.prodProtected)
        #expect(!apps.aiEnabled)
        #expect(apps.title(for: "payments-api") == "Payments API")
        // An app with no display_name falls back to its name.
        #expect(apps.title(for: "batch-worker") == "batch-worker")
        // An unknown app must not crash or return empty.
        #expect(apps.title(for: "nonexistent") == "nonexistent")
        #expect(apps.locations["web-frontend"]?.city == "London")
        #expect(apps.locations["payments-api"]?.region == "GB")
    }

    // MARK: - GET /api/apps/{app}/rings

    @Test("rings decode with versions, health and promotability")
    func ringsResponse() throws {
        let rings = try FixtureLoader
            .decode(RingsResponse.self, from: FixtureLoader.Name.rings).rings

        #expect(rings.count == 4)
        let int = try #require(rings.first { $0.ring.name == "int" })
        #expect(int.configured)
        #expect(int.currentVersion == "2.7.1")
        #expect(int.previousVersion == "2.7.2")
        #expect(int.healthy)
        #expect(int.liveHealthy)
        #expect(int.canPromoteFrom)
        #expect(int.canRollBack)
        #expect(!int.gates.isGated)
    }

    @Test("a never-deployed ring decodes Go's zero timestamp without failing")
    func zeroTimestamp() throws {
        let rings = try FixtureLoader
            .decode(RingsResponse.self, from: FixtureLoader.Name.ringsGated).rings
        let prod = try #require(rings.first { $0.ring.name == "prod" })

        #expect(prod.isEmpty)
        // "0001-01-01T00:00:00Z" must decode, and must not be shown as a date.
        #expect(!prod.hasBeenDeployedTo)
        #expect(prod.updatedAt < Date(timeIntervalSince1970: 0))
    }

    @Test("gates decode per ring, including the CR provider")
    func gatesDecode() throws {
        let rings = try FixtureLoader
            .decode(RingsResponse.self, from: FixtureLoader.Name.ringsGated).rings

        let int = try #require(rings.first { $0.ring.name == "int" })
        #expect(!int.gates.isGated)

        let acc = try #require(rings.first { $0.ring.name == "acc" })
        #expect(acc.gates.qaSignoff)
        #expect(acc.gates.changeRequest)
        #expect(acc.gates.changeRequestProvider == "test")
        #expect(!acc.gates.maintenanceWindow)

        let prod = try #require(rings.first { $0.ring.name == "prod" })
        #expect(prod.gates.maintenanceWindow)
        #expect(prod.gates.qaSignoff)
        #expect(prod.gates.changeRequest)
    }

    @Test("a config-managed auto-promote ring is flagged as managed")
    func managedAutoPromote() throws {
        let rings = try FixtureLoader
            .decode(RingsResponse.self, from: FixtureLoader.Name.ringsManaged).rings
        let test = try #require(rings.first { $0.ring.name == "test" })

        #expect(test.autoPromote)
        #expect(test.autoPromoteManaged)
    }

    @Test("an unhealthy ring carries its live health error")
    func unhealthyRing() throws {
        let rings = try FixtureLoader
            .decode(RingsResponse.self, from: FixtureLoader.Name.ringsUnhealthy).rings
        let test = try #require(rings.first { $0.ring.name == "test" })

        #expect(!test.liveHealthy)
        #expect(test.liveHealthError == "unhealthy: status 503")
        #expect(test.needsAttention)
    }

    // MARK: - Jobs

    @Test("a failed-and-rolled-back job decodes every step and its result")
    func rolledBackJob() throws {
        let job = try FixtureLoader.decode(Job.self, from: FixtureLoader.Name.jobFailedRolledBack)

        #expect(job.status == .failed)
        #expect(job.isFinished)
        #expect(job.steps.count == 4)
        #expect(job.steps.map(\.id) == ["source-health", "deploy", "health", "rollback"])
        #expect(job.steps[0].status == .success)
        #expect(job.steps[2].status == .failed)
        #expect(job.steps[2].logs.count == 6)

        let result = try #require(job.result)
        #expect(!result.success)
        #expect(result.rolledBack)
        #expect(result.fromRing == "int")
        #expect(result.ring == "test")
        // Rolled back is its own outcome, never merged with plain failure.
        #expect(job.outcome == .failedAndRolledBack)
        #expect(result.outcome == .failedAndRolledBack)
    }

    @Test("a successful job reports success and offers no diagnosis")
    func successfulJob() throws {
        let job = try FixtureLoader.decode(Job.self, from: FixtureLoader.Name.jobSuccess)

        #expect(job.status == .success)
        #expect(job.outcome == .succeeded)
        #expect(job.isFinished)
        #expect(!job.canRequestDiagnosis)
        #expect(job.result?.success == true)
        // `rolled_back` is omitted when false, so it must default rather than
        // fail to decode.
        #expect(job.result?.rolledBack == false)
    }

    @Test("a running job has no result and is not finished")
    func runningJob() throws {
        let job = try FixtureLoader.decode(Job.self, from: FixtureLoader.Name.jobRunning)

        #expect(!job.isFinished)
        #expect(job.outcome == .running)
        #expect(job.finishedAt == nil)
    }

    @Test("the cross-app job feed decodes")
    func jobsFeed() throws {
        let jobs = try FixtureLoader.decode(JobsResponse.self, from: FixtureLoader.Name.jobs).jobs
        #expect(!jobs.isEmpty)
        // The endpoint returns the newest job per app, so app names are unique.
        #expect(Set(jobs.map(\.app)).count == jobs.count)
    }

    // MARK: - History

    @Test("history decodes newest-first with version transitions")
    func history() throws {
        let entries = try FixtureLoader
            .decode(HistoryResponse.self, from: FixtureLoader.Name.history).history

        #expect(!entries.isEmpty)
        let newest = try #require(entries.first)
        #expect(newest.id == 5)
        #expect(newest.action == "rollback")
        #expect(newest.succeeded)
        #expect(newest.versionTransition == "2.7.2 → 2.7.1")
    }

    @Test("a failed history entry can be diagnosed")
    func failedHistory() throws {
        let entries = try FixtureLoader
            .decode(HistoryResponse.self, from: FixtureLoader.Name.historyWithFailures).history
        let failure = try #require(entries.first { !$0.succeeded })

        #expect(failure.canRequestDiagnosis)
        #expect(!failure.message.isEmpty)
    }

    // MARK: - Gates

    @Test("the maintenance view decodes PascalCase recurring windows")
    func maintenanceDecoding() throws {
        let status = try FixtureLoader
            .decode(MaintenanceStatus.self, from: FixtureLoader.Name.maintenanceGated)

        #expect(status.gated)
        #expect(status.gatedRings == ["prod"])
        #expect(status.isOpen(for: "prod"))
        // The Go config struct has no JSON tags, so these keys are "Days",
        // "Start", "End", "Timezone" — not snake_case like the rest of the API.
        let recurring = try #require(status.recurring.first)
        #expect(recurring.days == ["Sat", "Sun"])
        #expect(recurring.start == "02:00")
        #expect(recurring.end == "04:00")
        #expect(recurring.timezone == "Europe/London")
        #expect(recurring.summary == "Sat, Sun 02:00–04:00 Europe/London")

        let window = try #require(status.windows.first)
        #expect(window.ring == "prod")
        #expect(window.reason == "Emergency payment gateway fix")
        #expect(window.createdBy == "a.patel")
    }

    @Test("an ungated app decodes null gated_rings and recurring as empty")
    func ungatedMaintenance() throws {
        let status = try FixtureLoader
            .decode(MaintenanceStatus.self, from: FixtureLoader.Name.maintenanceUngated)

        #expect(!status.gated)
        // Both arrive as JSON null and must not throw.
        #expect(status.gatedRings.isEmpty)
        #expect(status.recurring.isEmpty)
        #expect(status.windows.isEmpty)
        #expect(!status.isOpen(for: "prod"))
    }

    @Test("sign-offs decode and match only the exact version")
    func signoffs() throws {
        let signoffs = try FixtureLoader
            .decode(SignoffsResponse.self, from: FixtureLoader.Name.signoffs).signoffs

        #expect(signoffs.count == 4)
        let go = try #require(signoffs.signoff(ring: "acc", version: "v5.0.0"))
        #expect(go.isGo)
        #expect(go.engineer == "j.rivera")
        #expect(go.qaStatus == "passed")

        let noGo = try #require(signoffs.signoff(ring: "prod", version: "v4.1.0"))
        #expect(!noGo.isGo)
        #expect(noGo.note == "Checkout latency regression")

        // A GO for v5.0.0 must never be read as authorising another version.
        #expect(signoffs.signoff(ring: "acc", version: "v5.0.1") == nil)
    }

    // MARK: - Miscellaneous

    @Test("groups decode with their members")
    func groups() throws {
        let groups = try FixtureLoader
            .decode(GroupsResponse.self, from: FixtureLoader.Name.groups).groups

        #expect(groups.count == 2)
        let core = try #require(groups.first { $0.name == "Core Platform" })
        #expect(core.apps == ["web-frontend", "payments-api"])
        #expect(core.contains("payments-api"))
        #expect(!core.contains("batch-worker"))
    }

    @Test("an unsupported versions response falls back to free-form input")
    func unsupportedVersions() throws {
        let versions = try FixtureLoader
            .decode(VersionsResponse.self, from: FixtureLoader.Name.versionsUnsupported)

        #expect(!versions.supported)
        #expect(versions.versions.isEmpty)
        #expect(!versions.offersPicker)
    }

    @Test("the version endpoint decodes build metadata")
    func serverVersion() throws {
        let version = try FixtureLoader
            .decode(ServerVersion.self, from: FixtureLoader.Name.serverVersion)

        #expect(version.version == "dev")
        #expect(version.commit == "none")
        #expect(version.startedAt != nil)
    }

    @Test("a 422 body decodes as a Result, not as an error")
    func failedResultBody() throws {
        // 422 means "it ran and failed" — a first-class outcome carrying a full
        // Result, which the app must render rather than treat as a transport
        // error.
        let result = try FixtureLoader
            .decode(ActionResult.self, from: FixtureLoader.Name.result422Failed)

        #expect(!result.success)
        #expect(result.message.contains("health check failed"))
        #expect(result.outcome == .failed)
    }

    @Test("timestamps decode with and without fractional seconds")
    func timestampFormats() throws {
        struct Wrapper: Decodable { let at: Date }
        let decoder = JSONCoding.makeDecoder()

        // Both forms occur in a single real response.
        let fractional = try decoder.decode(
            Wrapper.self, from: Data(#"{"at":"2026-07-29T10:38:47.905917Z"}"#.utf8)
        )
        let plain = try decoder.decode(
            Wrapper.self, from: Data(#"{"at":"2026-07-29T10:08:52Z"}"#.utf8)
        )
        #expect(abs(fractional.at.timeIntervalSince(plain.at) - 1_795.9) < 1)

        let zero = try decoder.decode(
            Wrapper.self, from: Data(#"{"at":"0001-01-01T00:00:00Z"}"#.utf8)
        )
        #expect(zero.at == .distantPast)

        #expect(throws: (any Error).self) {
            try decoder.decode(Wrapper.self, from: Data(#"{"at":"not a date"}"#.utf8))
        }
    }
}

/// Every fixture the app ships, so a model change cannot silently orphan one.
enum FixtureCorpus {
    static let all = [
        "apps", "rings", "rings-gated", "rings-managed", "rings-unhealthy",
        "history", "history-with-failures", "versions-unsupported",
        "job-running", "job-success", "job-failed-rolledback", "jobs",
        "groups", "signoffs", "signoff-created",
        "maintenance-gated", "maintenance-gated-closed", "maintenance-ungated",
        "window-created", "seed-result", "promote-result", "rollback-result",
        "result-422-failed", "version", "healthz",
        "error-400-bad-group", "error-400-bad-signoff", "error-400-cr-invalid",
        "error-400-cr-required", "error-400-empty-version", "error-400-no-next-ring",
        "error-401", "error-403-prod-password", "error-403-wrong-password",
        "error-404-app", "error-404-group", "error-404-job", "error-404-window",
        "error-409-nothing-to-promote", "error-409-nothing-to-rollback",
        "error-409-signoff-required", "error-409-window-closed", "error-501-diagnose",
        "autopromote-ok", "autopromote-403-prod", "autopromote-409-managed",
    ]
}
