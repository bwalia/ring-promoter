import SwiftUI

/// The Ring Promoter mark: concentric rings with production lit at the centre
/// and a promotion in flight on the outermost one.
///
/// **This is the single definition.** The app icon is this view on its
/// squircle (see `ConsoleAppIcon`), and the sidebar draws the same thing at
/// 28pt — the two used to be unrelated, a flat SF Symbol in the sidebar and
/// this artwork in the Dock, which is exactly as odd as it sounds.
///
/// It simplifies as it shrinks. Three hairline rings inside 28pt is mush, so
/// below 48pt it drops to two and thickens them. That is the same concession
/// Apple's own icons make, and the family resemblance survives it: a dark
/// rounded square, rings, a lit core.
struct ConsoleBrandMark: View {
    var size: CGFloat
    /// The rounded square behind the rings. Off when the mark sits on a
    /// surface that is already dark.
    var showsBackplate = true

    private var isSmall: Bool { size < 48 }

    /// Ring diameters as a fraction of the frame, outermost first.
    private var diameters: [CGFloat] {
        isSmall ? [0.80, 0.46] : [0.57, 0.395, 0.22]
    }

    private var stroke: CGFloat {
        isSmall ? max(1.6, size * 0.075) : size * 0.0254
    }

    private var coreDiameter: CGFloat {
        isSmall ? max(4, size * 0.20) : size * 0.129
    }

    var body: some View {
        ZStack {
            if showsBackplate { backplate }
            rings
            trail
            head
            core
        }
        .frame(width: size, height: size)
    }

    private var backplate: some View {
        RoundedRectangle(cornerRadius: size * (isSmall ? 0.28 : 0.223), style: .continuous)
            .fill(
                LinearGradient(
                    colors: [Color(rpHex: 0x242428), Color(rpHex: 0x060608)],
                    startPoint: .topLeading, endPoint: .bottomTrailing
                )
            )
            .overlay {
                RoundedRectangle(cornerRadius: size * (isSmall ? 0.28 : 0.223), style: .continuous)
                    .strokeBorder(Color.white.opacity(0.14), lineWidth: max(0.5, size * 0.003))
            }
            .padding(isSmall ? 0 : size * 0.0938)
    }

    private var rings: some View {
        ZStack {
            ForEach(Array(diameters.enumerated()), id: \.offset) { index, diameter in
                Circle()
                    .strokeBorder(
                        Color.white.opacity(0.26 + Double(index) * 0.15),
                        lineWidth: stroke
                    )
                    .frame(width: size * diameter, height: size * diameter)
            }
        }
    }

    /// Where the promotion in flight has got to. SwiftUI's trim starts at
    /// three o'clock and runs clockwise.
    private var headFraction: CGFloat { 0.92 }
    private var outerDiameter: CGFloat { size * diameters[0] }

    private var trail: some View {
        Circle()
            .trim(from: headFraction - 0.22, to: headFraction)
            .stroke(
                AngularGradient(
                    gradient: Gradient(stops: [
                        .init(color: Color(rpHex: 0x5CCC87).opacity(0), location: headFraction - 0.22),
                        .init(color: Color(rpHex: 0x5CCC87), location: headFraction),
                    ]),
                    center: .center
                ),
                style: StrokeStyle(lineWidth: stroke, lineCap: .round)
            )
            .frame(width: outerDiameter, height: outerDiameter)
    }

    /// Placed by the same angle the trail ends at, so the two are one object.
    private var head: some View {
        let angle = headFraction * 2 * .pi
        let radius = outerDiameter / 2 - stroke / 2
        return Circle()
            .fill(Color(rpHex: 0x6FE49B))
            .frame(width: stroke * 1.9, height: stroke * 1.9)
            .shadow(color: Color(rpHex: 0x5CCC87).opacity(0.9), radius: size * 0.033)
            .offset(x: radius * cos(angle), y: radius * sin(angle))
    }

    /// Production, at the centre, lit. The one element that still reads at
    /// 16pt.
    private var core: some View {
        Circle()
            .fill(
                RadialGradient(
                    colors: [Color.white, Color(rpHex: 0xC9A2F5)],
                    center: .init(x: 0.35, y: 0.3),
                    startRadius: size * 0.002, endRadius: size * 0.117
                )
            )
            .frame(width: coreDiameter, height: coreDiameter)
            .shadow(color: Color(rpHex: 0xB86CF0).opacity(0.65), radius: size * 0.051)
    }
}
