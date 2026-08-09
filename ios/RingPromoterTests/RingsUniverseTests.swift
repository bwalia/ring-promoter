import Foundation
import Testing

@testable import RingPromoter

/// The Rings of Applications screen's layout maths and status roll-ups.
///
/// The geometry is a port of the web console's `solar-layout.ts`; these tests
/// pin the numbers so the two clients keep drawing the same sky.
@Suite("Rings of Applications")
struct RingsUniverseTests {

    // MARK: - Geometry

    @Test("the hash is deterministic and stays in 0..<1")
    func hashDeterminism() {
        for name in ["web-frontend", "payments-api", "batch-worker", "", "α-app"] {
            let first = SolarLayout.hash01(name)
            #expect(first == SolarLayout.hash01(name))
            #expect(first >= 0 && first < 1)
        }
        #expect(SolarLayout.hash01("a") != SolarLayout.hash01("b"))
    }

    @Test("no latency parks a body on the default track")
    func defaultRadius() {
        #expect(SolarLayout.radius(forLatencyMs: nil) == SolarLayout.defaultRadius)
    }

    @Test("latency maps onto the discrete tracks, monotonically")
    func latencyToTracks() {
        #expect(SolarLayout.radius(forLatencyMs: 0) == SolarLayout.tracks.first)
        #expect(SolarLayout.radius(forLatencyMs: 800) == SolarLayout.tracks.last)
        // Anything past the compression ceiling stays on the outermost track.
        #expect(SolarLayout.radius(forLatencyMs: 5_000) == SolarLayout.tracks.last)

        var previous = 0.0
        for ms in [1, 10, 50, 100, 250, 500, 800] {
            let radius = SolarLayout.radius(forLatencyMs: ms)
            #expect(SolarLayout.tracks.contains(radius))
            #expect(radius >= previous)
            previous = radius
        }
    }

    @Test("bodies on one track are spread evenly and ordered alphabetically")
    func evenSpread() {
        let planets = SolarLayout.planets(for: [
            ("charlie", 100), ("alpha", 100), ("bravo", 100),
        ])
        #expect(planets.map(\.id) == ["alpha", "bravo", "charlie"])
        #expect(Set(planets.map(\.angle0)).count == 3)

        // Even spread: successive seed angles are 2π/3 apart, give or take the
        // small deterministic jitter (±0.06 rad each). Axis clearance is shared.
        let gap = 2 * Double.pi / 3
        for pair in zip(planets, planets.dropFirst()) {
            let delta = pair.1.angle0 - pair.0.angle0
            #expect(abs(delta - gap) < 0.15)
        }
    }

    @Test("outer orbits revolve slower than inner ones")
    func orbitalPeriods() {
        let planets = SolarLayout.planets(for: [
            ("inner", SolarLayout.tracks.first ?? 68),
            ("outer", SolarLayout.tracks.last ?? 162),
        ])
        let inner = planets.first { $0.id == "inner" }
        let outer = planets.first { $0.id == "outer" }
        #expect((inner?.period ?? 0) < (outer?.period ?? 0))
    }

    @Test("a body's position stays on its orbit as time passes")
    func positionsStayOnOrbit() {
        let planet = SolarLayout.planets(for: [("web-frontend", 132)])[0]
        for elapsed in [0.0, 13.7, 92.4] {
            let pos = SolarLayout.position(of: planet, at: elapsed)
            let dx = pos.x - SolarLayout.center
            let dy = pos.y - SolarLayout.center
            #expect(abs((dx * dx + dy * dy).squareRoot() - planet.r) < 0.001)
        }
    }

    @Test("orbit tracks and latency bands match the web console")
    func tracksMatchWeb() {
        #expect(SolarLayout.tracks == [68, 100, 132, 162])
        #expect(SolarLayout.defaultRadius == 100)
        #expect(SolarLayout.orbitBands.map(\.r) == SolarLayout.tracks)
        #expect(!SolarLayout.orbitBands[0].label.isEmpty)
    }

    @Test("crowded tracks stagger draw radius off the latency band")
    func crowdedStagger() {
        let ids = (0..<8).map { "app-\($0)" }
        let planets = SolarLayout.planets(for: ids.map { ($0, 100.0) })
        #expect(planets.count == 8)
        #expect(Set(planets.map(\.track)) == [100])
        #expect(Set(planets.map(\.r)).count > 1)
    }

    // MARK: - Latency selection

    @Test("production's latency wins when it reported one")
    func prodLatencyPreferred() {
        let rings = [
            PreviewData.ring("int", label: "Integration", version: "1.0", latency: 20),
            PreviewData.ring("prod", label: "Production", version: "1.0", latency: 90),
        ]
        #expect(SolarLayout.latency(for: rings) == 90)
    }

    @Test("without production, the prod-most ring that reported wins")
    func fallbackLatency() {
        let rings = [
            PreviewData.ring("int", label: "Integration", version: "1.0", latency: 20),
            PreviewData.ring("test", label: "Test", version: "1.0", latency: 55),
            PreviewData.ring("acc", label: "Acceptance", version: "1.0"),
        ]
        #expect(SolarLayout.latency(for: rings) == 55)
        #expect(SolarLayout.latency(for: []) == nil)
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

    // MARK: - Nodes

    @Test("ring nodes cover every application exactly once")
    func ringNodesPartition() {
        let summaries = ["a", "b", "c"].map {
            AppSummary(
                name: $0, title: $0.uppercased(), rings: PreviewData.healthyRings,
                latestJob: nil, loadError: nil
            )
        }
        let group = AppGroup(id: "g1", name: "Core", apps: ["a", "b"], updatedAt: .now)
        let nodes = FleetNode.ringNodes(from: summaries, groups: [group])

        #expect(nodes.map(\.id) == ["g1", FleetNode.ungroupedID])
        #expect(nodes[0].apps == ["a", "b"])
        #expect(nodes[0].subtitle == "2 apps")
        #expect(nodes[1].apps == ["c"])
        #expect(nodes[1].subtitle == "1 app")
        // Every member reported the same latency; the roll-up takes the max.
        #expect(nodes[0].latencyMs == SolarLayout.latency(for: PreviewData.healthyRings))
    }

    @Test("an app node carries the prod-most version and deploy facts")
    func appNodeFacts() {
        let node = FleetNode(
            summary: AppSummary(
                name: "web-frontend", title: "Web Frontend",
                rings: PreviewData.healthyRings, latestJob: nil, loadError: nil
            )
        )
        #expect(node.latestVersion == "2.6.9 · prod")
        #expect(node.activeCount == 4)
        #expect(node.healthyCount == 4)
        #expect(node.latencyMs == 47)
        #expect(node.status == .healthy)
    }

    // MARK: - Model & navigation

    @Test("latency_ms decodes when present and defaults to nil when absent")
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
                    "change_request": false, "maintenance_window_open": false
                }
            }
            """
        let decoder = JSONCoding.makeDecoder()
        let decoded = try decoder.decode(
            RingStatus.self, from: try #require(json.data(using: .utf8))
        )
        #expect(decoded.latencyMs == 42)

        let without = try decoder.decode(
            RingStatus.self,
            from: try #require(
                json.replacingOccurrences(of: "\"latency_ms\": 42,", with: "")
                    .data(using: .utf8)
            )
        )
        #expect(without.latencyMs == nil)
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
