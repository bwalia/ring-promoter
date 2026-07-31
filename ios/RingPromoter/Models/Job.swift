import Foundation

/// Status of one step within a job.
enum StepStatus: String, Codable, Hashable, Sendable {
    case running
    case success
    case failed
    /// Anything the server adds later renders neutrally rather than crashing.
    case unknown

    init(from decoder: any Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(String.self)
        self = StepStatus(rawValue: raw) ?? .unknown
    }

    var isTerminal: Bool { self != .running }
}

/// Status of a whole job.
enum JobStatus: String, Codable, Hashable, Sendable {
    case pending
    case running
    case success
    case failed
    case unknown

    init(from decoder: any Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(String.self)
        self = JobStatus(rawValue: raw) ?? .unknown
    }

    /// Polling stops here. `unknown` is deliberately NOT terminal: an
    /// unrecognised status is more likely a newer in-flight state than a
    /// finished one, and the job's own `finishedAt` still ends the poll.
    var isTerminal: Bool { self == .success || self == .failed }
}

/// Progress of the AI diagnosis attached to a failed job.
enum DiagnosisStatus: String, Codable, Hashable, Sendable {
    case none
    case running
    case done
    case failed

    init(from decoder: any Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(String.self)
        self = DiagnosisStatus(rawValue: raw) ?? .none
    }
}

/// One step of a running operation, with its own log lines.
struct JobStep: Codable, Hashable, Sendable, Identifiable {
    let id: String
    let title: String
    let status: StepStatus
    let logs: [String]
    let startedAt: Date
    let finishedAt: Date?

    enum CodingKeys: String, CodingKey {
        case id, title, status, logs
        case startedAt = "started_at"
        case finishedAt = "finished_at"
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        title = try c.decode(String.self, forKey: .title)
        status = try c.decode(StepStatus.self, forKey: .status)
        logs = try c.decodeIfPresent([String].self, forKey: .logs) ?? []
        startedAt = try c.decode(Date.self, forKey: .startedAt)
        finishedAt = try c.decodeIfPresent(Date.self, forKey: .finishedAt)
    }

    init(
        id: String, title: String, status: StepStatus, logs: [String],
        startedAt: Date, finishedAt: Date? = nil
    ) {
        self.id = id
        self.title = title
        self.status = status
        self.logs = logs
        self.startedAt = startedAt
        self.finishedAt = finishedAt
    }

    var duration: TimeInterval? {
        finishedAt.map { $0.timeIntervalSince(startedAt) }
    }
}

/// A live seed / promote / rollback started with `?async=1`.
///
/// Mirrors the API's `jobState`. Polled from `GET /api/apps/{app}/jobs/{id}`.
struct Job: Codable, Hashable, Sendable, Identifiable {
    let id: String
    let app: String
    /// "seed" | "promote" | "rollback"
    let action: String
    let status: JobStatus
    let steps: [JobStep]
    /// Present once the operation finished, whether or not it succeeded.
    let result: ActionResult?
    /// A transport/precondition error that stopped the job before it produced
    /// a result.
    let error: String?
    let diagnosis: String?
    let diagnosisStatus: DiagnosisStatus
    let diagnosisError: String?
    let startedAt: Date
    let finishedAt: Date?

    enum CodingKeys: String, CodingKey {
        case id, app, action, status, steps, result, error, diagnosis
        case diagnosisStatus = "diagnosis_status"
        case diagnosisError = "diagnosis_error"
        case startedAt = "started_at"
        case finishedAt = "finished_at"
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        app = try c.decode(String.self, forKey: .app)
        action = try c.decode(String.self, forKey: .action)
        status = try c.decode(JobStatus.self, forKey: .status)
        steps = try c.decodeIfPresent([JobStep].self, forKey: .steps) ?? []
        result = try c.decodeIfPresent(ActionResult.self, forKey: .result)
        error = try c.decodeIfPresent(String.self, forKey: .error)
        diagnosis = try c.decodeIfPresent(String.self, forKey: .diagnosis)
        diagnosisStatus = try c.decodeIfPresent(DiagnosisStatus.self, forKey: .diagnosisStatus) ?? .none
        diagnosisError = try c.decodeIfPresent(String.self, forKey: .diagnosisError)
        startedAt = try c.decode(Date.self, forKey: .startedAt)
        finishedAt = try c.decodeIfPresent(Date.self, forKey: .finishedAt)
    }

