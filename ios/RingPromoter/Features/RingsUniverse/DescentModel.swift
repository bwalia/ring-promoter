import SwiftUI

/// The health of one application on the Rings of Applications screen.
///
/// This mirrors the web console's fleet view exactly, so an operator moving
/// between the two sees the same verdicts: `deploying` overrides everything
/// because a running job makes the health snapshot momentarily untrustworthy,
/// and the aggregate is worst-first because one failing service is what the
/// screen exists to surface.
enum FleetStatus: Hashable, Sendable, CaseIterable {
    case healthy
    case deploying
    case degraded
    case failed
    case empty
    case loading

    /// Worst-first, matching the web console's aggregation order.
    static let aggregatePriority: [FleetStatus] = [
        .failed, .degraded, .deploying, .loading, .healthy, .empty,
    ]

    /// The overall verdict for a set of applications.
    static func aggregate(_ statuses: [FleetStatus]) -> FleetStatus {
        aggregatePriority.first { statuses.contains($0) } ?? .empty
    }

    /// Health of one application from its rings and job state.
    ///
    /// - `isBusy`: a job is running right now — shown as deploying regardless
    ///   of the (soon to be stale) health snapshot.
    /// - `ringsUnknown`: the rings request failed and nothing is cached, so no
    ///   verdict can honestly be given.
    init(rings: [RingStatus], isBusy: Bool, ringsUnknown: Bool = false) {
        if isBusy {
            self = .deploying
            return
        }
        if ringsUnknown {
            self = .loading
            return
        }
        let active = rings.filter { $0.configured && !$0.isEmpty }
        if active.isEmpty {
            self = .empty
        } else {
            let healthy = active.count(where: \.isHealthy)
            if healthy == active.count {
                self = .healthy
            } else {
                self = healthy == 0 ? .failed : .degraded
            }
        }
    }

    var label: String {
        switch self {
        case .healthy: "Healthy"
        case .deploying: "Deploying"
        case .degraded: "Degraded"
        case .failed: "Failing"
        case .empty: "No version"
        case .loading: "Checking…"
        }
    }

    /// Colour is never the only signal — every status also carries a symbol.
    var systemImage: String {
        switch self {
        case .healthy: "checkmark.circle.fill"
        case .deploying: "arrow.triangle.2.circlepath"
        case .degraded: "exclamationmark.circle.fill"
        case .failed: "exclamationmark.triangle.fill"
        case .empty: "circle.dotted"
        case .loading: "ellipsis.circle"
        }
    }
}

extension FleetStatus {
    /// Design-system colours, so the stage agrees with every other screen.
    var tint: Color {
        switch self {
        case .healthy: .rpHealthy
        case .deploying: .rpInFlight
        case .degraded: .rpGate
        case .failed: .rpUnhealthy
        case .empty: .rpNeutral
        case .loading: .rpDisabled
        }
    }
}

/// One application as the Descent stage and the lanes draw it: its spoke
/// (per-ring state along the promotion order) plus the facts the detail card
/// needs. Built once per refresh so the views stay pure functions of data.
struct DescentApp: Identifiable, Hashable, Sendable {
    let id: String
    let title: String
    let status: FleetStatus
    let spoke: DescentLayout.Spoke
    /// A job is running for this app — the comet flies.
    let isBusy: Bool
    let busyAction: String?
    let loadError: String?

    init(summary: AppSummary, order: [String]) {
        id = summary.name
        title = summary.title
        status = FleetStatus(
            rings: summary.rings, isBusy: summary.isBusy,
            ringsUnknown: summary.rings.isEmpty && summary.loadError != nil
        )
        spoke = DescentLayout.buildSpoke(summary.rings, order: order)
        isBusy = summary.isBusy
        busyAction = summary.busyAction
        loadError = summary.loadError
    }

    var sentence: String { DescentLayout.spokeSentence(spoke) }

    /// "2.3.1 · acc", or "no version" — the second line under the name.
    var subtitle: String {
        guard let newest = spoke.newest, spoke.frontier >= 0 else { return "no version" }
        return "\(newest) · \(spoke.nodes[spoke.frontier].ring)"
    }

    /// Colour of one node, by its ring's live health.
    static func tint(for state: DescentLayout.NodeState) -> Color {
        switch state {
        case .healthy: .rpHealthy
        case .failed: .rpUnhealthy
        case .empty, .off: .rpDisabled
        }
    }

    /// Ordered for the stage: grouped apps contiguous, ungrouped last.
    static func ordered(
        summaries: [AppSummary], groups: [AppGroup], order: [String]
    ) -> (apps: [DescentApp], spans: [DescentLayout.GroupSpan]) {
        let byName = Dictionary(
            summaries.map { ($0.name, $0) }, uniquingKeysWith: { first, _ in first }
        )
        let (names, spans) = DescentLayout.orderByGroup(summaries.map(\.name), groups: groups)
        let apps = names.compactMap { byName[$0].map { DescentApp(summary: $0, order: order) } }
        return (apps, spans)
    }
}
