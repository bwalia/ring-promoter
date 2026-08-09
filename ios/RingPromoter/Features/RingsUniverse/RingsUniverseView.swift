import SwiftUI

/// The Rings of Applications screen: the whole fleet as a solar system, ported
/// from the web console. Services orbit a central "Rings" sun — inner orbits
/// mean lower production latency — and each body's colour, symbol and motion
/// say how that service is doing right now.
///
/// Reuses `OverviewStore` for data: the same summaries, jobs and groups the
/// Overview list shows, so the two screens can never disagree about health.
struct RingsUniverseView: View {
    @Environment(AppSession.self) private var session
    @Environment(\.scenePhase) private var scenePhase
    @State private var store: OverviewStore?

    var body: some View {
        NavigationStack {
            Group {
                if let store {
                    RingsUniverseContent(store: store)
                } else {
                    ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
                }
            }
            .navigationTitle("Rings of Applications")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    InstanceBanner(
                        name: session.instanceName, tint: session.instanceTint,
                        isDemo: session.isDemo
                    )
                }
            }
        }
        .task {
            if store == nil { store = OverviewStore(session: session) }
            await store?.refresh()
        }
        .task(id: session.settings.refreshInterval) {
            await autoRefreshLoop()
        }
        .onChange(of: scenePhase) { _, phase in
            if phase == .active { Task { await store?.refresh() } }
        }
    }

    /// Periodic refresh while this tab is on screen; `.task` owns and cancels it.
    private func autoRefreshLoop() async {
        let interval = session.settings.refreshInterval
        guard interval > 0 else { return }
        while !Task.isCancelled {
            try? await Task.sleep(for: .seconds(interval))
            guard !Task.isCancelled else { return }
            await store?.refresh()
        }
    }
}

/// Which bodies orbit: every service, or one body per ring (group).
enum FleetMode: String, CaseIterable, Identifiable {
    case apps, rings

    var id: String { rawValue }

    var label: String {
        switch self {
        case .apps: "All services"
        case .rings: "Rings"
        }
    }
}

/// Everything below the navigation bar, split out so the loading branch above
/// stays readable.
private struct RingsUniverseContent: View {
    @Bindable var store: OverviewStore
    @Environment(Router.self) private var router
    @Environment(\.dynamicTypeSize) private var typeSize
    @State private var mode: FleetMode = .apps
    /// Group filter for apps mode, set by "Show services" on a ring's card.
    @State private var filterGroupID: String?
    @State private var selectedID: String?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                if let error = store.error, store.summaries.isEmpty {
                    ErrorRow(error: error) { Task { await store.refresh() } }
                }

                if store.isStale {
                    Label(
                        "Showing the last data this app was able to load.",
                        systemImage: "wifi.exclamationmark"
                    )
                    .font(.subheadline)
                    .foregroundStyle(Color.rpGate)
                }

                Picker("View", selection: $mode) {
                    ForEach(FleetMode.allCases) { mode in
                        Text(mode.label).tag(mode)
                    }
                }
                .pickerStyle(.segmented)

                if let group = filterGroup, mode == .apps {
                    FilterChip(name: group.name) {
                        filterGroupID = nil
                        selectedID = nil
                    }
                }

                if nodes.isEmpty {
                    if store.error == nil, !store.isLoading {
                        CalmEmptyState(
                            title: "No applications",
                            message: "This control plane has no applications configured.",
                            systemImage: "circle.dotted",
                            tint: .rpNeutral
                        )
                    }
                } else if typeSize.isAccessibilitySize {
                    // A radial layout is unreadable at accessibility text sizes;
                    // reflow to plain rows carrying the same information.
                    FleetNodeList(nodes: nodes, mode: mode, onOpen: open(node:))
                } else {
                    SolarStage(
                        nodes: nodes, mode: mode, selectedID: $selectedID,
                        onOpen: open(node:)
                    )
                    legend
                }

