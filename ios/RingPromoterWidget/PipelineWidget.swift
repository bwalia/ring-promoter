import SwiftUI
import WidgetKit

/// The widget's data source.
///
/// It reads only the snapshot the app wrote into the shared App Group — the
/// widget never talks to the network and never sees a token. That is a
/// deliberate limit: an extension cannot prompt for a password or handle a 401,
/// so giving it credentials would add risk with nothing to show for it.
struct SnapshotProvider: TimelineProvider {
    func placeholder(in context: Context) -> SnapshotEntry {
        SnapshotEntry(date: .now, snapshot: .placeholder)
    }

    func getSnapshot(in context: Context, completion: @escaping (SnapshotEntry) -> Void) {
        completion(SnapshotEntry(date: .now, snapshot: WidgetSnapshotStore.read() ?? .placeholder))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<SnapshotEntry>) -> Void) {
        let entry = SnapshotEntry(date: .now, snapshot: WidgetSnapshotStore.read())
        // Ask to be woken in 15 minutes. The app also nudges WidgetKit after
        // every refresh, so in practice this is only the floor.
        completion(Timeline(entries: [entry], policy: .after(.now.addingTimeInterval(900))))
    }
}

struct SnapshotEntry: TimelineEntry {
    let date: Date
    let snapshot: WidgetSnapshot?
}

/// One application's pipeline at a glance.
struct PipelineWidget: Widget {
    var body: some WidgetConfiguration {
        StaticConfiguration(kind: SharedContainer.WidgetKind.pipeline, provider: SnapshotProvider()) {
            entry in
            PipelineWidgetView(entry: entry)
                .containerBackground(.fill.tertiary, for: .widget)
        }
        .configurationDisplayName("Pipeline")
        .description("The ring pipeline for the application that most needs your attention.")
        .supportedFamilies([.systemSmall, .systemMedium])
    }
}

struct PipelineWidgetView: View {
    let entry: SnapshotEntry
    @Environment(\.widgetFamily) private var family

    /// Show whichever app is in trouble; a healthy system shows the first.
    private var featured: WidgetSnapshot.AppCell? {
        guard let apps = entry.snapshot?.apps, !apps.isEmpty else { return nil }
        return apps.first { $0.unhealthyCount > 0 } ?? apps.first
    }

    var body: some View {
        if let snapshot = entry.snapshot, let app = featured {
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 5) {
                    Circle()
                        .fill(WidgetTint.color(named: snapshot.instanceTint))
                        .frame(width: 7, height: 7)
                    Text(snapshot.isDemo ? "Demo" : snapshot.instanceName)
                        .font(.caption2.weight(.semibold))
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                    Spacer(minLength: 0)
                    if app.unhealthyCount > 0 {
                        Image(systemName: "exclamationmark.triangle.fill")
                            .font(.caption2)
                            .foregroundStyle(WidgetTint.unhealthy)
                    }
                }
                Text(app.title)
                    .font(.subheadline.weight(.semibold))
                    .lineLimit(1)
                    .minimumScaleFactor(0.8)

                VStack(spacing: 3) {
                    ForEach(app.rings.filter(\.configured)) { ring in
                        WidgetRingRow(ring: ring, showsVersion: family != .systemSmall)
                    }
                }
                Spacer(minLength: 0)
                Text(snapshot.capturedAt, style: .relative)
                    .font(.system(size: 9))
                    .foregroundStyle(.tertiary)
            }
            .widgetURL(Router.widgetURL(forApp: app.name))
            .accessibilityLabel(accessibilityLabel(app: app, snapshot: snapshot))
        } else {
            WidgetEmptyState()
        }
    }

    private func accessibilityLabel(
        app: WidgetSnapshot.AppCell, snapshot: WidgetSnapshot
    ) -> String {
        let states = app.rings.filter(\.configured).map { ring in
            "\(ring.label) \(ring.empty ? "not deployed" : ring.version), "
                + (ring.healthy ? "healthy" : "unhealthy")
        }
        return "\(app.title) on \(snapshot.instanceName). " + states.joined(separator: ". ")
    }
}

/// A count of everything that is wrong, for people watching many applications.
struct HealthWidget: Widget {
    var body: some WidgetConfiguration {
        StaticConfiguration(kind: SharedContainer.WidgetKind.health, provider: SnapshotProvider()) {
            entry in
            HealthWidgetView(entry: entry)
                .containerBackground(.fill.tertiary, for: .widget)
        }
        .configurationDisplayName("Unhealthy rings")
        .description("How many rings are failing their health check right now.")
        .supportedFamilies([.systemSmall])
    }
}

