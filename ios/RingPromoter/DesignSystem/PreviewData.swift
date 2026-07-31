import Foundation

/// Canned values for SwiftUI previews.
///
/// Everything here is shaped like a real API response — several values are
/// lifted straight from the captured fixtures — so a preview that looks right
/// is evidence the real screen will too.
enum PreviewData {
    static let rings = [
        Ring(name: "int", label: "Integration"),
        Ring(name: "test", label: "Test"),
        Ring(name: "acc", label: "Acceptance"),
        Ring(name: "prod", label: "Production"),
    ]

    static let pipeline = RingPipeline(rings)

    static let fullyGated = RingGates(
        maintenanceWindow: true, qaSignoff: true, changeRequest: true,
        changeRequestProvider: "jira", maintenanceWindowOpen: false
    )

    static let openWindowGates = RingGates(
        maintenanceWindow: true, qaSignoff: true, changeRequest: true,
        changeRequestProvider: "test", maintenanceWindowOpen: true
    )

    static func ring(
        _ name: String, label: String, version: String = "", previous: String = "",
        live: String? = nil, healthy: Bool = true, configured: Bool = true,
        autoPromote: Bool = false, managed: Bool = false, gates: RingGates = .none,
        healthError: String? = nil, updatedAt: Date = .now.addingTimeInterval(-900)
    ) -> RingStatus {
        RingStatus(
            ring: Ring(name: name, label: label),
            configured: configured,
            currentVersion: version,
            previousVersion: previous,
            liveVersion: live ?? version,
            healthy: healthy,
            liveHealthy: healthy,
            liveHealthError: healthError,
            autoPromote: autoPromote,
            autoPromoteManaged: managed,
            updatedAt: version.isEmpty ? .distantPast : updatedAt,
            canPromoteFrom: configured && !version.isEmpty && healthy && name != "prod",
            gates: gates
        )
    }

    static let healthyRings: [RingStatus] = [
        ring("int", label: "Integration", version: "2.7.2", previous: "2.7.1"),
        ring("test", label: "Test", version: "2.7.1"),
        ring("acc", label: "Acceptance", version: "2.7.0", previous: "2.6.9"),
        ring("prod", label: "Production", version: "2.6.9", previous: "2.6.8"),
    ]

    static let troubledRings: [RingStatus] = [
        ring("int", label: "Integration", version: "v9.1.0"),
        ring(
            "test", label: "Test", version: "v9.0.0", previous: "v9.1.0",
            healthy: false, healthError: "unhealthy: status 503"
        ),
        ring("acc", label: "Acceptance", gates: openWindowGates),
        ring("prod", label: "Production", configured: true, gates: fullyGated),
    ]

    static let managedRings: [RingStatus] = [
        ring("int", label: "Integration", version: "1.0.9"),
        ring("test", label: "Test", version: "1.0.9", autoPromote: true, managed: true),
        ring("acc", label: "Acceptance"),
        ring("prod", label: "Production", configured: false),
    ]

    static let appsResponse = AppsResponse(
        apps: ["web-frontend", "payments-api", "batch-worker"],
        titles: [
            "web-frontend": "Web Frontend",
            "payments-api": "Payments API",
            "batch-worker": "batch-worker",
        ],
        rings: rings,
        prodProtected: true,
        aiEnabled: true
    )

    static let groups = [
        AppGroup(
            id: "g-1", name: "Core Platform", apps: ["web-frontend", "payments-api"],
            updatedAt: .now
        ),
        AppGroup(id: "g-2", name: "Batch", apps: ["batch-worker"], updatedAt: .now),
    ]

    static let signoffs = [
        Signoff(
            app: "payments-api", ring: "acc", version: "v5.0.0", decision: .go,
            engineer: "j.rivera", qaStatus: "passed", note: "Regression suite green",
            updatedAt: .now.addingTimeInterval(-3_600)
        ),
        Signoff(
            app: "payments-api", ring: "prod", version: "v4.1.0", decision: .noGo,
            engineer: "j.rivera", qaStatus: "failed", note: "Checkout latency regression",
            updatedAt: .now.addingTimeInterval(-86_400)
        ),
    ]