                if let lastUpdated = store.lastUpdated {
                    RelativeTimestamp(date: lastUpdated, prefix: "Updated")
                        .frame(maxWidth: .infinity, alignment: .center)
                }
            }
            .padding()
        }
        .refreshable { await store.refresh() }
        .onChange(of: mode) { _, _ in selectedID = nil }
    }

    private var filterGroup: AppGroup? {
        guard let filterGroupID else { return nil }
        return store.groups.first { $0.id == filterGroupID }
    }

    private var nodes: [FleetNode] {
        switch mode {
        case .apps:
            var summaries = store.summaries
            if let group = filterGroup {
                summaries = summaries.filter { group.contains($0.name) }
            }
            return FleetNode.appNodes(from: summaries)
        case .rings:
            return FleetNode.ringNodes(from: store.summaries, groups: store.groups)
        }
    }

    /// The card's primary action: open a service, or drill a ring open into
    /// its member services.
    private func open(node: FleetNode) {
        switch mode {
        case .apps:
            router.show(app: node.id)
        case .rings:
            // Mirrors the web console: the synthetic Ungrouped body just
            // switches back to the unfiltered services view.
            filterGroupID = node.id == FleetNode.ungroupedID ? nil : node.id
            selectedID = nil
            mode = .apps
        }
    }

    private var legend: some View {
        HStack {
            let aggregate = FleetStatus.aggregate(nodes.map(\.status))
            Label(summaryLine(for: aggregate), systemImage: aggregate.systemImage)
                .foregroundStyle(aggregate.tint)
            Spacer()
            Text("Inner orbits = lower latency")
                .foregroundStyle(.secondary)
        }
        .font(.caption2)
        .accessibilityElement(children: .combine)
    }

    private func summaryLine(for aggregate: FleetStatus) -> String {
        switch aggregate {
        case .healthy: "All systems operational"
        case .deploying: "Deployment in progress"
        case .degraded: "Partially degraded"
        case .failed:
            {
                let failing = nodes.count { $0.status == .failed }
                let body = mode == .apps
                    ? (failing == 1 ? "service" : "services")
                    : (failing == 1 ? "ring" : "rings")
                return "\(failing) \(body) failing"
            }()
        case .empty: "Nothing deployed yet"
        case .loading: "Checking health…"
        }
    }
}

/// "Showing: Payments ✕" — the active group filter in apps mode.
private struct FilterChip: View {
    let name: String
    let clear: () -> Void

    var body: some View {
        Button(action: clear) {
            HStack(spacing: 4) {
                Text("Showing: \(name)")
                Image(systemName: "xmark.circle.fill")
            }
            .font(.caption.weight(.semibold))
            .padding(.horizontal, 10)
            .padding(.vertical, 5)
            .background(Color.rpInFlight.opacity(0.15), in: .capsule)
            .foregroundStyle(Color.rpInFlight)
        }
        .accessibilityLabel("Showing only \(name). Clears the filter.")
    }
}

// MARK: - The stage

/// The sky itself: stars, orbit tracks, the sun, and the orbiting bodies.
/// A pure function of (nodes, elapsed time) — all state lives in the parent.
private struct SolarStage: View {
    let nodes: [FleetNode]
    let mode: FleetMode
    @Binding var selectedID: String?
    let onOpen: (FleetNode) -> Void

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    /// t=0 for the orbital clock. Bodies drift from their seeded positions.
    @State private var start = Date()
    /// Entrance reveal: bodies spring in once the stage appears.
    @State private var revealed = false

    private var planets: [SolarLayout.Planet] {
        SolarLayout.planets(
            for: nodes.map { ($0.id, SolarLayout.radius(forLatencyMs: $0.latencyMs)) }
        )
    }

