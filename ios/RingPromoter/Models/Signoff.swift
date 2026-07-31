import Foundation

/// A QA / release-engineer Go-No-Go decision for one **exact** version.
///
/// Mirrors `store.Signoff`. Sign-offs are version-specific: a GO for `v1` does
/// not authorise `v2`, and the app must never imply otherwise.
struct Signoff: Codable, Hashable, Sendable, Identifiable {
    /// The decision values the server accepts.
    enum Decision: String, Codable, Hashable, Sendable, CaseIterable {
        case go
        case noGo = "no_go"

        var title: String {
            switch self {
            case .go: "Go"
            case .noGo: "No-Go"
            }
        }
    }

    let app: String
    let ring: String
    let version: String
    let decision: Decision
    /// The release engineer who recorded the decision. Required by the server.
    let engineer: String
    /// The QA outcome this attests to (free text, e.g. "passed").
    let qaStatus: String
    let note: String?
    let updatedAt: Date

    /// Unique per (app, ring, version) — the server stores at most one.
    var id: String { "\(app)/\(ring)/\(version)" }

    enum CodingKeys: String, CodingKey {
        case app, ring, version, decision, engineer, note
        case qaStatus = "qa_status"
        case updatedAt = "updated_at"
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        app = try c.decode(String.self, forKey: .app)
        ring = try c.decode(String.self, forKey: .ring)
        version = try c.decode(String.self, forKey: .version)
        let raw = try c.decode(String.self, forKey: .decision)
        // An unrecognised decision is treated as NOT a go: failing closed is the
        // only safe reading for a gate.
        decision = Decision(rawValue: raw) ?? .noGo
        engineer = try c.decodeIfPresent(String.self, forKey: .engineer) ?? ""
        qaStatus = try c.decodeIfPresent(String.self, forKey: .qaStatus) ?? ""
        note = try c.decodeIfPresent(String.self, forKey: .note)
        updatedAt = try c.decode(Date.self, forKey: .updatedAt)
    }

    init(
        app: String, ring: String, version: String, decision: Decision,
        engineer: String, qaStatus: String, note: String? = nil, updatedAt: Date
    ) {
        self.app = app
        self.ring = ring
        self.version = version
        self.decision = decision
        self.engineer = engineer
        self.qaStatus = qaStatus
        self.note = note
        self.updatedAt = updatedAt
    }

    var isGo: Bool { decision == .go }
}

/// `GET /api/apps/{app}/signoffs`
struct SignoffsResponse: Codable, Hashable, Sendable {
    let signoffs: [Signoff]

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        signoffs = try c.decodeIfPresent([Signoff].self, forKey: .signoffs) ?? []
    }

    init(signoffs: [Signoff]) { self.signoffs = signoffs }

    enum CodingKeys: String, CodingKey { case signoffs }
}

/// Body of `POST /api/apps/{app}/signoffs`.
struct NewSignoff: Codable, Hashable, Sendable {
    var ring: String
    var version: String
    var decision: Signoff.Decision
    var engineer: String
    var qaStatus: String
    var note: String

    enum CodingKeys: String, CodingKey {
        case ring, version, decision, engineer, note
        case qaStatus = "qa_status"
    }
}

extension Collection where Element == Signoff {
    /// The sign-off guarding one exact (ring, version), if any. This is the only
    /// lookup the QA gate permits — matching on ring alone would let a GO for an
    /// older version appear to authorise a new one.
    func signoff(ring: String, version: String) -> Signoff? {
        first { $0.ring == ring && $0.version == version }
    }
}
