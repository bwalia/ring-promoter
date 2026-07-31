import Foundation

/// A permanent recurring window declared in the server's config file.
///
/// Read-only from the app's point of view — it lives in git, not in the API.
///
/// - Note: the Go struct carries no JSON tags, so this object is serialised
///   with **PascalCase** keys ("Days", "Start"), unlike everything else in the
///   API. Verified against a captured response; do not "tidy" these to
///   snake_case.
struct RecurringWindow: Codable, Hashable, Sendable, Identifiable {
    /// Day abbreviations ("Sat", "Sun"). Empty means every day.
    let days: [String]
    /// "HH:MM" in `timezone`.
    let start: String
    /// "HH:MM"; an end before the start crosses midnight.
    let end: String
    /// IANA name; empty means UTC.
    let timezone: String

    var id: String { "\(days.joined(separator: ","))-\(start)-\(end)-\(timezone)" }

    enum CodingKeys: String, CodingKey {
        case days = "Days"
        case start = "Start"
        case end = "End"
        case timezone = "Timezone"
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        days = try c.decodeIfPresent([String].self, forKey: .days) ?? []
        start = try c.decodeIfPresent(String.self, forKey: .start) ?? ""
        end = try c.decodeIfPresent(String.self, forKey: .end) ?? ""
        timezone = try c.decodeIfPresent(String.self, forKey: .timezone) ?? ""
    }

    init(days: [String], start: String, end: String, timezone: String) {
        self.days = days
        self.start = start
        self.end = end
        self.timezone = timezone
    }

    /// "Sat, Sun 02:00–04:00 Europe/London"
    var summary: String {
        let when = days.isEmpty ? "Every day" : days.joined(separator: ", ")
        let zone = timezone.isEmpty ? "UTC" : timezone
        return "\(when) \(start)–\(end) \(zone)"
    }
}

/// An operator-created ad-hoc window during which deploys into a ring are
/// permitted. Mirrors `store.MaintenanceWindow`.
struct MaintenanceWindow: Codable, Hashable, Sendable, Identifiable {
    let id: String
    let app: String
    /// Empty means "all guarded rings".
    let ring: String
    let startsAt: Date
    let endsAt: Date
    let reason: String
    let createdBy: String
    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id, app, ring, reason
        case startsAt = "starts_at"
        case endsAt = "ends_at"
        case createdBy = "created_by"
        case createdAt = "created_at"
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        app = try c.decode(String.self, forKey: .app)
        ring = try c.decodeIfPresent(String.self, forKey: .ring) ?? ""
        startsAt = try c.decode(Date.self, forKey: .startsAt)
        endsAt = try c.decode(Date.self, forKey: .endsAt)
        reason = try c.decodeIfPresent(String.self, forKey: .reason) ?? ""
        createdBy = try c.decodeIfPresent(String.self, forKey: .createdBy) ?? ""
        createdAt = try c.decode(Date.self, forKey: .createdAt)
    }

    init(
        id: String, app: String, ring: String, startsAt: Date, endsAt: Date,
        reason: String, createdBy: String, createdAt: Date
    ) {
        self.id = id
        self.app = app
        self.ring = ring
        self.startsAt = startsAt
        self.endsAt = endsAt
        self.reason = reason
        self.createdBy = createdBy
        self.createdAt = createdAt
    }

    /// Mirrors `MaintenanceWindow.Active` on the server: start inclusive, end
    /// exclusive.
    func isActive(at instant: Date) -> Bool {
        instant >= startsAt && instant < endsAt
    }

    func covers(ring target: String) -> Bool {
        ring.isEmpty || ring == target
    }

    var hasExpired: Bool { endsAt <= Date() }

    /// "All guarded rings" when the window is not ring-specific.
    var ringLabel: String { ring.isEmpty ? "All guarded rings" : ring }
}

/// The aggregate maintenance read model for an app.
///
/// Mirrors `promoter.MaintenanceView` from
/// `GET /api/apps/{app}/maintenance-windows`.
struct MaintenanceStatus: Codable, Hashable, Sendable {
    /// The app gates at least one ring behind a maintenance window.
    let gated: Bool
    /// The rings the gate guards, in pipeline order.
    let gatedRings: [String]
    /// Permanent windows from config (read-only).
    let recurring: [RecurringWindow]
    /// Operator-created ad-hoc windows, newest first.
    let windows: [MaintenanceWindow]
    /// Guarded ring → whether a window is open right now.
    let openRings: [String: Bool]

    enum CodingKeys: String, CodingKey {
        case gated, recurring, windows
        case gatedRings = "gated_rings"
        case openRings = "open_rings"
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        gated = try c.decodeIfPresent(Bool.self, forKey: .gated) ?? false
        // gated_rings and recurring are null for an ungated app.
        gatedRings = try c.decodeIfPresent([String].self, forKey: .gatedRings) ?? []
        recurring = try c.decodeIfPresent([RecurringWindow].self, forKey: .recurring) ?? []
        windows = try c.decodeIfPresent([MaintenanceWindow].self, forKey: .windows) ?? []
        openRings = try c.decodeIfPresent([String: Bool].self, forKey: .openRings) ?? [:]
    }

    init(
        gated: Bool, gatedRings: [String], recurring: [RecurringWindow],
        windows: [MaintenanceWindow], openRings: [String: Bool]
    ) {
        self.gated = gated
        self.gatedRings = gatedRings
        self.recurring = recurring
        self.windows = windows
        self.openRings = openRings
    }

    func isOpen(for ring: String) -> Bool { openRings[ring] ?? false }

    /// Ad-hoc windows that have not yet ended, newest first.
    var activeAndUpcomingWindows: [MaintenanceWindow] {
        windows.filter { !$0.hasExpired }
    }

    static let ungated = MaintenanceStatus(
        gated: false, gatedRings: [], recurring: [], windows: [], openRings: [:]
    )
}

/// Body of `POST /api/apps/{app}/maintenance-windows`.
struct NewMaintenanceWindow: Codable, Hashable, Sendable {
    /// Empty string = all guarded rings.
    var ring: String
    var startsAt: Date
    var endsAt: Date
    var reason: String
    var createdBy: String

    enum CodingKeys: String, CodingKey {
        case ring, reason
        case startsAt = "starts_at"
        case endsAt = "ends_at"
        case createdBy = "created_by"
    }
}
