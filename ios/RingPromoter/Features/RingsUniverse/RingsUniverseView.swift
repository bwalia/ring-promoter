import SwiftUI

/// The Rings of Applications screen — "Descent".
///
/// The promotion rings are the orbits around Earth (live users): the first
/// ring (int) is outermost, the last (prod) sits just above Earth. Every app
/// owns one straight spoke; a lit trail on it shows how far the app's newest
/// version has travelled toward production. Geometry lives in
/// `DescentLayout` (a port of the web console's `descent-layout.ts`).
///
/// Where the orbit can't be read — a narrow stage (iPhone portrait) or a fleet
/// past `DescentLayout.maxSpokes` — the same data renders as "lanes": one row
/// per app, one column per ring.
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
            // The console is near-black in both appearances; keep the title
            // and status bar legible on it.
            .toolbarBackground(DescentPalette.space, for: .navigationBar)
            .toolbarBackground(.visible, for: .navigationBar)
            .toolbarColorScheme(.dark, for: .navigationBar)
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

/// How the fleet is drawn: the Descent orbit, or the lanes fallback.
enum DescentMode: String, CaseIterable, Identifiable {
    case orbit, lanes

    var id: String { rawValue }

    var label: String {
        switch self {
        case .orbit: "Orbit"
        case .lanes: "Lanes"
        }
    }
}

/// Near-black console colours shared by the stage and the lanes.
enum DescentPalette {
    static let space = Color(red: 0.027, green: 0.027, blue: 0.039)
    static let hairline = Color.white.opacity(0.13)
    /// Group arcs cycle through existing design tokens — never new colours.
    static let groups: [Color] = [.rpInFlight, .rpProduction, .rpNeutral]

    static func group(_ index: Int) -> Color { groups[index % groups.count] }
}

/// Everything below the navigation bar, split out so the loading branch above
/// stays readable.
private struct RingsUniverseContent: View {
    @Bindable var store: OverviewStore
    @Environment(AppSession.self) private var session
    @Environment(Router.self) private var router
    @Environment(\.dynamicTypeSize) private var typeSize
    /// The user's choice; overridden by lanes when the orbit doesn't fit.
    @State private var preferredMode: DescentMode = .orbit
    /// Group filter, set by tapping a group's arc or lane header.
    @State private var filterGroupID: String?
    @State private var selectedID: String?

    var body: some View {
        Group {
            if let error = store.error, store.summaries.isEmpty {
                ErrorRow(error: error) { Task { await store.refresh() } }
                    .padding()
            } else if store.summaries.isEmpty {
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
                        if let group = filterGroup {
                            FilterChip(name: group.name, clear: clearFilter)
                        }
                        AccessibleFleetList(apps: layout.apps) { router.show(app: $0.id) }
                    }
                    .padding()
                }
                .refreshable { await store.refresh() }
            } else {
                GeometryReader { geo in
                    let fits = DescentLayout.descentFits(
                        stageWidth: geo.size.width, spokes: layout.apps.count
                    )
                    let mode: DescentMode = fits ? preferredMode : .lanes
                    Group {
                        switch mode {
                        case .orbit: orbit(fits: fits)
                        case .lanes: lanes(fits: fits, width: geo.size.width)
                        }
                    }
                    .frame(width: geo.size.width, height: geo.size.height)
                }
                .background(DescentPalette.space.ignoresSafeArea())
                .environment(\.colorScheme, .dark)
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
        .onChange(of: filterGroupID) { _, _ in selectedID = nil }
    }

    // MARK: - Data

    private var filterGroup: AppGroup? {
        guard let filterGroupID else { return nil }
        return store.groups.first { $0.id == filterGroupID }
    }

    /// Canonical promotion order from `/api/apps`; falls back to the first
    /// app's reported rings before capabilities load.
    private var ringOrder: [String] {
        if let rings = session.capabilities?.rings, !rings.isEmpty { return rings.map(\.name) }
        return store.summaries.first { !$0.rings.isEmpty }?.rings.map(\.ring.name) ?? []
    }

    private var layout: (apps: [DescentApp], spans: [DescentLayout.GroupSpan]) {
        var summaries = store.summaries
        if let group = filterGroup {
            summaries = summaries.filter { group.contains($0.name) }
        }
        return DescentApp.ordered(summaries: summaries, groups: store.groups, order: ringOrder)
    }

    private var selectedApp: DescentApp? {
        guard let selectedID else { return nil }
        return layout.apps.first { $0.id == selectedID }
    }

    private func groupName(for app: DescentApp) -> String? {
        let l = layout
        guard let index = l.apps.firstIndex(where: { $0.id == app.id }) else { return nil }
        return l.spans.first { index >= $0.from && index <= $0.to }?.name
    }

    // MARK: - Actions

