#if DEBUG
import SwiftUI

/// The app icon, drawn rather than painted, so every size macOS asks for comes
/// from one definition and none of them can drift.
///
/// The mark is the product's own metaphor rather than decoration: three rings
/// with production lit at the centre — int, test, acc, prod — and a promotion
/// in flight on the outermost one, its head leading a fading trail. Someone
/// who has used the app recognises their own pipeline.
///
/// Drawn once on a 1024pt canvas and rendered at each scale. Strokes are
/// heavier than they need to be at 512pt because the same artwork has to
/// survive being shrunk to 32: below that only the lit core really reads, and
/// that is the part worth keeping.
struct ConsoleAppIcon: View {
    private let canvas: CGFloat = 1024
    /// Ring diameters as a fraction of the canvas, outermost first.
    private let diameters: [CGFloat] = [0.57, 0.395, 0.22]
    private let stroke: CGFloat = 26

    /// Where the promotion in flight has got to, as a fraction around the
    /// outer ring. SwiftUI's trim starts at three o'clock and runs clockwise.
    private let headFraction: CGFloat = 0.92
    private let trailLength: CGFloat = 0.22

    private var outerRadius: CGFloat { canvas * diameters[0] / 2 }

    var body: some View {
        ZStack {
            squircle
            rings
            trail
            head
            core
        }
        .frame(width: canvas, height: canvas)
    }

    /// macOS applies no mask of its own, so the rounded square is part of the
    /// artwork. Inset and radius follow Apple's grid: the shape fills about
    /// four fifths of the canvas.
    private var squircle: some View {
        RoundedRectangle(cornerRadius: 228, style: .continuous)
            .fill(
                LinearGradient(
                    colors: [Color(rpHex: 0x242428), Color(rpHex: 0x060608)],
                    startPoint: .topLeading,
                    endPoint: .bottomTrailing
                )
            )
            .overlay {
                RoundedRectangle(cornerRadius: 228, style: .continuous)
                    .strokeBorder(
                        LinearGradient(
                            colors: [Color.white.opacity(0.16), Color.white.opacity(0.02)],
                            startPoint: .top,
                            endPoint: .bottom
                        ),
                        lineWidth: 3
                    )
            }
            .padding(96)
    }

    /// The pipeline. Each ring is brighter than the one outside it, so the eye
    /// travels inward the way a version does.
    private var rings: some View {
        ZStack {
            ForEach(Array(diameters.enumerated()), id: \.offset) { index, diameter in
                Circle()
                    .strokeBorder(
                        Color.white.opacity(0.26 + Double(index) * 0.15),
                        lineWidth: stroke
                    )
                    .frame(width: canvas * diameter, height: canvas * diameter)
            }
        }
    }

    /// The trail behind the deploy. Angular rather than linear, so it fades
    /// along the arc instead of across the frame.
    private var trail: some View {
        Circle()
            .trim(from: headFraction - trailLength, to: headFraction)
            .stroke(
                AngularGradient(
                    gradient: Gradient(stops: [
                        .init(color: Color(rpHex: 0x5CCC87).opacity(0), location: headFraction - trailLength),
                        .init(color: Color(rpHex: 0x5CCC87), location: headFraction),
                    ]),
                    center: .center
                ),
                style: StrokeStyle(lineWidth: stroke, lineCap: .round)
            )
            .frame(width: canvas * diameters[0], height: canvas * diameters[0])
    }

    /// The deploy itself, placed by the same angle the trail ends at so the
    /// two are one object. Offsetting by hand rather than rotating a padded
    /// circle is what keeps them attached.
    private var head: some View {
        let angle = headFraction * 2 * .pi
        // Trim's zero is three o'clock and its sweep is clockwise, which in
        // this coordinate space (y downward) is +sin.
        let radius = outerRadius - stroke / 2
        return Circle()
            .fill(Color(rpHex: 0x6FE49B))
            .frame(width: stroke * 1.9, height: stroke * 1.9)
            .shadow(color: Color(rpHex: 0x5CCC87).opacity(0.9), radius: 34)
            .offset(x: radius * cos(angle), y: radius * sin(angle))
    }

    /// Production, at the centre, lit. The one element that still reads at
    /// 32pt.
    private var core: some View {
        Circle()
            .fill(
                RadialGradient(
                    colors: [Color.white, Color(rpHex: 0xC9A2F5)],
                    center: .init(x: 0.35, y: 0.3), startRadius: 2, endRadius: 120
                )
            )
            .frame(width: 132, height: 132)
            .shadow(color: Color(rpHex: 0xB86CF0).opacity(0.65), radius: 52)
    }
}
#endif
