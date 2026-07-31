import AppIntents
import Foundation
import SwiftUI

/// Shortcuts and Siri support.
///
/// Read-only by design. "Show me the pipeline for X" opens the app; it does not
/// deploy anything. A voice command that could promote to production — with no
/// gate sheet, no password field and no confirmation — is exactly the affordance
/// this app exists to avoid.
///
/// Rollback is offered as an intent, but as `openAppWhenRun`: it takes the
/// operator straight to the ring's rollback sheet rather than firing it, so the
/// one destructive shortcut still ends in a human tap. The API-gap document
/// records what a genuinely hands-free rollback would need.
struct ShowPipelineIntent: AppIntent {
    static let title: LocalizedStringResource = "Show pipeline"
    static let description = IntentDescription(
        "Open an application's ring pipeline in Ring Promoter."
    )
    static let openAppWhenRun = true

    @Parameter(title: "Application")
    var app: String

    init() {}

    init(app: String) {
        self.app = app
    }

    @MainActor
    func perform() async throws -> some IntentResult {
        IntentDeepLink.pending = .app(app)
        return .result()
    }

    static var parameterSummary: some ParameterSummary {
        Summary("Show the pipeline for \(\.$app)")
    }
}

/// Takes the operator to the rollback sheet for one ring.
struct PrepareRollbackIntent: AppIntent {
    static let title: LocalizedStringResource = "Prepare a rollback"
    static let description = IntentDescription(
        "Open the rollback confirmation for one ring. Nothing is rolled back until you confirm."
    )
    static let openAppWhenRun = true

    @Parameter(title: "Application")
    var app: String

    @Parameter(title: "Ring")
    var ring: String

    init() {}

    init(app: String, ring: String) {
        self.app = app
        self.ring = ring
    }

    @MainActor
    func perform() async throws -> some IntentResult {
        IntentDeepLink.pending = .rollback(app: app, ring: ring)
        return .result()
    }

    static var parameterSummary: some ParameterSummary {
        Summary("Roll back \(\.$app) in \(\.$ring)")
    }
}

/// Where an intent leaves its request for the app to pick up on launch.
///
/// An intent runs before the UI exists, so it cannot navigate directly.
@MainActor
enum IntentDeepLink {
    enum Request: Equatable {
        case app(String)
        case rollback(app: String, ring: String)
    }

    static var pending: Request?

    /// Consume the pending request, if any.
    static func take() -> Request? {
        defer { pending = nil }
        return pending
    }
}

struct RingPromoterShortcuts: AppShortcutsProvider {
    static var appShortcuts: [AppShortcut] {
        AppShortcut(
            intent: ShowPipelineIntent(),
            phrases: [
                "Show me the pipeline for \(.applicationName)",
                "Open \(.applicationName) pipeline",
            ],
            shortTitle: "Show pipeline",
            systemImageName: "square.stack.3d.up"
        )
        AppShortcut(
            intent: PrepareRollbackIntent(),
            phrases: [
                "Roll back with \(.applicationName)",
                "Prepare a \(.applicationName) rollback",
            ],
            shortTitle: "Prepare rollback",
            systemImageName: "arrow.uturn.backward.circle"
        )
    }
}
