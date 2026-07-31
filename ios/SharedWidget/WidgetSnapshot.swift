import Foundation

/// Identifiers shared between the app and its extensions.
///
/// Kept in one file compiled into both targets so the two can never drift.
enum SharedContainer {
    /// The App Group both targets belong to. Change this and the matching
    /// entitlement together.
    static let appGroupID = "group.com.example.RingPromoter"

    /// Keychain access group, so the widget could read credentials **if it ever
    /// needed to**. It deliberately does not: the widget reads only the cached
    /// snapshot below, which contains no secrets.
    static let keychainAccessGroup = "$(AppIdentifierPrefix)com.example.RingPromoter"

    /// Where the app writes the widget's snapshot.
    static let snapshotFilename = "widget-snapshot.json"

    /// The widget kinds, matched by `WidgetCenter.reloadTimelines(ofKind:)`.
    enum WidgetKind {
        static let pipeline = "RingPromoterPipelineWidget"
        static let health = "RingPromoterHealthWidget"
    }

    /// The App Group container URL, or nil when the entitlement is missing.
    static var containerURL: URL? {
        FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: appGroupID)
    }

    static var snapshotURL: URL? {
        containerURL?.appending(path: snapshotFilename)
    }
}

/// The minimum the widget needs to draw a pipeline, written by the app after
/// every successful refresh.
///
/// Deliberately **contains no token, no URL and no secret** — only what is
/// already visible on the Lock Screen anyway: app names, versions and health.
/// Anything more would put credentials somewhere the app cannot revoke.
struct WidgetSnapshot: Codable, Hashable, Sendable {
    /// One ring's state, flattened for display.
    struct RingCell: Codable, Hashable, Sendable, Identifiable {
        let name: String
        let label: String
        let version: String
        let healthy: Bool
        /// The app declares this ring; an unconfigured ring renders as a gap.
        let configured: Bool
        /// Nothing has ever been deployed here.
        let empty: Bool

        var id: String { name }
    }

    struct AppCell: Codable, Hashable, Sendable, Identifiable {
        let name: String
        let title: String
        let rings: [RingCell]

        var id: String { name }

        var unhealthyCount: Int {
            rings.filter { $0.configured && !$0.empty && !$0.healthy }.count
        }
    }

    /// A short, human label for which control plane this came from, so a
    /// staging widget is never mistaken for production.
    let instanceName: String
    /// The instance's colour tag, as a stable name the widget maps to a colour.
    let instanceTint: String
    let apps: [AppCell]
    let capturedAt: Date
    /// True when the snapshot came from demo mode, so the widget can say so.
    let isDemo: Bool

    var unhealthyCount: Int { apps.reduce(0) { $0 + $1.unhealthyCount } }

    var totalRings: Int {
        apps.reduce(0) { $0 + $1.rings.filter { $0.configured && !$0.empty }.count }
    }

    static let placeholder = WidgetSnapshot(
        instanceName: "Production",
        instanceTint: "red",
        apps: [
            AppCell(
                name: "web-frontend", title: "Web Frontend",
                rings: [
                    RingCell(
                        name: "int", label: "Integration", version: "2.7.2",
                        healthy: true, configured: true, empty: false
                    ),
                    RingCell(
                        name: "test", label: "Test", version: "2.7.1",
                        healthy: true, configured: true, empty: false
                    ),
                    RingCell(
                        name: "acc", label: "Acceptance", version: "2.7.1",
                        healthy: false, configured: true, empty: false
                    ),
                    RingCell(
                        name: "prod", label: "Production", version: "2.6.9",
                        healthy: true, configured: true, empty: false
                    ),
                ]
            )
        ],
        capturedAt: .now,
        isDemo: false
    )
}

/// Reads and writes the snapshot in the shared container.
///
/// Writes go through a temporary file and an atomic replace so the widget can
/// never read a half-written file.
enum WidgetSnapshotStore {
    static func write(_ snapshot: WidgetSnapshot) {
        guard let url = SharedContainer.snapshotURL else { return }
        do {
            let encoder = JSONEncoder()
            encoder.dateEncodingStrategy = .iso8601
            try encoder.encode(snapshot).write(to: url, options: .atomic)
        } catch {
            // A widget that cannot be updated is not worth failing a refresh
            // for; the app carries on and the widget shows its last snapshot.
        }
    }

    static func read() -> WidgetSnapshot? {
        guard let url = SharedContainer.snapshotURL,
              let data = try? Data(contentsOf: url)
        else { return nil }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try? decoder.decode(WidgetSnapshot.self, from: data)
    }

    static func clear() {
        guard let url = SharedContainer.snapshotURL else { return }
        try? FileManager.default.removeItem(at: url)
    }
}
