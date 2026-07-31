import Foundation

/// One stage in the shared promotion pipeline.
///
/// Mirrors `ring.Ring` in the backend. The ordered list of rings is served by
/// `GET /api/apps` — the app never hard-codes it, so adding a ring server-side
/// flows through without an app release.
struct Ring: Codable, Hashable, Sendable, Identifiable {
    /// Stable identifier used in every API path and body.
    let name: String
    /// Human-friendly label shown in the UI ("Integration", "Production").
    let label: String

    var id: String { name }

    enum CodingKeys: String, CodingKey {
        case name, label
    }
}

/// The ordered ring pipeline for this control plane.
///
/// Wraps the server's ring list so promotion legality ("what is the next ring?",
/// "is this the last ring?") is answered in one place instead of being
/// re-derived at each call site.
struct RingPipeline: Hashable, Sendable {
    let rings: [Ring]

    init(_ rings: [Ring]) {
        self.rings = rings
    }

    /// The last ring — the one a production password protects.
    var productionRing: Ring? { rings.last }

    var names: [String] { rings.map(\.name) }

    func index(of name: String) -> Int? {
        rings.firstIndex { $0.name == name }
    }

    func ring(named name: String) -> Ring? {
        rings.first { $0.name == name }
    }

    /// The ring immediately after `name`, or nil when `name` is the last ring
    /// or is unknown. Promotion always targets exactly this ring — the app must
    /// never offer a target the server would refuse.
    func next(after name: String) -> Ring? {
        guard let i = index(of: name), i + 1 < rings.count else { return nil }
        return rings[i + 1]
    }

    /// Whether promoting out of `name` would deploy into the production ring.
    /// This is the condition that requires the production password.
    func promotingTargetsProduction(from name: String) -> Bool {
        guard let next = next(after: name), let prod = productionRing else { return false }
        return next.name == prod.name
    }

    /// Whether seeding `name` directly deploys into the production ring.
    func isProduction(_ name: String) -> Bool {
        productionRing?.name == name
    }

    static let empty = RingPipeline([])
}
