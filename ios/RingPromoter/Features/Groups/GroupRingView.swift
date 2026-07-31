import SwiftUI

/// The deployment ring: a group's applications orbiting its name.
///
/// The phone counterpart of `web/src/components/group-ring.tsx`, built to the
/// same geometry (nodes at `i/n · 2π − π/2`, orbit at 37.5% of the stage, badges
/// at 48%) and the same status colours, so the two read as one product.
///
/// It is not decoration. A group is the unit a team owns, and this answers
/// "is my group healthy?" in one glance: the orbit wears the group's most
/// urgent status, and each node wears its own.
struct GroupRingView: View {
    let group: AppGroup
    let members: [AppSummary]

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var selected: String?
    @State private var hasAppeared = false

    private var statuses: [NodeStatus] { members.map(NodeStatus.of) }
    private var aggregate: NodeStatus { NodeStatus.aggregate(of: statuses) }

    /// Orbit radius as a fraction of the stage, matching the web's 150/400.
    private let orbitFraction: CGFloat = 0.34
    /// Badges sit outside the orbit but must stay inside the stage — the web
    /// can let them overhang its wider canvas, a phone cannot.
    private let badgeFraction: CGFloat = 0.43

    var body: some View {
        GeometryReader { proxy in
            let side = min(proxy.size.width, proxy.size.height)
            let centre = CGPoint(x: proxy.size.width / 2, y: proxy.size.height / 2)
            let orbit = side * orbitFraction

            ZStack {
                Starfield()
                AmbientGlow(tint: aggregate.color, animates: !reduceMotion)

                guideRings(side: side)
                orbitRing(side: side)
                if !reduceMotion { travellingLight(side: side) }

                connectors(centre: centre, orbit: orbit)
                centrepiece

                ForEach(Array(members.enumerated()), id: \.element.id) { index, summary in
                    node(summary, status: statuses[index], index: index, centre: centre, orbit: orbit)
                }
            }
            .frame(width: proxy.size.width, height: proxy.size.height)
            .padding(6)
            .contentShape(.rect)
            .onTapGesture { selected = nil }
        }
        .aspectRatio(1, contentMode: .fit)
        .background(Color(hex: 0x090909))
        .clipShape(.rect(cornerRadius: 20))
        .overlay {
            RoundedRectangle(cornerRadius: 20).strokeBorder(.white.opacity(0.08))
        }
        .overlay(alignment: .bottom) { nodeCard }
        .onAppear { withAnimation(.easeOut(duration: 0.6)) { hasAppeared = true } }
        .accessibilityElement(children: .contain)
        .accessibilityLabel("\(group.name), \(summaryLine)")
    }

    // MARK: - Backdrop

    private func guideRings(side: CGFloat) -> some View {
        ZStack {
            Circle()
                .strokeBorder(.white.opacity(0.045), lineWidth: 1)
                .frame(width: side * 0.56)
            Circle()
                .strokeBorder(
                    .white.opacity(0.09),
                    style: StrokeStyle(lineWidth: 1, dash: [1, 7])
                )
                .frame(width: side * 0.65)
            // The outermost ring drifts the other way, which reads as depth.
            Circle()
                .strokeBorder(
                    .white.opacity(0.07),
                    style: StrokeStyle(lineWidth: 1, dash: [3, 14])
                )
                .frame(width: side * 0.93)
                .rotating(duration: 80, clockwise: false, enabled: !reduceMotion)
        }
    }

    /// The main orbit: a gradient that flows as the ring turns, drawn on first
    /// appearance rather than simply appearing.
    private func orbitRing(side: CGFloat) -> some View {
        Circle()
            .trim(from: 0, to: hasAppeared ? 1 : 0)
            .stroke(
                AngularGradient(
                    colors: [
                        aggregate.color.opacity(0.75),
                        aggregate.color.opacity(0.08),
                        aggregate.color.opacity(0.55),
                        aggregate.color.opacity(0.75),
                    ],
                    center: .center
                ),
                style: StrokeStyle(lineWidth: 2.5, lineCap: .round)
            )
            .frame(width: side * orbitFraction * 2)
            .rotating(duration: 30, enabled: !reduceMotion)
    }

