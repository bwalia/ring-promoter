import Foundation

/// `GET /api/apps` — the control plane's shape: which apps exist, the ring
/// pipeline they all share, and two capability flags the UI must respect.
struct AppsResponse: Codable, Hashable, Sendable {
    /// Application names. Every API path uses these, never the display titles.
    let apps: [String]
    /// Display titles per app (config `display_name`, falling back to the name).
    /// Purely cosmetic.
    let titles: [String: String]
    /// The ordered ring pipeline, lowest environment first.
    let rings: [Ring]
    /// The server was started with a production password, so any action that
    /// deploys into the last ring must carry it.
    let prodProtected: Bool
    /// AI failure diagnosis is configured, so "Diagnose with AI" is offered.
    let aiEnabled: Bool

    enum CodingKeys: String, CodingKey {
        case apps, titles, rings
        case prodProtected = "prod_protected"
        case aiEnabled = "ai_enabled"
    }

    /// Display title for an app, falling back to its name.
    func title(for app: String) -> String {
        titles[app] ?? app
    }

    var pipeline: RingPipeline { RingPipeline(rings) }
}
