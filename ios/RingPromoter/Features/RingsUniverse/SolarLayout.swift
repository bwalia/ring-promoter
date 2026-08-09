import Foundation

/// The health of one orbiting body on the Rings of Applications screen.
///
/// This mirrors the web console's fleet view exactly, so an operator moving
/// between the two sees the same verdicts: `deploying` overrides everything
/// because a running job makes the health snapshot momentarily untrustworthy,
/// and the aggregate is worst-first because one failing service is what the
/// screen exists to surface.
enum FleetStatus: Hashable, Sendable, CaseIterable {
    case healthy
    case deploying
    case degraded
    case failed
    case empty
    case loading

    /// Worst-first, matching the web console's aggregation order.
    static let aggregatePriority: [FleetStatus] = [
        .failed, .degraded, .deploying, .loading, .healthy, .empty,
    ]

    /// The overall verdict for a set of bodies (the whole stage, or one ring's
    /// member apps).
    static func aggregate(_ statuses: [FleetStatus]) -> FleetStatus {
        aggregatePriority.first { statuses.contains($0) } ?? .empty
    }

    /// Health of one application from its rings and job state.
    ///
    /// - `isBusy`: a job is running right now — shown as deploying regardless
    ///   of the (soon to be stale) health snapshot.
    /// - `ringsUnknown`: the rings request failed and nothing is cached, so no
    ///   verdict can honestly be given.
    init(rings: [RingStatus], isBusy: Bool, ringsUnknown: Bool = false) {
        if isBusy {
            self = .deploying
            return
        }
        if ringsUnknown {
            self = .loading
            return
        }
        let active = rings.filter { $0.configured && !$0.isEmpty }
        if active.isEmpty {
            self = .empty
        } else {
            let healthy = active.count(where: \.isHealthy)
            if healthy == active.count {
                self = .healthy
            } else {
                self = healthy == 0 ? .failed : .degraded
            }
        }
    }

    var label: String {
        switch self {
        case .healthy: "Healthy"
        case .deploying: "Deploying"
        case .degraded: "Degraded"
        case .failed: "Failing"
        case .empty: "No version"
        case .loading: "Checking…"
        }
    }

    /// Colour is never the only signal — every status also carries a symbol.
    var systemImage: String {
        switch self {
        case .healthy: "checkmark.circle.fill"
        case .deploying: "arrow.triangle.2.circlepath"
        case .degraded: "exclamationmark.circle.fill"
        case .failed: "exclamationmark.triangle.fill"
        case .empty: "circle.dotted"
        case .loading: "ellipsis.circle"
        }
    }
}

/// One orbiting body: an application in "All services" mode, or a group of
/// applications in "Rings" mode. Built once per refresh from `AppSummary`
/// values so the stage itself stays a pure function of (nodes, elapsed time).
struct FleetNode: Identifiable, Hashable, Sendable {
    /// The synthetic ring that collects every app not in any group, mirroring
    /// the web console's "Ungrouped" planet.
    static let ungroupedID = "__ungrouped__"

    let id: String
    let title: String
    /// "3 apps" for a ring node; nil for an app node.
    let subtitle: String?
    let status: FleetStatus
    /// Drives the orbit radius; nil parks the body on the default track.
    let latencyMs: Int?
    /// Deployed-and-healthy over deployed, across every ring this node covers.
    let healthyCount: Int
    let activeCount: Int
    /// "2.7.1 · prod" — the prod-most deployment, app nodes only.
    let latestVersion: String?
    let lastDeploy: Date?
    /// The applications behind this body — one for an app node.
    let apps: [String]

    /// An application body.
    init(summary: AppSummary) {
        let active = summary.rings.filter { $0.configured && !$0.isEmpty }
        id = summary.name
        title = summary.title
        subtitle = nil
        status = FleetStatus(
            rings: summary.rings, isBusy: summary.isBusy,
            ringsUnknown: summary.rings.isEmpty && summary.loadError != nil
        )
        latencyMs = SolarLayout.latency(for: summary.rings)
        healthyCount = active.count(where: \.isHealthy)
        activeCount = active.count
        // Rings arrive ordered dev → prod, so the last active ring is the
        // prod-most place this version has reached.
        latestVersion = active.last.map { "\($0.currentVersion) · \($0.ring.name)" }
        lastDeploy = active.filter(\.hasBeenDeployedTo).map(\.updatedAt).max()
        apps = [summary.name]
    }