    /// A bright point running the orbit with a soft tail behind it.
    private func travellingLight(side: CGFloat) -> some View {
        ZStack {
            Circle()
                .trim(from: 0, to: 0.068)
                .stroke(
                    aggregate.color.opacity(0.4),
                    style: StrokeStyle(lineWidth: 2.5, lineCap: .round)
                )
            Circle()
                .trim(from: 0.058, to: 0.0655)
                .stroke(
                    aggregate.color,
                    style: StrokeStyle(lineWidth: 3.5, lineCap: .round)
                )
                .shadow(color: aggregate.color, radius: 7)
        }
        .frame(width: side * orbitFraction * 2)
        .rotating(duration: 14, enabled: true)
    }

    // MARK: - Nodes

    private func angle(_ index: Int) -> CGFloat {
        guard !members.isEmpty else { return 0 }
        return CGFloat(index) / CGFloat(members.count) * 2 * .pi - .pi / 2
    }

    private func point(_ index: Int, centre: CGPoint, radius: CGFloat) -> CGPoint {
        let a = angle(index)
        return CGPoint(x: centre.x + radius * cos(a), y: centre.y + radius * sin(a))
    }

    /// Short spokes from the orbit out to each badge.
    private func connectors(centre: CGPoint, orbit: CGFloat) -> some View {
        ForEach(Array(members.enumerated()), id: \.element.id) { index, _ in
            Path { path in
                let a = angle(index)
                path.move(
                    to: CGPoint(
                        x: centre.x + (orbit + 4) * cos(a), y: centre.y + (orbit + 4) * sin(a)
                    )
                )
                path.addLine(
                    to: CGPoint(
                        x: centre.x + (orbit + 22) * cos(a), y: centre.y + (orbit + 22) * sin(a)
                    )
                )
            }
            .stroke(
                statuses[index].color.opacity(selected == members[index].name ? 0.95 : 0.35),
                style: StrokeStyle(lineWidth: 1.5, lineCap: .round)
            )
        }
    }

