import SwiftUI

/// A small labelled badge. Always icon + text, never colour alone.
struct StatusBadge: View {
    let text: String
    let systemImage: String
    var tint: Color = .rpNeutral
    var isProminent: Bool = false

    var body: some View {
        Label {
            Text(text)
        } icon: {
            Image(systemName: systemImage)
        }
        .font(.caption2.weight(.semibold))
        .lineLimit(1)
        .padding(.horizontal, 7)
        .padding(.vertical, 3)
        .foregroundStyle(isProminent ? Color.white : tint)
        .background(
            isProminent ? AnyShapeStyle(tint) : AnyShapeStyle(tint.opacity(0.15)),
            in: .capsule
        )
        .accessibilityLabel(text)
    }
}

/// The gates guarding entry into a ring, shown wherever an operator might be
/// about to deploy into it.
///
/// Only ever describes what is *required*; whether it is currently satisfied is
/// carried by the maintenance badge's own state, because that is the only gate
/// whose status is version-independent and therefore knowable here.
struct GateBadges: View {
    let gates: RingGates
    var isCompact: Bool = false

    var body: some View {
        if gates.isGated {
            ViewThatFits(in: .horizontal) {
                HStack(spacing: 4) { badges }
                VStack(alignment: .leading, spacing: 4) { badges }
            }
        }
    }

    @ViewBuilder private var badges: some View {
        if gates.maintenanceWindow {
            StatusBadge(
                text: isCompact
                    ? (gates.maintenanceWindowOpen ? "Window open" : "Window shut")
                    : (gates.maintenanceWindowOpen
                        ? "Maintenance window open" : "Maintenance window shut"),
                systemImage: gates.maintenanceWindowOpen ? "lock.open.fill" : "lock.fill",
                tint: gates.maintenanceWindowOpen ? .rpHealthy : .rpGate
            )
        }
        if gates.qaSignoff {
            StatusBadge(
                text: isCompact ? "QA" : "QA sign-off", systemImage: "checkmark.seal",
                tint: .rpGate
            )
        }
        if gates.changeRequest {
            StatusBadge(
                text: isCompact ? "CR" : changeRequestLabel, systemImage: "doc.text",
                tint: .rpGate
            )
        }
    }

    private var changeRequestLabel: String {
        guard let provider = gates.changeRequestProvider, !provider.isEmpty else {
            return "Change request"
        }
        return "Change request (\(provider))"
    }
}

/// "Updated 4 minutes ago", refreshing itself as time passes.
struct RelativeTimestamp: View {
    let date: Date
    var prefix: String?
    /// Text shown for a ring nothing has ever touched, whose timestamp is Go's
    /// zero value rather than a real instant.
    var neverText: String = "never"

    var body: some View {
        Group {
            if date <= Date(timeIntervalSince1970: 0) {
                Text(prefix.map { "\($0) \(neverText)" } ?? neverText)
            } else if let prefix {
                Text("\(prefix) \(date, format: .relative(presentation: .named))")
            } else {
                Text(date, format: .relative(presentation: .named))
            }
        }
        .font(.caption)
        .foregroundStyle(.secondary)
        .accessibilityLabel(accessibilityText)
    }

    private var accessibilityText: String {
        guard date > Date(timeIntervalSince1970: 0) else {
            return prefix.map { "\($0) \(neverText)" } ?? neverText
        }
        let formatted = date.formatted(date: .abbreviated, time: .shortened)
        return prefix.map { "\($0) \(formatted)" } ?? formatted
    }
}

/// The banner that names the control plane currently being operated on.
///
/// Present on every screen that can change something, because "which cluster am
/// I on?" is the question behind most 2am mistakes.
struct InstanceBanner: View {
    let name: String
    let tint: InstanceTint
    var isDemo: Bool = false

    var body: some View {
        HStack(spacing: 6) {
            Circle()
                .fill(tint.color)
                .frame(width: 8, height: 8)
                .overlay(Circle().strokeBorder(.primary.opacity(0.25)))
            Text(isDemo ? "Demo mode" : name)
                .font(.caption.weight(.semibold))
            if isDemo {
                Image(systemName: "theatermasks.fill").font(.caption2)
            }
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 5)
        .background(tint.color.opacity(0.15), in: .capsule)
        .overlay(Capsule().strokeBorder(tint.color.opacity(0.45)))
        .accessibilityElement(children: .combine)
        .accessibilityLabel(
            isDemo ? "Demo mode, no server" : "Connected to \(name)"
        )
    }
}

/// Used wherever a list can legitimately be empty, so a healthy system reads as
/// calm rather than broken.
struct CalmEmptyState: View {
    let title: String
    let message: String
    var systemImage: String = "checkmark.circle"
    var tint: Color = .rpHealthy

    var body: some View {
        VStack(spacing: 10) {
            Image(systemName: systemImage)
                .font(.system(size: 34, weight: .light))
                .foregroundStyle(tint)
            Text(title).font(.headline)
            Text(message)
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 28)
        .padding(.horizontal, 24)
        .accessibilityElement(children: .combine)
    }
}

#Preview("Badges", traits: .sizeThatFitsLayout) {
    VStack(alignment: .leading, spacing: 12) {
        InstanceBanner(name: "Production", tint: .red)
        InstanceBanner(name: "Demo", tint: .purple, isDemo: true)
        GateBadges(gates: PreviewData.fullyGated)
        StatusBadge(text: "Rolled back", systemImage: "arrow.uturn.backward", tint: .rpUnhealthy, isProminent: true)
        RelativeTimestamp(date: .now.addingTimeInterval(-360), prefix: "Updated")
        CalmEmptyState(title: "All rings healthy", message: "Nothing needs your attention right now.")
    }
    .padding()
}
