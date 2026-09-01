import Foundation

/// The promotion-policy gates guarding entry INTO a ring.
///
/// Mirrors `promoter.RingGates`. The app uses this to decide what a promote or
/// seed sheet must collect before it is allowed to submit.
struct RingGates: Codable, Hashable, Sendable {
    /// A window must be open before anything may enter this ring.
    var maintenanceWindow: Bool = false
    /// A release engineer must have recorded a GO for the exact version.
    var qaSignoff: Bool = false
    /// The operation must carry a valid change-request code.
    var changeRequest: Bool = false
    /// The CR validation backend ("test", "jira"). Informational only.
    var changeRequestProvider: String?
    /// Whether a window is open right now. Only meaningful when
    /// `maintenanceWindow` is true.
    var maintenanceWindowOpen: Bool = false

    enum CodingKeys: String, CodingKey {
        case maintenanceWindow = "maintenance_window"
        case qaSignoff = "qa_signoff"
        case changeRequest = "change_request"
        case changeRequestProvider = "change_request_provider"
        case maintenanceWindowOpen = "maintenance_window_open"
    }

    /// Whether any gate guards this ring at all.
    var isGated: Bool { maintenanceWindow || qaSignoff || changeRequest }

    /// The maintenance gate is guarding and currently shut. A promotion into
    /// this ring would be refused with 409 until a window opens.
    var isBlockedByClosedWindow: Bool { maintenanceWindow && !maintenanceWindowOpen }

    static let none = RingGates()
}

/// The tracked deploy state of one app in one ring.
///
/// Mirrors `store.RingState`, carried inside an `ActionResult`.
struct RingState: Codable, Hashable, Sendable {
    var app: String = ""
    var ring: String = ""
    var currentVersion: String = ""
    var previousVersion: String = ""
    var healthy: Bool = false
    var autoPromote: Bool = false
    var updatedAt: Date = .distantPast

    enum CodingKeys: String, CodingKey {
        case app, ring, healthy
        case currentVersion = "current_version"
        case previousVersion = "previous_version"
        case autoPromote = "auto_promote"
        case updatedAt = "updated_at"
    }
}

/// The read model for one ring of an application.
///
/// Mirrors `promoter.RingView` from `GET /api/apps/{app}/rings`. Named
/// `RingStatus` here so it is never mistaken for a SwiftUI view.
struct RingStatus: Codable, Hashable, Sendable, Identifiable {
    let ring: Ring
    /// The app declares this ring in config. An unconfigured ring is shown as
    /// "not part of this app's pipeline", never as an empty deployable ring.
    let configured: Bool
    let currentVersion: String
    let previousVersion: String
    /// What the ring's health endpoint reports it is actually running, which can
    /// differ from `currentVersion` when a deploy has drifted.
    let liveVersion: String
    /// Health as last stored by a promotion.
    let healthy: Bool
    /// Health as freshly checked when this response was built.
    let liveHealthy: Bool
    /// Why the live check failed, when it did.
    let liveHealthError: String?
    /// Round-trip time of the live health check, in milliseconds. Absent when
    /// the ring has no health endpoint or the check never completed. Drives the
    /// orbit radius on the Rings of Applications screen.
    var latencyMs: Int? = nil
    /// Time to first byte of the live health check, when reported.
    var ttfbMs: Int? = nil
    let autoPromote: Bool
    /// Config owns this ring's auto-promote switch: the API toggle returns 409,
    /// so the control must be rendered disabled with an explanation.
    let autoPromoteManaged: Bool
    let updatedAt: Date
    /// The server will accept a promotion out of this ring. The app must not
    /// offer Promote when this is false.
    let canPromoteFrom: Bool
    /// Gates guarding entry into THIS ring.
    let gates: RingGates

    var id: String { ring.name }

    enum CodingKeys: String, CodingKey {
        case ring, configured, healthy, gates
        case currentVersion = "current_version"
        case previousVersion = "previous_version"
        case liveVersion = "live_version"
        case liveHealthy = "live_healthy"
        case liveHealthError = "live_health_error"
        case latencyMs = "latency_ms"
        case ttfbMs = "ttfb_ms"
        case autoPromote = "auto_promote"
        case autoPromoteManaged = "auto_promote_managed"
        case updatedAt = "updated_at"
        case canPromoteFrom = "can_promote_from"
    }

    /// A ring holding no version has never been deployed to.
    var isEmpty: Bool { currentVersion.isEmpty }

    /// Rolling back is only legal when a previous version exists to return to.
    var canRollBack: Bool { !previousVersion.isEmpty }

    /// `updatedAt` is Go's zero time for a ring nothing has ever touched; the UI
    /// must show "never", not "1 Jan 0001".
    var hasBeenDeployedTo: Bool { !isEmpty && updatedAt.timeIntervalSince1970 > 0 }

    /// The stored health and the fresh check disagree — worth surfacing, because
    /// it usually means the ring degraded since the last promotion.
    var healthIsDrifting: Bool { configured && !isEmpty && healthy != liveHealthy }

    /// The endpoint is serving a different version than the one recorded here.
    var versionIsDrifting: Bool {
        configured && !isEmpty && !liveVersion.isEmpty && liveVersion != currentVersion
    }

    /// The single health verdict the UI leads with. The fresh check wins: it is
    /// what is true right now.
    var isHealthy: Bool { liveHealthy }

    /// Rings needing an operator's attention: deployed, but not healthy.
    var needsAttention: Bool { configured && !isEmpty && !liveHealthy }
}

/// `GET /api/apps/{app}/rings`
struct RingsResponse: Codable, Hashable, Sendable {
    let rings: [RingStatus]
}
