import CoreGraphics
import Foundation
import SwiftUI

/// Descent — the Rings of Apps stage.
///
/// The promotion rings ARE the orbits: the first ring in promotion order (int)
/// is the outermost circle and the last (prod) sits just above Earth, which
/// stands for live users. Every app owns one fixed spoke; the lit trail on it
/// shows how far the app's newest version has travelled toward Earth.
///
/// Pure geometry on a 1000×1000 design canvas centred on (0, 0), +y down.
///
/// THIS FILE IS A NUMBER-FOR-NUMBER PORT OF `web/src/lib/descent-layout.ts`.
/// The two must stay in lockstep: change a constant or a rule in one, change
/// it in the other in the same PR. Naming: the web's `DESCENT_FOO_BAR`
/// constants are `DescentLayout.fooBar` here; functions keep their names.
/// The web's `RingView` is `RingStatus` in the iOS models.
enum DescentLayout {
    /// Design canvas edge; the stage scales it to fit. Origin is the centre.
    static let design: Double = 1000
    /// Earth's disc radius.
    static let earthR: Double = 86
    /// Radius of the last (prod-most) ring.
    static let ringInner: Double = 132
    /// Radius of the first (int-most) ring.
    static let ringOuter: Double = 318
    /// Group arcs run just outside the outer ring.
    static let groupArcR: Double = ringOuter + 16
    /// Where an app's name plate starts, measured from the centre.
    static let labelR: Double = ringOuter + 32
    /// Radians left free at 12 o'clock for the ring name tags.
    static let topGap: Double = 0.34
    /// |cos(angle)| below this centres the label under/over its spoke.
    static let labelCenterBand: Double = 0.12
    /// Node radii: a deployed ring, and the dot for "nothing deployed".
    static let nodeR: Double = 8
    static let emptyR: Double = 3
    /// Past this many spokes the labels crowd — fall back to lanes.
    static let maxSpokes = 36
    /// Below this stage width (points) the orbit is unreadable — lanes.
    /// 320 covers iPhone SE and up; the stage scales labels to fit.
    static let minWidth: Double = 320
    /// Half-width of the amber gate bar drawn across a spoke.
    static let gateHalf: Double = 11
    /// One comet pass between two rings, seconds.
    static let cometSeconds: Double = 1.6

    // MARK: - Geometry

    /// Radius of the ring at `index` in promotion order (0 = first = outermost).
    static func ringRadius(index: Int, count: Int) -> Double {
        if count <= 1 { return (ringInner + ringOuter) / 2 }
        let t = Double(index) / Double(count - 1)
        return ringOuter + (ringInner - ringOuter) * t
    }

    /// Angle of spoke `index` of `count`, radians, clockwise from 3 o'clock
    /// (+y points down). Spokes share the circle evenly except for the gap at
    /// 12 o'clock that holds the ring tags.
    static func spokeAngle(index: Int, count: Int) -> Double {
        let span = Double.pi * 2 - topGap
        return -Double.pi / 2 + topGap / 2 + (span * (Double(index) + 0.5)) / Double(max(1, count))
    }

    /// Half the angular width of one spoke's slice.
    static func spokeHalfWidth(count: Int) -> Double {
        (Double.pi * 2 - topGap) / Double(max(1, count)) / 2
    }

    static func polar(_ angle: Double, _ r: Double) -> CGPoint {
        CGPoint(x: cos(angle) * r, y: sin(angle) * r)
    }

    enum LabelAnchor: Equatable, Sendable {
        case start, middle, end
    }

    /// Labels read outward: right half left-aligned, left half right-aligned.
    static func labelAnchor(_ angle: Double) -> LabelAnchor {
        let c = cos(angle)
        if c > labelCenterBand { return .start }
        if c < -labelCenterBand { return .end }
        return .middle
    }

    /// True when the orbit layout is readable; otherwise render lanes.
    static func descentFits(stageWidth: Double, spokes: Int) -> Bool {
        stageWidth >= minWidth && spokes <= maxSpokes
    }

    // MARK: - Data → spoke state

    /// One ring on one app's spoke.
    /// - `off`: this app doesn't use the ring (not configured) — nothing drawn.
    /// - `empty`: configured, nothing deployed — a small dot.
    /// - `healthy` / `failed`: deployed; live health check passes or not.
    enum NodeState: Hashable, Sendable {
        case off, empty, healthy, failed
    }

    struct SpokeNode: Hashable, Sendable {
        var ring: String
        var label: String
        var version: String?
        var state: NodeState
        /// Holds the app's newest version (solid) rather than an older one (outline).
        var fresh: Bool
        var ttfbMs: Int?
        /// Why entry into this ring is blocked right now, if it is.
        var gateClosed: String?
    }

    struct Spoke: Hashable, Sendable {
        var nodes: [SpokeNode]
        /// Newest version anywhere on the spoke — the one travelling inward.
        var newest: String?
        /// Deepest ring index holding `newest`; -1 when nothing is deployed.
        var frontier: Int
        /// Ring index right after the frontier whose gate is closed; -1 if none.
        var gateAt: Int
    }

    /// Live reason a ring can't be entered, or nil. Only states we can observe.
    static func closedGate(_ view: RingStatus?) -> String? {
        guard let g = view?.gates else { return nil }
        if g.maintenanceWindow && !g.maintenanceWindowOpen { return "Maintenance window closed" }
        if g.grafana == true && g.grafanaStatus?.verdict == "no_go" { return "Grafana no-go" }
        return nil
    }

