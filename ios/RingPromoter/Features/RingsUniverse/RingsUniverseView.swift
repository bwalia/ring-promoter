import SwiftUI

/// The Rings of Applications screen: Sun (= Ring Promoter) and a spinning
/// Earth, with one isolated orbital ring per app — unique radius so a busy
/// fleet stays readable. TTFB only orders inner vs outer; two apps never
/// share an ellipse. A persistent roster identifies every service by name.
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
        Group {
            if let error = store.error, store.summaries.isEmpty {
                ErrorRow(error: error) { Task { await store.refresh() } }
                    .padding()
            } else if nodes.isEmpty {
                if store.isLoading {
                    ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
                } else if store.error == nil {
                    CalmEmptyState(
                        title: "No applications",
                        message: "This control plane has no applications configured.",
                        systemImage: "circle.dotted",
                        tint: .rpNeutral
                    )
                }
            } else if typeSize.isAccessibilitySize {
                ScrollView {
                    VStack(alignment: .leading, spacing: 14) {
                        modePicker
                        if let group = filterGroup, mode == .apps {
                            FilterChip(name: group.name) {
                                filterGroupID = nil
                                selectedID = nil
                            }
                        }
                        FleetNodeList(nodes: nodes, mode: mode, onOpen: open(node:))
                    }
                    .padding()
                }
                .refreshable { await store.refresh() }
            } else {
                ZStack {
                    SolarStage(
                        nodes: nodes, mode: mode, selectedID: $selectedID,
                        onOpen: open(node:)
                    )
                    .ignoresSafeArea(edges: .bottom)
                }
                .overlay(alignment: .top) { topChrome }
                .overlay(alignment: .bottom) { bottomChrome }
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button {
                    Task { await store.refresh() }
                } label: {
                    Image(systemName: "arrow.clockwise")
                }
                .accessibilityLabel("Refresh")
            }
        }
        .onChange(of: mode) { _, _ in selectedID = nil }
    }

    private var modePicker: some View {
        Picker("View", selection: $mode) {
            ForEach(FleetMode.allCases) { mode in
                Text(mode.label).tag(mode)
            }
        }
        .pickerStyle(.segmented)
    }

    private var topChrome: some View {
        VStack(alignment: .leading, spacing: 8) {
            if store.isStale {
                Label(
                    "Showing the last data this app was able to load.",
                    systemImage: "wifi.exclamationmark"
                )
                .font(.subheadline)
                .foregroundStyle(Color.rpGate)
            }
            modePicker
            if let group = filterGroup, mode == .apps {
                FilterChip(name: group.name) {
                    filterGroupID = nil
                    selectedID = nil
                }
            }
        }
        .padding(.horizontal, 12)
        .padding(.top, 8)
        .padding(.bottom, 10)
        .background(
            LinearGradient(
                colors: [Color.black.opacity(0.55), Color.black.opacity(0)],
                startPoint: .top, endPoint: .bottom
            )
            .ignoresSafeArea(edges: .top)
            .allowsHitTesting(false)
        )
        .environment(\.colorScheme, .dark)
    }

    private var bottomChrome: some View {
        VStack(spacing: 8) {
            FleetRoster(
                nodes: rosterNodes, mode: mode, selectedID: $selectedID,
                onOpen: open(node:)
            )
            .frame(maxHeight: 168)
            legend
            if let lastUpdated = store.lastUpdated {
                RelativeTimestamp(date: lastUpdated, prefix: "Updated")
                    .frame(maxWidth: .infinity, alignment: .center)
            }
        }
        .padding(.horizontal, 12)
        .padding(.top, 10)
        .padding(.bottom, 8)
        .background(.ultraThinMaterial, in: UnevenRoundedRectangle(
            topLeadingRadius: 16, topTrailingRadius: 16
        ))
        .environment(\.colorScheme, .dark)
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

    /// Inner (faster TTFB) first, matching isolated ring order.
    private var rosterNodes: [FleetNode] {
        nodes.sorted { a, b in
            let ma = a.ttfbMs ?? a.latencyMs
            let mb = b.ttfbMs ?? b.latencyMs
            switch (ma, mb) {
            case (nil, nil):
                return a.title.localizedCaseInsensitiveCompare(b.title) == .orderedAscending
            case (nil, _): return false
            case (_, nil): return true
            case let (x?, y?) where x != y: return x < y
            default:
                return a.title.localizedCaseInsensitiveCompare(b.title) == .orderedAscending
            }
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
            Text("Ring Promoter · Earth · one ring per \(mode == .apps ? "app" : "group") · closer = lower TTFB")
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

/// The sky itself: Earth, one orbital ring per app, and the orbiting bodies.
/// A pure function of (nodes, elapsed time) — all state lives in the parent.
private struct SolarStage: View {
    let nodes: [FleetNode]
    let mode: FleetMode
    @Binding var selectedID: String?
    let onOpen: (FleetNode) -> Void

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    /// t=0 for the orbital clock. Earth spin and satellite drift start here.
    @State private var start = Date()
    /// Entrance reveal: bodies spring in once the stage appears.
    @State private var revealed = false

    private func bodies(metrics: SolarLayout.GlobeMetrics) -> [SolarLayout.GlobeBody] {
        SolarLayout.globeBodies(for: nodes, metrics: metrics)
    }

    private var crowded: Bool { nodes.count > SolarLayout.densityCap }

    var body: some View {
        let byID = Dictionary(uniqueKeysWithValues: nodes.map { ($0.id, $0) })

        TimelineView(.animation(minimumInterval: 1.0 / 60, paused: reduceMotion)) { timeline in
            let elapsed = reduceMotion ? 0 : timeline.date.timeIntervalSince(start)
            let spin = SolarLayout.earthSpin(elapsed: elapsed, reduceMotion: reduceMotion)
            GeometryReader { geo in
                let metrics = SolarLayout.GlobeMetrics(size: geo.size)
                let bodies = bodies(metrics: metrics)
                let focusBody = selectedID.flatMap { id in bodies.first { $0.id == id } }
                let focusPos = focusBody.map {
                    SolarLayout.point(of: $0, elapsed: elapsed, spin: spin, metrics: metrics)
                }
                let camScale: CGFloat = (selectedID != nil && !reduceMotion) ? 1.12 : 1
                let camX = (focusPos.map { (metrics.cx - $0.x) / metrics.width * 18 } ?? 0)
                let camY = (focusPos.map { (metrics.cy - $0.y) / metrics.height * 18 } ?? 0)
                ZStack {
                    StarField(elapsed: elapsed)
                    orbitRings(
                        bodies: bodies, spin: spin, metrics: metrics, front: false,
                        byID: byID, selectedID: selectedID, crowded: crowded
                    )
                    EarthGlobe(spin: spin, metrics: metrics)
                    SunHub(metrics: metrics)
                    orbitRings(
                        bodies: bodies, spin: spin, metrics: metrics, front: true,
                        byID: byID, selectedID: selectedID, crowded: crowded
                    )
                    stems(
                        bodies: bodies, elapsed: elapsed, spin: spin, metrics: metrics,
                        selectedID: selectedID
                    )
                    ForEach(Array(bodies.enumerated()), id: \.element.id) { index, body in
                        if let node = byID[body.id] {
                            let pos = SolarLayout.point(
                                of: body, elapsed: elapsed, spin: spin, metrics: metrics
                            )
                            let point = CGPoint(x: pos.x, y: pos.y)
                            let plateBelow = pos.y >= metrics.cy
                            let dim = selectedID != nil && selectedID != node.id
                            let showPlate = !crowded || selectedID == node.id
                            PlanetView(
                                node: node, elapsed: elapsed,
                                isSelected: selectedID == node.id,
                                appearDelay: Double(min(index, 12)) * 0.04,
                                revealed: revealed,
                                placed: body.placed
                            ) {
                                withAnimation(.spring(duration: 0.32, bounce: 0.28)) {
                                    selectedID = selectedID == node.id ? nil : node.id
                                }
                            }
                            .opacity(dim ? (pos.front ? 0.28 : 0.1) : (pos.front || selectedID == node.id ? 1 : 0.28))
                            .zIndex(pos.front ? 20 + pos.z : 2)
                            .allowsHitTesting(pos.front || selectedID == node.id)
                            .position(point)
                            NamePlate(node: node, emphasized: selectedID == node.id)
                                .opacity(
                                    (revealed || reduceMotion)
                                        && (pos.front || selectedID == node.id)
                                        && showPlate
                                        ? 1 : 0
                                )
                                .position(
                                    x: point.x,
                                    y: point.y + (plateBelow ? 24 : -24)
                                )
                        }
                    }
                }
                .scaleEffect(camScale)
                .offset(x: camX, y: camY)
                .animation(
                    reduceMotion ? nil : .easeOut(duration: 0.55),
                    value: selectedID
                )
                .frame(width: geo.size.width, height: geo.size.height)
            }
        }
        .background(spaceBackground)
        .overlay(alignment: .center) {
            if let node = selectedID.flatMap({ id in nodes.first { $0.id == id } }) {
                NodeCard(
                    node: node, mode: mode,
                    estimatedMs: estimatedTTFB(for: node),
                    onOpen: onOpen
                ) {
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
        .contentShape(Rectangle())
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

    /// Near-black space with a restrained atmosphere at the centre.
    private var spaceBackground: some View {
        ZStack {
            Color(red: 0.027, green: 0.027, blue: 0.039)
            RadialGradient(
                colors: [Color(red: 0.22, green: 0.74, blue: 0.97).opacity(0.06), .clear],
                center: .center, startRadius: 0, endRadius: 420
            )
        }
    }

    private func orbitRings(
        bodies: [SolarLayout.GlobeBody],
        spin: Double,
        metrics: SolarLayout.GlobeMetrics,
        front: Bool,
        byID: [String: FleetNode],
        selectedID: String?,
        crowded: Bool
    ) -> some View {
        Canvas { context, _ in
            let sw = metrics.strokeScale
            for body in bodies {
                let pts = SolarLayout.sampleOrbit(body, spin: spin, metrics: metrics)
                let lit = selectedID == body.id
                let dim = selectedID != nil && selectedID != body.id
                let tint = byID[body.id]?.status.tint ?? Color(white: 0.5)
                var path = Path()
                var started = false
                for p in pts {
                    guard p.front == front else {
                        started = false
                        continue
                    }
                    let pt = CGPoint(x: p.x, y: p.y)
                    if started {
                        path.addLine(to: pt)
                    } else {
                        path.move(to: pt)
                        started = true
                    }
                }
                let opacity: Double
                if front {
                    opacity = dim ? (crowded ? 0.1 : 0.18) : (lit ? 0.95 : (crowded ? 0.4 : 0.58))
                } else {
                    opacity = dim ? 0.05 : (lit ? 0.42 : 0.18)
                }
                let width: CGFloat
                if front {
                    width = (lit ? 2.2 : (crowded ? 1.05 : 1.25)) * sw
                } else {
                    width = (lit ? 1.3 : 0.85) * sw
                }
                context.stroke(
                    path,
                    with: .color(tint.opacity(opacity)),
                    style: StrokeStyle(
                        lineWidth: width,
                        lineCap: .round,
                        lineJoin: .round
                    )
                )
            }
        }
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }

    private func stems(
        bodies: [SolarLayout.GlobeBody], elapsed: TimeInterval, spin: Double,
        metrics: SolarLayout.GlobeMetrics, selectedID: String?
    ) -> some View {
        Canvas { context, _ in
            for body in bodies {
                let sat = SolarLayout.point(of: body, elapsed: elapsed, spin: spin, metrics: metrics)
                let ground = SolarLayout.surface(of: body, elapsed: elapsed, spin: spin, metrics: metrics)
                let dim = selectedID != nil && selectedID != body.id
                var path = Path()
                path.move(to: CGPoint(x: ground.x, y: ground.y))
                path.addLine(to: CGPoint(x: sat.x, y: sat.y))
                context.stroke(
                    path,
                    with: .color(Color(red: 0.49, green: 0.83, blue: 0.99).opacity(
                        dim ? 0.04 : (sat.front ? 0.28 : 0.06)
                    )),
                    lineWidth: 0.7 * metrics.strokeScale
                )
            }
        }
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }

    private func estimatedTTFB(for node: FleetNode) -> Int? {
        guard let loc = node.location,
              let centroid = SolarLayout.centroid(of: nodes.compactMap(\.location))
        else { return nil }
        return SolarLayout.estimateRttMs(km: SolarLayout.haversineKm(loc, centroid))
    }
}

/// Orthographic Earth matching the web console canvas: ocean, graticule,
/// coarse land, camera-fixed terminator and specular. Continents rotating
/// under that lighting is what reads as a sphere instead of a flat disc.
/// Spin is the only input that moves; reduced-motion callers pass 0.
private struct EarthGlobe: View {
    let spin: Double
    let metrics: SolarLayout.GlobeMetrics

    var body: some View {
        Canvas { context, _ in
            let s = metrics.strokeScale
            let c = CGPoint(x: metrics.cx, y: metrics.cy)
            let r = metrics.earthR
            let earth = Path(ellipseIn: CGRect(x: c.x - r, y: c.y - r, width: r * 2, height: r * 2))

            // Atmosphere
            context.fill(
                Path(ellipseIn: CGRect(
                    x: c.x - r * 1.28, y: c.y - r * 1.28,
                    width: r * 2.56, height: r * 2.56
                )),
                with: .radialGradient(
                    Gradient(stops: [
                        .init(color: Color(red: 56 / 255, green: 189 / 255, blue: 248 / 255).opacity(0), location: 0),
                        .init(color: Color(red: 56 / 255, green: 189 / 255, blue: 248 / 255).opacity(0.07), location: 0.72),
                        .init(color: .clear, location: 1),
                    ]),
                    center: c, startRadius: r * 0.92, endRadius: r * 1.28
                )
            )

            context.drawLayer { ctx in
                ctx.clip(to: earth)

                // Ocean — lighting is camera-fixed so the globe reads as a sphere.
                ctx.fill(
                    earth,
                    with: .radialGradient(
                        Gradient(stops: [
                            .init(color: Color(red: 28 / 255, green: 74 / 255, blue: 110 / 255), location: 0),
                            .init(color: Color(red: 13 / 255, green: 42 / 255, blue: 68 / 255), location: 0.45),
                            .init(color: Color(red: 7 / 255, green: 20 / 255, blue: 31 / 255), location: 1),
                        ]),
                        center: CGPoint(x: c.x - r * 0.28, y: c.y - r * 0.34),
                        startRadius: r * 0.08, endRadius: r
                    )
                )

                var grid = Path()
                for lng in stride(from: -180.0, to: 180.0, by: 30) {
                    var started = false
                    for lat in stride(from: -90.0, through: 90.0, by: 4) {
                        let p = SolarLayout.projectOrtho(
                            latDeg: lat, lngDeg: lng, radius: metrics.earthR, spin: spin,
                            metrics: metrics
                        )
                        guard p.front else { started = false; continue }
                        let pt = CGPoint(x: p.x, y: p.y)
                        if started { grid.addLine(to: pt) } else { grid.move(to: pt); started = true }
                    }
                }
                for lat in stride(from: -60.0, through: 60.0, by: 30) {
                    var started = false
                    for lng in stride(from: -180.0, through: 180.0, by: 5) {
                        let p = SolarLayout.projectOrtho(
                            latDeg: lat, lngDeg: lng, radius: metrics.earthR, spin: spin,
                            metrics: metrics
                        )
                        guard p.front else { started = false; continue }
                        let pt = CGPoint(x: p.x, y: p.y)
                        if started { grid.addLine(to: pt) } else { grid.move(to: pt); started = true }
                    }
                }
                ctx.stroke(
                    grid,
                    with: .color(Color(red: 186 / 255, green: 230 / 255, blue: 253 / 255).opacity(0.11)),
                    lineWidth: 0.55 * s
                )

                let landFill = Color(red: 134 / 255, green: 168 / 255, blue: 128 / 255).opacity(0.78)
                let landStroke = Color(red: 190 / 255, green: 210 / 255, blue: 170 / 255).opacity(0.18)
                for poly in SolarLayout.landPolys {
                    var path = Path()
                    var started = false
                    var frontCount = 0
                    for pt in poly {
                        let p = SolarLayout.projectOrtho(
                            latDeg: pt.lat, lngDeg: pt.lng, radius: metrics.earthR, spin: spin,
                            metrics: metrics
                        )
                        guard p.front else { started = false; continue }
                        frontCount += 1
                        let cg = CGPoint(x: p.x, y: p.y)
                        if started { path.addLine(to: cg) } else { path.move(to: cg); started = true }
                    }
                    guard frontCount >= 3 else { continue }
                    path.closeSubpath()
                    ctx.fill(path, with: .color(landFill))
                    ctx.stroke(path, with: .color(landStroke), lineWidth: 0.4 * s)
                }

                // Terminator / night side — the cue that this is a globe, not a disc.
                let night = Color(red: 2 / 255, green: 6 / 255, blue: 12 / 255)
                ctx.fill(
                    earth,
                    with: .linearGradient(
                        Gradient(stops: [
                            .init(color: night.opacity(0.22), location: 0),
                            .init(color: night.opacity(0), location: 0.42),
                            .init(color: night.opacity(0), location: 0.62),
                            .init(color: night.opacity(0.55), location: 1),
                        ]),
                        startPoint: CGPoint(x: c.x - r, y: c.y),
                        endPoint: CGPoint(x: c.x + r, y: c.y)
                    )
                )

                // Specular highlight on the ocean.
                let specCenter = CGPoint(x: c.x - r * 0.32, y: c.y - r * 0.4)
                ctx.fill(
                    Path(ellipseIn: CGRect(
                        x: specCenter.x - r * 0.55, y: specCenter.y - r * 0.55,
                        width: r * 1.1, height: r * 1.1
                    )),
                    with: .radialGradient(
                        Gradient(stops: [
                            .init(color: .white.opacity(0.22), location: 0),
                            .init(color: Color(red: 186 / 255, green: 230 / 255, blue: 253 / 255).opacity(0.06), location: 0.35),
                            .init(color: .clear, location: 1),
                        ]),
                        center: specCenter, startRadius: 0, endRadius: r * 0.55
                    )
                )
            }

            context.stroke(
                earth,
                with: .color(Color(red: 125 / 255, green: 211 / 255, blue: 252 / 255).opacity(0.28)),
                lineWidth: 1.1 * s
            )
            context.stroke(
                Path(ellipseIn: CGRect(
                    x: c.x - r - 1.6 * s, y: c.y - r - 1.6 * s,
                    width: (r + 1.6 * s) * 2, height: (r + 1.6 * s) * 2
                )),
                with: .color(Color(red: 245 / 255, green: 185 / 255, blue: 66 / 255).opacity(0.12)),
                lineWidth: 0.7 * s
            )
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Earth. \(nodesCaption)")
    }

    private var nodesCaption: String { "Applications ride isolated orbital rings around Earth." }
}

/// Ring Promoter hub to Earth's left — the branded sun in this two-body sky.
private struct SunHub: View {
    let metrics: SolarLayout.GlobeMetrics

    var body: some View {
        let r = max(18, metrics.sunR)
        let fontSize = max(7, min(11, r * 0.29))
        ZStack {
            Circle()
                .fill(
                    RadialGradient(
                        colors: [
                            Color(red: 1, green: 0.97, blue: 0.84),
                            Color(red: 0.96, green: 0.73, blue: 0.26),
                            Color(red: 0.76, green: 0.25, blue: 0.05),
                        ],
                        center: .init(x: 0.35, y: 0.32),
                        startRadius: 0, endRadius: r
                    )
                )
                .shadow(color: Color(red: 0.96, green: 0.73, blue: 0.26).opacity(0.45), radius: 12)
            VStack(spacing: 0) {
                Text("Ring")
                Text("Promoter")
            }
            .font(.system(size: fontSize, weight: .semibold, design: .rounded))
            .foregroundStyle(Color(red: 0.23, green: 0.1, blue: 0.015).opacity(0.9))
            .multilineTextAlignment(.center)
            .lineSpacing(-1)
            .tracking(0.4)
            .textCase(.uppercase)
            .allowsHitTesting(false)
        }
        .frame(width: r * 2, height: r * 2)
        .position(
            x: metrics.cx + metrics.sunOffsetX,
            y: metrics.cy
        )
        .allowsHitTesting(false)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Ring Promoter")
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
    var placed: Bool = false
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
        .accessibilityLabel(placedLabel)
        .accessibilityHint("Shows details")
        .accessibilityIdentifier("planet-\(node.id)")
    }

    private var placedLabel: String {
        if placed, let loc = node.location {
            return "\(node.title) in \(loc.label): \(node.status.label)"
        }
        return "\(node.title): \(node.status.label)"
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
            if let ms = node.ttfbMs ?? node.latencyMs {
                Text("\(ms)ms TTFB")
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
    var estimatedMs: Int? = nil
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
                if let loc = node.location {
                    row("Location") { Text(loc.label) }
                }
                row("TTFB") {
                    if let ms = node.ttfbMs {
                        Text("\(ms)ms").monospacedDigit()
                    } else if let est = estimatedMs {
                        Text("~\(est)ms est.").monospacedDigit()
                    } else {
                        Text("—")
                    }
                }
                row("Check") {
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
                    if node.apps.count > 1 {
                        Text(node.apps.joined(separator: " · "))
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
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

// MARK: - Persistent roster

/// Always-visible identification: name, health, TTFB, location. Tap focuses
/// that ring; the globe is never the only way to tell apps apart.
private struct FleetRoster: View {
    let nodes: [FleetNode]
    let mode: FleetMode
    @Binding var selectedID: String?
    let onOpen: (FleetNode) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(mode == .rings ? "Rings" : "Applications")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
                .textCase(.uppercase)
            ScrollView {
                VStack(spacing: 4) {
                ForEach(nodes) { node in
                    Button {
                        withAnimation(.spring(duration: 0.32, bounce: 0.22)) {
                            selectedID = selectedID == node.id ? nil : node.id
                        }
                    } label: {
                        HStack(alignment: .top, spacing: 8) {
                            Circle()
                                .fill(node.status.tint)
                                .frame(width: 8, height: 8)
                                .padding(.top, 5)
                            VStack(alignment: .leading, spacing: 2) {
                                Text(node.title)
                                    .font(.subheadline.weight(.medium))
                                    .foregroundStyle(.primary)
                                    .lineLimit(1)
                                HStack(spacing: 8) {
                                    Text(node.status.label)
                                        .foregroundStyle(node.status.tint)
                                    if let ms = node.ttfbMs ?? node.latencyMs {
                                        Text("\(ms)ms TTFB").monospacedDigit()
                                    } else {
                                        Text("—")
                                    }
                                    if let loc = node.location {
                                        Text(loc.label).lineLimit(1)
                                    }
                                    if let subtitle = node.subtitle {
                                        Text(subtitle)
                                    }
                                }
                                .font(.caption2)
                                .foregroundStyle(.secondary)
                                if selectedID == node.id, mode == .rings, node.apps.count > 1 {
                                    Text(node.apps.joined(separator: " · "))
                                        .font(.caption2)
                                        .foregroundStyle(.secondary)
                                }
                            }
                            Spacer(minLength: 0)
                        }
                        .padding(.horizontal, 10)
                        .padding(.vertical, 8)
                        .background(
                            RoundedRectangle(cornerRadius: 10)
                                .fill(selectedID == node.id ? Color.white.opacity(0.08) : Color.white.opacity(0.03))
                        )
                    }
                    .buttonStyle(.plain)
                    .accessibilityHint("Focuses this ring. Opens details from the card.")
                    .accessibilityIdentifier("roster-\(node.id)")
                    .simultaneousGesture(
                        TapGesture(count: 2).onEnded { onOpen(node) }
                    )
                }
                }
            }
        }
        .environment(\.colorScheme, .dark)
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
                            if let loc = node.location {
                                Text(loc.label)
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                            if let ms = node.ttfbMs ?? node.latencyMs {
                                Text("\(ms)ms TTFB")
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
