import Foundation

/// Everything the app must know before it may offer an action.
///
/// The server enforces these rules; this type exists so the **UI never invites
/// an operator to do something illegal**. Every enable/disable decision in the
/// action sheets comes from here, so the rules are stated once and unit-tested
/// once rather than re-derived in each view.
struct PromotionLegality: Hashable, Sendable {
    /// Why an action cannot be offered. Each case maps to the status the server
    /// would have returned, so the copy the user reads matches what would have
    /// happened.
    enum Blocker: Hashable, Sendable {
        /// The app does not declare this ring (404).
        case ringNotConfigured
        /// The source ring holds no version (409 `nothing to promote`).
        case sourceRingEmpty
        /// The source ring is unhealthy; the server refuses to promote out of it.
        case sourceRingUnhealthy
        /// Already at the last ring (400 `no next ring`).
        case noNextRing
        /// No previous version to return to (409 `nothing to roll back`).
        case noPreviousVersion
        /// A maintenance window guards the target and none is open (409).
        case maintenanceWindowClosed(ring: String)
        /// Config owns the auto-promote switch (409).
        case autoPromoteManagedByConfig

        var explanation: String {
            switch self {
            case .ringNotConfigured:
                "This ring is not part of the application's pipeline."
            case .sourceRingEmpty:
                "There is no version in this ring to promote."
            case .sourceRingUnhealthy:
                "The source ring is unhealthy. Promotion is blocked until it recovers."
            case .noNextRing:
                "This is the last ring — there is nowhere further to promote."
            case .noPreviousVersion:
                "This ring has no previous version to roll back to."
            case .maintenanceWindowClosed(let ring):
                "No maintenance window is open for \(ring). Open one, or wait for a scheduled window."
            case .autoPromoteManagedByConfig:
                "Auto-promote for this ring is declared in the server's config and can only be changed there."
            }
        }
    }

    /// Requirements a sheet must collect before the request can be sent.
    struct Requirements: Hashable, Sendable {
        /// The target ring needs a change-request code.
        var changeRequestCode = false
        /// The CR validation backend, for the field's help text.
        var changeRequestProvider: String?
        /// The target ring needs a recorded GO for the exact version.
        var qaSignoff = false
        /// The target ring is behind a maintenance-window gate.
        var maintenanceWindow = false
        /// A window is open right now.
        var maintenanceWindowOpen = false
        /// The action deploys into the last ring on a password-protected server.
        var productionPassword = false

        var isEmpty: Bool {
            !changeRequestCode && !qaSignoff && !maintenanceWindow && !productionPassword
        }
    }

    /// The action can be offered.
    let isAllowed: Bool
    /// Why not, when it cannot. Nil when allowed.
    let blocker: Blocker?
    /// What the sheet must collect. Meaningful only when allowed.
    let requirements: Requirements
    /// The ring the action deploys into.
    let targetRing: String?

    static func allowed(target: String?, requirements: Requirements = Requirements()) -> Self {
        Self(isAllowed: true, blocker: nil, requirements: requirements, targetRing: target)
    }

    static func blocked(_ blocker: Blocker, target: String? = nil) -> Self {
        Self(isAllowed: false, blocker: blocker, requirements: Requirements(), targetRing: target)
    }
}

/// The rules, evaluated against a pipeline snapshot.
///
/// `prodProtected` comes from `GET /api/apps`; it is never assumed.
struct PromotionRules: Sendable {
    let pipeline: RingPipeline
    let prodProtected: Bool

    init(pipeline: RingPipeline, prodProtected: Bool) {
        self.pipeline = pipeline
        self.prodProtected = prodProtected
    }

    /// May a version be promoted out of `source`, and what must be collected
    /// first?
    ///
    /// Deliberately trusts the server's `canPromoteFrom` as the primary gate —
    /// it is the same predicate the server applies — and only then derives the
    /// specific reason for the "no" so the user gets a real explanation.
    func promote(from source: RingStatus, rings: [RingStatus]) -> PromotionLegality {
        guard source.configured else { return .blocked(.ringNotConfigured) }
        guard let next = pipeline.next(after: source.ring.name) else {
            return .blocked(.noNextRing)
        }
        guard !source.isEmpty else { return .blocked(.sourceRingEmpty, target: next.name) }
        guard source.canPromoteFrom else {
            // The server said no while a version is present: the remaining
            // reason it enforces is source health.
            return .blocked(
                source.isHealthy ? .ringNotConfigured : .sourceRingUnhealthy,
                target: next.name
            )
        }
        let target = rings.first { $0.ring.name == next.name }
        return .allowed(
            target: next.name,
            requirements: requirements(
                enteringRing: next.name, gates: target?.gates ?? .none
            )
        )
    }

    /// May `ring` be seeded with a version?
    ///
    /// Seeding is legal for any configured ring; the gates guarding entry into
    /// that ring still apply, and seeding the last ring needs the production
    /// password just as promoting into it does.
    func seed(into ring: RingStatus) -> PromotionLegality {
        guard ring.configured else { return .blocked(.ringNotConfigured) }
        return .allowed(
            target: ring.ring.name,
            requirements: requirements(enteringRing: ring.ring.name, gates: ring.gates)
        )
    }

    /// May `ring` be rolled back?
    ///
    /// Rollback is deliberately the least gated action in the whole app:
    /// incident response is never blocked, and the server exempts it from the
    /// production password.
    func rollback(_ ring: RingStatus) -> PromotionLegality {
        guard ring.configured else { return .blocked(.ringNotConfigured) }
        guard ring.canRollBack else {
            return .blocked(.noPreviousVersion, target: ring.ring.name)
        }
        return .allowed(target: ring.ring.name)
    }

    /// May the auto-promote switch for `ring` be toggled to `enabled`?
    ///
    /// Enabling the hands-free path INTO production needs the production
    /// password — otherwise auto-promote would be a way around it. Disabling is
    /// always allowed because it only makes things safer.
    func toggleAutoPromote(_ ring: RingStatus, enabled: Bool) -> PromotionLegality {
        guard !ring.autoPromoteManaged else {
            return .blocked(.autoPromoteManagedByConfig, target: ring.ring.name)
        }
        var reqs = PromotionLegality.Requirements()
        reqs.productionPassword =
            enabled && prodProtected && pipeline.promotingTargetsProduction(from: ring.ring.name)
        return .allowed(target: ring.ring.name, requirements: reqs)
    }

    /// What entering `ring` demands, given its gates.
    private func requirements(
        enteringRing ring: String, gates: RingGates
    ) -> PromotionLegality.Requirements {
        var reqs = PromotionLegality.Requirements()
        reqs.changeRequestCode = gates.changeRequest
        reqs.changeRequestProvider = gates.changeRequestProvider
        reqs.qaSignoff = gates.qaSignoff
        reqs.maintenanceWindow = gates.maintenanceWindow
        reqs.maintenanceWindowOpen = gates.maintenanceWindowOpen
        reqs.productionPassword = prodProtected && pipeline.isProduction(ring)
        return reqs
    }
}