struct HealthWidgetView: View {
    let entry: SnapshotEntry

    var body: some View {
        if let snapshot = entry.snapshot {
            let count = snapshot.unhealthyCount
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 5) {
                    Circle()
                        .fill(WidgetTint.color(named: snapshot.instanceTint))
                        .frame(width: 7, height: 7)
                    Text(snapshot.isDemo ? "Demo" : snapshot.instanceName)
                        .font(.caption2.weight(.semibold))
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
                Spacer(minLength: 0)
                Image(systemName: count == 0 ? "checkmark.circle.fill" : "exclamationmark.triangle.fill")
                    .font(.title2)
                    .foregroundStyle(count == 0 ? WidgetTint.healthy : WidgetTint.unhealthy)
                Text("\(count)")
                    .font(.system(size: 34, weight: .bold, design: .rounded))
                    .contentTransition(.numericText())
                Text(count == 1 ? "unhealthy ring" : "unhealthy rings")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                Spacer(minLength: 0)
                Text(snapshot.capturedAt, style: .relative)
                    .font(.system(size: 9))
                    .foregroundStyle(.tertiary)
            }
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(
                count == 0
                    ? "All rings healthy on \(snapshot.instanceName)"
                    : "\(count) unhealthy rings on \(snapshot.instanceName)"
            )
        } else {
            WidgetEmptyState()
        }
    }
}

/// One ring inside a widget. Icon plus colour, exactly like the app.
private struct WidgetRingRow: View {
    let ring: WidgetSnapshot.RingCell
    var showsVersion: Bool

    private var systemImage: String {
        if ring.empty { return "circle.dotted" }
        return ring.healthy ? "checkmark.circle.fill" : "exclamationmark.triangle.fill"
    }

    private var tint: Color {
        if ring.empty { return WidgetTint.neutral }
        return ring.healthy ? WidgetTint.healthy : WidgetTint.unhealthy
    }

    var body: some View {
        HStack(spacing: 5) {
            Image(systemName: systemImage)
                .font(.system(size: 10, weight: .semibold))
                .foregroundStyle(tint)
            Text(ring.name.uppercased())
                .font(.system(size: 10, weight: .bold))
                .foregroundStyle(.secondary)
            if showsVersion {
                Spacer(minLength: 4)
                Text(ring.empty ? "—" : ring.version)
                    .font(.system(size: 10, design: .monospaced))
                    .lineLimit(1)
                    .truncationMode(.middle)
            } else {
                Spacer(minLength: 0)
            }
        }
    }
}

/// Shown before the app has ever refreshed, so the widget explains itself
/// rather than looking broken.
private struct WidgetEmptyState: View {
    var body: some View {
        VStack(spacing: 6) {
            Image(systemName: "square.stack.3d.up")
                .font(.title3)
                .foregroundStyle(.secondary)
            Text("Open Ring Promoter")
                .font(.caption.weight(.medium))
            Text("Connect to a control plane to see your pipelines here.")
                .font(.caption2)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
        }
        .padding(6)
    }
}

/// The widget's copy of the app's semantic colours.
///
/// Duplicated rather than shared because the app's versions are built on
/// `UIColor` trait resolution, which is more machinery than a widget needs.
enum WidgetTint {
    static let healthy = Color(red: 0.16, green: 0.55, blue: 0.33)
    static let unhealthy = Color(red: 0.80, green: 0.20, blue: 0.18)
    static let neutral = Color(red: 0.50, green: 0.53, blue: 0.57)

    static func color(named name: String) -> Color {
        switch name {
        case "red": Color(red: 0.80, green: 0.20, blue: 0.18)
        case "orange": Color(red: 0.85, green: 0.55, blue: 0.10)
        case "green": healthy
        case "blue": Color(red: 0.20, green: 0.48, blue: 0.85)
        case "purple": Color(red: 0.55, green: 0.30, blue: 0.70)
        default: neutral
        }
    }
}

/// Deep-link builder, duplicated in the extension because `Router` lives in the
/// app target.
enum Router {
    static func widgetURL(forApp app: String) -> URL? {
        var components = URLComponents()
        components.scheme = "ringpromoter"
        components.host = "app"
        components.path = "/\(app)"
        return components.url
    }
}

#Preview("Pipeline", as: .systemMedium) {
    PipelineWidget()
} timeline: {
    SnapshotEntry(date: .now, snapshot: .placeholder)
}