    init(
        id: String, app: String, action: String, status: JobStatus, steps: [JobStep],
        result: ActionResult? = nil, error: String? = nil, diagnosis: String? = nil,
        diagnosisStatus: DiagnosisStatus = .none, diagnosisError: String? = nil,
        startedAt: Date, finishedAt: Date? = nil
    ) {
        self.id = id
        self.app = app
        self.action = action
        self.status = status
        self.steps = steps
        self.result = result
        self.error = error
        self.diagnosis = diagnosis
        self.diagnosisStatus = diagnosisStatus
        self.diagnosisError = diagnosisError
        self.startedAt = startedAt
        self.finishedAt = finishedAt
    }

    var promotionAction: PromotionAction? { PromotionAction(rawValue: action) }

    /// Polling stops when the server says the job reached a terminal status OR
    /// stamped a finish time. Either alone is enough.
    var isFinished: Bool { status.isTerminal || finishedAt != nil }

    /// The terminal state to render. Distinguishes rolled-back from plain
    /// failure, which the prompt requires be explicit.
    enum Outcome: Hashable, Sendable {
        case running
        case succeeded
        case failedAndRolledBack
        case failed
    }

    var outcome: Outcome {
        guard isFinished else { return .running }
        if let result { return Outcome(result.outcome) }
        return status == .success ? .succeeded : .failed
    }

    /// The best single sentence explaining what happened.
    var summaryMessage: String? {
        if let result, !result.message.isEmpty { return result.message }
        if let error, !error.isEmpty { return error }
        return nil
    }

    /// Only failed jobs can be diagnosed (the server returns 409 otherwise).
    var canRequestDiagnosis: Bool {
        isFinished && status == .failed && (diagnosis?.isEmpty ?? true)
            && diagnosisStatus != .running
    }

    /// Every log line in order, prefixed by step, for share/copy.
    var transcript: String {
        steps.map { step in
            ([step.title] + step.logs.map { "  \($0)" }).joined(separator: "\n")
        }.joined(separator: "\n\n")
    }
}

extension Job.Outcome {
    init(_ outcome: ActionResult.Outcome) {
        switch outcome {
        case .succeeded: self = .succeeded
        case .failedAndRolledBack: self = .failedAndRolledBack
        case .failed: self = .failed
        }
    }
}

/// `POST .../seed|promote|rollback?async=1` → 202
struct JobHandle: Codable, Hashable, Sendable {
    let jobID: String

    enum CodingKeys: String, CodingKey {
        case jobID = "job_id"
    }
}

/// `GET /api/jobs` — the newest job per app, shared across all operators.
struct JobsResponse: Codable, Hashable, Sendable {
    let jobs: [Job]
}

/// Response of the diagnose endpoints.
struct DiagnosisResponse: Codable, Hashable, Sendable {
    let diagnosisStatus: DiagnosisStatus
    let diagnosis: String?

    enum CodingKeys: String, CodingKey {
        case diagnosisStatus = "diagnosis_status"
        case diagnosis
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        diagnosisStatus = try c.decodeIfPresent(DiagnosisStatus.self, forKey: .diagnosisStatus) ?? .none
        diagnosis = try c.decodeIfPresent(String.self, forKey: .diagnosis)
    }

    init(diagnosisStatus: DiagnosisStatus, diagnosis: String?) {
        self.diagnosisStatus = diagnosisStatus
        self.diagnosis = diagnosis
    }
}
