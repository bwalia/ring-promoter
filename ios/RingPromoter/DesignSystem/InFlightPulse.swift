import SwiftUI

/// The "something is happening here" animation.
///
/// A slow breathing pulse rather than a spinner: it reads as *this ring is
/// being worked on* from across a desk, and it is calm enough to sit on screen
/// for the two minutes a real deploy takes without becoming irritating.
///
/// Honours Reduce Motion by falling back to a static emphasis — the state is
/// still legible, it just does not move.
struct InFlightPulse: ViewModifier {
    let isActive: Bool
    var tint: Color = .rpInFlight
    var cornerRadius: CGFloat = 10

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var isPulsing = false

    func body(content: Content) -> some View {
        content
            .background {
                if isActive {
                    RoundedRectangle(cornerRadius: cornerRadius)
                        .fill(tint.opacity(isPulsing ? 0.18 : 0.06))
                }
            }
            .overlay {
                if isActive {
                    RoundedRectangle(cornerRadius: cornerRadius)
                        .strokeBorder(
                            tint.opacity(isPulsing ? 0.85 : 0.25),
                            lineWidth: isPulsing ? 1.6 : 1
                        )
                }
            }
            .onAppear { start() }
            .onChange(of: isActive) { _, _ in start() }
    }

    private func start() {
        guard isActive, !reduceMotion else {
            isPulsing = isActive && reduceMotion
            return
        }
        isPulsing = false
        withAnimation(.easeInOut(duration: 1.1).repeatForever(autoreverses: true)) {
            isPulsing = true
        }
    }
}

/// A slowly rotating glyph, for the ring a deploy is currently touching.
struct SpinningGlyph: View {
    var systemName: String = "arrow.triangle.2.circlepath"
    var isActive: Bool = true

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var angle: Double = 0

    var body: some View {
        Image(systemName: systemName)
            .rotationEffect(.degrees(angle))
            .onAppear { start() }
            .onChange(of: isActive) { _, _ in start() }
    }

    private func start() {
        guard isActive, !reduceMotion else {
            angle = 0
            return
        }
        angle = 0
        withAnimation(.linear(duration: 1.6).repeatForever(autoreverses: false)) {
            angle = 360
        }
    }
}

extension View {
    /// Mark this element as the one a deploy is currently working on.
    func inFlight(_ isActive: Bool, tint: Color = .rpInFlight, cornerRadius: CGFloat = 10)
        -> some View {
        modifier(InFlightPulse(isActive: isActive, tint: tint, cornerRadius: cornerRadius))
    }
}