    private func toggleSelection(_ id: String) {
        withAnimation(.spring(duration: 0.28, bounce: 0.18)) {
            selectedID = selectedID == id ? nil : id
        }
    }

    private func clearSelection() {
        withAnimation(.spring(duration: 0.28, bounce: 0.18)) { selectedID = nil }
    }

    /// Open a group: show only its members. Opening the open group clears it.
    private func openGroup(_ id: String) {
        filterGroupID = filterGroupID == id ? nil : id
    }

    private func clearFilter() { filterGroupID = nil }

    // MARK: - Orbit

    private func orbit(fits: Bool) -> some View {
        let l = layout
        let selected = selectedApp
        return ZStack {
            DescentStage(
                apps: l.apps, spans: l.spans, ringNames: ringOrder,
                selectedID: selectedID, topInset: 8, bottomInset: 8,
                onSelect: toggleSelection, onDeselect: clearSelection, onOpenGroup: openGroup
            )
        }
        // Chrome lives in the corners so the circle can use the full height.
        .overlay(alignment: .topLeading) { chrome(fits: fits).padding(.leading, 12).padding(.top, 8) }
        .overlay(alignment: .bottomLeading) {
            summaryColumn.padding(.leading, 12).padding(.bottom, 6)
        }
        .overlay(alignment: .bottomTrailing) {
            legend.padding(.trailing, 12).padding(.bottom, 6)
        }
        .overlay(alignment: cardAlignment(for: selected, in: l.apps)) {
            if let selected {
                SpokeCard(
                    app: selected, groupName: groupName(for: selected),
                    onOpen: { router.show(app: selected.id) }, onClose: clearSelection
                )
                .frame(maxWidth: 300)
                .padding(.horizontal, 10)
                .padding(.vertical, 8)
                .transition(.opacity.combined(with: .scale(scale: 0.96)))
            }
        }
    }

    /// Put the card on the side away from the selected spoke's label.
    private func cardAlignment(for app: DescentApp?, in apps: [DescentApp]) -> Alignment {
        guard let app, let index = apps.firstIndex(where: { $0.id == app.id }) else {
            return .trailing
        }
        let angle = DescentLayout.spokeAngle(index: index, count: apps.count)
        return cos(angle) > 0 ? .leading : .trailing
    }

    // MARK: - Lanes

    private func lanes(fits: Bool, width: Double) -> some View {
        let l = layout
        return VStack(spacing: 0) {
            chrome(fits: fits)
                .padding(.horizontal, 12)
                .padding(.vertical, 8)
            LanesView(
                apps: l.apps, spans: l.spans, ringNames: ringOrder, width: width,
                selectedID: selectedID, filtering: filterGroupID != nil,
                onSelect: toggleSelection, onOpenGroup: openGroup
            ) {
                summaryLine.padding(.top, 8)
            }
            .refreshable { await store.refresh() }
        }
        .safeAreaInset(edge: .bottom) {
            if let selected = selectedApp {
                SpokeCard(
                    app: selected, groupName: groupName(for: selected),
                    onOpen: { router.show(app: selected.id) }, onClose: clearSelection
                )
                .frame(maxWidth: 520)
                .padding(.horizontal, 10)
                .padding(.bottom, 6)
                .transition(.move(edge: .bottom).combined(with: .opacity))
            }
        }
    }

    // MARK: - Chrome

