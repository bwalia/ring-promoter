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

    @Test("a body at lat 0 lng 0 faces the camera at the disc centre")
    func globeNadirIsCentre() {
        let p = SolarLayout.projectOrtho(latDeg: 0, lngDeg: 0, radius: SolarLayout.earthR, spin: 0)
        #expect(abs(p.x - SolarLayout.center) < 0.001)
        #expect(abs(p.y - SolarLayout.center) < 0.001)
        #expect(p.front)
        #expect(abs(p.z - SolarLayout.earthR) < 0.001)
    }

    @Test("north pole projects to the top of the disc")
    func globeNorthPole() {
        let p = SolarLayout.projectOrtho(latDeg: 90, lngDeg: 0, radius: SolarLayout.earthR, spin: 0)
        #expect(abs(p.x - SolarLayout.center) < 0.001)
        #expect(p.y < SolarLayout.center)
        #expect(abs(p.y - (SolarLayout.center - SolarLayout.earthR)) < 0.001)
    }

    @Test("earth spin hides a point that started on the front")
    func globeSpinMovesPointToBack() {
        let front = SolarLayout.projectOrtho(latDeg: 0, lngDeg: 0, radius: SolarLayout.earthR, spin: 0)
        let back = SolarLayout.projectOrtho(latDeg: 0, lngDeg: 0, radius: SolarLayout.earthR, spin: .pi)
        #expect(front.front)
        #expect(!back.front)
    }

    @Test("Earth completes one revolution every 96 seconds")
    func earthSpinPeriodMatchesWeb() {
        #expect(SolarLayout.earthSpinPeriod == 96)
        #expect(SolarLayout.earthSpinPeriodReduced == 20 * 60)
        #expect(abs(SolarLayout.earthSpin(elapsed: 48, reduceMotion: false) - .pi) < 0.001)
        #expect(abs(SolarLayout.earthSpin(elapsed: 96, reduceMotion: false)) < 0.001)
        // Reduce Motion: freeze (elapsed 0) or a 20-minute crawl.
        #expect(SolarLayout.earthSpin(elapsed: 0, reduceMotion: true) == 0)
        #expect(SolarLayout.earthSpin(elapsed: 96, reduceMotion: true) < 0.6)
    }

    @Test("haversine London–New York is in the expected band")
    func haversineLondonNY() {
        let london = AppLocation(lat: 51.5074, lng: -0.1278, city: "London", region: "GB")
        let ny = AppLocation(lat: 40.7128, lng: -74.0060, city: "New York", region: "US")
        let km = SolarLayout.haversineKm(london, ny)
        #expect(km > 5_000 && km < 6_500)
        #expect(SolarLayout.estimateRttMs(km: km) > 40)
    }

    @Test("placed bodies keep their geography; unplaced ones still appear")
    func globeBodiesPlacement() {
        let london = AppLocation(lat: 51.5, lng: -0.1, city: "London", region: "GB")
        let placed = FleetNode(
            summary: AppSummary(
                name: "web-frontend", title: "Web Frontend",
                rings: PreviewData.healthyRings, latestJob: nil, loadError: nil,
                location: london
            )
        )
        let unplaced = FleetNode(
            summary: AppSummary(
                name: "batch-worker", title: "batch-worker",
                rings: PreviewData.healthyRings, latestJob: nil, loadError: nil
            )
        )
        let bodies = SolarLayout.globeBodies(for: [placed, unplaced])
        #expect(bodies.count == 2)
        let pin = bodies.first { $0.id == "web-frontend" }
        let sat = bodies.first { $0.id == "batch-worker" }
        #expect(pin?.placed == true)
        #expect(sat?.placed == false)
        #expect(abs((pin?.lat ?? 0) - 51.5) < 2)
        #expect((pin?.inclination ?? 0) >= SolarLayout.minInclination - 4)
        #expect(sat?.driftDegPerSec ?? 0 > 0)
        #expect(pin?.driftDegPerSec ?? 0 > 0)
        #expect(abs((pin?.r ?? 0) - (sat?.r ?? 0)) > 1)
    }

    @Test("an equatorial orbit matches lat 0 orthographic projection")
    func orbitMatchesEquator() {
        let fromOrbit = SolarLayout.projectOrbit(
            radius: SolarLayout.earthR, inclinationDeg: 0, raanDeg: 0, argDeg: 40, spin: 0
        )
        let fromLatLng = SolarLayout.projectOrtho(
            latDeg: 0, lngDeg: 40, radius: SolarLayout.earthR, spin: 0
        )
        #expect(abs(fromOrbit.x - fromLatLng.x) < 0.001)
        #expect(abs(fromOrbit.y - fromLatLng.y) < 0.001)
        #expect(abs(fromOrbit.z - fromLatLng.z) < 0.001)
    }

    @Test("every app gets a distinctly different ring radius")
    func isolatedRadiiAreUnique() {
        let radii17 = SolarLayout.isolatedRadii(count: 17)
        #expect(radii17.count == 17)
        #expect(Set(radii17.map { ($0 * 1000).rounded() }).count == 17)
        let inner = SolarLayout.earthR + SolarLayout.ringInnerPad
        #expect(abs(radii17[0] - inner) < 0.001)
        #expect(abs(radii17[16] - SolarLayout.ringOuter) < 0.001)
        let gap = radii17[1] - radii17[0]
        #expect(gap > 6)
        for pair in zip(radii17, radii17.dropFirst()) {
            #expect(pair.1 - pair.0 > 6)
        }
        #expect(SolarLayout.isolatedRadii(count: 1).count == 1)
        #expect(SolarLayout.densityCap == 10)
        #expect(SolarLayout.saturnInclination == 24)
        #expect(SolarLayout.sunOffsetX == -112)
    }

    @Test("a larger stage grows Earth and the isolated ring band")
    func stageMetricsScaleWithViewSize() {
        let small = SolarLayout.GlobeMetrics.canonical
        let large = SolarLayout.GlobeMetrics(width: 800, height: 800)
        #expect(abs(large.earthR - small.earthR * 2) < 0.001)
        #expect(abs(large.ringOuter - small.ringOuter * 2) < 0.001)
        #expect(abs(large.sunOffsetX - small.sunOffsetX * 2) < 0.001)
        let rSmall = SolarLayout.isolatedRadii(count: 5, metrics: small)
        let rLarge = SolarLayout.isolatedRadii(count: 5, metrics: large)
        #expect(rSmall.count == 5)
        #expect(abs(rLarge[0] - rSmall[0] * 2) < 0.001)
        #expect(abs(rLarge[4] - rSmall[4] * 2) < 0.001)

        let wide = SolarLayout.GlobeMetrics(
            width: 1600, height: 800, topInset: 0, bottomInset: 0, verticalBias: 0.5
        )
        #expect(abs(wide.earthR - large.earthR) < 0.001)
        #expect(abs(wide.cx - 800) < 0.001)
        #expect(abs(wide.cy - 400) < 0.001)
        let pin = SolarLayout.projectOrtho(
            latDeg: 90, lngDeg: 0, radius: wide.earthR, spin: 0, metrics: wide
        )
        #expect(abs(pin.x - wide.cx) < 0.001)
        #expect(abs(pin.y - (wide.cy - wide.earthR)) < 0.001)

        // Bottom roster overlay: Earth must sit in the clear band above it,
        // not at raw mid-canvas (which reads as bottom-aligned empty sky).
        let tall = SolarLayout.GlobeMetrics(
            width: 390, height: 800, topInset: 78, bottomInset: 220, verticalBias: 0.46
        )
        let usable = 800.0 - 78 - 220
        let expectedCy = 78 + usable * 0.46
        #expect(abs(tall.cy - expectedCy) < 0.001)
        #expect(tall.cy < 800 / 2)
        #expect(tall.cy > 78)
        #expect(tall.cy + tall.ringOuter < 800 - 220 + 8)
    }

    @Test("faster TTFB parks closer in; equal TTFB still gets different sizes")
    func ttfbOrdersRadiiButNeverShares() {
        func node(_ name: String, ms: Int) -> FleetNode {
            FleetNode(
                summary: AppSummary(
                    name: name, title: name,
                    rings: [
                        PreviewData.ring("prod", label: "Production", version: "1.0", latency: ms)
                    ],
                    latestJob: nil, loadError: nil
                )
            )
        }
        let fast = node("alpha", ms: 12)
        let slow = node("omega", ms: 400)
        let alsoFast = node("beta", ms: 12)
        let bodies = SolarLayout.globeBodies(for: [slow, alsoFast, fast])
        let byID = Dictionary(uniqueKeysWithValues: bodies.map { ($0.id, $0) })
        #expect((byID["alpha"]?.r ?? 0) < (byID["omega"]?.r ?? 0))
        #expect((byID["beta"]?.r ?? 0) < (byID["omega"]?.r ?? 0))
        #expect(abs((byID["alpha"]?.r ?? 0) - (byID["beta"]?.r ?? 0)) > 1)
    }

    @Test("apps in the same city get distinct orbital rings")
    func sameCityRingsDiverge() {
        let london = AppLocation(lat: 51.5, lng: -0.1, city: "London", region: "GB")
        func node(_ name: String) -> FleetNode {
            FleetNode(
                summary: AppSummary(
                    name: name, title: name,
                    rings: PreviewData.healthyRings, latestJob: nil, loadError: nil,
                    location: london
                )
            )
        }
        let bodies = SolarLayout.globeBodies(for: [node("web-frontend"), node("payments-api")])
        #expect(bodies.count == 2)
        let a = bodies[0], b = bodies[1]
        #expect(abs(a.r - b.r) > 1)
        #expect(a.inclination >= SolarLayout.minInclination - 4)
        #expect(b.inclination >= SolarLayout.minInclination - 4)
    }

    @Test("orbit samples stay on a sphere of the body's radius")
    func orbitSamplesStayOnSphere() {
        let london = AppLocation(lat: 51.5, lng: -0.1, city: "London", region: "GB")
        let node = FleetNode(
            summary: AppSummary(
                name: "web-frontend", title: "Web Frontend",
                rings: PreviewData.healthyRings, latestJob: nil, loadError: nil,
                location: london
            )
        )
        let body = SolarLayout.globeBodies(for: [node])[0]
        for p in SolarLayout.sampleOrbit(body, spin: 0.4) {
            let dx = p.x - SolarLayout.center
            let dy = p.y - SolarLayout.center
            let radius = (dx * dx + dy * dy + p.z * p.z).squareRoot()
            #expect(abs(radius - body.r) < 0.001)
        }
        #expect(SolarLayout.minInclination == 22)
        #expect(SolarLayout.orbitSamples == 80)
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