    var body: some View {
        let planets = planets
        let byID = Dictionary(uniqueKeysWithValues: nodes.map { ($0.id, $0) })
        let occupied = Set(planets.map(\.track))

        TimelineView(.animation(minimumInterval: 1 / 30, paused: reduceMotion)) { timeline in
            let elapsed = reduceMotion ? 0 : timeline.date.timeIntervalSince(start)
            GeometryReader { geo in
                let side = min(geo.size.width, geo.size.height)
                let scale = side / SolarLayout.canvasSize
                ZStack {
                    StarField(elapsed: elapsed)
                    latencyAxis(scale: scale)
                    orbitTracks(occupied: occupied, scale: scale)
                    orbitBandLabels(scale: scale)
                    sun(scale: scale)
                    ForEach(Array(planets.enumerated()), id: \.element.id) { index, planet in
                        if let node = byID[planet.id] {
                            let pos = SolarLayout.position(of: planet, at: elapsed)
                            let angle = SolarLayout.angle(of: planet, at: elapsed)
                            let point = CGPoint(x: pos.x * scale, y: pos.y * scale)
                            // Name plates sit radially outward of the body,
                            // flipping inward near the left/right edges so they
                            // stay on stage (matches the web console).
                            let outwardBelow = sin(angle) >= 0
                            let nearEdge = pos.x > 300 || pos.x < 100
                            let plateBelow = nearEdge ? !outwardBelow : outwardBelow
                            PlanetView(
                                node: node, elapsed: elapsed,
                                isSelected: selectedID == node.id,
                                appearDelay: Double(min(index, 12)) * 0.04,
                                revealed: revealed
                            ) {
                                withAnimation(.spring(duration: 0.32, bounce: 0.28)) {
                                    selectedID = selectedID == node.id ? nil : node.id
                                }
                            }
                            .position(point)
                            NamePlate(node: node, emphasized: selectedID == node.id)
                                .opacity(revealed || reduceMotion ? 1 : 0)
                                .position(
                                    x: point.x,
                                    y: point.y + (plateBelow ? 24 : -24)
                                )
                        }
                    }
                }
                .frame(width: side, height: side)
                .frame(maxWidth: .infinity)
            }
            .aspectRatio(1, contentMode: .fit)
        }
        .background(spaceBackground)
        .clipShape(RoundedRectangle(cornerRadius: 16))
        .overlay(RoundedRectangle(cornerRadius: 16).strokeBorder(.black.opacity(0.2)))
        .overlay(alignment: .bottom) {
            if let node = selectedID.flatMap({ id in nodes.first { $0.id == id } }) {
                NodeCard(node: node, mode: mode, onOpen: onOpen) {
                    withAnimation(.spring(duration: 0.28, bounce: 0.2)) {
                        selectedID = nil
                    }
                }
                .padding(10)
                .transition(
                    .asymmetric(
                        insertion: .scale(scale: 0.92).combined(with: .opacity)
                            .combined(with: .move(edge: .bottom)),
                        removal: .scale(scale: 0.96).combined(with: .opacity)
                    )
                )
            }
        }
        .animation(.spring(duration: 0.32, bounce: 0.22), value: selectedID)
        // Space is dark in both appearances, like the web console's stage.
        .environment(\.colorScheme, .dark)
        .contentShape(RoundedRectangle(cornerRadius: 16))
        .onTapGesture {
            withAnimation(.spring(duration: 0.28, bounce: 0.2)) {
                selectedID = nil
            }
        }
        .onAppear {
            guard !reduceMotion else {
                revealed = true
                return
            }
            withAnimation(.spring(response: 0.55, dampingFraction: 0.78)) {
                revealed = true
            }
        }
        .onChange(of: nodes.map(\.id)) { _, _ in
            // Re-entrance when the set of bodies changes (mode / filter).
            if reduceMotion {
                revealed = true
                return
            }
            revealed = false
            withAnimation(.spring(response: 0.55, dampingFraction: 0.78).delay(0.02)) {
                revealed = true
            }
        }
    }

    /// Near-black space with a restrained warm glow at the centre — quiet
    /// enough that the latency rings and bodies carry the eye.
    private var spaceBackground: some View {
        ZStack {
            Color(red: 0.027, green: 0.027, blue: 0.039)
            RadialGradient(
                colors: [Color(red: 0.96, green: 0.73, blue: 0.26).opacity(0.05), .clear],
                center: .center, startRadius: 0, endRadius: 220
            )
        }
    }

    /// Radial latency axis drawn straight up; tick marks at each track.
    private func latencyAxis(scale: CGFloat) -> some View {
        let maxR = (SolarLayout.tracks.last ?? SolarLayout.defaultRadius) + 6
        return ZStack {
            Path { path in
                path.move(to: CGPoint(x: SolarLayout.center * scale, y: SolarLayout.center * scale))
                path.addLine(
                    to: CGPoint(
                        x: SolarLayout.center * scale,
                        y: (SolarLayout.center - maxR) * scale
                    )
                )
            }
            .stroke(.white.opacity(0.1), lineWidth: 0.8)
            ForEach(SolarLayout.tracks, id: \.self) { track in
                Path { path in
                    let y = (SolarLayout.center - track) * scale
                    let cx = SolarLayout.center * scale
                    path.move(to: CGPoint(x: cx - 2.5 * scale, y: y))
                    path.addLine(to: CGPoint(x: cx + 2.5 * scale, y: y))
                }
                .stroke(.white.opacity(0.22), lineWidth: 0.9)
            }
        }
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }

