import Foundation
import Testing

@testable import RingPromoter

/// The rules that decide what the UI may offer.
///
/// These are the tests that matter most: every one of them is a case where
/// getting it wrong means inviting an operator to do something illegal, or
/// hiding an action they need at 2am.
@Suite("Promotion legality")
struct PromotionRulesTests {
    private let pipeline = RingPipeline(PreviewData.rings)

    private func rules(prodProtected: Bool = true) -> PromotionRules {
        PromotionRules(pipeline: pipeline, prodProtected: prodProtected)
    }

    // MARK: - Pipeline shape

    @Test("promotion always targets exactly the next ring")
    func nextRing() {
        #expect(pipeline.next(after: "int")?.name == "test")
        #expect(pipeline.next(after: "test")?.name == "acc")
        #expect(pipeline.next(after: "acc")?.name == "prod")
        // The last ring has nowhere to go — never skip, never wrap.
        #expect(pipeline.next(after: "prod") == nil)
        #expect(pipeline.next(after: "nonsense") == nil)
    }

    @Test("the production ring is the last one, whatever it is called")
    func productionRing() {
        #expect(pipeline.isProduction("prod"))
        #expect(!pipeline.isProduction("acc"))
        #expect(pipeline.promotingTargetsProduction(from: "acc"))
        #expect(!pipeline.promotingTargetsProduction(from: "test"))

        // A server with a different pipeline must still work: "last" is
        // derived, never hard-coded.
        let short = RingPipeline([
            Ring(name: "dev", label: "Dev"), Ring(name: "live", label: "Live"),
        ])
        #expect(short.isProduction("live"))
        #expect(short.promotingTargetsProduction(from: "dev"))
    }

    // MARK: - Promote

    @Test("a healthy ring holding a version can be promoted")
    func promoteAllowed() {
        let source = PreviewData.ring("int", label: "Integration", version: "2.7.1")
        let target = PreviewData.ring("test", label: "Test")
        let legality = rules().promote(from: source, rings: [source, target])

        #expect(legality.isAllowed)
        #expect(legality.targetRing == "test")
        #expect(legality.requirements.isEmpty)
    }

    @Test("an empty ring cannot be promoted, and says so")
    func promoteEmpty() {
        let source = PreviewData.ring("int", label: "Integration")
        let legality = rules().promote(from: source, rings: [source])

        #expect(!legality.isAllowed)
        #expect(legality.blocker == .sourceRingEmpty)
    }

    @Test("an unhealthy source ring blocks promotion")
    func promoteUnhealthy() {
        // canPromoteFrom is false server-side when the source is unhealthy.
        let source = PreviewData.ring(
            "int", label: "Integration", version: "2.7.1", healthy: false
        )
        let legality = rules().promote(from: source, rings: [source])

        #expect(!legality.isAllowed)
        #expect(legality.blocker == .sourceRingUnhealthy)
        #expect(legality.blocker?.explanation.contains("unhealthy") == true)
    }

    @Test("the last ring offers no promotion at all")
    func promoteFromLastRing() {
        let prod = PreviewData.ring("prod", label: "Production", version: "2.6.9")
        let legality = rules().promote(from: prod, rings: [prod])

        #expect(!legality.isAllowed)
        #expect(legality.blocker == .noNextRing)
    }

    @Test("an unconfigured ring is not part of the pipeline")
    func promoteUnconfigured() {
        let ring = PreviewData.ring("acc", label: "Acceptance", configured: false)
        #expect(rules().promote(from: ring, rings: [ring]).blocker == .ringNotConfigured)
        #expect(rules().seed(into: ring).blocker == .ringNotConfigured)
        #expect(rules().rollback(ring).blocker == .ringNotConfigured)
    }

    // MARK: - Production password

    @Test("promoting into production requires the password when the server has one")
    func promoteIntoProduction() {
        let source = PreviewData.ring("acc", label: "Acceptance", version: "2.7.1")
        let prod = PreviewData.ring("prod", label: "Production")
        let legality = rules(prodProtected: true).promote(from: source, rings: [source, prod])

        #expect(legality.isAllowed)
        #expect(legality.requirements.productionPassword)
    }