    private func chrome(fits: Bool) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            if store.isStale {
                Label(
                    "Showing the last data this app was able to load.",
                    systemImage: "wifi.exclamationmark"
                )
                .font(.caption)
                .foregroundStyle(Color.rpGate)
            }
            HStack(spacing: 8) {
                ModeSwitch(
                    selection: fits ? preferredMode : .lanes, orbitEnabled: fits
                ) { preferredMode = $0 }
                if let group = filterGroup {
                    FilterChip(name: group.name, clear: clearFilter)
                }
                Spacer(minLength: 0)
            }
        }
    }

    private var legend: some View {
        VStack(alignment: .trailing, spacing: 2) {
            Text("solid = newest version")
            Text("outline = older version")
            Text("amber bar = gate closed")
            Text("Earth = live users")
        }
        .font(.system(size: 10))
        .foregroundStyle(.white.opacity(0.5))
        .accessibilityHidden(true)
    }

    /// The summary stacked for a corner of the orbit.
    private var summaryColumn: some View {
        let apps = layout.apps
        let inProd = apps.count { $0.spoke.frontier >= 0 && $0.spoke.frontier == $0.spoke.nodes.count - 1 }
        let gated = apps.count { $0.spoke.gateAt >= 0 }
        let attention = apps.count { $0.status == .failed || $0.status == .degraded }
        return VStack(alignment: .leading, spacing: 2) {
            Text("\(inProd)/\(apps.count) newest in prod")
            Text("\(gated) gated")
                .foregroundStyle(gated > 0 ? Color.rpGate : .secondary)
            Text("\(attention) need attention")
                .foregroundStyle(attention > 0 ? Color.rpUnhealthy : .secondary)
            if let lastUpdated = store.lastUpdated {
                RelativeTimestamp(date: lastUpdated, prefix: "Updated")
            }
        }
        .font(.caption2.monospacedDigit())
        .foregroundStyle(.secondary)
        .accessibilityElement(children: .combine)
    }

    private var summaryLine: some View {
        let apps = layout.apps
        let inProd = apps.count { $0.spoke.frontier >= 0 && $0.spoke.frontier == $0.spoke.nodes.count - 1 }
        let gated = apps.count { $0.spoke.gateAt >= 0 }
        let attention = apps.count { $0.status == .failed || $0.status == .degraded }
        return HStack(spacing: 10) {
            Text("\(inProd)/\(apps.count) newest in prod")
            Text("\(gated) gated")
                .foregroundStyle(gated > 0 ? Color.rpGate : .secondary)
            Text("\(attention) need attention")
                .foregroundStyle(attention > 0 ? Color.rpUnhealthy : .secondary)
            if let lastUpdated = store.lastUpdated {
                Spacer(minLength: 4)
                RelativeTimestamp(date: lastUpdated, prefix: "Updated")
            }
        }
        .font(.caption2.monospacedDigit())
        .foregroundStyle(.secondary)
        .lineLimit(1)
        .minimumScaleFactor(0.8)
        .accessibilityElement(children: .combine)
    }
}

/// "Orbit | Lanes". Orbit is disabled when the stage is too narrow or the
/// fleet too large to read as spokes.
private struct ModeSwitch: View {
    let selection: DescentMode
    let orbitEnabled: Bool
    let onChange: (DescentMode) -> Void

    var body: some View {
        HStack(spacing: 2) {
            ForEach(DescentMode.allCases) { mode in
                let enabled = mode == .lanes || orbitEnabled
                Button {
                    onChange(mode)
                } label: {
                    Text(mode.label)
                        .font(.caption.weight(.semibold))
                        .padding(.horizontal, 12)
                        .padding(.vertical, 5)
                        .foregroundStyle(
                            selection == mode ? Color.primary
                                : enabled ? Color.secondary : Color.rpDisabled
                        )
                        .background(
                            Capsule().fill(selection == mode ? Color.white.opacity(0.14) : .clear)
                        )
                }
                .buttonStyle(.plain)
                .disabled(!enabled)
                .accessibilityAddTraits(selection == mode ? .isSelected : [])
                .accessibilityHint(enabled ? "" : "Needs a wider screen or fewer apps")
                .accessibilityIdentifier("mode-\(mode.rawValue)")
            }
        }
        .padding(2)
        .background(Capsule().fill(Color.white.opacity(0.06)))
        .overlay(Capsule().strokeBorder(Color.white.opacity(0.1)))
    }
}

/// "Showing: Payments ✕" — the active group filter.
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

// MARK: - The orbit stage

/// Maps the 1000×1000 design canvas (origin at centre, +y down) onto the view.
struct DescentFrame: Equatable {
    var center: CGPoint
    var scale: Double

    func point(_ p: CGPoint) -> CGPoint {
        CGPoint(x: center.x + p.x * scale, y: center.y + p.y * scale)
    }

    func polar(_ angle: Double, _ r: Double) -> CGPoint {
        point(DescentLayout.polar(angle, r))
    }

    /// Fit the rings plus label room into `size`, leaving the chrome insets
    /// clear. Labels need ~150 pt beside the circle and ~36 pt under it.
    static func fit(size: CGSize, topInset: Double, bottomInset: Double) -> DescentFrame {
        let labelSide = 150.0, labelBelow = 36.0, labelAbove = 12.0
        let r = DescentLayout.labelR
        let availH = max(1, size.height - topInset - bottomInset)
        let s = max(0.12, min(
            size.width / DescentLayout.design,
            (size.width - 2 * labelSide) / (2 * r),
            (availH - labelBelow - labelAbove) / (2 * r)
        ))
        let used = 2 * r * s + labelAbove + labelBelow
        let cy = topInset + max(0, (availH - used) / 2) + labelAbove + r * s
        return DescentFrame(center: CGPoint(x: size.width / 2, y: cy), scale: s)
    }
}

/// The orbit: rings, spokes, trails, gates, group arcs, labels, Earth and the
/// comets. Everything static is one Canvas; only Earth and comets redraw per
/// frame, and neither does under Reduce Motion.
private struct DescentStage: View {
    let apps: [DescentApp]
    let spans: [DescentLayout.GroupSpan]
    let ringNames: [String]
    let selectedID: String?
    let topInset: Double
    let bottomInset: Double
    let onSelect: (String) -> Void
    let onDeselect: () -> Void
    let onOpenGroup: (String) -> Void

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var start = Date()

