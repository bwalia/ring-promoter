import Foundation

/// One recorded seed / promote / rollback, success or failure.
///
/// Mirrors `store.HistoryEntry` from `GET /api/apps/{app}/history` (newest
/// first).
struct HistoryEntry: Codable, Hashable, Sendable, Identifiable {
    let id: Int64
    let app: String
    let ring: String
    /// "seed" | "promote" | "rollback"
    let action: String
    let fromVersion: String
    let toVersion: String
    /// "success" | "failure"
    let result: String
    let message: String
    /// The stored AI explanation, present once someone has asked for one.
    let diagnosis: String?
    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id, app, ring, action, result, message, diagnosis
        case fromVersion = "from_version"
        case toVersion = "to_version"
        case createdAt = "created_at"
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(Int64.self, forKey: .id)
        app = try c.decode(String.self, forKey: .app)
        ring = try c.decode(String.self, forKey: .ring)
        action = try c.decode(String.self, forKey: .action)
        fromVersion = try c.decodeIfPresent(String.self, forKey: .fromVersion) ?? ""
        toVersion = try c.decodeIfPresent(String.self, forKey: .toVersion) ?? ""
        result = try c.decode(String.self, forKey: .result)
        message = try c.decodeIfPresent(String.self, forKey: .message) ?? ""
        diagnosis = try c.decodeIfPresent(String.self, forKey: .diagnosis)
        createdAt = try c.decode(Date.self, forKey: .createdAt)
    }

    init(
        id: Int64, app: String, ring: String, action: String, fromVersion: String,
        toVersion: String, result: String, message: String, diagnosis: String? = nil,
        createdAt: Date
    ) {
        self.id = id
        self.app = app
        self.ring = ring
        self.action = action
        self.fromVersion = fromVersion
        self.toVersion = toVersion
        self.result = result
        self.message = message
        self.diagnosis = diagnosis
        self.createdAt = createdAt
    }

    var succeeded: Bool { result == "success" }
    var promotionAction: PromotionAction? { PromotionAction(rawValue: action) }

    /// Only failed entries can be diagnosed (the server returns 409 otherwise).
    var canRequestDiagnosis: Bool { !succeeded && (diagnosis?.isEmpty ?? true) }

    /// "2.7.1 → 2.7.2", or just the target when there was nothing before it.
    var versionTransition: String {
        fromVersion.isEmpty ? toVersion : "\(fromVersion) → \(toVersion)"
    }
}

/// `GET /api/apps/{app}/history`
struct HistoryResponse: Codable, Hashable, Sendable {
    let history: [HistoryEntry]

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        history = try c.decodeIfPresent([HistoryEntry].self, forKey: .history) ?? []
    }

    init(history: [HistoryEntry]) { self.history = history }

    enum CodingKeys: String, CodingKey { case history }
}
