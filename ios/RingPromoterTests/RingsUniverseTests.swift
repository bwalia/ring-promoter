import Foundation
import Testing

@testable import RingPromoter

/// The Rings of Applications screen ("Descent"): layout maths, spoke state and
/// status roll-ups.
///
/// The geometry is a port of the web console's `descent-layout.ts`; these tests
/// pin the numbers so the two clients keep drawing the same stage.
@Suite("Rings of Applications")
struct RingsUniverseTests {

    private let order = ["int", "test", "acc", "prod"]

    private func ring(
        _ name: String, _ version: String = "", healthy: Bool = true,
        configured: Bool = true, gates: RingGates = .none, latency: Int? = nil
    ) -> RingStatus {
        PreviewData.ring(
            name, label: name.capitalized, version: version, healthy: healthy,
            configured: configured, gates: gates, latency: latency
        )
    }

    // MARK: - Constants

    @Test("constants match web descent-layout.ts")
    func constantsMatchWeb() {
        #expect(DescentLayout.design == 1000)
        #expect(DescentLayout.earthR == 86)
        #expect(DescentLayout.ringInner == 132)
        #expect(DescentLayout.ringOuter == 318)
        #expect(DescentLayout.groupArcR == 334)
        #expect(DescentLayout.labelR == 350)
        #expect(DescentLayout.topGap == 0.34)
        #expect(DescentLayout.labelCenterBand == 0.12)
        #expect(DescentLayout.nodeR == 8)
        #expect(DescentLayout.emptyR == 3)
        #expect(DescentLayout.maxSpokes == 36)
        #expect(DescentLayout.minWidth == 560)
        #expect(DescentLayout.gateHalf == 11)
        #expect(DescentLayout.cometSeconds == 1.6)
    }

    // MARK: - Geometry

    @Test("the first ring is outermost, the last sits just above Earth")
    func ringRadiusEndpoints() {
        #expect(DescentLayout.ringRadius(index: 0, count: 4) == DescentLayout.ringOuter)
        #expect(DescentLayout.ringRadius(index: 3, count: 4) == DescentLayout.ringInner)
        #expect(abs(DescentLayout.ringRadius(index: 1, count: 4) - 256) < 1e-9)
        #expect(abs(DescentLayout.ringRadius(index: 2, count: 4) - 194) < 1e-9)
        // A single ring sits midway.
        #expect(DescentLayout.ringRadius(index: 0, count: 1) == 225)
        #expect(DescentLayout.ringRadius(index: 0, count: 0) == 225)
        #expect(DescentLayout.ringInner > DescentLayout.earthR)
    }

    @Test("spokes leave a symmetric gap at 12 o'clock")
    func spokeAngleGapSymmetry() {
        for count in [1, 2, 3, 7, 14, 36] {
            let first = DescentLayout.spokeAngle(index: 0, count: count)
            let last = DescentLayout.spokeAngle(index: count - 1, count: count)
            let top = -Double.pi / 2
            // Distance from 12 o'clock to the first spoke (clockwise) equals
            // the distance from the last spoke to 12 o'clock (clockwise).
            let before = first - top
            let after = (top + 2 * .pi) - last
            #expect(abs(before - after) < 1e-9)
            #expect(before >= DescentLayout.topGap / 2)
            // Even spacing.
            if count > 1 {
                let step = DescentLayout.spokeAngle(index: 1, count: count) - first
                #expect(abs(step - 2 * DescentLayout.spokeHalfWidth(count: count)) < 1e-9)
            }
        }
        // One spoke points straight down.
        #expect(abs(DescentLayout.spokeAngle(index: 0, count: 1) - .pi / 2) < 1e-9)
    }

    @Test("spoke index inverts spoke angle and the gap hits nothing")
    func spokeIndexInverse() {
        for count in [1, 5, 36] {
            for i in 0..<count {
                let a = DescentLayout.spokeAngle(index: i, count: count)
                #expect(DescentLayout.spokeIndex(atAngle: a, count: count) == i)
                #expect(DescentLayout.spokeIndex(atAngle: a + 2 * .pi, count: count) == i)
            }
            #expect(DescentLayout.spokeIndex(atAngle: -.pi / 2, count: count) == nil)
        }
    }