    static let history: [HistoryEntry] = [
        HistoryEntry(
            id: 5, app: "web-frontend", ring: "int", action: "rollback",
            fromVersion: "2.7.2", toVersion: "2.7.1", result: "success",
            message: "rolled back to 2.7.1", createdAt: .now.addingTimeInterval(-1_200)
        ),
        HistoryEntry(
            id: 4, app: "web-frontend", ring: "test", action: "promote",
            fromVersion: "", toVersion: "v9.1.0", result: "failure",
            message: "promote of v9.1.0 failed health check; rolled back to v9.0.0",
            createdAt: .now.addingTimeInterval(-9_600)
        ),
        HistoryEntry(
            id: 3, app: "web-frontend", ring: "int", action: "seed",
            fromVersion: "2.7.1", toVersion: "2.7.2", result: "success",
            message: "seeded 2.7.2 and healthy", createdAt: .now.addingTimeInterval(-90_000)
        ),
    ]

    static let runningJob = Job(
        id: "job-4", app: "payments-api", action: "promote", status: .running,
        steps: [
            JobStep(
                id: "source-health", title: "Verify int (v9.1.0) is healthy",
                status: .success, logs: ["source healthy"],
                startedAt: .now.addingTimeInterval(-6), finishedAt: .now.addingTimeInterval(-5)
            ),
            JobStep(
                id: "deploy", title: "Deploy v9.1.0 to test", status: .running,
                logs: ["image set to v9.1.0"], startedAt: .now.addingTimeInterval(-5)
            ),
        ],
        startedAt: .now.addingTimeInterval(-6)
    )

    static let rolledBackJob = Job(
        id: "job-1", app: "payments-api", action: "promote", status: .failed,
        steps: [
            JobStep(
                id: "source-health", title: "Verify int (v9.1.0) is healthy",
                status: .success, logs: ["source healthy"],
                startedAt: .now.addingTimeInterval(-40), finishedAt: .now.addingTimeInterval(-39)
            ),
            JobStep(
                id: "deploy", title: "Deploy v9.1.0 to test", status: .success,
                logs: ["image set to v9.1.0"],
                startedAt: .now.addingTimeInterval(-39), finishedAt: .now.addingTimeInterval(-38)
            ),
            JobStep(
                id: "health", title: "Health check test", status: .failed,
                logs: [
                    "attempt 1/3 failed: unhealthy: status 503",
                    "waiting 5s before retry",
                    "attempt 2/3 failed: unhealthy: status 503",
                    "waiting 5s before retry",
                    "attempt 3/3 failed: unhealthy: status 503",
                    "health check failed after retries: unhealthy: status 503",
                ],
                startedAt: .now.addingTimeInterval(-38), finishedAt: .now.addingTimeInterval(-20)
            ),
            JobStep(
                id: "rollback", title: "Auto-rollback test to v9.0.0", status: .success,
                logs: ["deploying previous version v9.0.0", "rolled back to v9.0.0"],
                startedAt: .now.addingTimeInterval(-20), finishedAt: .now.addingTimeInterval(-12)
            ),
        ],
        result: ActionResult(
            app: "payments-api", action: "promote", ring: "test", fromRing: "int",
            version: "v9.1.0", success: false, rolledBack: true,
            message: "promote of v9.1.0 failed health check; rolled back to v9.0.0"
        ),
        startedAt: .now.addingTimeInterval(-40), finishedAt: .now.addingTimeInterval(-12)
    )

    static let maintenance = MaintenanceStatus(
        gated: true,
        gatedRings: ["prod"],
        recurring: [
            RecurringWindow(
                days: ["Sat", "Sun"], start: "02:00", end: "04:00", timezone: "Europe/London"
            )
        ],
        windows: [
            MaintenanceWindow(
                id: "mw-1", app: "payments-api", ring: "prod",
                startsAt: .now.addingTimeInterval(-1_800),
                endsAt: .now.addingTimeInterval(5_400),
                reason: "Emergency payment gateway fix", createdBy: "a.patel",
                createdAt: .now.addingTimeInterval(-1_800)
            )
        ],
        openRings: ["prod": true]
    )
}
