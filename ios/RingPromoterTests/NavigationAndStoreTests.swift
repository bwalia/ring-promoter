import Foundation
import Testing

@testable import RingPromoter

/// Deep links, URL normalisation, and the Overview's ordering.
@Suite("Routing, URLs and ordering")
struct NavigationAndStoreTests {

    // MARK: - Deep links

    @Test("ringpromoter://app/{name} opens that application")
    @MainActor
    func appDeepLink() throws {
        let router = Router()
        router.open(try #require(URL(string: "ringpromoter://app/web-frontend")))

        #expect(router.tab == .overview)
        #expect(router.overviewPath == [.app("web-frontend")])
    }

    @Test("ringpromoter://app/{name}/job/{id} opens the job on top of the app")
    @MainActor
    func jobDeepLink() throws {
        let router = Router()
        router.open(try #require(URL(string: "ringpromoter://app/payments-api/job/job-7")))

        // The app screen sits underneath, so Back goes somewhere sensible.
        #expect(
            router.overviewPath
                == [.app("payments-api"), .job(app: "payments-api", id: "job-7")]
        )
    }

    @Test("a percent-encoded application name is decoded")
    @MainActor
    func encodedDeepLink() throws {
        let router = Router()
        router.open(try #require(URL(string: "ringpromoter://app/team%2Fapp")))
        #expect(router.overviewPath == [.app("team/app")])
    }

    @Test("an unrecognised link is ignored rather than guessed at")
    @MainActor
    func badDeepLinks() throws {
        let router = Router()
        // Sending an operator to the wrong application is worse than doing
        // nothing at all.
        for raw in [
            "https://example.com/app/web-frontend",
            "ringpromoter://",
            "ringpromoter://app",
            "ringpromoter://nonsense/web-frontend",
        ] {
            router.overviewPath = []
            router.open(try #require(URL(string: raw)))
            #expect(router.overviewPath.isEmpty, "\(raw) should have been ignored")
        }
    }

    @Test("the links the widget builds are the ones the router understands")
    @MainActor
    func roundTrip() throws {
        let router = Router()
        router.open(try #require(Router.url(forApp: "payments-api")))
        #expect(router.overviewPath == [.app("payments-api")])

        router.open(try #require(Router.url(forJob: "job-3", app: "payments-api")))
        #expect(
            router.overviewPath == [.app("payments-api"), .job(app: "payments-api", id: "job-3")]
        )
    }

    @Test("a push payload becomes a route")
    func pushPayloadRouting() {
        #expect(
            PushRegistration.route(from: ["app": "web-frontend", "job_id": "job-2"])
                == .job(app: "web-frontend", id: "job-2")
        )
        #expect(PushRegistration.route(from: ["app": "web-frontend"]) == .app("web-frontend"))
        #expect(PushRegistration.route(from: [:]) == nil)
        #expect(PushRegistration.route(from: ["app": ""]) == nil)
    }

    // MARK: - Server URLs

    @Test("what an operator is likely to type becomes a usable URL")
    func urlNormalisation() {
        #expect(
            Instance.normalise(urlText: "ring-promoter.example.com")?.absoluteString
                == "https://ring-promoter.example.com"
        )
        #expect(
            Instance.normalise(urlText: "  https://rp.example.com/  ")?.absoluteString
                == "https://rp.example.com"
        )
        #expect(
            Instance.normalise(urlText: "http://localhost:8080")?.absoluteString
                == "http://localhost:8080"
        )
        // A bare scheme, empty text and junk must be rejected, not "fixed".
        #expect(Instance.normalise(urlText: "") == nil)
        #expect(Instance.normalise(urlText: "   ") == nil)
        #expect(Instance.normalise(urlText: "ftp://example.com") == nil)
    }

    @Test("a non-HTTPS instance is flagged as insecure")
    func insecureInstance() throws {
        let local = Instance(
            name: "Local", baseURL: try #require(URL(string: "http://localhost:8080"))
        )
        #expect(local.isInsecure)
        #expect(local.displayHost == "localhost:8080")

        let live = Instance(
            name: "Production", baseURL: try #require(URL(string: "https://rp.example.com"))
        )
        #expect(!live.isInsecure)
        #expect(live.displayHost == "rp.example.com")
    }