    /// Latency band labels along the axis so "inner = faster" is readable.
    private func orbitBandLabels(scale: CGFloat) -> some View {
        ForEach(SolarLayout.orbitBands, id: \.r) { band in
            Text(band.label)
                .font(.system(size: 9, weight: .medium, design: .monospaced))
                .foregroundStyle(.white.opacity(0.38))
                .position(
                    x: (SolarLayout.center - 28) * scale,
                    y: (SolarLayout.center - band.r) * scale
                )
        }
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }

    /// All four tracks are always drawn; occupied ones take a slightly stronger
    /// stroke so the latency bands read as an axis, not decoration.
    private func orbitTracks(occupied: Set<Double>, scale: CGFloat) -> some View {
        ForEach(SolarLayout.tracks, id: \.self) { track in
            let isOccupied = occupied.contains(track)
            Circle()
                .stroke(
                    .white.opacity(isOccupied ? 0.14 : 0.05),
                    style: StrokeStyle(
                        lineWidth: isOccupied ? 0.9 : 0.6,
                        dash: isOccupied ? [] : [2, 8]
                    )
                )
                .frame(width: track * 2 * scale, height: track * 2 * scale)
        }
    }

    private func sun(scale: CGFloat) -> some View {
        ZStack {
            Circle()
                .fill(
                    RadialGradient(
                        stops: [
                            .init(color: Color(red: 0.16, green: 0.14, blue: 0.09), location: 0),
                            .init(color: Color(red: 0.07, green: 0.07, blue: 0.09), location: 1),
                        ],
                        center: .init(x: 0.42, y: 0.36),
                        startRadius: 0, endRadius: 42 * scale
                    )
                )
                .overlay(
                    Circle().strokeBorder(
                        Color(red: 0.96, green: 0.73, blue: 0.26).opacity(0.28),
                        lineWidth: 1
                    )
                )
                .frame(width: 84 * scale, height: 84 * scale)
            Circle()
                .strokeBorder(
                    Color(red: 0.96, green: 0.73, blue: 0.26).opacity(0.12),
                    lineWidth: 0.6
                )
                .frame(width: 70 * scale, height: 70 * scale)
            VStack(spacing: 1) {
                Text("Rings")
                    .font(.system(size: 11, weight: .semibold))
                    .tracking(1.2)
                    .textCase(.uppercase)
                    .foregroundStyle(Color(red: 0.96, green: 0.73, blue: 0.26))
                Text("\(nodes.count)")
                    .font(.system(size: 15, weight: .medium, design: .monospaced))
                    .foregroundStyle(.white.opacity(0.92))
                Text(bodyWord)
                    .font(.system(size: 9, weight: .medium, design: .monospaced))
                    .textCase(.uppercase)
                    .tracking(0.8)
                    .foregroundStyle(.white.opacity(0.45))
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Rings. \(nodes.count) \(bodyWord).")
    }

    private var bodyWord: String {
        switch mode {
        case .apps: nodes.count == 1 ? "service" : "services"
        case .rings: nodes.count == 1 ? "ring" : "rings"
        }
    }
}

/// Sparse star field — quiet atmosphere so latency rings and bodies dominate.
private struct StarField: View {
    let elapsed: TimeInterval

    var body: some View {
        Canvas { context, size in
            func h(_ n: Int) -> Double {
                Double((n * 9301 + 49297) % 233_280) / 233_280
            }
            for i in 0..<28 {
                let x = h(i * 3 + 1) * size.width
                let y = h(i * 7 + 2) * size.height
                let radius = (0.8 + h(i * 11 + 3) * 1.2) / 2
                let period = 3.5 + h(i * 13 + 5) * 5
                let phase = h(i * 17 + 7)
                let twinkle = 0.55 + 0.45 * sin(2 * .pi * (elapsed / period + phase))
                let rect = CGRect(x: x - radius, y: y - radius, width: radius * 2, height: radius * 2)
                context.fill(
                    Path(ellipseIn: rect),
                    with: .color(.white.opacity(0.04 + 0.18 * twinkle))
                )
            }
        }
        .accessibilityHidden(true)
    }
}

/// One orbiting body: a sphere in the node's status colour. Motion is reserved
/// for states that mean something (deploying spin, failing pulse) plus the
/// staged entrance and selection ring.
private struct PlanetView: View {
    let node: FleetNode
    let elapsed: TimeInterval
    let isSelected: Bool
    let appearDelay: Double
    let revealed: Bool
    let onTap: () -> Void

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private var size: CGFloat {
        node.status == .failed || node.status == .deploying ? 16 : 13
    }

    var body: some View {
        let tint = node.status.tint
        Button(action: onTap) {
            ZStack {
                // Selection halo — scales in with the spring on `isSelected`.
                Circle()
                    .strokeBorder(.white.opacity(isSelected ? 0.85 : 0), lineWidth: 2)
                    .frame(width: size + 12, height: size + 12)
                    .scaleEffect(isSelected ? 1 : 0.7)
                    .opacity(isSelected ? 1 : 0)

                if node.status == .failed || node.status == .loading, !reduceMotion {
                    let pulse = 0.55 + 0.45 * sin(elapsed * 2.4)
                    Circle()
                        .fill(tint.opacity(0.35 * pulse))
                        .frame(width: size + 10, height: size + 10)
                }

                if node.status == .deploying {
                    Circle()
                        .trim(from: 0, to: 0.22)
                        .stroke(tint, style: StrokeStyle(lineWidth: 2.2, lineCap: .round))
                        .frame(width: size + 10, height: size + 10)
                        .rotationEffect(.degrees(reduceMotion ? 0 : elapsed * 225))
                }

                Circle()
                    .fill(
                        RadialGradient(
                            colors: [.white.opacity(0.55), tint, tint.opacity(0.85)],
                            center: .init(x: 0.35, y: 0.3),
                            startRadius: 0, endRadius: size * 0.7
                        )
                    )
                    .frame(width: size, height: size)
                    .overlay(Circle().strokeBorder(.black.opacity(0.35), lineWidth: 1))
                    .shadow(color: tint.opacity(isSelected ? 0.75 : 0.45), radius: isSelected ? 8 : 5)
            }
            .scaleEffect(isSelected ? 1.12 : 1)
            // A comfortable hit target around a small body.
            .frame(width: 44, height: 44)
            .contentShape(Circle())
        }
        .buttonStyle(.plain)
        .scaleEffect(revealed || reduceMotion ? 1 : 0.6)
        .opacity(revealed || reduceMotion ? 1 : 0)
        .animation(
            reduceMotion
                ? nil
                : .spring(response: 0.5, dampingFraction: 0.78).delay(appearDelay),
            value: revealed
        )
        .animation(.spring(duration: 0.32, bounce: 0.28), value: isSelected)
        .accessibilityLabel("\(node.title): \(node.status.label)")
        .accessibilityHint("Shows details")
        .accessibilityIdentifier("planet-\(node.id)")
    }
}

/// The always-visible name pill riding along with its body.
private struct NamePlate: View {
    let node: FleetNode
    var emphasized: Bool = false

    var body: some View {
        HStack(spacing: 3) {
            Text(node.title)
                .lineLimit(1)
            if let ms = node.latencyMs {
                Text("\(ms)ms")
                    .font(.system(size: 9, design: .monospaced))
                    .foregroundStyle(.white.opacity(0.45))
            }
        }
        .font(.system(size: 10, weight: .medium))
        .foregroundStyle(.white.opacity(emphasized ? 0.95 : 0.8))
        .padding(.horizontal, 7)
        .padding(.vertical, 2)
        .background(.black.opacity(0.55), in: .capsule)
        .overlay(Capsule().strokeBorder(.white.opacity(emphasized ? 0.22 : 0.1)))
        .frame(maxWidth: 130)
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }
}

/// The detail card for a selected body — status, latency, deploy facts, and
/// the way in.
private struct NodeCard: View {
    let node: FleetNode
    let mode: FleetMode
    let onOpen: (FleetNode) -> Void
    let onClose: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 10) {
                Circle()
                    .fill(
                        RadialGradient(
                            colors: [.white.opacity(0.5), node.status.tint],
                            center: .init(x: 0.35, y: 0.3),
                            startRadius: 0, endRadius: 12
                        )
                    )
                    .frame(width: 22, height: 22)
                VStack(alignment: .leading, spacing: 0) {
                    Text(node.title).font(.subheadline.weight(.semibold))
                    if let subtitle = node.subtitle {
                        Text(subtitle).font(.caption2).foregroundStyle(.secondary)
                    }
                }
                Spacer()
                Button(action: onClose) {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundStyle(.secondary)
                }
                .accessibilityLabel("Close details")
            }

            VStack(spacing: 5) {
                row("Status") {
                    Label(node.status.label, systemImage: node.status.systemImage)
                        .foregroundStyle(node.status.tint)
                }
                row("Latency") {
                    Text(node.latencyMs.map { "\($0)ms" } ?? "—").monospacedDigit()
                }
                if mode == .apps {
                    row("Version") { Text(node.latestVersion ?? "nothing deployed") }
                    row("Rings") {
                        Text(
                            node.activeCount > 0
                                ? "\(node.healthyCount)/\(node.activeCount) healthy" : "—"
                        )
                    }
                    row("Last deploy") {
                        if let date = node.lastDeploy {
                            Text(date, format: .relative(presentation: .named))
                        } else {
                            Text("never")
                        }
                    }
                } else {
                    row("Apps") { Text(node.subtitle ?? "—") }
                    row("Health") {
                        Text(
                            node.activeCount > 0
                                ? "\(node.healthyCount)/\(node.activeCount) rings healthy"
                                : "nothing deployed"
                        )
                    }
                }
            }
            .font(.caption)

            Button {
                onOpen(node)
            } label: {
                Text(openLabel).frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .controlSize(.small)
        }
        .padding(12)
        .background(.ultraThinMaterial, in: RoundedRectangle(cornerRadius: 14))
        .overlay(RoundedRectangle(cornerRadius: 14).strokeBorder(.white.opacity(0.15)))
        .frame(maxWidth: 280)
        .accessibilityElement(children: .contain)
    }