    /// Build one app's spoke. `order` is the canonical promotion order
    /// (int → … → prod) from `/api/apps`; rings the server didn't report are `off`.
    static func buildSpoke(_ rings: [RingStatus]?, order: [String]) -> Spoke {
        let all = rings ?? []
        var byName: [String: RingStatus] = [:]
        for r in all { byName[r.ring.name] = r }
        let names = order.isEmpty ? all.map(\.ring.name) : order
        let raw: [SpokeNode] = names.map { name in
            let v = byName[name]
            let configured = v?.configured ?? false
            let version: String? = configured && !(v?.currentVersion.isEmpty ?? true)
                ? v?.currentVersion : nil
            let state: NodeState = !configured
                ? .off
                : version == nil
                    ? .empty
                    : (v?.liveHealthy ?? false) ? .healthy : .failed
            return SpokeNode(
                ring: name,
                label: v?.ring.label ?? name,
                version: version,
                state: state,
                fresh: false,
                ttfbMs: v?.ttfbMs ?? v?.latencyMs,
                gateClosed: configured ? closedGate(v) : nil
            )
        }
        // Versions enter at the first ring, so the outermost deployed version
        // is the newest one in flight.
        let newest = raw.first { $0.version != nil }?.version
        var frontier = -1
        for (i, n) in raw.enumerated() where newest != nil && n.version == newest {
            frontier = i
        }
        var gateAt = -1
        if frontier >= 0 {
            var i = frontier + 1
            while i < raw.count {
                if raw[i].state == .off { i += 1; continue }
                if raw[i].gateClosed != nil { gateAt = i }
                break
            }
        }
        return Spoke(
            nodes: raw.map { n in
                var n = n
                n.fresh = newest != nil && n.version == newest
                return n
            },
            newest: newest,
            frontier: frontier,
            gateAt: gateAt
        )
    }

    /// Next ring the newest version would enter, skipping unused rings.
    static func nextRingIndex(_ spoke: Spoke) -> Int {
        if spoke.frontier < 0 { return -1 }
        var i = spoke.frontier + 1
        while i < spoke.nodes.count {
            if spoke.nodes[i].state != .off { return i }
            i += 1
        }
        return -1
    }

    /// One-line reading of a spoke, for accessibility labels and the detail card.
    static func spokeSentence(_ spoke: Spoke) -> String {
        if spoke.frontier < 0 { return "Nothing deployed yet" }
        let at = spoke.nodes[spoke.frontier]
        let newest = spoke.newest ?? ""
        let next = nextRingIndex(spoke)
        if next < 0 { return "\(newest) is live in \(at.ring)" }
        var hops = 0
        for i in (spoke.frontier + 1)..<spoke.nodes.count where spoke.nodes[i].state != .off {
            hops += 1
        }
        var s = "\(newest) has reached \(at.ring), \(hops) ring\(hops == 1 ? "" : "s") to go"
        if spoke.gateAt >= 0 {
            let g = spoke.nodes[spoke.gateAt]
            s += " · \(g.ring) gate closed: \(g.gateClosed ?? "")"
        }
        return s
    }

    // MARK: - Groups

    struct GroupSpan: Hashable, Sendable {
        var id: String
        var name: String
        var from: Int
        var to: Int
    }

    /// Order apps so each group's spokes sit together, groups in config order,
    /// ungrouped apps last. An app in several groups goes with the first one.
    static func orderByGroup(
        _ apps: [String], groups: [AppGroup]
    ) -> (ordered: [String], spans: [GroupSpan]) {
        let known = Set(apps)
        var placed = Set<String>()
        var ordered: [String] = []
        var spans: [GroupSpan] = []
        for g in groups {
            var members: [String] = []
            for a in g.apps where known.contains(a) && !placed.contains(a) && !members.contains(a) {
                members.append(a)
            }
            if members.isEmpty { continue }
            spans.append(GroupSpan(
                id: g.id, name: g.name, from: ordered.count, to: ordered.count + members.count - 1
            ))
            for a in members {
                placed.insert(a)
                ordered.append(a)
            }
        }
        for a in apps where !placed.contains(a) { ordered.append(a) }
        return (ordered, spans)
    }

    /// Start and end angles of a group span's arc, with a small gap at each
    /// end. The web's `groupArcPath` draws exactly this arc at `groupArcR`.
    static func groupArcAngles(_ span: GroupSpan, count: Int) -> (start: Double, end: Double) {
        let half = spokeHalfWidth(count: count) - 0.035
        return (
            spokeAngle(index: span.from, count: count) - half,
            spokeAngle(index: span.to, count: count) + half
        )
    }

    /// The group arc as a path in design coordinates (the web's `groupArcPath`).
    static func groupArcPath(_ span: GroupSpan, count: Int) -> Path {
        let (a0, a1) = groupArcAngles(span, count: count)
        var p = Path()
        p.addArc(
            center: .zero, radius: groupArcR,
            startAngle: .radians(a0), endAngle: .radians(a1), clockwise: false
        )
        return p
    }

    // MARK: - iOS-only helpers (no web counterpart; the web uses DOM hit-testing)

    /// Which spoke a direction from the centre falls in, or nil inside the
    /// 12 o'clock gap. Inverse of `spokeAngle`.
    static func spokeIndex(atAngle angle: Double, count: Int) -> Int? {
        guard count > 0 else { return nil }
        let start = -Double.pi / 2 + topGap / 2
        var rel = (angle - start).truncatingRemainder(dividingBy: 2 * .pi)
        if rel < 0 { rel += 2 * .pi }
        let span = Double.pi * 2 - topGap
        guard rel < span else { return nil }
        return min(count - 1, Int(rel / (span / Double(count))))
    }
}
