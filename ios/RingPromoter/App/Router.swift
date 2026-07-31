import Foundation
import SwiftUI

/// Everything that can be pushed onto the Overview's navigation stack.
enum Route: Hashable, Sendable {
    case app(String)
    /// A group's own page: the deployment ring plus its members.
    case group(String)
    case job(app: String, id: String)
    case history(app: String)
    case maintenance(app: String)
}

/// Navigation state, kept outside the views so a deep link, a widget tap, an
/// App Intent and a push notification can all reach the same screen.
@MainActor
@Observable
final class Router {
    /// Which tab is showing.
    enum Tab: String, Hashable, CaseIterable, Identifiable {
        case overview, activity, settings

        var id: String { rawValue }

        var label: String {
            switch self {
            case .overview: "Overview"
            case .activity: "Activity"
            case .settings: "Settings"
            }
        }

        var systemImage: String {
            switch self {
            case .overview: "square.stack.3d.up"
            case .activity: "clock.arrow.circlepath"
            case .settings: "gearshape"
            }
        }
    }

    var tab: Tab = .overview
    var overviewPath: [Route] = []

    /// Handle `ringpromoter://app/{name}` and
    /// `ringpromoter://app/{name}/job/{id}`.
    ///
    /// Unrecognised links are ignored rather than guessed at: sending an
    /// operator to the wrong application would be worse than doing nothing.
    func open(_ url: URL) {
        guard url.scheme?.lowercased() == "ringpromoter" else { return }
        // For ringpromoter://app/web-frontend the host is "app" and the path
        // carries the rest.
        //
        // The path is split while still percent-encoded, then each component is
        // decoded: `pathComponents` decodes first, so an application name
        // containing an escaped slash ("team%2Fapp") would be split in two and
        // open the wrong application.
        var parts: [String] = []
        if let host = url.host(percentEncoded: false), !host.isEmpty { parts.append(host) }
        parts.append(
            contentsOf: url.path(percentEncoded: true)
                .split(separator: "/", omittingEmptySubsequences: true)
                .map { String($0).removingPercentEncoding ?? String($0) }
        )
        guard parts.first == "app", parts.count >= 2 else { return }

        let app = parts[1]
        tab = .overview
        if parts.count >= 4, parts[2] == "job" {
            overviewPath = [.app(app), .job(app: app, id: parts[3])]
        } else if parts.count >= 3, parts[2] == "history" {
            overviewPath = [.app(app), .history(app: app)]
        } else {
            overviewPath = [.app(app)]
        }
    }

    func show(app: String) {
        tab = .overview
        overviewPath = [.app(app)]
    }

    func show(group id: String) {
        tab = .overview
        overviewPath = [.group(id)]
    }

    func showJob(app: String, id: String) {
        tab = .overview
        overviewPath = [.app(app), .job(app: app, id: id)]
    }

    /// The deep link that points at an application, used by the widget and by
    /// Live Activities.
    static func url(forApp app: String) -> URL? {
        var components = URLComponents()
        components.scheme = "ringpromoter"
        components.host = "app"
        components.path = "/\(app)"
        return components.url
    }

    static func url(forJob id: String, app: String) -> URL? {
        var components = URLComponents()
        components.scheme = "ringpromoter"
        components.host = "app"
        components.path = "/\(app)/job/\(id)"
        return components.url
    }
}