    @Test("polar is clockwise from 3 o'clock with +y down")
    func polarConvention() {
        let p = DescentLayout.polar(.pi / 2, 10)
        #expect(abs(p.x) < 1e-9)
        #expect(abs(p.y - 10) < 1e-9)
    }

    @Test("labels read outward: start on the right, end on the left, middle near 12/6")
    func labelAnchorBands() {
        #expect(DescentLayout.labelAnchor(0) == .start)
        #expect(DescentLayout.labelAnchor(.pi) == .end)
        #expect(DescentLayout.labelAnchor(.pi / 2) == .middle)
        #expect(DescentLayout.labelAnchor(-.pi / 2) == .middle)
        // Band edges: |cos| must exceed 0.12 to leave the middle.
        let edge = acos(0.12)
        #expect(DescentLayout.labelAnchor(edge + 0.001) == .middle)
        #expect(DescentLayout.labelAnchor(edge - 0.001) == .start)
        #expect(DescentLayout.labelAnchor(.pi - edge + 0.001) == .end)
    }

    @Test("orbit fits only on wide stages with at most 36 spokes")
    func descentFitsThresholds() {
        #expect(DescentLayout.descentFits(stageWidth: 560, spokes: 36))
        #expect(!DescentLayout.descentFits(stageWidth: 559.9, spokes: 5))
        #expect(!DescentLayout.descentFits(stageWidth: 1200, spokes: 37))
        #expect(!DescentLayout.descentFits(stageWidth: 402, spokes: 3))
        #expect(DescentLayout.descentFits(stageWidth: 874, spokes: 0))
    }

    // MARK: - Spokes

    @Test("the frontier is the deepest ring holding the newest version")
    func buildSpokeFrontier() {
        let spoke = DescentLayout.buildSpoke(
            [ring("int", "2.3.1"), ring("test", "2.3.1"), ring("acc", "2.3.0"), ring("prod", "2.2.9")],
            order: order
        )
        #expect(spoke.newest == "2.3.1")
        #expect(spoke.frontier == 1)
        #expect(spoke.gateAt == -1)
        #expect(spoke.nodes.map(\.fresh) == [true, true, false, false])
        #expect(spoke.nodes.map(\.state) == [.healthy, .healthy, .healthy, .healthy])
        #expect(DescentLayout.nextRingIndex(spoke) == 2)
        #expect(DescentLayout.spokeSentence(spoke) == "2.3.1 has reached test, 2 rings to go")
    }

    @Test("newest is the outermost deployed version, even when int is empty")
    func buildSpokeNewestSkipsEmpty() {
        let spoke = DescentLayout.buildSpoke(
            [ring("int"), ring("test", "1.1"), ring("acc", "1.1"), ring("prod", "1.0")],
            order: order
        )
        #expect(spoke.newest == "1.1")
        #expect(spoke.frontier == 2)
        #expect(spoke.nodes[0].state == .empty)
        #expect(!spoke.nodes[0].fresh)
    }

    @Test("unconfigured and unreported rings are off; failing health is failed")
    func buildSpokeOffAndFailed() {
        let spoke = DescentLayout.buildSpoke(
            [
                ring("int", "3.0", healthy: false),
                ring("test", "2.9", configured: false),
            ],
            order: order
        )
        #expect(spoke.nodes.map(\.state) == [.failed, .off, .off, .off])
        #expect(spoke.nodes[1].version == nil, "an unconfigured ring never shows a version")
        #expect(spoke.nodes[2].label == "acc", "unreported rings fall back to their name")
        #expect(spoke.frontier == 0)
        #expect(DescentLayout.nextRingIndex(spoke) == -1)
        #expect(DescentLayout.spokeSentence(spoke) == "3.0 is live in int")
    }