    private func node(
        _ summary: AppSummary, status: NodeStatus, index: Int, centre: CGPoint, orbit: CGFloat
    ) -> some View {
        let dot = point(index, centre: centre, radius: orbit)
        let badge = point(index, centre: centre, radius: orbit * (badgeFraction / orbitFraction))

        return ZStack {
            NodeDot(status: status, animates: !reduceMotion)
                .position(dot)
            NodeBadge(
                title: summary.title,
                status: status,
                isSelected: selected == summary.name
            )
            .position(badge)
            .onTapGesture {
                selected = selected == summary.name ? nil : summary.name
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(summary.title), \(status.word)")
        .accessibilityAddTraits(.isButton)
    }

    // MARK: - Centre

    private var summaryLine: String {
        switch aggregate {
        case .healthy: return "All systems operational"
        case .deploying: return "Deployment in progress"
        case .degraded: return "Partially degraded"
        case .empty: return "Nothing deployed yet"
        case .loading: return "Checking health…"
        case .failed:
            let count = statuses.filter { $0 == .failed }.count
            return "\(count) app\(count == 1 ? "" : "s") failing"
        }
    }

    private var centrepiece: some View {
        VStack(spacing: 5) {
            Text(group.name)
                .font(group.name.count <= 10 ? .title.bold() : .title2.bold())
                .foregroundStyle(.white)
                .multilineTextAlignment(.center)
                .lineLimit(2)
                .minimumScaleFactor(0.6)
            Text(summaryLine)
                .font(.footnote.weight(.semibold))
                .foregroundStyle(aggregate.color)
            Text("\(members.count) Application\(members.count == 1 ? "" : "s")")
                .font(.caption2)
                .foregroundStyle(Color(hex: 0x9CA3AF))
        }
        .padding(.horizontal, 20)
        .frame(maxWidth: .infinity)
        // A soft shadow keeps the name readable over the ambient glow without
        // needing a plate behind it.
        .shadow(color: .black.opacity(0.85), radius: 10)
        .scaleEffect(hasAppeared ? 1 : 0.95)
    }

    // MARK: - Card

    @ViewBuilder
    private var nodeCard: some View {
        if let name = selected,
           let index = members.firstIndex(where: { $0.name == name }) {
            NodeCard(summary: members[index], status: statuses[index]) {
                selected = nil
            }
            .padding(12)
            .transition(.move(edge: .bottom).combined(with: .opacity))
        }
    }
}

// MARK: - Pieces

/// The dot sitting on the orbit. Healthy breathes; deploying spins.
private struct NodeDot: View {
    let status: NodeStatus
    let animates: Bool

    @State private var pulse = false

    var body: some View {
        ZStack {
            if status == .healthy, animates {
                Circle()
                    .fill(status.color)
                    .frame(width: 14, height: 14)
                    .scaleEffect(pulse ? 2.2 : 1)
                    .opacity(pulse ? 0 : 0.6)
            }
            if status == .deploying {
                Circle()
                    .trim(from: 0, to: 0.3)
                    .stroke(status.color, style: StrokeStyle(lineWidth: 2, lineCap: .round))
                    .frame(width: 22, height: 22)
                    .rotating(duration: 1.1, enabled: animates)
            }
            Circle()
                .fill(status.color)
                .frame(width: 14, height: 14)
                .overlay(Circle().strokeBorder(Color(hex: 0x090909), lineWidth: 2))
                .shadow(color: status.color.opacity(0.5), radius: 5)
        }
        .onAppear {
            guard animates, status == .healthy else { return }
            withAnimation(.easeOut(duration: 3).repeatForever(autoreverses: false)) {
                pulse = true
            }
        }
    }
}

/// The glass pill naming the application.
private struct NodeBadge: View {
    let title: String
    let status: NodeStatus
    let isSelected: Bool

    var body: some View {
        HStack(spacing: 5) {
            Text(String(title.prefix(1)).uppercased())
                .font(.system(size: 9, weight: .semibold))
                .foregroundStyle(Color(hex: 0xE5E5E5))
                .frame(width: 16, height: 16)
                .background(.white.opacity(0.1), in: .rect(cornerRadius: 5))
                .overlay {
                    RoundedRectangle(cornerRadius: 5)
                        .strokeBorder(status.color.opacity(0.35))
                }
            Text(title)
                .font(.caption2.weight(.medium))
                .foregroundStyle(Color(hex: 0xE5E5E5))
                .lineLimit(1)
                .truncationMode(.tail)
                .frame(maxWidth: 96, alignment: .leading)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(.horizontal, 7)
        .padding(.vertical, 4)
        .background(.ultraThinMaterial, in: .capsule)
        .overlay {
            Capsule().strokeBorder(.white.opacity(isSelected ? 0.3 : 0.12))
        }
    }
}

/// The card a node expands into.
private struct NodeCard: View {
    let summary: AppSummary
    let status: NodeStatus
    let onClose: () -> Void

    @Environment(Router.self) private var router

    var body: some View {
        let rings = RingsSummary(summary.rings)

        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text(summary.title)
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(Color(hex: 0xFAFAFA))
                Spacer()
                Button(action: onClose) {
                    Image(systemName: "xmark")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(Color(hex: 0xA3A3A3))
                }
                .accessibilityLabel("Close")
            }

            VStack(spacing: 5) {
                row("Status") {
                    HStack(spacing: 5) {
                        Circle().fill(status.color).frame(width: 6, height: 6)
                        Text(status.word).foregroundStyle(status.color)
                    }
                }
                row("Version") {
                    if let latest = rings.latest {
                        Text("\(latest.currentVersion) · \(latest.ring.name)")
                            .monospaced()
                            .foregroundStyle(Color(hex: 0xF5F5F5))
                    } else {
                        Text("nothing deployed").foregroundStyle(Color(hex: 0x737373))
                    }
                }
                row("Rings") {
                    Text(rings.ringsLabel).foregroundStyle(Color(hex: 0xF5F5F5))
                }
                row("Last deploy") {
                    if let last = rings.lastDeploy {
                        Text(last, format: .relative(presentation: .named))
                            .foregroundStyle(Color(hex: 0xF5F5F5))
                    } else {
                        Text("never").foregroundStyle(Color(hex: 0x737373))
                    }
                }
            }
            .font(.caption)

            Button {
                router.show(app: summary.name)
            } label: {
                Text("Open")
                    .font(.caption.weight(.medium))
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 7)
                    .background(.white, in: .rect(cornerRadius: 8))
                    .foregroundStyle(Color(hex: 0x171717))
            }
            .buttonStyle(.plain)
        }
        .padding(14)
        .background(.ultraThinMaterial, in: .rect(cornerRadius: 16))
        .overlay {
            RoundedRectangle(cornerRadius: 16).strokeBorder(.white.opacity(0.15))
        }
        .frame(maxWidth: 280)
    }

