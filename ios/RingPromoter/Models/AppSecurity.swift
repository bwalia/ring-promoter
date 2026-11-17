import Foundation

/// An application's security posture, from `GET /api/apps/{app}/security`.
///
/// The server evaluates each ring's deployed version against a policy — which
/// scan kinds are required, and how many findings of each severity are
/// tolerated — and the result can block a promotion. The apps could promote
/// into a guarded ring without ever showing why it was refused.
///
/// `scans` and `waivers` are deliberately not decoded. Each scan carries its
/// whole SARIF document, which is megabytes an operator never reads on a
/// phone; the counts in `posture` are what the screens draw. Decoding is
/// tolerant, so those keys are ignored rather than fatal.
struct AppSecurity: Codable, Hashable, Sendable {
    let policy: SecurityPolicy
    let posture: [SecurityEvaluation]

    enum CodingKeys: String, CodingKey { case policy, posture }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        policy = try c.decodeIfPresent(SecurityPolicy.self, forKey: .policy) ?? .unenforced
        posture = try c.decodeIfPresent([SecurityEvaluation].self, forKey: .posture) ?? []
    }

    init(policy: SecurityPolicy, posture: [SecurityEvaluation]) {
        self.policy = policy
        self.posture = posture
    }

    static let empty = AppSecurity(policy: .unenforced, posture: [])

    /// Nothing to say: no policy worth naming and nothing evaluated.
    var isSilent: Bool { posture.isEmpty && !policy.isEnforced }
}

/// How many findings, by severity.
struct SeverityCounts: Codable, Hashable, Sendable {
    let critical: Int
    let high: Int
    let medium: Int
    let low: Int

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        critical = try c.decodeIfPresent(Int.self, forKey: .critical) ?? 0
        high = try c.decodeIfPresent(Int.self, forKey: .high) ?? 0
        medium = try c.decodeIfPresent(Int.self, forKey: .medium) ?? 0
        low = try c.decodeIfPresent(Int.self, forKey: .low) ?? 0
    }

    init(critical: Int = 0, high: Int = 0, medium: Int = 0, low: Int = 0) {
        self.critical = critical
        self.high = high
        self.medium = medium
        self.low = low
    }

    static let none = SeverityCounts()
    var total: Int { critical + high + medium + low }
    /// "0C 0H 0M 0L", the shorthand the web console uses.
    var shorthand: String { "\(critical)C \(high)H \(medium)M \(low)L" }
}

/// One scan kind's result — SAST, SCA, secrets, and so on.
///
/// The Go type embeds `SeverityCounts`, which flattens its fields into the
/// same JSON object, so the counts are decoded from this level rather than a
/// nested one.
struct SecurityCheck: Codable, Hashable, Sendable, Identifiable {
    let kind: String
    /// "pass" | "missing" | "stale" | "fail" — kept as the server's own string.
    let status: String
    let scanners: [String]
    let counts: SeverityCounts
    let waived: Int
    let scannedAt: Date?
    let detail: String?

    var id: String { kind }

    enum CodingKeys: String, CodingKey {
        case kind, status, scanners, waived, detail
        case scannedAt = "scanned_at"
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        kind = try c.decodeIfPresent(String.self, forKey: .kind) ?? ""
        status = try c.decodeIfPresent(String.self, forKey: .status) ?? ""
        scanners = try c.decodeIfPresent([String].self, forKey: .scanners) ?? []
        waived = try c.decodeIfPresent(Int.self, forKey: .waived) ?? 0
        scannedAt = try c.decodeIfPresent(Date.self, forKey: .scannedAt)
        detail = try c.decodeIfPresent(String.self, forKey: .detail)
        counts = try SeverityCounts(from: decoder)
    }

    init(
        kind: String, status: String, scanners: [String] = [],
        counts: SeverityCounts = .none, waived: Int = 0,
        scannedAt: Date? = nil, detail: String? = nil
    ) {
        self.kind = kind
        self.status = status
        self.scanners = scanners
        self.counts = counts
        self.waived = waived
        self.scannedAt = scannedAt
        self.detail = detail
    }

    enum Outcome: Sendable { case passed, missing, stale, failed }

