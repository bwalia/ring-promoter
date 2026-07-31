import ActivityKit
import Foundation

/// Starts, updates and ends the Live Activity for a running promotion.
///
/// Driven from the same job polling that feeds the live job screen, so the Lock
/// Screen can never disagree with the app. Every call is a no-op when Live
/// Activities are unavailable or the operator has turned them off, which is why
/// nothing here throws.
@MainActor
final class LiveActivityController {
    static let shared = LiveActivityController()

    /// Job ids this controller started an activity for.
    ///
    /// Only the ids are held: `Activity` values are looked up from
    /// `Activity.activities` inside the nonisolated methods that await them, so
    /// a non-`Sendable` handle is never carried across an isolation boundary.
    private var startedJobIDs: Set<String> = []

    private init() {}

    private var isEnabled: Bool {
        ActivityAuthorizationInfo().areActivitiesEnabled
    }

    /// Begin an activity for a job that has just started.
    func start(job: Job, appTitle: String, targetRing: String, version: String) {
        guard isEnabled, !startedJobIDs.contains(job.id) else { return }
        let attributes = PromotionActivityAttributes(
            app: job.app, appTitle: appTitle, action: job.action,
            targetRing: targetRing, version: version, jobID: job.id
        )
        let state = Self.state(from: job)
        do {
            _ = try Activity.request(
                attributes: attributes,
                content: ActivityContent(state: state, staleDate: nil),
                pushType: nil
            )
            startedJobIDs.insert(job.id)
        } catch {
            // Denied, or too many activities. The in-app job view is unaffected.
        }
    }

    func update(with job: Job) async {
        guard startedJobIDs.contains(job.id) else { return }
        await Self.apply(ActivityContent(state: Self.state(from: job), staleDate: nil), to: job.id)
    }

    /// Finish the activity on the job's terminal state, leaving it on screen
    /// briefly so the outcome is actually seen.
    func end(with job: Job) async {
        guard startedJobIDs.remove(job.id) != nil else { return }
        await Self.finish(
            ActivityContent(state: Self.state(from: job), staleDate: nil), for: job.id
        )
    }

    /// Tidy up anything left behind by a previous launch — a crashed app can
    /// leave an activity running for hours otherwise.
    func endOrphans() async {
        let known = startedJobIDs
        await Self.endActivities(excluding: known)
    }

    // The `Activity` handle is fetched and awaited entirely inside these
    // nonisolated helpers, so it never crosses an isolation boundary.

    private nonisolated static func apply(
        _ content: ActivityContent<PromotionActivityAttributes.ContentState>, to jobID: String
    ) async {
        for activity in Activity<PromotionActivityAttributes>.activities
        where activity.attributes.jobID == jobID {
            await activity.update(content)
        }
    }

    private nonisolated static func finish(
        _ content: ActivityContent<PromotionActivityAttributes.ContentState>, for jobID: String
    ) async {
        for activity in Activity<PromotionActivityAttributes>.activities
        where activity.attributes.jobID == jobID {
            await activity.end(
                content, dismissalPolicy: .after(Date().addingTimeInterval(90))
            )
        }
    }

    private nonisolated static func endActivities(excluding known: Set<String>) async {
        for activity in Activity<PromotionActivityAttributes>.activities
        where !known.contains(activity.attributes.jobID) {
            await activity.end(nil, dismissalPolicy: .immediate)
        }
    }

    private static func state(from job: Job) -> PromotionActivityAttributes.ContentState {
        let status: PromotionActivityAttributes.ContentState.Status =
            switch job.outcome {
            case .running: .running
            case .succeeded: .succeeded
            case .failedAndRolledBack: .rolledBack
            case .failed: .failed
            }
        return PromotionActivityAttributes.ContentState(
            stepTitle: job.steps.last?.title ?? "Starting…",
            stepIndex: job.steps.count,
            // The server does not publish a total up front, so the count is the
            // best honest estimate: at least what has been seen so far.
            stepCount: max(job.steps.count, expectedSteps(for: job)),
            status: status,
            message: job.summaryMessage
        )
    }

    /// A promote runs source-health → deploy → health, plus a rollback step if
    /// it fails; seed and rollback are shorter. Used only to size the progress
    /// bar, never to decide anything.
    private static func expectedSteps(for job: Job) -> Int {
        switch job.promotionAction {
        case .promote: 3
        case .seed: 2
        case .rollback: 2
        case nil: 3
        }
    }
}
