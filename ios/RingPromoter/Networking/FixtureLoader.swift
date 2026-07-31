import Foundation

/// Loads the JSON fixtures bundled with the app.
///
/// These files are **real captured responses** from a locally-run Ring Promoter
/// (`internal/api`), not hand-written approximations — which is what makes them
/// worth testing against. See `Resources/Fixtures/README.md` for how they were
/// captured and re-captured.
enum FixtureLoader {
    /// Thrown rather than crashing so a missing resource shows up as a readable
    /// test failure instead of a trap in an unrelated line.
    enum Failure: Error, CustomStringConvertible {
        case missing(String)
        case undecodable(String, any Error)

        var description: String {
            switch self {
            case .missing(let name):
                "Fixture \"\(name).json\" is not in the bundle. Is it in Resources/Fixtures?"
            case .undecodable(let name, let error):
                "Fixture \"\(name).json\" did not decode: \(error)"
            }
        }
    }

    /// Searches the host app bundle first (app-hosted unit tests run inside it)
    /// then the bundle this code was compiled into.
    private static var candidateBundles: [Bundle] {
        var bundles = [Bundle.main]
        let own = Bundle(for: BundleToken.self)
        if own != Bundle.main { bundles.append(own) }
        return bundles
    }

    static func data(_ name: String) throws -> Data {
        for bundle in candidateBundles {
            if let url = bundle.url(forResource: name, withExtension: "json") {
                return try Data(contentsOf: url)
            }
            // Tolerate the fixtures being copied in as a folder reference.
            if let url = bundle.url(
                forResource: name, withExtension: "json", subdirectory: "Fixtures"
            ) {
                return try Data(contentsOf: url)
            }
        }
        throw Failure.missing(name)
    }

    static func decode<T: Decodable>(_ type: T.Type, from name: String) throws -> T {
        let data = try data(name)
        do {
            return try JSONCoding.makeDecoder().decode(type, from: data)
        } catch {
            throw Failure.undecodable(name, error)
        }
    }

    /// Every fixture name the app ships, so a test can assert the whole corpus
    /// still decodes after a model change.
    enum Name {
        static let apps = "apps"
        static let rings = "rings"
        static let ringsGated = "rings-gated"
        static let ringsManaged = "rings-managed"
        static let ringsUnhealthy = "rings-unhealthy"
        static let history = "history"
        static let historyWithFailures = "history-with-failures"
        static let versionsUnsupported = "versions-unsupported"
        static let jobRunning = "job-running"
        static let jobSuccess = "job-success"
        static let jobFailedRolledBack = "job-failed-rolledback"
        static let jobs = "jobs"
        static let groups = "groups"
        static let signoffs = "signoffs"
        static let signoffCreated = "signoff-created"
        static let maintenanceGated = "maintenance-gated"
        static let maintenanceGatedClosed = "maintenance-gated-closed"
        static let maintenanceUngated = "maintenance-ungated"
        static let windowCreated = "window-created"
        static let seedResult = "seed-result"
        static let promoteResult = "promote-result"
        static let rollbackResult = "rollback-result"
        static let result422Failed = "result-422-failed"
        static let serverVersion = "version"
    }
}

/// Anchors `Bundle(for:)` to whichever bundle this file was compiled into.
private final class BundleToken {}
