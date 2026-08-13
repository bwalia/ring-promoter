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
    /// Time to first byte, when the live check reported it.
    let ttfbMs: Int?
    /// Config geographic pin; nil parks the body on a satellite belt.
    let location: AppLocation?
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
        ttfbMs = SolarLayout.ttfb(for: summary.rings)
        location = summary.location
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
        ttfbMs = members.compactMap { SolarLayout.ttfb(for: $0.rings) }.max()
        location = SolarLayout.centroid(of: members.compactMap(\.location))
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

    // MARK: - Globe (ported from web `globe-layout.ts`)

    static let earthR: Double = 58
    static let earthSpinPeriod: Double = 96
    static let altMin: Double = 14
    static let altMax: Double = 78

    struct GlobePoint: Sendable {
        let x: Double
        let y: Double
        let z: Double
        var front: Bool { z >= -0.5 }
        var cgPoint: CGPoint { CGPoint(x: x, y: y) }
    }

    struct GlobeBody: Hashable, Sendable {
        let id: String
        let r: Double
        let track: Double
        let lat: Double
        let lng0: Double
        let driftDegPerSec: Double
        let placed: Bool
    }

    static func ttfb(for rings: [RingStatus]) -> Int? {
        let configured = rings.filter(\.configured)
        if let prod = configured.first(where: { $0.ring.name == "prod" }),
           let ms = prod.ttfbMs {
            return ms
        }
        return configured.reversed().lazy.compactMap(\.ttfbMs).first
    }

    static func altitude(forLatencyMs ms: Int?) -> Double {
        let track = radius(forLatencyMs: ms)
        let low = tracks.first ?? defaultRadius
        let high = tracks.last ?? defaultRadius
        let t = (track - low) / max(high - low, 1)
        return altMin + t * (altMax - altMin)
    }

    static func earthSpin(elapsed: TimeInterval, reduceMotion: Bool) -> Double {
        if reduceMotion { return 0 }
        return (elapsed / earthSpinPeriod) * 2 * .pi
    }

    static func projectOrtho(latDeg: Double, lngDeg: Double, radius: Double, spin: Double) -> GlobePoint {
        let lat = latDeg * .pi / 180
        let lng = lngDeg * .pi / 180 + spin
        let cosLat = cos(lat)
        let x = radius * cosLat * sin(lng)
        let y = -radius * sin(lat)
        let z = radius * cosLat * cos(lng)
        return GlobePoint(x: center + x, y: center + y, z: z)
    }

    static func point(of body: GlobeBody, elapsed: TimeInterval, spin: Double) -> GlobePoint {
        let lng = body.lng0 + body.driftDegPerSec * elapsed
        return projectOrtho(latDeg: body.lat, lngDeg: lng, radius: body.r, spin: spin)
    }

    static func surface(of body: GlobeBody, elapsed: TimeInterval, spin: Double) -> GlobePoint {
        let lng = body.lng0 + body.driftDegPerSec * elapsed
        return projectOrtho(latDeg: body.lat, lngDeg: lng, radius: earthR, spin: spin)
    }

    static func globeBodies(for nodes: [FleetNode]) -> [GlobeBody] {
        var placed: [(id: String, loc: AppLocation, alt: Double, track: Double)] = []
        var unplaced: [(id: String, alt: Double, track: Double)] = []
        for node in nodes {
            let ms = node.ttfbMs ?? node.latencyMs
            let track = radius(forLatencyMs: ms)
            let alt = altitude(forLatencyMs: ms)
            if let loc = node.location {
                placed.append((node.id, loc, alt, track))
            } else {
                unplaced.append((node.id, alt, track))
            }
        }
        var out: [GlobeBody] = []
        for p in placed {
            let jitterLat = (hash01(p.id + ":lat") - 0.5) * 1.6
            let jitterLng = (hash01(p.id + ":lng") - 0.5) * 2.2
            out.append(
                GlobeBody(
                    id: p.id, r: earthR + p.alt, track: p.track,
                    lat: min(80, max(-80, p.loc.lat + jitterLat)),
                    lng0: wrapLng(p.loc.lng + jitterLng),
                    driftDegPerSec: 0, placed: true
                )
            )
        }
        let sorted = unplaced.sorted { $0.id.localizedCaseInsensitiveCompare($1.id) == .orderedAscending }
        let n = Double(max(sorted.count, 1))
        let maxTrack = tracks.last ?? defaultRadius
        for (index, p) in sorted.enumerated() {
            let lat = -18 + hash01(p.id) * 36
            let lng0 = (Double(index) / n) * 360 - 180
            let period = 72 + (p.track / maxTrack) * 50
            out.append(
                GlobeBody(
                    id: p.id, r: earthR + p.alt, track: p.track,
                    lat: lat, lng0: lng0, driftDegPerSec: 360 / period, placed: false
                )
            )
        }
        return out
    }

    static func centroid(of pins: [AppLocation]) -> AppLocation? {
        guard let first = pins.first else { return nil }
        if pins.count == 1 { return first }
        var x = 0.0, y = 0.0, z = 0.0
        for p in pins {
            let lat = p.lat * .pi / 180
            let lng = p.lng * .pi / 180
            x += cos(lat) * cos(lng)
            y += cos(lat) * sin(lng)
            z += sin(lat)
        }
        let n = Double(pins.count)
        x /= n; y /= n; z /= n
        let hyp = hypot(x, y)
        return AppLocation(
            lat: atan2(z, hyp) * 180 / .pi,
            lng: atan2(y, x) * 180 / .pi,
            city: first.city, region: first.region
        )
    }

    static func haversineKm(_ a: AppLocation, _ b: AppLocation) -> Double {
        let R = 6371.0
        let dLat = (b.lat - a.lat) * .pi / 180
        let dLng = (b.lng - a.lng) * .pi / 180
        let s = sin(dLat / 2) * sin(dLat / 2)
            + cos(a.lat * .pi / 180) * cos(b.lat * .pi / 180)
            * sin(dLng / 2) * sin(dLng / 2)
        return 2 * R * asin(min(1, sqrt(s)))
    }

    static func estimateRttMs(km: Double) -> Int {
        Int((2 * (km / 200) + 18).rounded())
    }

    private static func wrapLng(_ lng: Double) -> Double {
        var x = lng
        while x > 180 { x -= 360 }
        while x < -180 { x += 360 }
        return x
    }

    /// Coarse continent outlines (lng, lat) matching the web globe.
    static let landPolys: [[(lng: Double, lat: Double)]] = [
        [(-168, 65), (-141, 70), (-128, 71), (-105, 68), (-89, 68), (-80, 62),
         (-70, 58), (-60, 47), (-67, 44), (-74, 40), (-81, 25), (-97, 26),
         (-106, 22), (-110, 24), (-117, 32), (-124, 40), (-124, 48), (-130, 55),
         (-153, 57), (-166, 54), (-168, 65)],
        [(-73, 76), (-60, 82), (-20, 81), (-22, 70), (-44, 60), (-58, 61), (-73, 76)],
        [(-81, 12), (-60, 8), (-50, 0), (-35, -8), (-38, -20), (-54, -35),
         (-68, -55), (-75, -50), (-73, -18), (-81, -5), (-81, 12)],
        [(-10, 52), (-9, 43), (-1, 43), (3, 42), (10, 44), (16, 40), (29, 41),
         (30, 46), (24, 60), (12, 58), (5, 61), (-5, 59), (-10, 52)],
        [(-17, 21), (-10, 12), (8, 5), (10, -4), (14, -12), (40, -16),
         (32, -28), (20, -35), (18, -32), (12, -17), (-5, -5), (-14, 4),
         (-17, 14), (-6, 36), (10, 37), (25, 32), (32, 31), (11, 33),
         (-5, 36), (-17, 28), (-17, 21)],
        [(27, 40), (36, 36), (44, 40), (60, 37), (67, 25), (77, 8), (80, 15),
         (88, 22), (73, 25), (62, 25), (48, 30), (36, 21), (32, 31), (27, 40)],
        [(30, 60), (40, 68), (70, 72), (90, 75), (130, 71), (160, 66), (180, 65),
         (170, 60), (142, 46), (130, 43), (122, 30), (105, 20), (100, 10),
         (104, 1), (98, 8), (94, 18), (78, 28), (74, 40), (80, 50), (60, 50),
         (45, 55), (30, 60)],
        [(95, 6), (104, -6), (119, -8), (131, -8), (120, 5), (105, 7), (95, 6)],
        [(113, -22), (114, -34), (137, -35), (153, -28), (153, -12),
         (142, -11), (129, -14), (113, -22)],
        [(166, -41), (178, -37), (178, -46), (166, -47), (166, -41)],
        [(-180, -72), (-90, -70), (0, -70), (90, -72), (180, -72), (180, -90),
         (-180, -90), (-180, -72)],
    ]
}