    /// A ring (group) body rolled up from its member applications.
    init(id: String, name: String, members: [AppSummary]) {
        let allRings = members.flatMap(\.rings)
        let active = allRings.filter { $0.configured && !$0.isEmpty }
        self.id = id
        title = name
        subtitle = members.count == 1 ? "1 app" : "\(members.count) apps"
        status = FleetStatus.aggregate(members.map {
            FleetStatus(
                rings: $0.rings, isBusy: $0.isBusy,
                ringsUnknown: $0.rings.isEmpty && $0.loadError != nil
            )
        })
        // The slowest member sets the orbit: a ring is only as close as its
        // farthest service.
        latencyMs = members.compactMap { SolarLayout.latency(for: $0.rings) }.max()
        healthyCount = active.count(where: \.isHealthy)
        activeCount = active.count
        latestVersion = nil
        lastDeploy = active.filter(\.hasBeenDeployedTo).map(\.updatedAt).max()
        apps = members.map(\.name)
    }

    /// All application bodies, ordered as given.
    static func appNodes(from summaries: [AppSummary]) -> [FleetNode] {
        summaries.map(FleetNode.init(summary:))
    }

    /// One body per server-side group plus, when needed, the synthetic
    /// "Ungrouped" body — so every application is represented exactly once.
    static func ringNodes(from summaries: [AppSummary], groups: [AppGroup]) -> [FleetNode] {
        var nodes = groups.map { group in
            FleetNode(
                id: group.id, name: group.name,
                members: summaries.filter { group.contains($0.name) }
            )
        }
        let grouped = Set(groups.flatMap(\.apps))
        let ungrouped = summaries.filter { !grouped.contains($0.name) }
        if !ungrouped.isEmpty {
            nodes.append(FleetNode(id: ungroupedID, name: "Ungrouped", members: ungrouped))
        }
        return nodes
    }
}

/// The geometry of the solar system, ported number-for-number from the web
/// console's `solar-layout.ts` so both clients draw the same sky.
///
/// Everything is expressed on a fixed 400×400 canvas with the sun at the
/// centre; the view scales positions to its actual size. All functions are
/// pure and deterministic — the only input that moves is elapsed time.
enum SolarLayout {
    /// The logical canvas. Positions are fractions of this, sun at the centre.
    static let canvasSize: Double = 400
    static let center = 200.0

    /// Discrete orbit tracks (solar-system style). Inner = low latency.
    /// Matches web `ORBIT_TRACKS` — outermost stops short of the edge so name
    /// plates riding outside the body still fit on stage.
    static let tracks: [Double] = [68, 100, 132, 162]
    static let defaultRadius: Double = 100

    /// Bodies are nudged off straight-up so the radial latency axis stays readable.
    static let axisClearance: Double = 0.22

    /// Radial stagger applied to consecutive bodies on a crowded track.
    static let stagger: [Double] = [0, 15, -15, 30]

    /// A body on its track: everything needed to compute its position at any
    /// instant.
    struct Planet: Hashable, Sendable {
        let id: String
        /// Draw radius: the latency track plus any anti-crowding stagger.
        let r: Double
        /// The latency band this body belongs to (always one of `tracks`).
        let track: Double
        /// Radians at t=0; bodies are spread evenly starting from 12 o'clock.
        let angle0: Double
        /// Seconds per revolution — outer orbits drift slower, like real ones.
        let period: Double
    }