    var outcome: Outcome {
        switch status.lowercased() {
        case "pass", "ok", "passed": .passed
        case "missing", "none": .missing
        case "stale", "old": .stale
        default: .failed
        }
    }
}

/// One ring's verdict on the version deployed in it.
struct SecurityEvaluation: Codable, Hashable, Sendable, Identifiable {
    let ring: String
    let version: String
    /// Whether the policy is actually enforced on promotions into this ring.
    let guarded: Bool
    /// "pass" | "fail" | "unknown" — the server's own string.
    let verdict: String
    let detail: String
    let checks: [SecurityCheck]
    let counts: SeverityCounts
    let waived: Int

    var id: String { ring.isEmpty ? version : ring }

    enum CodingKeys: String, CodingKey {
        case ring, version, guarded, verdict, detail, checks, counts, waived
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        ring = try c.decodeIfPresent(String.self, forKey: .ring) ?? ""
        version = try c.decodeIfPresent(String.self, forKey: .version) ?? ""
        guarded = try c.decodeIfPresent(Bool.self, forKey: .guarded) ?? false
        verdict = try c.decodeIfPresent(String.self, forKey: .verdict) ?? ""
        detail = try c.decodeIfPresent(String.self, forKey: .detail) ?? ""
        checks = try c.decodeIfPresent([SecurityCheck].self, forKey: .checks) ?? []
        counts = try c.decodeIfPresent(SeverityCounts.self, forKey: .counts) ?? .none
        waived = try c.decodeIfPresent(Int.self, forKey: .waived) ?? 0
    }

    init(
        ring: String, version: String, guarded: Bool, verdict: String,
        detail: String, checks: [SecurityCheck], counts: SeverityCounts, waived: Int
    ) {
        self.ring = ring
        self.version = version
        self.guarded = guarded
        self.verdict = verdict
        self.detail = detail
        self.checks = checks
        self.counts = counts
        self.waived = waived
    }

    var passed: Bool { verdict.lowercased() == "pass" }
    /// Nothing has been reported for this version at all.
    var isUnscanned: Bool { checks.allSatisfy { $0.outcome == .missing } }
}

/// The policy a version is judged against.
struct SecurityPolicy: Codable, Hashable, Sendable {
    let rings: [String]
    let require: [String]
    let max: [String: Int]
    let maxAgeSeconds: Int
    /// "app" | "default" | "builtin". `builtin` means nothing is configured,
    /// so the policy is never enforced on a promotion.
    let source: String

    enum CodingKeys: String, CodingKey {
        case rings, require, max, source
        case maxAgeSeconds = "max_age_seconds"
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        rings = try c.decodeIfPresent([String].self, forKey: .rings) ?? []
        require = try c.decodeIfPresent([String].self, forKey: .require) ?? []
        max = try c.decodeIfPresent([String: Int].self, forKey: .max) ?? [:]
        maxAgeSeconds = try c.decodeIfPresent(Int.self, forKey: .maxAgeSeconds) ?? 0
        source = try c.decodeIfPresent(String.self, forKey: .source) ?? "builtin"
    }

    init(
        rings: [String], require: [String], max: [String: Int],
        maxAgeSeconds: Int, source: String
    ) {
        self.rings = rings
        self.require = require
        self.max = max
        self.maxAgeSeconds = maxAgeSeconds
        self.source = source
    }

    static let unenforced = SecurityPolicy(
        rings: [], require: [], max: [:], maxAgeSeconds: 0, source: "builtin"
    )

    var isEnforced: Bool { source != "builtin" && !rings.isEmpty }

    /// "requires SAST · code, SCA · dependencies, Secrets · critical ≤ 0"
    var summary: String {
        var parts: [String] = []
        if !require.isEmpty {
            parts.append("requires " + require.map { $0.uppercased() }.joined(separator: ", "))
        }
        for key in ["critical", "high", "medium", "low"] where max[key] != nil {
            parts.append("\(key) ≤ \(max[key]!)")
        }
        return parts.isEmpty ? "No policy configured — nothing is enforced." :
            parts.joined(separator: " · ")
    }
}
