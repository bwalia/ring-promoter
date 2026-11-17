import Foundation

/// The readiness scorecard for one promotion.
///
/// `GET /api/apps/{app}/controls` answers the question an operator asks before
/// every promotion — *can this move, and if not, what is stopping it* — and
/// answers it per hop rather than per ring: int → test is a different question
/// from acc → prod, because the gates differ.
///
/// The web console has shown this since it existed; the apps have not, which
/// left the phone and the Mac able to *start* a promotion without being able
/// to say whether it should happen.
struct ControlsView: Codable, Hashable, Sendable {
    let app: String
    /// Whether the server has an LLM configured, which decides if the plain
    /// language briefing is worth offering.
    let aiEnabled: Bool
    let hops: [HopScore]

    enum CodingKeys: String, CodingKey {
        case app, hops
        case aiEnabled = "ai_enabled"
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        app = try c.decodeIfPresent(String.self, forKey: .app) ?? ""
        aiEnabled = try c.decodeIfPresent(Bool.self, forKey: .aiEnabled) ?? false
        hops = try c.decodeIfPresent([HopScore].self, forKey: .hops) ?? []
    }

    init(app: String, aiEnabled: Bool, hops: [HopScore]) {
        self.app = app
        self.aiEnabled = aiEnabled
        self.hops = hops
    }

    static let empty = ControlsView(app: "", aiEnabled: false, hops: [])

    func hop(from ring: String) -> HopScore? { hops.first { $0.from == ring } }
}

/// One promotion's readiness: `int → test`, scored out of 100.
struct HopScore: Codable, Hashable, Sendable, Identifiable {
    let from: String
    let to: String
    let toLabel: String
    let version: String
    let score: Int
    let ready: Bool
    let blockers: [String]
    let controls: [ControlScore]
    let summary: String

    var id: String { "\(from)->\(to)" }

    enum CodingKeys: String, CodingKey {
        case from, to, version, score, ready, blockers, controls, summary
        case toLabel = "to_label"
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        from = try c.decodeIfPresent(String.self, forKey: .from) ?? ""
        to = try c.decodeIfPresent(String.self, forKey: .to) ?? ""
        toLabel = try c.decodeIfPresent(String.self, forKey: .toLabel) ?? ""
        version = try c.decodeIfPresent(String.self, forKey: .version) ?? ""
        score = try c.decodeIfPresent(Int.self, forKey: .score) ?? 0
        ready = try c.decodeIfPresent(Bool.self, forKey: .ready) ?? false
        blockers = try c.decodeIfPresent([String].self, forKey: .blockers) ?? []
        controls = try c.decodeIfPresent([ControlScore].self, forKey: .controls) ?? []
        summary = try c.decodeIfPresent(String.self, forKey: .summary) ?? ""
    }

    init(
        from: String, to: String, toLabel: String, version: String, score: Int,
        ready: Bool, blockers: [String], controls: [ControlScore], summary: String
    ) {
        self.from = from
        self.to = to
        self.toLabel = toLabel
        self.version = version
        self.score = score
        self.ready = ready
        self.blockers = blockers
        self.controls = controls
        self.summary = summary
    }
}

/// One rule inside a hop: QA sign-off, a maintenance window, source health,
/// recent hop success, and so on.
struct ControlScore: Codable, Hashable, Sendable, Identifiable {
    let id: String
    let label: String
    /// Server-side grouping, e.g. "gate", "health", "history".
    let kind: String
    /// How much this rule counts toward the hop's score.
    let weight: Int
    let score: Int
    /// "pass" | "warn" | "fail" | "skip" — kept as the server's own string so
    /// a new status does not decode as nonsense.
    let status: String
    let detail: String
    let nextAction: String?

    enum CodingKeys: String, CodingKey {
        case id, label, kind, weight, score, status, detail
        case nextAction = "next_action"
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decodeIfPresent(String.self, forKey: .id) ?? UUID().uuidString
        label = try c.decodeIfPresent(String.self, forKey: .label) ?? ""
        kind = try c.decodeIfPresent(String.self, forKey: .kind) ?? ""
        weight = try c.decodeIfPresent(Int.self, forKey: .weight) ?? 0
        score = try c.decodeIfPresent(Int.self, forKey: .score) ?? 0
        status = try c.decodeIfPresent(String.self, forKey: .status) ?? ""
        detail = try c.decodeIfPresent(String.self, forKey: .detail) ?? ""
        nextAction = try c.decodeIfPresent(String.self, forKey: .nextAction)
    }

    init(
        id: String, label: String, kind: String, weight: Int, score: Int,
        status: String, detail: String, nextAction: String?
    ) {
        self.id = id
        self.label = label
        self.kind = kind
        self.weight = weight
        self.score = score
        self.status = status
        self.detail = detail
        self.nextAction = nextAction
    }

    /// Passing, needing attention, or blocking. Mapped from the server's
    /// string so the UI never switches on a raw literal.
    enum Outcome: Sendable { case passed, warned, failed, skipped }

    var outcome: Outcome {
        switch status.lowercased() {
        case "pass", "ok", "passed": .passed
        case "warn", "warning": .warned
        case "skip", "skipped", "n/a": .skipped
        default: .failed
        }
    }
}