    private var ringCount: Int { max(1, ringNames.count) }

    var body: some View {
        GeometryReader { geo in
            let frame = DescentFrame.fit(size: geo.size, topInset: topInset, bottomInset: bottomInset)
            ZStack {
                StarField(elapsed: 0)
                staticLayer(frame: frame)
                TimelineView(.animation(minimumInterval: 1.0 / 30, paused: reduceMotion)) { timeline in
                    let elapsed = reduceMotion ? 0 : timeline.date.timeIntervalSince(start)
                    ZStack {
                        EarthGlobe(
                            spin: EarthGeometry.earthSpin(elapsed: elapsed, reduceMotion: reduceMotion),
                            center: frame.center,
                            radius: DescentLayout.earthR * frame.scale
                        )
                        if !reduceMotion {
                            comets(frame: frame, elapsed: elapsed)
                        }
                    }
                }
                ForEach(Array(apps.enumerated()), id: \.element.id) { index, app in
                    label(for: app, index: index, frame: frame)
                }
            }
            .contentShape(Rectangle())
            .onTapGesture(coordinateSpace: .local) { location in
                handleTap(at: location, frame: frame)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel(
            "Promotion rings \(ringNames.joined(separator: ", ")) around Earth, one spoke per app"
        )
    }

    private func dimmed(_ id: String) -> Bool { selectedID != nil && selectedID != id }

    // MARK: Static drawing

    private func staticLayer(frame: DescentFrame) -> some View {
        Canvas { context, _ in
            let s = frame.scale
            let count = apps.count

            // Rings — the orbits. Prod (last) a touch brighter.
            for i in 0..<ringCount {
                let r = DescentLayout.ringRadius(index: i, count: ringCount) * s
                let rect = CGRect(
                    x: frame.center.x - r, y: frame.center.y - r, width: r * 2, height: r * 2
                )
                context.stroke(
                    Path(ellipseIn: rect),
                    with: .color(.white.opacity(i == ringCount - 1 ? 0.22 : 0.13)),
                    lineWidth: 1
                )
            }

            // Group arcs just outside the outer ring.
            let toView = CGAffineTransform(translationX: frame.center.x, y: frame.center.y)
                .scaledBy(x: s, y: s)
            for (i, span) in spans.enumerated() {
                let path = DescentLayout.groupArcPath(span, count: count).applying(toView)
                context.stroke(
                    path, with: .color(DescentPalette.group(i).opacity(0.55)),
                    style: StrokeStyle(lineWidth: max(2 * s, 1.6), lineCap: .round)
                )
            }

            // Spokes.
            for (k, app) in apps.enumerated() {
                var ctx = context
                ctx.opacity = dimmed(app.id) ? 0.22 : 1
                drawSpoke(app, index: k, count: count, frame: frame, in: &ctx)
            }

            // Ring name tags in the 12 o'clock gap.
            for i in 0..<ringNames.count {
                let r = DescentLayout.ringRadius(index: i, count: ringCount)
                let at = frame.point(CGPoint(x: 0, y: -r))
                let text = context.resolve(
                    Text(ringNames[i].uppercased())
                        .font(.system(size: 9, weight: .medium, design: .monospaced))
                        .tracking(1)
                        .foregroundStyle(.white.opacity(0.55))
                )
                let size = text.measure(in: CGSize(width: 200, height: 40))
                let bg = CGRect(
                    x: at.x - size.width / 2 - 4, y: at.y - size.height / 2 - 1,
                    width: size.width + 8, height: size.height + 2
                )
                context.fill(Path(roundedRect: bg, cornerRadius: 3), with: .color(DescentPalette.space))
                context.draw(text, at: at, anchor: .center)
            }
        }
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }

    private func drawSpoke(
        _ app: DescentApp, index k: Int, count: Int, frame: DescentFrame,
        in ctx: inout GraphicsContext
    ) {
        let s = frame.scale
        let a = DescentLayout.spokeAngle(index: k, count: count)
        let spoke = app.spoke
        let outer = DescentLayout.ringRadius(index: 0, count: ringCount)
        let selected = selectedID == app.id

        var rail = Path()
        rail.move(to: frame.polar(a, outer))
        rail.addLine(to: frame.polar(a, DescentLayout.earthR + 8))
        ctx.stroke(
            rail, with: .color(.white.opacity(selected ? 0.35 : 0.12)),
            lineWidth: max(1.2 * s, 0.8)
        )

        // The lit trail: outer ring → frontier.
        if spoke.frontier >= 0 {
            var trail = Path()
            trail.move(to: frame.polar(a, outer))
            trail.addLine(to: frame.polar(
                a, DescentLayout.ringRadius(index: spoke.frontier, count: ringCount)
            ))
            let tint = app.status == .empty ? Color.rpHealthy : app.status.tint
            ctx.stroke(
                trail, with: .color(tint.opacity(0.85)),
                style: StrokeStyle(lineWidth: max(3 * s, 2.5), lineCap: .round)
            )
        }

        // Closed gate: an amber bar across the spoke, midway to that ring.
        if spoke.gateAt >= 0, spoke.frontier >= 0 {
            let mid = (DescentLayout.ringRadius(index: spoke.frontier, count: ringCount)
                + DescentLayout.ringRadius(index: spoke.gateAt, count: ringCount)) / 2
            let m = DescentLayout.polar(a, mid)
            let half = max(DescentLayout.gateHalf, 6 / s)
            let px = -sin(a) * half, py = cos(a) * half
            var bar = Path()
            bar.move(to: frame.point(CGPoint(x: m.x - px, y: m.y - py)))
            bar.addLine(to: frame.point(CGPoint(x: m.x + px, y: m.y + py)))
            ctx.stroke(
                bar, with: .color(.rpGate),
                style: StrokeStyle(lineWidth: max(3 * s, 2.5), lineCap: .round)
            )
        }

        // Nodes where the spoke crosses each ring.
        for (i, node) in spoke.nodes.enumerated() where node.state != .off {
            let c = frame.polar(a, DescentLayout.ringRadius(index: i, count: ringCount))
            if node.state == .empty {
                let r = max(DescentLayout.emptyR * s, 1.8)
                ctx.fill(
                    Path(ellipseIn: CGRect(x: c.x - r, y: c.y - r, width: r * 2, height: r * 2)),
                    with: .color(.rpDisabled)
                )
                continue
            }
            let tint = DescentApp.tint(for: node.state)
            let r = max(DescentLayout.nodeR * s, 4.5)
            let disc = Path(ellipseIn: CGRect(x: c.x - r, y: c.y - r, width: r * 2, height: r * 2))
            if node.fresh {
                ctx.fill(disc, with: .color(tint))
            } else {
                ctx.fill(disc, with: .color(DescentPalette.space))
                let w = max(2.5 * s, 1.5)
                let inner = Path(ellipseIn: CGRect(
                    x: c.x - r + w / 2, y: c.y - r + w / 2, width: r * 2 - w, height: r * 2 - w
                ))
                ctx.stroke(inner, with: .color(tint), lineWidth: w)
            }
        }
    }

    // MARK: Comets

    /// A dot with a short tail flying from the frontier ring toward the next,
    /// looping, for every app with a job in flight.
    private func comets(frame: DescentFrame, elapsed: TimeInterval) -> some View {
        Canvas { context, _ in
            let s = frame.scale
            let count = apps.count
            let p = (elapsed / DescentLayout.cometSeconds).truncatingRemainder(dividingBy: 1)
            let e = p < 0.5 ? 2 * p * p : 1 - pow(-2 * p + 2, 2) / 2
            for (k, app) in apps.enumerated() where app.isBusy {
                let next = DescentLayout.nextRingIndex(app.spoke)
                guard next >= 0 else { continue }
                var ctx = context
                ctx.opacity = dimmed(app.id) ? 0.22 : 1
                let a = DescentLayout.spokeAngle(index: k, count: count)
                let r0 = DescentLayout.ringRadius(index: app.spoke.frontier, count: ringCount)
                let r1 = DescentLayout.ringRadius(index: next, count: ringCount)
                let rr = r0 + (r1 - r0) * e
                let rt = min(r0, rr + 34)
                let head = frame.polar(a, rr)
                let tint = Color.rpInFlight

                var tail = Path()
                tail.move(to: frame.polar(a, rt))
                tail.addLine(to: head)
                ctx.stroke(
                    tail, with: .color(tint.opacity(0.6)),
                    style: StrokeStyle(lineWidth: max(3 * s, 2), lineCap: .round)
                )
                // Soft pulse around the head.
                let hr = max(6 * s, 3.5)
                let pr = hr * (1 + p * 1.6)
                ctx.fill(
                    Path(ellipseIn: CGRect(x: head.x - pr, y: head.y - pr, width: pr * 2, height: pr * 2)),
                    with: .color(tint.opacity(0.35 * (1 - p)))
                )
                ctx.fill(
                    Path(ellipseIn: CGRect(x: head.x - hr, y: head.y - hr, width: hr * 2, height: hr * 2)),
                    with: .color(.white.opacity(0.9))
                )
            }
        }
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }

    // MARK: Labels

    private func label(for app: DescentApp, index: Int, frame: DescentFrame) -> some View {
        let a = DescentLayout.spokeAngle(index: index, count: apps.count)
        let anchor = DescentLayout.labelAnchor(a)
        let at = frame.polar(a, DescentLayout.labelR)
        let alignment: Alignment
        let textAlignment: HorizontalAlignment
        switch anchor {
        case .start: alignment = .leading; textAlignment = .leading
        case .end: alignment = .trailing; textAlignment = .trailing
        case .middle: alignment = sin(a) > 0 ? .top : .bottom; textAlignment = .center
        }
        return Color.clear
            .frame(width: 1, height: 1)
            .overlay(alignment: alignment) {
                SpokeLabel(
                    app: app, alignment: textAlignment,
                    selected: selectedID == app.id, dimmed: dimmed(app.id)
                ) {
                    onSelect(app.id)
                }
                .fixedSize()
            }
            .position(at)
    }

    // MARK: Hit testing

    private func handleTap(at location: CGPoint, frame: DescentFrame) {
        let dx = (location.x - frame.center.x) / frame.scale
        let dy = (location.y - frame.center.y) / frame.scale
        let r = (dx * dx + dy * dy).squareRoot()
        let angle = atan2(dy, dx)
        let index = DescentLayout.spokeIndex(atAngle: angle, count: apps.count)
        let slop = max(10, 12 / frame.scale)

        if abs(r - DescentLayout.groupArcR) <= slop / 2 + 4, let index,
           let span = spans.first(where: { index >= $0.from && index <= $0.to }) {
            onOpenGroup(span.id)
            return
        }
        if r >= DescentLayout.earthR, r <= DescentLayout.labelR, let index {
            onSelect(apps[index].id)
            return
        }
        onDeselect()
    }
}

/// The name plate at the end of a spoke: app name, then "version · ring".
private struct SpokeLabel: View {
    let app: DescentApp
    let alignment: HorizontalAlignment
    let selected: Bool
    let dimmed: Bool
    let onTap: () -> Void

    var body: some View {
        Button(action: onTap) {
            VStack(alignment: alignment, spacing: 1) {
                Text(Self.clip(app.title))
                    .font(.system(size: 11, weight: selected ? .semibold : .medium))
                    .foregroundStyle(.white.opacity(selected ? 1 : 0.88))
                Text(app.subtitle)
                    .font(.system(size: 9, design: .monospaced))
                    .foregroundStyle(.white.opacity(0.5))
            }
            .lineLimit(1)
            .padding(.horizontal, 3)
            .padding(.vertical, 2)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .opacity(dimmed ? 0.22 : 1)
        .accessibilityLabel("\(app.title): \(app.status.label). \(app.sentence)")
        .accessibilityHint("Shows details")
        .accessibilityAddTraits(selected ? .isSelected : [])
        .accessibilityIdentifier("spoke-\(app.id)")
    }

    /// Keep labels from running off the stage; the card has the full name.
    static func clip(_ s: String) -> String {
        s.count > 24 ? String(s.prefix(23)) + "…" : s
    }
}

// MARK: - Lanes

/// One row per app, one column per ring (int → prod), a small Earth at the
/// end. Groups become section headers; tapping one opens the group.
private struct LanesView<Footer: View>: View {
    let apps: [DescentApp]
    let spans: [DescentLayout.GroupSpan]
    let ringNames: [String]
    let width: Double
    let selectedID: String?
    let filtering: Bool
    let onSelect: (String) -> Void
    let onOpenGroup: (String) -> Void
    @ViewBuilder let footer: () -> Footer

    private var nameWidth: Double { min(180, max(84, width * 0.27)) }

    private struct Section: Identifiable {
        let id: String
        let name: String?
        let colorIndex: Int
        let apps: ArraySlice<DescentApp>
    }

    private var sections: [Section] {
        var out: [Section] = []
        var next = 0
        for (i, span) in spans.enumerated() {
            out.append(Section(
                id: span.id, name: span.name, colorIndex: i, apps: apps[span.from...span.to]
            ))
            next = span.to + 1
        }
        if next < apps.count {
            out.append(Section(
                id: "__ungrouped__", name: spans.isEmpty ? nil : "Ungrouped",
                colorIndex: -1, apps: apps[next...]
            ))
        }
        return out
    }

    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 6, pinnedViews: [.sectionHeaders]) {
                SwiftUI.Section {
                    ForEach(sections) { section in
                        if let name = section.name {
                            groupHeader(section, name: name)
                        }
                        ForEach(section.apps) { app in
                            LaneRow(
                                app: app, nameWidth: nameWidth,
                                selected: selectedID == app.id,
                                dimmed: selectedID != nil && selectedID != app.id
                            ) {
                                onSelect(app.id)
                            }
                        }
                    }
                    footer()
                } header: {
                    header
                }
            }
            .padding(.horizontal, 12)
            .padding(.bottom, 12)
        }
    }