    private var openLabel: String {
        guard mode == .rings else { return "Open service" }
        return node.id == FleetNode.ungroupedID ? "Show all services" : "Show services"
    }

    private func row(_ label: String, @ViewBuilder value: () -> some View) -> some View {
        HStack {
            Text(label).foregroundStyle(.secondary)
            Spacer()
            value().lineLimit(1)
        }
    }
}

// MARK: - Accessibility fallback

/// The same fleet as plain rows, used at accessibility text sizes where a
/// radial layout cannot be read.
private struct FleetNodeList: View {
    let nodes: [FleetNode]
    let mode: FleetMode
    let onOpen: (FleetNode) -> Void

    var body: some View {
        VStack(spacing: 8) {
            ForEach(nodes) { node in
                Button {
                    onOpen(node)
                } label: {
                    VStack(alignment: .leading, spacing: 6) {
                        Text(node.title).font(.headline)
                        HStack(spacing: 8) {
                            StatusBadge(
                                text: node.status.label,
                                systemImage: node.status.systemImage,
                                tint: node.status.tint
                            )
                            if let ms = node.latencyMs {
                                Text("\(ms)ms")
                                    .font(.caption.monospacedDigit())
                                    .foregroundStyle(.secondary)
                            }
                            if let subtitle = node.subtitle {
                                Text(subtitle)
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(12)
                    .background(.quaternary.opacity(0.4), in: RoundedRectangle(cornerRadius: 12))
                }
                .buttonStyle(.plain)
                .accessibilityHint(
                    mode == .apps
                        ? "Opens the pipeline for \(node.title)"
                        : "Shows the services in \(node.title)"
                )
            }
        }
    }
}

extension FleetStatus {
    /// Design-system colours, so the sky agrees with every other screen.
    var tint: Color {
        switch self {
        case .healthy: .rpHealthy
        case .deploying: .rpInFlight
        case .degraded: .rpGate
        case .failed: .rpUnhealthy
        case .empty: .rpNeutral
        case .loading: .rpDisabled
        }
    }
}
