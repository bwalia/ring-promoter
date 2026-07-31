import ActivityKit
import SwiftUI
import WidgetKit

/// The Lock Screen and Dynamic Island presentation of a running promotion.
///
/// The compact and minimal forms carry the status icon only — at that size an
/// icon that distinguishes running / succeeded / failed / rolled back is worth
/// more than a truncated step title.
struct PromotionLiveActivity: Widget {
    var body: some WidgetConfiguration {
        ActivityConfiguration(for: PromotionActivityAttributes.self) { context in
            LockScreenView(context: context)
                .activityBackgroundTint(Color.black.opacity(0.35))
                .activitySystemActionForegroundColor(.white)
        } dynamicIsland: { context in
            DynamicIsland {
                DynamicIslandExpandedRegion(.leading) {
                    Label {
                        Text(context.attributes.appTitle).lineLimit(1)
                    } icon: {
                        Image(systemName: context.state.status.systemImage)
                            .foregroundStyle(tint(for: context.state.status))
                    }
                    .font(.caption.weight(.semibold))
                }
                DynamicIslandExpandedRegion(.trailing) {
                    Text("→ \(context.attributes.targetRing)")
                        .font(.caption.monospaced())
                        .foregroundStyle(.secondary)
                }
                DynamicIslandExpandedRegion(.bottom) {
                    VStack(alignment: .leading, spacing: 4) {
                        Text(context.state.message ?? context.state.stepTitle)
                            .font(.caption)
                            .lineLimit(2)
                        if !context.state.status.isTerminal {
                            ProgressView(value: context.state.fraction)
                                .tint(tint(for: context.state.status))
                        }
                    }
                }
            } compactLeading: {
                Image(systemName: context.state.status.systemImage)
                    .foregroundStyle(tint(for: context.state.status))
            } compactTrailing: {
                Text(context.attributes.targetRing)
                    .font(.caption2.weight(.semibold))
            } minimal: {
                Image(systemName: context.state.status.systemImage)
                    .foregroundStyle(tint(for: context.state.status))
            }
            .widgetURL(
                Router.widgetURL(
                    forJob: context.attributes.jobID, app: context.attributes.app
                )
            )
        }
    }

    private func tint(for status: PromotionActivityAttributes.ContentState.Status) -> Color {
        switch status {
        case .running: Color(red: 0.35, green: 0.62, blue: 0.95)
        case .succeeded: WidgetTint.healthy
        case .failed, .rolledBack: WidgetTint.unhealthy
        }
    }
}

private struct LockScreenView: View {
    let context: ActivityViewContext<PromotionActivityAttributes>

    private var tint: Color {
        switch context.state.status {
        case .running: Color(red: 0.35, green: 0.62, blue: 0.95)
        case .succeeded: WidgetTint.healthy
        case .failed, .rolledBack: WidgetTint.unhealthy
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Image(systemName: context.state.status.systemImage)
                    .foregroundStyle(tint)
                Text(context.attributes.appTitle)
                    .font(.headline)
                    .lineLimit(1)
                Spacer(minLength: 4)
                Text(context.state.status.headline)
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(tint)
            }

            Text(
                "\(context.attributes.action.capitalized) \(context.attributes.version) "
                    + "→ \(context.attributes.targetRing)"
            )
            .font(.caption.monospaced())
            .foregroundStyle(.secondary)
            .lineLimit(1)

            if context.state.status.isTerminal {
                if let message = context.state.message {
                    Text(message)
                        .font(.caption)
                        .lineLimit(2)
                }
            } else {
                VStack(alignment: .leading, spacing: 4) {
                    Text(context.state.stepTitle)
                        .font(.caption)
                        .lineLimit(1)
                    ProgressView(value: context.state.fraction)
                        .tint(tint)
                    Text("Step \(context.state.stepIndex) of \(context.state.stepCount)")
                        .font(.system(size: 10))
                        .foregroundStyle(.tertiary)
                }
            }
        }
        .padding(14)
        .accessibilityElement(children: .combine)
        .accessibilityLabel(
            "\(context.attributes.appTitle), \(context.state.status.headline). "
                + (context.state.message ?? context.state.stepTitle)
        )
    }
}

extension Router {
    static func widgetURL(forJob id: String, app: String) -> URL? {
        var components = URLComponents()
        components.scheme = "ringpromoter"
        components.host = "app"
        components.path = "/\(app)/job/\(id)"
        return components.url
    }
}
