import Foundation
import Testing

@testable import RingPromoter

/// Demo mode has to enforce the same rules as the server, or the demo teaches
/// the wrong lesson — and App Store review would see a gate that waves
/// everything through.
@Suite("Demo mode enforces the real rules")
struct DemoModeTests {

    @Test("the demo world loads from the bundled fixtures")
    func worldLoads() throws {
        let world = try DemoWorld.loadThrowing()

        #expect(world.apps.apps.count == 3)
        #expect(world.pipeline.names == ["int", "test", "acc", "prod"])
        // Demo mode turns AI diagnosis on so the feature is demonstrable.
        #expect(world.apps.aiEnabled)
        #expect(world.apps.prodProtected)
        #expect(world.rings(for: "payments-api").count == 4)
    }

    @Test("demo mode always opens onto something, even without fixtures")
    func fallbackWorld() {
        let world = DemoWorld.fallback(now: .now)
        #expect(!world.apps.apps.isEmpty)
        #expect(!world.rings(for: "web-frontend").isEmpty)
    }

    @Test("the demo Overview has something genuinely unhealthy to show")
    func hasTrouble() throws {
        let world = try DemoWorld.loadThrowing()
        let unhealthy = world.rings(for: "payments-api").filter(\.needsAttention)
        #expect(!unhealthy.isEmpty)
    }

