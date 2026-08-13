import Foundation

/// Geographic pin from config `location`. Apps without one are omitted.
struct AppLocation: Codable, Hashable, Sendable {
    let lat: Double
    let lng: Double
    var city: String?
    var region: String?

    var label: String {
        switch (city?.nilIfEmpty, region?.nilIfEmpty) {
        case let (city?, region?): "\(city), \(region)"
        case let (city?, nil): city
        case let (nil, region?): region
        default: String(format: "%.1f°, %.1f°", lat, lng)
        }
    }
}

private extension String {
    var nilIfEmpty: String? { isEmpty ? nil : self }
}

/// `GET /api/apps` — the control plane's shape: which apps exist, the ring
/// pipeline they all share, and two capability flags the UI must respect.
struct AppsResponse: Codable, Hashable, Sendable {
    /// Application names. Every API path uses these, never the display titles.
    let apps: [String]
    /// Display titles per app (config `display_name`, falling back to the name).
    /// Purely cosmetic.
    let titles: [String: String]
    /// Config `location` pins. Missing key means the app is unplaced.
    var locations: [String: AppLocation] = [:]
    /// The ordered ring pipeline, lowest environment first.
    let rings: [Ring]
    /// The server was started with a production password, so any action that
    /// deploys into the last ring must carry it.
    let prodProtected: Bool
    /// AI failure diagnosis is configured, so "Diagnose with AI" is offered.
    let aiEnabled: Bool

    enum CodingKeys: String, CodingKey {
        case apps, titles, locations, rings
        case prodProtected = "prod_protected"
        case aiEnabled = "ai_enabled"
    }

    /// Display title for an app, falling back to its name.
    func title(for app: String) -> String {
        titles[app] ?? app
    }

    var pipeline: RingPipeline { RingPipeline(rings) }
}
