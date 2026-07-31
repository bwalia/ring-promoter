import Foundation
import Testing

@testable import RingPromoter

/// Polling: cadence, backoff, stopping, and cancellation.
///
/// This is the part that costs battery and hammers a struggling control plane
/// when it is wrong, so it is tested directly rather than by eye.
@Suite("Job polling")
@MainActor
struct JobPollerTests {

    @Test("polling stops as soon as the job reaches a terminal state")
    func stopsOnTerminal() async throws {
        let api = ScriptedJobAPI(
            script: [
                .running(steps: 1),
                .running(steps: 2),
                .succeeded,
                // Anything after this would mean the poller kept going.
                .succeeded,
            ]
        )
        let poller = JobPoller(app: "app", jobID: "job-1", api: api, clock: ImmediateClock())
        await poller.run()

        #expect(poller.job?.isFinished == true)
        #expect(poller.didFinish)
        #expect(await api.callCount == 3)
    }

    @Test("a rolled-back job is reported as its own outcome")
    func rolledBackOutcome() async throws {
        let api = ScriptedJobAPI(script: [.rolledBack])
        let poller = JobPoller(app: "app", jobID: "job-1", api: api, clock: ImmediateClock())
        await poller.run()

        #expect(poller.job?.outcome == .failedAndRolledBack)
    }

    @Test("cancelling the task stops the poll loop")
    func cancellation() async throws {
        // A script that never finishes: only cancellation can end this.
        let api = ScriptedJobAPI(script: [.running(steps: 1)], repeatsLast: true)
        let poller = JobPoller(app: "app", jobID: "job-1", api: api, clock: ImmediateClock())

        let task = Task { await poller.run() }
        // Let it get a few polls in, then cancel — as `.task` does on disappear.
        try await Task.sleep(for: .milliseconds(50))
        task.cancel()
        await task.value

        #expect(!poller.didFinish)
        #expect(!poller.isPolling)
    }

    @Test("a transient failure keeps the last good snapshot on screen")
    func transientFailureKeepsSnapshot() async throws {
        let api = ScriptedJobAPI(script: [.running(steps: 2), .failure, .succeeded])
        let poller = JobPoller(app: "app", jobID: "job-1", api: api, clock: ImmediateClock())
        await poller.run()

        // One failure in the middle must not blank the view or stop the poll.
        #expect(poller.job?.isFinished == true)
        #expect(poller.error == nil)
    }

    @Test("repeated failures eventually surface an error")
    func repeatedFailuresSurface() async throws {
        let api = ScriptedJobAPI(script: [.failure], repeatsLast: true)
        let poller = JobPoller(app: "app", jobID: "job-1", api: api, clock: ImmediateClock())

        let task = Task { await poller.run() }
        try await Task.sleep(for: .milliseconds(60))
        task.cancel()
        await task.value

        #expect(poller.error != nil)
    }

    @Test("the cadence stays about a second while a job is young")
    func fastCadenceEarly() {
        let poller = JobPoller(
            app: "app", jobID: "job-1", api: ScriptedJobAPI(script: []), clock: ImmediateClock()
        )
        #expect(poller.interval(tick: 1) == .milliseconds(1_000))
        #expect(poller.interval(tick: 30) == .milliseconds(1_000))
    }

    @Test("the cadence eases off for a long-running job and is capped")
    func backoffOnLongJobs() {
        let poller = JobPoller(
            app: "app", jobID: "job-1", api: ScriptedJobAPI(script: []), clock: ImmediateClock()
        )
        let late = poller.interval(tick: 60)
        #expect(late > .milliseconds(1_000))
        // Never so slow that a deploy feels frozen.
        #expect(poller.interval(tick: 10_000) == .seconds(8))
    }
}

// MARK: - Test doubles