    private var header: some View {
        HStack(spacing: 4) {
            Text("App").frame(width: nameWidth, alignment: .leading)
            ForEach(ringNames, id: \.self) { name in
                Text(name).frame(maxWidth: .infinity)
            }
            Color.clear.frame(width: 12)
        }
        .font(.system(size: 10, weight: .medium, design: .monospaced))
        .tracking(1)
        .textCase(.uppercase)
        .foregroundStyle(.white.opacity(0.5))
        .padding(.vertical, 6)
        .background(DescentPalette.space)
        .accessibilityHidden(true)
    }

    private func groupHeader(_ section: Section, name: String) -> some View {
        Button {
            if section.colorIndex >= 0 { onOpenGroup(section.id) }
        } label: {
            HStack(spacing: 6) {
                Capsule()
                    .fill(section.colorIndex >= 0 ? DescentPalette.group(section.colorIndex) : .rpDisabled)
                    .frame(width: 14, height: 3)
                Text(name)
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.secondary)
                if section.colorIndex >= 0 {
                    Image(systemName: filtering ? "xmark.circle" : "chevron.right")
                        .font(.caption2)
                        .foregroundStyle(.tertiary)
                }
                Spacer()
            }
            .padding(.top, 6)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .disabled(section.colorIndex < 0)
        .accessibilityLabel(filtering ? "\(name). Shows all apps." : "Group \(name). Shows only its apps.")
    }
}