    /// Human-readable bound for each orbit track, derived from
    /// `radius(forLatencyMs:)` so axis labels cannot drift from placement maths.
    static var orbitBands: [(r: Double, label: String)] {
        var upper: [Double: Int] = [:]
        for ms in 0...2000 {
            upper[radius(forLatencyMs: ms)] = ms
        }
        return tracks.enumerated().map { i, r in
            if i == tracks.count - 1 {
                let prev = upper[tracks[i - 1]]
                return (r, prev == nil ? "slowest" : ">\(prev!)ms")
            }
            let max = upper[r]
            return (r, max == nil ? "" : "≤\(max!)ms")
        }
    }

    /// FNV-1a folded to 0..<1. Deterministic so a body keeps its jitter across
    /// refreshes instead of hopping around the orbit.
    static func hash01(_ s: String) -> Double {
        var h: UInt32 = 2_166_136_261
        for byte in s.utf8 {
            h ^= UInt32(byte)
            h = h &* 16_777_619
        }
        return Double(h % 10_000) / 10_000
    }

    static func snapToTrack(_ r: Double) -> Double {
        tracks.min { abs($0 - r) < abs($1 - r) } ?? defaultRadius
    }

    /// Latency to orbit radius, log-compressed against 800 ms so the common
    /// 10–100 ms band still spreads across tracks, then snapped.
    static func radius(forLatencyMs ms: Int?) -> Double {
        guard let ms else { return defaultRadius }
        let t = min(1, log1p(Double(max(0, ms))) / log1p(800))
        let low = tracks.first ?? defaultRadius
        let high = tracks.last ?? defaultRadius
        return snapToTrack(low + t * (high - low))
    }

    /// The latency that represents an application: production's if it reported
    /// one, else the prod-most configured ring that did. Production is what
    /// operators actually care about; the fallback keeps pre-prod apps on a
    /// meaningful orbit.
    static func latency(for rings: [RingStatus]) -> Int? {
        let configured = rings.filter(\.configured)
        if let prod = configured.first(where: { $0.ring.name == "prod" }),
           let ms = prod.latencyMs {
            return ms
        }
        return configured.reversed().lazy.compactMap(\.latencyMs).first
    }

    /// Place bodies on their tracks: bucketed by radius, alphabetical within a
    /// track, spread evenly from 12 o'clock with axis clearance and a small
    /// deterministic spin. Crowded tracks stagger radially across the band.
    static func planets(for bodies: [(id: String, radius: Double)]) -> [Planet] {
        var byTrack: [Double: [String]] = [:]
        for body in bodies {
            byTrack[snapToTrack(body.radius), default: []].append(body.id)
        }
        var out: [Planet] = []
        let maxTrack = tracks.last ?? defaultRadius
        for track in tracks {
            guard var bucket = byTrack[track], !bucket.isEmpty else { continue }
            bucket.sort { $0.localizedCaseInsensitiveCompare($1) == .orderedAscending }
            // Outer orbits revolve more slowly (Kepler-ish), matching web.
            let period = 48 + (track / maxTrack) * 70
            let crowded = bucket.count > 6
            for (index, id) in bucket.enumerated() {
                let spin = (hash01(id) - 0.5) * 0.12
                let angle0 =
                    (Double(index) / Double(bucket.count)) * 2 * .pi
                    - .pi / 2 + axisClearance + spin
                let staggerOffset = crowded ? stagger[index % stagger.count] : 0
                out.append(
                    Planet(
                        id: id, r: track + staggerOffset, track: track,
                        angle0: angle0, period: period
                    )
                )
            }
        }
        return out
    }

    /// Where a body is `elapsed` seconds in, on the 400×400 canvas.
    static func position(of planet: Planet, at elapsed: TimeInterval) -> CGPoint {
        let angle = planet.angle0 + (elapsed / planet.period) * 2 * .pi
        return CGPoint(
            x: center + planet.r * cos(angle),
            y: center + planet.r * sin(angle)
        )
    }

    /// Angle of a body at `elapsed` (for name-plate side decisions).
    static func angle(of planet: Planet, at elapsed: TimeInterval) -> Double {
        planet.angle0 + (elapsed / planet.period) * 2 * .pi
    }
}
