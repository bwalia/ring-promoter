import SwiftUI

/// The status model behind the group ring.
///
/// Mirrors `NodeStatus` in `web/src/components/group-ring.tsx` exactly —
/// including the hex values — so the phone and the browser can never disagree
/// about the colour of the same application.
enum NodeStatus: String, Hashable, Sendable, CaseIterable {
    case healthy
    case deploying
    case degraded
    case failed
    case empty
    case loading

    var color: Color {
        switch self {
        case .healthy: Color(hex: 0x22C55E)
        case .deploying: Color(hex: 0x3B82F6)
        case .degraded: Color(hex: 0xF59E0B)
        case .failed: Color(hex: 0xEF4444)
        case .empty, .loading: Color(hex: 0x71717A)
        }
    }

    /// The word shown on a node card and spoken by VoiceOver.
    var word: String {
        switch self {
        case .healthy: "Healthy"
        case .deploying: "Deploying"
        case .degraded: "Degraded"
        case .failed: "Failing"
        case .empty: "No version"
        case .loading: "Checking…"
        }
    }

    /// Most-urgent-first. The first status present colours the whole ring.
    static let aggregatePriority: [NodeStatus] = [
        .failed, .degraded, .deploying, .loading, .healthy,
    ]

    /// One application's health across its active rings.
    ///
    /// "Active" means configured **and** holding a version — a ring nothing has
    /// ever been deployed to says nothing about health either way.
    static func of(_ summary: AppSummary) -> NodeStatus {
        if summary.isBusy { return .deploying }
        let active = summary.rings.filter { $0.configured && !$0.isEmpty }
        guard !active.isEmpty else { return summary.rings.isEmpty ? .loading : .empty }
        let healthy = active.filter(\.isHealthy).count
        if healthy == active.count { return .healthy }
        return healthy == 0 ? .failed : .degraded
    }

    /// The group's own state: whatever the most urgent member is.
    static func aggregate(of statuses: [NodeStatus]) -> NodeStatus {
        aggregatePriority.first { statuses.contains($0) } ?? .empty
    }
}

extension Color {
    /// Build from the same hex literals the web uses, so the two stay in step.
    init(hex: UInt32) {
        self.init(
            .sRGB,
            red: Double((hex >> 16) & 0xFF) / 255,
            green: Double((hex >> 8) & 0xFF) / 255,
            blue: Double(hex & 0xFF) / 255,
            opacity: 1
        )
    }
}

/// One application's aggregated numbers, shared by the ring and the node card
/// so they can never disagree. Mirrors `summarizeRings`.
struct RingsSummary {
    let active: [RingStatus]
    let healthy: Int
    let latest: RingStatus?
    let lastDeploy: Date?

    init(_ rings: [RingStatus]) {
        active = rings.filter { $0.configured && !$0.isEmpty }
        healthy = active.filter(\.isHealthy).count
        latest = active.last
        lastDeploy = active.map(\.updatedAt).filter { $0 > .distantPast }.max()
    }

    var ringsLabel: String {
        active.isEmpty ? "—" : "\(healthy)/\(active.count) healthy"
    }
}