private struct LaneRow: View {
    let app: DescentApp
    let nameWidth: Double
    let selected: Bool
    let dimmed: Bool
    let onTap: () -> Void

    var body: some View {
        Button(action: onTap) {
            HStack(spacing: 4) {
                HStack(spacing: 5) {
                    Circle().fill(app.status.tint).frame(width: 6, height: 6)
                    Text(app.title)
                        .font(.system(size: 13, weight: .medium))
                        .foregroundStyle(.white.opacity(0.92))
                        .lineLimit(2)
                        .minimumScaleFactor(0.85)
                }
                .frame(width: nameWidth, alignment: .leading)
                ForEach(Array(app.spoke.nodes.enumerated()), id: \.offset) { i, node in
                    VersionPill(node: node, gated: i == app.spoke.gateAt)
                        .frame(maxWidth: .infinity)
                }
                EarthDot().frame(width: 12, height: 12)
            }
            .padding(.vertical, 5)
            .padding(.horizontal, 6)
            .background(
                RoundedRectangle(cornerRadius: 8)
                    .fill(selected ? Color.white.opacity(0.08) : Color.white.opacity(0.025))
            )
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .opacity(dimmed ? 0.22 : 1)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(app.title): \(app.status.label). \(app.sentence)")
        .accessibilityHint("Shows details")
        .accessibilityAddTraits(selected ? [.isButton, .isSelected] : .isButton)
        .accessibilityIdentifier("spoke-\(app.id)")
    }
}

/// One ring's cell in a lane: solid tint = newest version, outline = older,
/// dashed "—" = nothing deployed (fainter when the app doesn't use the ring).
private struct VersionPill: View {
    let node: DescentLayout.SpokeNode
    let gated: Bool

