import Foundation

/// What a seed / promote / rollback did.
///
/// Mirrors `promoter.Result`. The server returns this with 200 when the
/// operation succeeded and **422 when it ran and failed** — both are real
/// outcomes carrying a `message`, not transport errors.
struct ActionResult: Codable, Hashable, Sendable {
    let app: String
    /// "seed" | "promote" | "rollback"
    let action: String
    /// The affected (target) ring.
    let ring: String
    /// The promote source ring; absent for seed and rollback.
    let fromRing: String?
    let version: String
    /// Deployed AND passed its health check.
    let success: Bool
    /// The health check failed and the target was returned to its previous
    /// version. The app shows this as its own terminal state.
    let rolledBack: Bool
    let message: String
    let state: RingState

    enum CodingKeys: String, CodingKey {
        case app, action, ring, version, success, message, state
        case fromRing = "from_ring"
        case rolledBack = "rolled_back"
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        app = try c.decode(String.self, forKey: .app)
        action = try c.decode(String.self, forKey: .action)
        ring = try c.decode(String.self, forKey: .ring)
        version = try c.decode(String.self, forKey: .version)
        success = try c.decode(Bool.self, forKey: .success)
        message = try c.decodeIfPresent(String.self, forKey: .message) ?? ""
        state = try c.decodeIfPresent(RingState.self, forKey: .state) ?? RingState()
        fromRing = try c.decodeIfPresent(String.self, forKey: .fromRing)
        // Omitted by the server when false (`omitempty`).
        rolledBack = try c.decodeIfPresent(Bool.self, forKey: .rolledBack) ?? false
    }

    init(
        app: String, action: String, ring: String, fromRing: String? = nil, version: String,
        success: Bool, rolledBack: Bool = false, message: String, state: RingState = RingState()
    ) {
        self.app = app
        self.action = action
        self.ring = ring
        self.fromRing = fromRing
        self.version = version
        self.success = success
        self.rolledBack = rolledBack
        self.message = message
        self.state = state
    }

    /// The three terminal outcomes the UI must distinguish.
    enum Outcome: Hashable, Sendable {
        case succeeded
        case failedAndRolledBack
        case failed
    }

    var outcome: Outcome {
        if success { return .succeeded }
        return rolledBack ? .failedAndRolledBack : .failed
    }
}

/// The action a request performs, used for labels, icons and confirmation copy.
enum PromotionAction: String, Codable, Hashable, Sendable, CaseIterable {
    case seed
    case promote
    case rollback

    var title: String {
        switch self {
        case .seed: "Seed"
        case .promote: "Promote"
        case .rollback: "Roll back"
        }
    }

    var pastTense: String {
        switch self {
        case .seed: "Seeded"
        case .promote: "Promoted"
        case .rollback: "Rolled back"
        }
    }

    var systemImage: String {
        switch self {
        case .seed: "square.and.pencil"
        case .promote: "arrow.right.circle.fill"
        case .rollback: "arrow.uturn.backward.circle.fill"
        }
    }
}
