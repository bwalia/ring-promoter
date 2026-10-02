import Foundation

/// One place to try or inspect a deployed version: the web UI, a TestFlight
/// build, an artifact, the workflow run.
///
/// Mirrors `store.TestLink`.
struct TestLink: Codable, Hashable, Sendable, Identifiable {
    let label: String
    let url: String
    /// web, ios, android, api, docs, dashboard, release, artifact, ci or other.
    let kind: String
    /// config, health (the ring's health host), run (deploy outputs) or log
    /// (found in the deploy logs).
    let source: String
    /// The AI's reason for recommending the link (AI picks only).
    var why: String? = nil

    var id: String { url }

    /// Found in the deploy logs: shown, but behind the curated links.
    var isFromLogs: Bool { source == "log" }

    /// Something a tester opens directly, as opposed to a build output.
    var isTryable: Bool {
        ["web", "ios", "android", "api", "docs", "dashboard"].contains(kind)
    }

    /// Only http(s) links are opened; anything else is ignored.
    var destination: URL? {
        guard let url = URL(string: url),
              let scheme = url.scheme?.lowercased(), scheme == "http" || scheme == "https"
        else { return nil }
        return url
    }

    var systemImage: String {
        switch kind {
        case "web": "globe"
        case "ios": "iphone"
        case "android": "candybarphone"
        case "api": "curlybraces"
        case "docs": "book"
        case "dashboard": "chart.xyaxis.line"
        case "release": "tag"
        case "artifact": "shippingbox"
        case "ci": "gearshape.2"
        default: "link"
        }
    }

    /// A one-word label for a compact button; the full label is the
    /// accessibility hint's job.
    var shortLabel: String {
        switch kind {
        case "web": "Open"
        case "ios": "iOS"
        case "android": "Android"
        case "api": "API"
        case "docs": "Docs"
        case "dashboard": "Dashboard"
        default: label
        }
    }

    /// Host and path, for a compact second line.
    var shortURL: String {
        guard let url = URL(string: url), let host = url.host() else { return self.url }
        let path = url.path()
        return path == "/" || path.isEmpty ? host : host + path
    }
}

/// The AI's suggestion of how to test a deployed version.
///
/// Mirrors `store.TestPlan`. Its links are always chosen from the kit's own
/// links by the server — never invented.
struct TestPlan: Codable, Hashable, Sendable {
    let summary: String
    let checklist: [String]
    let links: [TestLink]
    let generatedAt: Date

    enum CodingKeys: String, CodingKey {
        case summary, checklist, links
        case generatedAt = "generated_at"
    }
}

/// `GET /api/apps/{app}/rings/{ring}/test-kit` — where to test a ring's
/// current version, and where its AI test plan stands.
struct TestKit: Codable, Hashable, Sendable {
    let app: String
    let ring: String
    let version: String
    let links: [TestLink]
    var plan: TestPlan? = nil
    /// When the deploy's links were captured; absent for versions deployed
    /// before the server had test kits.
    var capturedAt: Date? = nil
    let aiEnabled: Bool
    let planStatus: DiagnosisStatus
    var planError: String? = nil

    enum CodingKeys: String, CodingKey {
        case app, ring, version, links, plan
        case capturedAt = "captured_at"
        case aiEnabled = "ai_enabled"
        case planStatus = "plan_status"
        case planError = "plan_error"
    }
}