    var body: some View {
        let shape = RoundedRectangle(cornerRadius: 6)
        Group {
            if let version = node.version {
                let tint = DescentApp.tint(for: node.state)
                cell(version)
                    .foregroundStyle(tint)
                    .background(shape.fill(node.fresh ? tint.opacity(0.15) : .clear))
                    .overlay(shape.strokeBorder(tint.opacity(node.fresh ? 0.35 : 0.6)))
            } else {
                cell("—")
                    .foregroundStyle(.white.opacity(node.state == .off ? 0.2 : 0.4))
                    .overlay(
                        shape.strokeBorder(
                            .white.opacity(node.state == .off ? 0.06 : 0.15),
                            style: StrokeStyle(lineWidth: 1, dash: [3, 2])
                        )
                    )
            }
        }
        .overlay(alignment: .topTrailing) {
            if gated {
                Image(systemName: "lock.fill")
                    .font(.system(size: 7))
                    .foregroundStyle(Color.rpGate)
                    .padding(2)
            }
        }
    }

    /// The text sized to the whole cell, so tint and border fill the column.
    private func cell(_ text: String) -> some View {
        Text(text)
            .font(.system(size: 10, design: .monospaced))
            .lineLimit(1)
            .minimumScaleFactor(0.6)
            .padding(.horizontal, 3)
            .frame(maxWidth: .infinity, minHeight: 24, maxHeight: 24)
    }
}

/// Earth — live users — at the end of each lane.
private struct EarthDot: View {
    var body: some View {
        Circle()
            .fill(
                RadialGradient(
                    colors: [
                        Color(red: 28 / 255, green: 74 / 255, blue: 110 / 255),
                        Color(red: 7 / 255, green: 20 / 255, blue: 31 / 255),
                    ],
                    center: .init(x: 0.35, y: 0.35), startRadius: 0, endRadius: 8
                )
            )
            .overlay(Circle().strokeBorder(Color(red: 125 / 255, green: 211 / 255, blue: 252 / 255).opacity(0.35)))
            .accessibilityHidden(true)
    }
}

// MARK: - Detail card

/// The selected app: status, the one-line reading, a per-ring table, and the
/// way into the service.
private struct SpokeCard: View {
    let app: DescentApp
    let groupName: String?
    let onOpen: () -> Void
    let onClose: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .top, spacing: 8) {
                VStack(alignment: .leading, spacing: 3) {
                    if let groupName {
                        Text(groupName)
                            .font(.caption2.weight(.semibold))
                            .textCase(.uppercase)
                            .foregroundStyle(.secondary)
                    }
                    Text(app.title).font(.subheadline.weight(.semibold))
                    HStack(spacing: 6) {
                        Label(app.status.label, systemImage: app.status.systemImage)
                            .foregroundStyle(app.status.tint)
                        if let busy = app.busyAction {
                            Text("· \(busy) running").foregroundStyle(Color.rpInFlight)
                        }
                    }
                    .font(.caption.weight(.medium))
                }
                Spacer()
                Button(action: onClose) {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundStyle(.secondary)
                        .font(.title3)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Close details")
            }

            Text(app.sentence)
                .font(.caption)
                .foregroundStyle(.primary.opacity(0.9))
                .fixedSize(horizontal: false, vertical: true)

            if let loadError = app.loadError {
                Label(loadError, systemImage: "exclamationmark.triangle")
                    .font(.caption2)
                    .foregroundStyle(Color.rpGate)
            }

            ScrollView {
                VStack(spacing: 4) {
                    ForEach(Array(app.spoke.nodes.enumerated()), id: \.offset) { _, node in
                        row(node)
                    }
                }
            }
            .scrollBounceBehavior(.basedOnSize)
            .frame(maxHeight: 150)

            Button(action: onOpen) {
                Text("Open service").frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .controlSize(.small)
        }
        .padding(12)
        .background(.ultraThinMaterial, in: RoundedRectangle(cornerRadius: 14))
        .overlay(RoundedRectangle(cornerRadius: 14).strokeBorder(.white.opacity(0.15)))
        .accessibilityElement(children: .contain)
    }

