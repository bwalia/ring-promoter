import Foundation

/// One deployable version known to an application's source repository.
///
/// Mirrors `deployer.Version`.
struct AppVersion: Codable, Hashable, Sendable, Identifiable {
    let name: String
    /// "branch" | "tag"
    let type: String

    var id: String { "\(type)/\(name)" }

    enum CodingKeys: String, CodingKey {
        case name, type
    }

    var isTag: Bool { type == "tag" }

    var systemImage: String { isTag ? "tag" : "arrow.triangle.branch" }
}

/// `GET /api/apps/{app}/versions`
///
/// `supported == false` means the app's deployer cannot enumerate versions, so
/// the Seed sheet must fall back to free-form text input.
struct VersionsResponse: Codable, Hashable, Sendable {
    let supported: Bool
    let versions: [AppVersion]

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        supported = try c.decodeIfPresent(Bool.self, forKey: .supported) ?? false
        versions = try c.decodeIfPresent([AppVersion].self, forKey: .versions) ?? []
    }

    init(supported: Bool, versions: [AppVersion]) {
        self.supported = supported
        self.versions = versions
    }

    enum CodingKeys: String, CodingKey { case supported, versions }

    /// Offer a picker only when the server can actually enumerate versions AND
    /// found some.
    var offersPicker: Bool { supported && !versions.isEmpty }

    var branches: [AppVersion] { versions.filter { !$0.isTag } }
    var tags: [AppVersion] { versions.filter(\.isTag) }

    static let unsupported = VersionsResponse(supported: false, versions: [])
}

/// `GET /version` — build metadata, shown in Settings. Unauthenticated.
struct ServerVersion: Codable, Hashable, Sendable {
    let version: String
    let commit: String
    let builtAt: String
    /// When this instance started which, with an immutable image, is when it
    /// was last deployed.
    let startedAt: Date?

    enum CodingKeys: String, CodingKey {
        case version, commit
        case builtAt = "built_at"
        case startedAt = "started_at"
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        version = try c.decodeIfPresent(String.self, forKey: .version) ?? "unknown"
        commit = try c.decodeIfPresent(String.self, forKey: .commit) ?? "unknown"
        builtAt = try c.decodeIfPresent(String.self, forKey: .builtAt) ?? "unknown"
        startedAt = try? c.decodeIfPresent(Date.self, forKey: .startedAt)
    }

    init(version: String, commit: String, builtAt: String, startedAt: Date?) {
        self.version = version
        self.commit = commit
        self.builtAt = builtAt
        self.startedAt = startedAt
    }

    /// Short commit for display.
    var shortCommit: String {
        commit.count > 8 ? String(commit.prefix(8)) : commit
    }
}