    @Test("promoting into production without the password is refused")
    func productionPasswordEnforced() async throws {
        let client = DemoClient()
        await #expect(throws: APIError.self) {
            _ = try await client.promote(
                app: "payments-api", fromRing: "acc", crCode: "test", password: nil
            )
        }
    }

    @Test("a wrong production password is refused")
    func wrongPassword() async throws {
        let client = DemoClient()
        do {
            _ = try await client.seed(
                app: "web-frontend", ring: "prod", version: "1.0", crCode: nil,
                password: "not-the-password"
            )
            Issue.record("expected a 403-equivalent")
        } catch {
            #expect(error.requiresProductionPassword)
        }
    }

    @Test("a change-request-gated ring refuses a missing code")
    func changeRequestRequired() async throws {
        // `acc` is CR- and sign-off-gated in the demo world; seeding it exercises
        // the gate without first tripping the production-password check.
        let client = DemoClient()
        do {
            _ = try await client.seed(
                app: "payments-api", ring: "acc", version: "v5.0.0", crCode: nil, password: nil
            )
            Issue.record("expected the CR gate to refuse")
        } catch {
            guard case .badRequest(let message) = error else {
                Issue.record("expected .badRequest, got \(error)")
                return
            }
            #expect(message.contains("change-request"))
        }
    }

    @Test("an invalid change-request code is refused")
    func changeRequestInvalid() async throws {
        let client = DemoClient()
        do {
            _ = try await client.seed(
                app: "payments-api", ring: "acc", version: "v5.0.0", crCode: "NOPE-1",
                password: nil
            )
            Issue.record("expected the CR gate to refuse")
        } catch {
            guard case .badRequest = error else {
                Issue.record("expected .badRequest, got \(error)")
                return
            }
        }
    }

    @Test("a version with no GO sign-off is refused by the QA gate")
    func signoffRequired() async throws {
        let client = DemoClient()
        do {
            // Valid CR code, but no sign-off exists for this version.
            _ = try await client.seed(
                app: "payments-api", ring: "acc", version: "v9.9.9",
                crCode: DemoClient.demoChangeRequestCode, password: nil
            )
            Issue.record("expected the sign-off gate to refuse")
        } catch {
            guard case .conflict(let message) = error else {
                Issue.record("expected .conflict, got \(error)")
                return
            }
            #expect(message.contains("sign-off"))
        }
    }

    @Test("promoting out of an unhealthy ring is refused")
    func unhealthySourceRefused() async throws {
        // The demo world's payments-api `test` ring is unhealthy, exactly as the
        // captured failure fixture left it.
        let client = DemoClient()
        do {
            _ = try await client.promote(
                app: "payments-api", fromRing: "test",
                crCode: DemoClient.demoChangeRequestCode, password: nil
            )
            Issue.record("expected an unhealthy source to be refused")
        } catch {
            guard case .conflict(let message) = error else {
                Issue.record("expected .conflict, got \(error)")
                return
            }
            #expect(message.contains("unhealthy"))
        }
    }

    @Test("rolling back an empty ring is refused")
    func nothingToRollBack() async throws {
        let client = DemoClient()
        do {
            _ = try await client.rollback(app: "batch-worker", ring: "acc")
            Issue.record("expected a conflict")
        } catch {
            guard case .conflict = error else {
                Issue.record("expected .conflict, got \(error)")
                return
            }
        }
    }

    @Test("a config-managed auto-promote switch returns a conflict")
    func managedAutoPromote() async throws {
        let client = DemoClient()
        do {
            try await client.setAutoPromote(
                app: "batch-worker", ring: "test", enabled: false, password: nil
            )
            Issue.record("expected a conflict")
        } catch {
            guard case .conflict(let message) = error else {
                Issue.record("expected .conflict, got \(error)")
                return
            }
            #expect(message.contains("managed by config"))
        }
    }

    @Test("an unknown application is a 404")
    func unknownApp() async throws {
        let client = DemoClient()
        do {
            _ = try await client.rings(app: "no-such-app")
            Issue.record("expected a not-found")
        } catch {
            guard case .notFound = error else {
                Issue.record("expected .notFound, got \(error)")
                return
            }
        }
    }

    @Test("a legal seed starts a job that progresses and lands")
    func seedRunsToCompletion() async throws {
        let client = DemoClient()
        let id = try await client.seed(
            app: "web-frontend", ring: "int", version: "3.0.0", crCode: nil, password: nil
        )

        let first = try await client.job(app: "web-frontend", id: id)
        #expect(!first.isFinished)
        #expect(!first.steps.isEmpty)

        // The simulation advances with wall-clock time.
        try await Task.sleep(for: .seconds(5))
        let finished = try await client.job(app: "web-frontend", id: id)
        #expect(finished.isFinished)
        #expect(finished.outcome == .succeeded)

        // The world must reflect it afterwards.
        let rings = try await client.rings(app: "web-frontend")
        let int = try #require(rings.first { $0.ring.name == "int" })
        #expect(int.currentVersion == "3.0.0")

        let history = try await client.history(app: "web-frontend")
        #expect(history.first?.toVersion == "3.0.0")
    }

    @Test("a version containing \"bad\" fails and rolls back, so the failure path is reachable")
    func failurePathIsDemonstrable() async throws {
        let client = DemoClient()
        // int already holds a version, so there is something to roll back to.
        let id = try await client.seed(
            app: "web-frontend", ring: "int", version: "bad-build", crCode: nil, password: nil
        )
        try await Task.sleep(for: .seconds(8))

        let job = try await client.job(app: "web-frontend", id: id)
        #expect(job.isFinished)
        #expect(job.outcome == .failedAndRolledBack)
        #expect(job.result?.rolledBack == true)
        #expect(job.canRequestDiagnosis)
    }

    @Test("groups can be created, renamed and deleted")
    func groupLifecycle() async throws {
        let client = DemoClient()
        let created = try await client.createGroup(name: "Mine", apps: ["web-frontend"])
        #expect(created.name == "Mine")

        let renamed = try await client.updateGroup(
            id: created.id, name: "Ours", apps: ["web-frontend", "payments-api"]
        )
        #expect(renamed.name == "Ours")
        #expect(renamed.apps.count == 2)

        try await client.deleteGroup(id: created.id)
        let remaining = try await client.groups()
        #expect(!remaining.contains { $0.id == created.id })
    }

    @Test("a group naming an unknown application is refused")
    func groupWithUnknownApp() async throws {
        let client = DemoClient()
        await #expect(throws: APIError.self) {
            _ = try await client.createGroup(name: "Ghosts", apps: ["not-an-app"])
        }
    }

    @Test("opening a window satisfies the maintenance gate")
    func windowOpensTheGate() async throws {
        let client = DemoClient()
        // Close whatever the fixtures left open.
        for window in try await client.maintenance(app: "payments-api").windows {
            try await client.closeMaintenanceWindow(app: "payments-api", id: window.id)
        }
        #expect(try await !client.maintenance(app: "payments-api").isOpen(for: "prod"))

        _ = try await client.openMaintenanceWindow(
            app: "payments-api",
            window: NewMaintenanceWindow(
                ring: "prod", startsAt: .now.addingTimeInterval(-60),
                endsAt: .now.addingTimeInterval(3_600), reason: "test", createdBy: "tester"
            )
        )
        #expect(try await client.maintenance(app: "payments-api").isOpen(for: "prod"))
    }

    @Test("a window that ends before it starts is refused")
    func invalidWindow() async throws {
        let client = DemoClient()
        await #expect(throws: APIError.self) {
            _ = try await client.openMaintenanceWindow(
                app: "payments-api",
                window: NewMaintenanceWindow(
                    ring: "prod", startsAt: .now, endsAt: .now.addingTimeInterval(-60),
                    reason: "backwards", createdBy: "tester"
                )
            )
        }
    }

    @Test("a sign-off with no engineer named is refused")
    func signoffNeedsAnEngineer() async throws {
        let client = DemoClient()
        await #expect(throws: APIError.self) {
            _ = try await client.recordSignoff(
                app: "payments-api",
                signoff: NewSignoff(
                    ring: "acc", version: "v9.9.9", decision: .go, engineer: "  ",
                    qaStatus: "passed", note: ""
                )
            )
        }
    }
}