    private func row(_ node: DescentLayout.SpokeNode) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Text(node.ring.uppercased())
                .font(.system(size: 10, weight: .medium, design: .monospaced))
                .foregroundStyle(.secondary)
                .frame(width: 44, alignment: .leading)
            VStack(alignment: .leading, spacing: 1) {
                Text(node.state == .off ? "not used" : (node.version ?? "—"))
                    .font(.system(size: 12, design: .monospaced))
                    .foregroundStyle(node.state == .off ? .tertiary : .primary)
                if node.version != nil, !node.fresh {
                    Text("older version").font(.caption2).foregroundStyle(.secondary)
                }
                if let gate = node.gateClosed {
                    Text(gate).font(.caption2).foregroundStyle(Color.rpGate)
                }
            }
            Spacer(minLength: 4)
            health(node)
                .font(.caption.monospacedDigit())
        }
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder
    private func health(_ node: DescentLayout.SpokeNode) -> some View {
        switch node.state {
        case .healthy:
            Text(node.ttfbMs.map { "\($0) ms" } ?? "Healthy").foregroundStyle(Color.rpHealthy)
        case .failed:
            Text("Failing").foregroundStyle(Color.rpUnhealthy)
        case .empty, .off:
            EmptyView()
        }
    }
}

// MARK: - Accessibility fallback

/// The same fleet as plain rows, used at accessibility text sizes where
/// neither the orbit nor the lane columns can be read.
private struct AccessibleFleetList: View {
    let apps: [DescentApp]
    let onOpen: (DescentApp) -> Void

    var body: some View {
        VStack(spacing: 8) {
            ForEach(apps) { app in
                Button {
                    onOpen(app)
                } label: {
                    VStack(alignment: .leading, spacing: 6) {
                        Text(app.title).font(.headline)
                        StatusBadge(
                            text: app.status.label,
                            systemImage: app.status.systemImage,
                            tint: app.status.tint
                        )
                        Text(app.sentence)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(12)
                    .background(.quaternary.opacity(0.4), in: RoundedRectangle(cornerRadius: 12))
                }
                .buttonStyle(.plain)
                .accessibilityHint("Opens the pipeline for \(app.title)")
                .accessibilityIdentifier("spoke-\(app.id)")
            }
        }
    }
}
