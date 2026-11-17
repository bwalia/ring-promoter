import SwiftUI

/// Loads the readiness scorecard for one application.
///
/// Separate from `AppDetailStore` so a screen that does not draw controls does
/// not pay for the request, and so a slow scorecard never holds up the rings.
@MainActor
@Observable
final class ControlsLoader {
    private(set) var view: ControlsView = .empty
    private(set) var isLoading = false
    private(set) var error: APIError?

    private let app: String
    private let session: AppSession

    init(app: String, session: AppSession) {
        self.app = app
        self.session = session
    }

    func refresh() async {
        isLoading = true
        defer { isLoading = false }
        do throws(APIError) {
            view = try await session.api.controls(app: app)
            error = nil
        } catch {
            // Keep what is on screen: a flaky scorecard must not blank the
            // only readout of what is blocking a promotion.
            self.error = error
            session.note(error)
        }
    }
}

/// One hop's scorecard: `int → test`, its score, and every rule behind it.
///
/// Shared by both platforms — the phone stacks these in a `Section`, the Mac
/// puts them in a panel under the pipeline. The web console calls this
/// "Deployment Controls" and it answers the question asked before every
/// promotion: *can this move, and if not, what is stopping it.*
struct HopScoreCard: View {
    let hop: HopScore
    var isExpanded: Bool = true
    var onToggle: (() -> Void)?

    private var tint: Color {
        if hop.ready { return .rpHealthy }
        return hop.score >= 50 ? .rpGate : .rpUnhealthy
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Button { onToggle?() } label: { header }
                .buttonStyle(.plain)
                .disabled(onToggle == nil)

            ScoreBar(score: hop.score, tint: tint)

            if !hop.summary.isEmpty {
                Text(hop.summary)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }

            if isExpanded {
                ForEach(hop.controls) { control in
                    ControlRow(control: control)
                }
                if !hop.blockers.isEmpty {
                    ForEach(hop.blockers, id: \.self) { blocker in
                        Label(blocker, systemImage: "hand.raised.fill")
                            .font(.caption)
                            .foregroundStyle(Color.rpUnhealthy)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
            }
        }
        .padding(.vertical, 4)
        .accessibilityElement(children: .contain)
        .accessibilityLabel(
            "\(hop.from) to \(hop.to), \(hop.ready ? "ready" : "blocked"), "
                + "score \(hop.score) out of 100"
        )
    }

    private var header: some View {
        HStack(spacing: 8) {
            Text("\(hop.from) → \(hop.to)")
                .font(.subheadline.weight(.semibold))
                .monospaced()
            if !hop.version.isEmpty {
                Text(hop.version)
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
            }
            Spacer(minLength: 6)
            StatusBadge(
                text: hop.ready ? "ready" : "blocked",
                systemImage: hop.ready ? "checkmark.circle.fill" : "hand.raised.fill",
                tint: tint
            )
            Text("\(hop.score)/100")
                .font(.caption.weight(.semibold))
                .monospacedDigit()
                .foregroundStyle(tint)
        }
        .contentShape(.rect)
    }
}

/// The weighted total, drawn. Never colour alone: the number sits beside it.
private struct ScoreBar: View {
    let score: Int
    let tint: Color

    var body: some View {
        GeometryReader { proxy in
            ZStack(alignment: .leading) {
                Capsule().fill(Color.rpNeutral.opacity(0.18))
                Capsule()
                    .fill(tint)
                    .frame(width: proxy.size.width * min(1, max(0, Double(score) / 100)))
            }
        }
        .frame(height: 5)
        .accessibilityHidden(true)
    }
}

/// One rule inside a hop.
private struct ControlRow: View {
    let control: ControlScore

    private var tint: Color {
        switch control.outcome {
        case .passed: .rpHealthy
        case .warned: .rpGate
        case .failed: .rpUnhealthy
        case .skipped: .rpNeutral
        }
    }

    private var symbol: String {
        switch control.outcome {
        case .passed: "checkmark.circle.fill"
        case .warned: "exclamationmark.triangle.fill"
        case .failed: "xmark.circle.fill"
        case .skipped: "minus.circle"
        }
    }

    var body: some View {
        HStack(alignment: .top, spacing: 8) {
            Image(systemName: symbol)
                .font(.caption)
                .foregroundStyle(tint)
            VStack(alignment: .leading, spacing: 2) {
                Text(control.label)
                    .font(.caption.weight(.medium))
                if !control.detail.isEmpty {
                    Text(control.detail)
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                if let next = control.nextAction, !next.isEmpty {
                    Label(next, systemImage: "arrow.turn.down.right")
                        .font(.caption2)
                        .foregroundStyle(Color.rpGate)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            Spacer(minLength: 4)
            Text("\(control.score)/100")
                .font(.caption2.monospacedDigit())
                .foregroundStyle(.secondary)
        }
        .accessibilityElement(children: .combine)
    }
}
