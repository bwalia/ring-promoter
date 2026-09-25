import SwiftUI

/// Earth — live users — at the centre of the Rings of Apps stage, plus the
/// quiet star field behind it. Moved here from the retired orbital layout; the
/// drawing is unchanged, only its inputs (centre, radius) are now explicit.

/// The pure maths behind the globe: orthographic projection, spin clock and
/// the coarse continent outlines matching the web console's globe.
enum EarthGeometry {
    /// Earth radius (points) the hairline widths were designed at.
    static let referenceRadius: Double = 58
    /// Seconds per revolution.
    static let earthSpinPeriod: Double = 96
    /// Reduce Motion: one revolution per 20 minutes, matching web.
    static let earthSpinPeriodReduced: Double = 20 * 60

    struct GlobePoint: Sendable {
        let x: Double
        let y: Double
        let z: Double
        var front: Bool { z >= -0.5 }
    }

    static func earthSpin(elapsed: TimeInterval, reduceMotion: Bool) -> Double {
        let period = reduceMotion ? earthSpinPeriodReduced : earthSpinPeriod
        let turns = elapsed / period
        return (turns - turns.rounded(.down)) * 2 * .pi
    }

    static func projectOrtho(
        latDeg: Double, lngDeg: Double, radius: Double, spin: Double,
        center: CGPoint = .zero
    ) -> GlobePoint {
        let lat = latDeg * .pi / 180
        let lng = lngDeg * .pi / 180 + spin
        let cosLat = cos(lat)
        let x = radius * cosLat * sin(lng)
        let y = -radius * sin(lat)
        let z = radius * cosLat * cos(lng)
        return GlobePoint(x: center.x + x, y: center.y + y, z: z)
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

/// Orthographic Earth matching the web console canvas: ocean, graticule,
/// coarse land, camera-fixed terminator and specular. Continents rotating
/// under that lighting is what reads as a sphere instead of a flat disc.
/// Spin is the only input that moves; reduced-motion callers pass 0.
struct EarthGlobe: View {
    let spin: Double
    /// Centre of the disc in the view's coordinate space.
    let center: CGPoint
    /// Disc radius, points.
    let radius: Double
    /// Hairline scale; 1 at a 400-point stage like the original design.
    var strokeScale: Double { max(0.6, radius / EarthGeometry.referenceRadius) }

    var body: some View {
        Canvas { context, _ in
            let s = strokeScale
            let c = center
            let r = radius
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
                        let p = EarthGeometry.projectOrtho(
                            latDeg: lat, lngDeg: lng, radius: r, spin: spin, center: c
                        )
                        guard p.front else { started = false; continue }
                        let pt = CGPoint(x: p.x, y: p.y)
                        if started { grid.addLine(to: pt) } else { grid.move(to: pt); started = true }
                    }
                }
                for lat in stride(from: -60.0, through: 60.0, by: 30) {
                    var started = false
                    for lng in stride(from: -180.0, through: 180.0, by: 5) {
                        let p = EarthGeometry.projectOrtho(
                            latDeg: lat, lngDeg: lng, radius: r, spin: spin, center: c
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
                for poly in EarthGeometry.landPolys {
                    var path = Path()
                    var started = false
                    var frontCount = 0
                    for pt in poly {
                        let p = EarthGeometry.projectOrtho(
                            latDeg: pt.lat, lngDeg: pt.lng, radius: r, spin: spin, center: c
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
        .allowsHitTesting(false)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Earth: live users")
    }
}

/// Sparse star field — quiet atmosphere so the rings and spokes dominate.
struct StarField: View {
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