    @Test("no password is asked for when the server was not started with one")
    func unprotectedServer() {
        let source = PreviewData.ring("acc", label: "Acceptance", version: "2.7.1")
        let prod = PreviewData.ring("prod", label: "Production")
        let legality = rules(prodProtected: false).promote(from: source, rings: [source, prod])

        #expect(legality.isAllowed)
        #expect(!legality.requirements.productionPassword)
    }

    @Test("seeding production directly needs the password too")
    func seedProduction() {
        let prod = PreviewData.ring("prod", label: "Production")
        #expect(rules().seed(into: prod).requirements.productionPassword)

        let acc = PreviewData.ring("acc", label: "Acceptance")
        #expect(!rules().seed(into: acc).requirements.productionPassword)
    }

    @Test("promoting into a non-production ring never asks for the password")
    func nonProductionPromote() {
        let source = PreviewData.ring("int", label: "Integration", version: "1.0")
        let target = PreviewData.ring("test", label: "Test")
        #expect(
            !rules().promote(from: source, rings: [source, target])
                .requirements.productionPassword
        )
    }

    // MARK: - Rollback: the one action that is never gated

    @Test("rollback needs only a previous version")
    func rollbackAllowed() {
        let ring = PreviewData.ring(
            "test", label: "Test", version: "2.7.1", previous: "2.7.0"
        )
        let legality = rules().rollback(ring)

        #expect(legality.isAllowed)
        #expect(legality.requirements.isEmpty)
    }

    @Test("rollback is refused only when there is nothing to return to")
    func rollbackNoPrevious() {
        let ring = PreviewData.ring("test", label: "Test", version: "2.7.1")
        #expect(rules().rollback(ring).blocker == .noPreviousVersion)
    }

    @Test("rolling production back is never gated — incident response is not blocked")
    func rollbackProductionIsUngated() {
        let prod = PreviewData.ring(
            "prod", label: "Production", version: "2.7.1", previous: "2.6.9",
            gates: PreviewData.fullyGated
        )
        let legality = rules(prodProtected: true).rollback(prod)

        #expect(legality.isAllowed)
        // No password, no window, no sign-off, no CR code. Deliberately.
        #expect(!legality.requirements.productionPassword)
        #expect(!legality.requirements.qaSignoff)
        #expect(!legality.requirements.changeRequestCode)
        #expect(!legality.requirements.maintenanceWindow)
        #expect(legality.requirements.isEmpty)
    }

    @Test("an unhealthy production ring can still be rolled back")
    func rollbackUnhealthyProduction() {
        let prod = PreviewData.ring(
            "prod", label: "Production", version: "bad", previous: "good", healthy: false,
            gates: PreviewData.fullyGated
        )
        #expect(rules().rollback(prod).isAllowed)
    }

    // MARK: - Gates

    @Test("the target ring's gates become the sheet's requirements")
    func gatesBecomeRequirements() {
        let source = PreviewData.ring("test", label: "Test", version: "v5.0.0")
        let acc = PreviewData.ring(
            "acc", label: "Acceptance",
            gates: RingGates(
                maintenanceWindow: false, qaSignoff: true, changeRequest: true,
                changeRequestProvider: "jira", maintenanceWindowOpen: false
            )
        )
        let legality = rules().promote(from: source, rings: [source, acc])

        #expect(legality.isAllowed)
        #expect(legality.requirements.qaSignoff)
        #expect(legality.requirements.changeRequestCode)
        #expect(legality.requirements.changeRequestProvider == "jira")
        #expect(!legality.requirements.maintenanceWindow)
    }

    @Test("gates come from the TARGET ring, not the source")
    func gatesComeFromTarget() {
        // The source is gated and the target is not: nothing should be asked
        // for, because gates guard *entering* a ring.
        let source = PreviewData.ring(
            "test", label: "Test", version: "v1", gates: PreviewData.fullyGated
        )
        let acc = PreviewData.ring("acc", label: "Acceptance", gates: .none)
        let legality = rules(prodProtected: false).promote(from: source, rings: [source, acc])

        #expect(legality.isAllowed)
        #expect(legality.requirements.isEmpty)
    }

    @Test("a shut maintenance window is reported so the sheet can offer to open one")
    func closedWindow() {
        let source = PreviewData.ring("acc", label: "Acceptance", version: "v5.0.0")
        let prod = PreviewData.ring(
            "prod", label: "Production", gates: PreviewData.fullyGated
        )
        let legality = rules().promote(from: source, rings: [source, prod])

        #expect(legality.requirements.maintenanceWindow)
        #expect(!legality.requirements.maintenanceWindowOpen)
        #expect(prod.gates.isBlockedByClosedWindow)
    }

    @Test("an open window is reported as satisfied")
    func openWindow() {
        let source = PreviewData.ring("acc", label: "Acceptance", version: "v5.0.0")
        let prod = PreviewData.ring(
            "prod", label: "Production", gates: PreviewData.openWindowGates
        )
        let legality = rules().promote(from: source, rings: [source, prod])

        #expect(legality.requirements.maintenanceWindowOpen)
        #expect(!prod.gates.isBlockedByClosedWindow)
    }

    @Test("a missing target ring degrades to no extra requirements, not a crash")
    func targetRingMissingFromResponse() {
        let source = PreviewData.ring("int", label: "Integration", version: "1.0")
        // The rings array is missing "test" entirely.
        let legality = rules().promote(from: source, rings: [source])

        #expect(legality.isAllowed)
        #expect(legality.targetRing == "test")
        #expect(legality.requirements.isEmpty)
    }

    // MARK: - Auto-promote

    @Test("a config-managed switch cannot be toggled")
    func managedAutoPromote() {
        let ring = PreviewData.ring(
            "test", label: "Test", version: "1.0", autoPromote: true, managed: true
        )
        let legality = rules().toggleAutoPromote(ring, enabled: false)

        #expect(!legality.isAllowed)
        #expect(legality.blocker == .autoPromoteManagedByConfig)
        #expect(legality.blocker?.explanation.contains("config") == true)
    }

    @Test("enabling auto-promote INTO production needs the password")
    func autoPromoteIntoProduction() {
        // acc → prod, so turning this on creates a hands-free path to production.
        let acc = PreviewData.ring("acc", label: "Acceptance", version: "1.0")
        #expect(rules().toggleAutoPromote(acc, enabled: true).requirements.productionPassword)
    }

    @Test("disabling auto-promote is always allowed — it only makes things safer")
    func disablingAutoPromote() {
        let acc = PreviewData.ring(
            "acc", label: "Acceptance", version: "1.0", autoPromote: true
        )
        let legality = rules().toggleAutoPromote(acc, enabled: false)

        #expect(legality.isAllowed)
        #expect(!legality.requirements.productionPassword)
    }

    @Test("auto-promote on a mid-pipeline ring needs no password")
    func autoPromoteMidPipeline() {
        let int = PreviewData.ring("int", label: "Integration", version: "1.0")
        let legality = rules().toggleAutoPromote(int, enabled: true)

        #expect(legality.isAllowed)
        #expect(!legality.requirements.productionPassword)
    }

    // MARK: - Ring state helpers

    @Test("drift between stored and live state is detected")
    func drift() {
        let drifting = PreviewData.ring(
            "test", label: "Test", version: "2.7.1", live: "2.7.0"
        )
        #expect(drifting.versionIsDrifting)

        let aligned = PreviewData.ring("test", label: "Test", version: "2.7.1")
        #expect(!aligned.versionIsDrifting)

        // An empty ring cannot drift, whatever the endpoint reports.
        let empty = PreviewData.ring("test", label: "Test", live: "2.7.0")
        #expect(!empty.versionIsDrifting)
        #expect(!empty.needsAttention)
    }

    @Test("the live check wins over stored health")
    func liveHealthWins() {
        let ring = RingStatus(
            ring: Ring(name: "test", label: "Test"), configured: true,
            currentVersion: "1.0", previousVersion: "", liveVersion: "1.0",
            healthy: true, liveHealthy: false, liveHealthError: "unhealthy: status 503",
            autoPromote: false, autoPromoteManaged: false, updatedAt: .now,
            canPromoteFrom: false, gates: .none
        )
        #expect(!ring.isHealthy)
        #expect(ring.needsAttention)
        #expect(ring.healthIsDrifting)
    }
}