    @Test("nothing deployed: frontier -1 and a plain sentence")
    func buildSpokeEmpty() {
        let spoke = DescentLayout.buildSpoke(
            order.map { ring($0) }, order: order
        )
        #expect(spoke.newest == nil)
        #expect(spoke.frontier == -1)
        #expect(spoke.gateAt == -1)
        #expect(spoke.nodes.allSatisfy { $0.state == .empty && !$0.fresh })
        #expect(DescentLayout.nextRingIndex(spoke) == -1)
        #expect(DescentLayout.spokeSentence(spoke) == "Nothing deployed yet")

        let none = DescentLayout.buildSpoke(nil, order: [])
        #expect(none.nodes.isEmpty)
        #expect(none.frontier == -1)
    }

    @Test("empty order falls back to the reported rings")
    func buildSpokeFallbackOrder() {
        let spoke = DescentLayout.buildSpoke([ring("a", "1"), ring("b")], order: [])
        #expect(spoke.nodes.map(\.ring) == ["a", "b"])
        #expect(spoke.frontier == 0)
    }

    @Test("a closed gate on the next used ring is reported, skipping off rings")
    func buildSpokeGateAt() {
        var grafanaNoGo = RingGates()
        grafanaNoGo.grafana = true
        grafanaNoGo.grafanaStatus = GrafanaVerdict(verdict: "no_go")
        let spoke = DescentLayout.buildSpoke(
            [
                ring("int", "2.3.1"), ring("test", "2.3.1"),
                ring("acc", configured: false),
                ring("prod", "2.2.0", gates: PreviewData.fullyGated),
            ],
            order: order
        )
        #expect(spoke.frontier == 1)
        #expect(spoke.gateAt == 3)
        #expect(spoke.nodes[3].gateClosed == "Maintenance window closed")
        #expect(
            DescentLayout.spokeSentence(spoke)
                == "2.3.1 has reached test, 1 ring to go · prod gate closed: Maintenance window closed"
        )

        // An open window doesn't block; a Grafana no-go does.
        let open = DescentLayout.buildSpoke(
            [ring("int", "1"), ring("test", gates: PreviewData.openWindowGates)], order: []
        )
        #expect(open.gateAt == -1)
        let grafana = DescentLayout.buildSpoke(
            [ring("int", "1"), ring("test", gates: grafanaNoGo)], order: []
        )
        #expect(grafana.gateAt == 1)
        #expect(grafana.nodes[1].gateClosed == "Grafana no-go")

        // Only the ring right after the frontier counts.
        let later = DescentLayout.buildSpoke(
            [ring("int", "1"), ring("test"), ring("acc", gates: PreviewData.fullyGated)],
            order: []
        )
        #expect(later.gateAt == -1)