/// A client that returns a scripted sequence of job states.
actor ScriptedJobAPI: RingPromoterAPI {
    enum Step: Sendable {
        case running(steps: Int)
        case succeeded
        case rolledBack
        case failure
    }

    private let script: [Step]
    private let repeatsLast: Bool
    private(set) var callCount = 0

    init(script: [Step], repeatsLast: Bool = false) {
        self.script = script
        self.repeatsLast = repeatsLast
    }

    func job(app: String, id: String) async throws(APIError) -> Job {
        let index = callCount
        callCount += 1
        let step: Step
        if index < script.count {
            step = script[index]
        } else if repeatsLast, let last = script.last {
            step = last
        } else {
            throw .notFound("job not found")
        }

        switch step {
        case .failure:
            throw .transport(.timedOut)
        case .running(let count):
            return Self.makeJob(id: id, app: app, stepCount: count, status: .running)
        case .succeeded:
            return Self.makeJob(id: id, app: app, stepCount: 3, status: .success)
        case .rolledBack:
            return Self.makeJob(id: id, app: app, stepCount: 4, status: .failed, rolledBack: true)
        }
    }

    private static func makeJob(
        id: String, app: String, stepCount: Int, status: JobStatus, rolledBack: Bool = false
    ) -> Job {
        let finished = status != .running
        return Job(
            id: id, app: app, action: "promote", status: status,
            steps: (0..<stepCount).map { index in
                JobStep(
                    id: "step-\(index)", title: "Step \(index)",
                    status: index == stepCount - 1 && !finished ? .running : .success,
                    logs: ["line"], startedAt: .now
                )
            },
            result: finished
                ? ActionResult(
                    app: app, action: "promote", ring: "test", fromRing: "int",
                    version: "1.0", success: status == .success, rolledBack: rolledBack,
                    message: "done"
                )
                : nil,
            startedAt: .now,
            finishedAt: finished ? .now : nil
        )
    }

    // The rest of the protocol is unused by these tests.
    func checkHealth() async throws(APIError) {}
    func serverVersion() async throws(APIError) -> ServerVersion { throw .notFound("") }
    func apps() async throws(APIError) -> AppsResponse { throw .notFound("") }
    func rings(app: String) async throws(APIError) -> [RingStatus] { [] }
    func history(app: String) async throws(APIError) -> [HistoryEntry] { [] }
    func versions(app: String) async throws(APIError) -> VersionsResponse { .unsupported }
    func recentJobs() async throws(APIError) -> [Job] { [] }
    func seed(
        app: String, ring: String, version: String, crCode: String?, password: String?
    ) async throws(APIError) -> String { "" }
    func promote(
        app: String, fromRing: String, crCode: String?, password: String?
    ) async throws(APIError) -> String { "" }
    func rollback(app: String, ring: String) async throws(APIError) -> String { "" }
    func setAutoPromote(
        app: String, ring: String, enabled: Bool, password: String?
    ) async throws(APIError) {}
    func maintenance(app: String) async throws(APIError) -> MaintenanceStatus { .ungated }
    func openMaintenanceWindow(
        app: String, window: NewMaintenanceWindow
    ) async throws(APIError) -> MaintenanceWindow { throw .notFound("") }
    func closeMaintenanceWindow(app: String, id: String) async throws(APIError) {}
    func signoffs(app: String) async throws(APIError) -> [Signoff] { [] }
    func recordSignoff(
        app: String, signoff: NewSignoff
    ) async throws(APIError) -> Signoff { throw .notFound("") }
    func groups() async throws(APIError) -> [AppGroup] { [] }
    func createGroup(
        name: String, apps: [String]
    ) async throws(APIError) -> AppGroup { throw .notFound("") }
    func updateGroup(
        id: String, name: String, apps: [String]
    ) async throws(APIError) -> AppGroup { throw .notFound("") }
    func deleteGroup(id: String) async throws(APIError) {}
    func diagnoseJob(
        app: String, id: String
    ) async throws(APIError) -> DiagnosisResponse { throw .notImplemented("") }
    func diagnoseHistoryEntry(
        app: String, id: Int64
    ) async throws(APIError) -> DiagnosisResponse { throw .notImplemented("") }
    func historyDiagnosis(
        app: String, id: Int64
    ) async throws(APIError) -> DiagnosisResponse { throw .notImplemented("") }
}

/// A clock whose sleeps return at once, so poll-loop tests run in milliseconds
/// instead of minutes.
struct ImmediateClock: Clock {
    struct Instant: InstantProtocol {
        var offset: Duration = .zero

        func advanced(by duration: Duration) -> Instant { Instant(offset: offset + duration) }
        func duration(to other: Instant) -> Duration { other.offset - offset }
        static func < (lhs: Instant, rhs: Instant) -> Bool { lhs.offset < rhs.offset }
    }

    var now: Instant { Instant() }
    var minimumResolution: Duration { .zero }

    func sleep(until deadline: Instant, tolerance: Duration?) async throws {
        // Yield so cancellation still has somewhere to land.
        try Task.checkCancellation()
        await Task.yield()
    }
}
