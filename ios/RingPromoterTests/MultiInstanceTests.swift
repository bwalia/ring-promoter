import Foundation
import Testing

@testable import RingPromoter

/// Moving between several saved control planes.
@Suite("Several control planes")
struct MultiInstanceTests {

    @MainActor
    private func makeSession() -> AppSession {
        let defaults = UserDefaults(suiteName: "MultiInstanceTests-\(UUID().uuidString)")!
        return AppSession(
            instances: InstanceStore(defaults: defaults),
            settings: AppSettings(defaults: defaults)
        )
    }

    private func instance(_ name: String) -> Instance {
        Instance(name: name, baseURL: URL(string: "https://\(name.lowercased()).example.com")!)
    }

    @Test("each control plane has its own connection key, so the tab tree is rebuilt")
    @MainActor
    func connectionKeyPerInstance() {
        let session = makeSession()
        let staging = instance("Staging")
        let production = instance("Production")

        #expect(session.connectionKey == nil)

        session.connect(to: staging, token: "staging-token")
        let stagingKey = session.connectionKey
        session.connect(to: production, token: "production-token")
        let productionKey = session.connectionKey

        #expect(stagingKey == staging.id.uuidString)
        #expect(productionKey == production.id.uuidString)
        #expect(stagingKey != productionKey)

        session.enterDemoMode()
        #expect(session.connectionKey == "demo")

        session.disconnect()
        #expect(session.connectionKey == nil)
    }

    @Test("switching drops the previous server's capabilities")
    @MainActor
    func switchingClearsCapabilities() async throws {
        let session = makeSession()
        session.enterDemoMode()
        _ = try await session.loadCapabilities()
        #expect(session.capabilities != nil)

        session.connect(to: instance("Production"), token: "token")
        #expect(session.capabilities == nil)
        #expect(session.activeInstance?.name == "Production")
    }

    @Test("a saved instance with no token fails to activate without blaming the active one")
    @MainActor
    func missingTokenDoesNotFlagActiveInstance() {
        let session = makeSession()
        let staging = instance("Staging")
        session.connect(to: staging, token: "staging-token")

        // Never saved, so the Keychain has no token for it.
        let orphan = instance("Orphan")
        #expect(session.activate(orphan) == false)

        // Still on Staging, and Staging's token was not called into question.
        #expect(session.activeInstance?.id == staging.id)
        #expect(session.needsReauthentication == false)
    }

    @Test("a blank name is filled from the host")
    func defaultNames() throws {
        func name(_ raw: String) throws -> String {
            Instance.defaultName(for: try #require(URL(string: raw)))
        }
        #expect(try name("https://rp.workstation.co.uk") == "Workstation")
        #expect(try name("https://ring-promoter.diytaxreturn.co.uk") == "Diytaxreturn")
        #expect(try name("https://staging.example.com") == "Staging")
        #expect(try name("http://localhost:8080") == "localhost")
        #expect(try name("http://192.168.1.5:8080") == "192.168.1.5")
    }

    @Test("a new control plane gets a colour no other one uses, red last")
    @MainActor
    func nextUnusedTint() throws {
        let defaults = UserDefaults(suiteName: "MultiInstanceTests-\(UUID().uuidString)")!
        let store = InstanceStore(defaults: defaults)
        #expect(store.nextUnusedTint() == .blue)

        var seen: [InstanceTint] = []
        for _ in 0..<5 {
            let tint = store.nextUnusedTint()
            seen.append(tint)
            try store.save(Instance(name: "x", baseURL: URL(string: "https://\(tint).example.com")!, tint: tint), token: "t")
        }
        #expect(seen == [.blue, .green, .orange, .purple, .graphite])
        #expect(store.nextUnusedTint() == .red)
        for saved in store.instances { store.remove(saved) }
    }

    @Test("moving to another control plane resets navigation to its Overview")
    @MainActor
    func routerResetsOnSwitch() {
        let router = Router()
        router.showJob(app: "payments-api", id: "job-3")
        router.tab = .settings

        router.resetForNewConnection()

        #expect(router.tab == .overview)
        #expect(router.overviewPath.isEmpty)
    }
}
