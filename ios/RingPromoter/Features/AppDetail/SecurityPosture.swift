import SwiftUI

/// Loads an application's security posture.
///
/// Separate from `AppDetailStore` for the same reason `ControlsLoader` is: a
/// screen that does not draw it should not pay for the request, and a slow
/// scan report must never hold up the rings.
@MainActor
@Observable
final class SecurityLoader {
    private(set) var security: AppSecurity = .empty
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
            security = try await session.api.security(app: app)
            error = nil
        } catch {
            // A server with no security module answers 404. That is not a
            // failure worth shouting about — it means this control plane does
            // not do security gating — so it is kept as an empty posture.
            if case .notFound = error {
                security = .empty
                self.error = nil
                return
            }
            self.error = error
            session.note(error)
        }
    }
}

/// One ring's scan posture. Shared: the phone stacks these in a `Section`, the
/// Mac lays them across a panel.
struct SecurityPostureCard: View {
    let evaluation: SecurityEvaluation

    private var tint: Color {
        if evaluation.passed { return .rpHealthy }
        return evaluation.isUnscanned ? .rpGate : .rpUnhealthy
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 6) {
                Image(systemName: evaluation.guarded ? "lock.shield.fill" : "shield")
                    .font(.caption)
                    .foregroundStyle(tint)
                Text(evaluation.ring.isEmpty ? "version" : evaluation.ring)
                    .font(.subheadline.weight(.semibold))
                Spacer(minLength: 4)
                Text(evaluation.verdict.isEmpty ? "unknown" : evaluation.verdict)
                    .font(.caption2.weight(.semibold))
                    .foregroundStyle(tint)
            }
            if !evaluation.version.isEmpty {
                Text(evaluation.version)
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
            }
            Text(evaluation.counts.shorthand)
                .font(.caption2.monospaced())
                .foregroundStyle(evaluation.counts.total == 0 ? .secondary : tint)
            if !evaluation.checks.isEmpty {
                Text(checkSummary)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if !evaluation.detail.isEmpty {
                Text(evaluation.detail)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if evaluation.waived > 0 {
                Label("\(evaluation.waived) waived", systemImage: "hand.raised")
                    .font(.caption2)
                    .foregroundStyle(Color.rpGate)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(
            "\(evaluation.ring.isEmpty ? "version" : evaluation.ring), "
                + "\(evaluation.verdict.isEmpty ? "unknown" : evaluation.verdict), "
                + "\(evaluation.counts.critical) critical, \(evaluation.counts.high) high"
        )
    }

    /// "sast missing · sca missing · secrets missing" — the web's own phrasing,
    /// and the thing that tells an operator what their CI is not posting.
    private var checkSummary: String {
        evaluation.checks
            .map { "\($0.kind) \($0.status)" }
            .joined(separator: " · ")
    }
}

/// What to run to make a missing scan appear.
///
/// A security panel that says "sast missing" and stops is a complaint. The web
/// prints the command, so this does too — an operator reaches it at the moment
/// they discover the gap, not after a search through the docs.
struct SecurityScanHint: View {
    let app: String

    private var command: String {
        """
        curl -fsS -X POST "$RP_URL/api/security/scans" \\
          -H "Authorization: Bearer $RP_TOKEN" \\
          -H 'Content-Type: application/json' \\
          -d '{"app":"\(app)","version":"'$TAG'","kind":"sca","sarif":…}'
        """
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("No scans reported yet. Post SARIF from your CI scanners — Trivy, Semgrep, Gitleaks, CodeQL, Checkov — and the gate checks every version against the policy before it moves on.")
                .font(.caption)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Text(command)
                .font(.caption2.monospaced())
                .foregroundStyle(.secondary)
                .textSelection(.enabled)
                .fixedSize(horizontal: false, vertical: true)
                .padding(8)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(.quaternary.opacity(0.4), in: .rect(cornerRadius: 8))
        }
    }
}