        // A gate on an unconfigured ring is never reported.
        #expect(DescentLayout.closedGate(nil) == nil)
    }

    @Test("TTFB prefers ttfb_ms, then latency_ms")
    func buildSpokeTTFB() {
        var r = ring("int", "1", latency: 40)
        #expect(DescentLayout.buildSpoke([r], order: []).nodes[0].ttfbMs == 40)
        r.ttfbMs = 12
        #expect(DescentLayout.buildSpoke([r], order: []).nodes[0].ttfbMs == 12)
    }

    // MARK: - Groups

    @Test("groups sit contiguously in config order, ungrouped apps last")
    func orderByGroupContiguous() {
        let groups = [
            AppGroup(id: "g1", name: "Core", apps: ["c", "a", "ghost"], updatedAt: .now),
            AppGroup(id: "g2", name: "Edge", apps: ["a", "e"], updatedAt: .now),
            AppGroup(id: "g3", name: "Empty", apps: ["ghost"], updatedAt: .now),
        ]
        let (ordered, spans) = DescentLayout.orderByGroup(["a", "b", "c", "d", "e"], groups: groups)
        #expect(ordered == ["c", "a", "e", "b", "d"])
        #expect(spans == [
            .init(id: "g1", name: "Core", from: 0, to: 1),
            // "a" already went with Core, its first group.
            .init(id: "g2", name: "Edge", from: 2, to: 2),
        ])

        let plain = DescentLayout.orderByGroup(["x", "y"], groups: [])
        #expect(plain.ordered == ["x", "y"])
        #expect(plain.spans.isEmpty)
    }

    @Test("a group arc spans its members' spokes with a small gap each end")
    func groupArcAngles() {
        let span = DescentLayout.GroupSpan(id: "g", name: "G", from: 1, to: 3)
        let (a0, a1) = DescentLayout.groupArcAngles(span, count: 8)
        let half = DescentLayout.spokeHalfWidth(count: 8) - 0.035
        #expect(abs(a0 - (DescentLayout.spokeAngle(index: 1, count: 8) - half)) < 1e-9)
        #expect(abs(a1 - (DescentLayout.spokeAngle(index: 3, count: 8) + half)) < 1e-9)
        let bounds = DescentLayout.groupArcPath(span, count: 8).boundingRect
        #expect(bounds.width > 0)
    }

    @Test("the descent view model orders by group and reads the spoke")
    func descentAppOrdering() {
        let summaries = ["a", "b", "c"].map {
            AppSummary(
                name: $0, title: $0.uppercased(), rings: PreviewData.healthyRings,
                latestJob: nil, loadError: nil
            )
        }
        let group = AppGroup(id: "g1", name: "Core", apps: ["c"], updatedAt: .now)
        let (apps, spans) = DescentApp.ordered(summaries: summaries, groups: [group], order: order)
        #expect(apps.map(\.id) == ["c", "a", "b"])
        #expect(spans.count == 1)
        // healthyRings: 2.7.2 only on int.
        #expect(apps[0].subtitle == "2.7.2 · int")
        #expect(apps[0].status == .healthy)
    }

    // MARK: - Status

    @Test("a running job shows as deploying whatever the health snapshot says")
    func busyOverridesHealth() {
        #expect(FleetStatus(rings: PreviewData.troubledRings, isBusy: true) == .deploying)
    }

    @Test("ring health rolls up to healthy, degraded, failed or empty")
    func statusRollup() {
        #expect(FleetStatus(rings: PreviewData.healthyRings, isBusy: false) == .healthy)
        // troubledRings: one healthy deploy, one failing deploy → degraded.
        #expect(FleetStatus(rings: PreviewData.troubledRings, isBusy: false) == .degraded)
        let allFailing = [
            PreviewData.ring("int", label: "Integration", version: "1.0", healthy: false)
        ]
        #expect(FleetStatus(rings: allFailing, isBusy: false) == .failed)
        let neverDeployed = [PreviewData.ring("int", label: "Integration")]
        #expect(FleetStatus(rings: neverDeployed, isBusy: false) == .empty)
        #expect(FleetStatus(rings: [], isBusy: false, ringsUnknown: true) == .loading)
    }

    @Test("the aggregate is worst-first")
    func aggregateOrder() {
        #expect(FleetStatus.aggregate([.healthy, .failed, .deploying]) == .failed)
        #expect(FleetStatus.aggregate([.healthy, .degraded]) == .degraded)
        #expect(FleetStatus.aggregate([.healthy, .empty]) == .healthy)
        #expect(FleetStatus.aggregate([]) == .empty)
    }

    // MARK: - Model & navigation

    @Test("latency_ms and grafana gate fields decode when present and default when absent")
    func latencyDecoding() throws {
        let json = """
            {
                "ring": {"name": "prod", "label": "Production"},
                "configured": true, "current_version": "1.0", "previous_version": "",
                "live_version": "1.0", "healthy": true, "live_healthy": true,
                "latency_ms": 42, "auto_promote": false, "auto_promote_managed": false,
                "updated_at": "2026-07-29T10:38:47Z", "can_promote_from": true,
                "gates": {
                    "maintenance_window": false, "qa_signoff": false,
                    "change_request": false, "maintenance_window_open": false,
                    "grafana": true,
                    "grafana_status": {"verdict": "no_go", "checked_at": "2026-07-29T10:38:47Z"}
                }
            }
            """
        let decoder = JSONCoding.makeDecoder()
        let decoded = try decoder.decode(
            RingStatus.self, from: try #require(json.data(using: .utf8))
        )
        #expect(decoded.latencyMs == 42)
        #expect(decoded.gates.grafana == true)
        #expect(decoded.gates.grafanaStatus?.verdict == "no_go")
        #expect(DescentLayout.closedGate(decoded) == "Grafana no-go")

        let without = try decoder.decode(
            RingStatus.self,
            from: try #require(
                json.replacingOccurrences(of: "\"latency_ms\": 42,", with: "")
                    .replacingOccurrences(
                        of: #",\s*"grafana": true,\s*"grafana_status": \{[^}]*\}"#,
                        with: "", options: .regularExpression
                    )
                    .data(using: .utf8)
            )
        )
        #expect(without.latencyMs == nil)
        #expect(without.gates.grafana == nil)
        #expect(without.gates.grafanaStatus == nil)
    }

    // MARK: - Earth

    @Test("a point at lat 0 lng 0 faces the camera at the disc centre")
    func globeNadirIsCentre() {
        let p = EarthGeometry.projectOrtho(latDeg: 0, lngDeg: 0, radius: 58, spin: 0)
        #expect(abs(p.x) < 0.001)
        #expect(abs(p.y) < 0.001)
        #expect(p.front)
        #expect(abs(p.z - 58) < 0.001)
    }

    @Test("north pole projects to the top of the disc")
    func globeNorthPole() {
        let p = EarthGeometry.projectOrtho(
            latDeg: 90, lngDeg: 0, radius: 58, spin: 0, center: CGPoint(x: 200, y: 200)
        )
        #expect(abs(p.x - 200) < 0.001)
        #expect(abs(p.y - 142) < 0.001)
    }

    @Test("earth spin hides a point that started on the front")
    func globeSpinMovesPointToBack() {
        let front = EarthGeometry.projectOrtho(latDeg: 0, lngDeg: 0, radius: 58, spin: 0)
        let back = EarthGeometry.projectOrtho(latDeg: 0, lngDeg: 0, radius: 58, spin: .pi)
        #expect(front.front)
        #expect(!back.front)
    }

    @Test("Earth completes one revolution every 96 seconds")
    func earthSpinPeriodMatchesWeb() {
        #expect(EarthGeometry.earthSpinPeriod == 96)
        #expect(EarthGeometry.earthSpinPeriodReduced == 20 * 60)
        #expect(abs(EarthGeometry.earthSpin(elapsed: 48, reduceMotion: false) - .pi) < 0.001)
        #expect(abs(EarthGeometry.earthSpin(elapsed: 96, reduceMotion: false)) < 0.001)
        #expect(EarthGeometry.earthSpin(elapsed: 0, reduceMotion: true) == 0)
        #expect(EarthGeometry.earthSpin(elapsed: 96, reduceMotion: true) < 0.6)
    }

    // MARK: - Stage fit

    @Test("the stage frame keeps the label ring inside the view")
    func stageFrameFits() {
        for size in [CGSize(width: 874, height: 300), CGSize(width: 1024, height: 1200)] {
            let f = DescentFrame.fit(size: size, topInset: 52, bottomInset: 44)
            let r = DescentLayout.labelR * f.scale
            #expect(f.center.y - r >= 52)
            #expect(f.center.y + r <= size.height - 44)
            #expect(f.center.x - r >= 0 && f.center.x + r <= size.width)
        }
    }

    @Test("the Rings tab exists and sits between Overview and Activity")
    @MainActor
    func ringsTabRegistered() {
        #expect(
            Router.Tab.allCases == [.overview, .rings, .activity, .settings]
        )
        #expect(Router.Tab.rings.label == "Rings")
    }
}