    private func row(_ label: String, @ViewBuilder value: () -> some View) -> some View {
        HStack {
            Text(label).foregroundStyle(Color(hex: 0x737373))
            Spacer()
            value()
        }
    }
}

/// A quiet star field behind the orbit.
///
/// Deterministic rather than random, so it does not reshuffle on every redraw.
private struct Starfield: View {
    private static let stars: [(x: CGFloat, y: CGFloat, size: CGFloat, delay: Double)] = {
        func hash(_ n: Int) -> Double {
            (Double((n &* 9301 &+ 49297) % 233280) / 233280 + 1).truncatingRemainder(dividingBy: 1)
        }
        return (0..<46).map { i in
            (
                x: CGFloat(hash(i * 3 + 1)),
                y: CGFloat(hash(i * 7 + 2)),
                size: 1 + CGFloat(hash(i * 11 + 3)) * 1.6,
                delay: hash(i * 17 + 7) * 5
            )
        }
    }()

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var twinkle = false

    var body: some View {
        GeometryReader { proxy in
            ForEach(Array(Self.stars.enumerated()), id: \.offset) { _, star in
                Circle()
                    .fill(.white)
                    .frame(width: star.size, height: star.size)
                    .position(x: star.x * proxy.size.width, y: star.y * proxy.size.height)
                    .opacity(twinkle ? 0.5 : 0.15)
                    .animation(
                        reduceMotion
                            ? nil
                            : .easeInOut(duration: 3 + star.delay)
                                .repeatForever(autoreverses: true)
                                .delay(star.delay),
                        value: twinkle
                    )
            }
        }
        .onAppear { twinkle = true }
        .allowsHitTesting(false)
    }
}

/// Soft drifting light in the corners, tinted by the group's status.
private struct AmbientGlow: View {
    let tint: Color
    let animates: Bool

    @State private var drift = false

    var body: some View {
        GeometryReader { proxy in
            let size = proxy.size.width * 0.8
            ZStack {
                Circle()
                    .fill(tint)
                    .frame(width: size, height: size)
                    .blur(radius: 90)
                    .opacity(0.15)
                    .offset(x: -size * 0.35, y: -size * 0.35 + (drift ? 14 : -14))
                Circle()
                    .fill(Color(hex: 0x3B82F6))
                    .frame(width: size, height: size)
                    .blur(radius: 90)
                    .opacity(0.08)
                    .offset(x: size * 0.35, y: size * 0.35 + (drift ? -18 : 18))
            }
            .animation(
                animates
                    ? .easeInOut(duration: 16).repeatForever(autoreverses: true) : nil,
                value: drift
            )
        }
        .onAppear { drift = true }
        .allowsHitTesting(false)
    }
}

// MARK: - Rotation

private struct Rotating: ViewModifier {
    let duration: Double
    var clockwise = true
    let enabled: Bool

    @State private var spin = false

    func body(content: Content) -> some View {
        content
            .rotationEffect(.degrees(spin ? (clockwise ? 360 : -360) : 0))
            .onAppear {
                guard enabled else { return }
                withAnimation(.linear(duration: duration).repeatForever(autoreverses: false)) {
                    spin = true
                }
            }
    }
}

extension View {
    fileprivate func rotating(duration: Double, clockwise: Bool = true, enabled: Bool) -> some View {
        modifier(Rotating(duration: duration, clockwise: clockwise, enabled: enabled))
    }
}

#Preview("Group ring") {
    PreviewHost {
        NavigationStack { GroupPageView(groupID: "g-1") }
    }
    .preferredColorScheme(.dark)
}