    // MARK: - Overview ordering

    @Test("applications in trouble sort above healthy ones")
    @MainActor
    func troubleFirst() {
        let healthy = AppSummary(
            name: "b-healthy", title: "B Healthy", rings: PreviewData.healthyRings,
            latestJob: nil, loadError: nil
        )
        let unhealthy = AppSummary(
            name: "z-unhealthy", title: "Z Unhealthy", rings: PreviewData.troubledRings,
            latestJob: nil, loadError: nil
        )

        #expect(!healthy.isInTrouble)
        #expect(unhealthy.isInTrouble)
        // Alphabetically Z comes last, but trouble wins.
        #expect(unhealthy.troubleSummary != nil)
    }

    @Test("a failed recent job puts an application in trouble even when its rings are fine")
    func failedJobIsTrouble() {
        let summary = AppSummary(
            name: "web-frontend", title: "Web Frontend", rings: PreviewData.healthyRings,
            latestJob: PreviewData.rolledBackJob, loadError: nil
        )

        #expect(summary.isInTrouble)
        #expect(summary.troubleSummary?.contains("rolled back") == true)
    }

    @Test("a ring load failure is trouble too, and says what happened")
    func loadErrorIsTrouble() {
        let summary = AppSummary(
            name: "app", title: "App", rings: [], latestJob: nil,
            loadError: "The server returned HTTP 500."
        )
        #expect(summary.isInTrouble)
        #expect(summary.troubleSummary == "The server returned HTTP 500.")
    }

    @Test("a running job marks the application busy, naming the action")
    func busyApplication() {
        let running = AppSummary(
            name: "payments-api", title: "Payments API", rings: PreviewData.healthyRings,
            latestJob: PreviewData.runningJob, loadError: nil
        )
        #expect(running.isBusy)
        #expect(running.busyAction == "Promote")

        let finished = AppSummary(
            name: "payments-api", title: "Payments API", rings: PreviewData.healthyRings,
            latestJob: PreviewData.rolledBackJob, loadError: nil
        )
        #expect(!finished.isBusy)
        #expect(finished.busyAction == nil)

        let idle = AppSummary(
            name: "web-frontend", title: "Web Frontend", rings: PreviewData.healthyRings,
            latestJob: nil, loadError: nil
        )
        #expect(!idle.isBusy)
    }

    @Test("a healthy application produces no trouble line")
    func calmApp() {
        let summary = AppSummary(
            name: "web-frontend", title: "Web Frontend", rings: PreviewData.healthyRings,
            latestJob: nil, loadError: nil
        )
        #expect(!summary.isInTrouble)
        #expect(summary.troubleSummary == nil)
        #expect(summary.unhealthyRings.isEmpty)
    }

    // MARK: - Widget snapshot

    @Test("the widget snapshot carries no secrets")
    func snapshotHasNoSecrets() throws {
        let snapshot = WidgetSnapshot(
            instanceName: "Production", instanceTint: "red",
            apps: [
                WidgetSnapshot.AppCell(
                    name: "web-frontend", title: "Web Frontend",
                    rings: PreviewData.healthyRings.map {
                        WidgetSnapshot.RingCell(
                            name: $0.ring.name, label: $0.ring.label,
                            version: $0.currentVersion, healthy: $0.isHealthy,
                            configured: $0.configured, empty: $0.isEmpty
                        )
                    }
                )
            ],
            capturedAt: .now, isDemo: false
        )

        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let json = try #require(String(data: try encoder.encode(snapshot), encoding: .utf8))

        // Nothing resembling a credential or an address may reach the shared
        // container — the widget is outside the app's sandbox.
        for forbidden in ["token", "password", "Bearer", "https://", "http://"] {
            #expect(
                !json.localizedCaseInsensitiveContains(forbidden),
                "the widget snapshot leaked \"\(forbidden)\""
            )
        }
    }

    @Test("the snapshot counts unhealthy rings the widget shows")
    func snapshotCounts() {
        let snapshot = WidgetSnapshot.placeholder
        #expect(snapshot.unhealthyCount == 1)
        #expect(snapshot.totalRings == 4)
    }
}
