import Foundation

/// Polls one job until it reaches a terminal state.
///
/// The cadence is deliberate: a running deploy is polled about once a second so
/// it feels live, but the interval backs off as the job drags on, and any
/// transient failure backs off further rather than hammering a struggling
/// control plane from a phone. Polling stops the moment the job is finished,
/// and the owning `Task` is cancelled when the view disappears.
@MainActor
@Observable
final class JobPoller {
    private(set) var job: Job?
    private(set) var error: APIError?
    private(set) var isPolling = false
    /// Set once, when the job first reaches a terminal state, so the UI can
    /// fire haptics and a Live Activity update exactly once.
    private(set) var didFinish = false

    let app: String
    let jobID: String

    private let api: any RingPromoterAPI
    private let clock: any Clock<Duration>

    /// Poll cadence. `base` while a job is young; it stretches towards `max` as
    /// the job runs on, since a deploy still going after five minutes is not
    /// going to change in the next 900ms.
    private let base: Duration = .milliseconds(1_000)
    private let maximum: Duration = .seconds(8)
    /// Consecutive transport failures, used to back off separately from the
    /// normal cadence.
    private var consecutiveFailures = 0

    init(
        app: String, jobID: String, api: any RingPromoterAPI,
        clock: any Clock<Duration> = ContinuousClock()
    ) {
        self.app = app
        self.jobID = jobID
        self.api = api
        self.clock = clock
    }

    /// Run until the job finishes or the enclosing task is cancelled.
    func run() async {
        isPolling = true
        defer { isPolling = false }

        var tick = 0
        while !Task.isCancelled {
            do {
                let latest = try await api.job(app: app, id: jobID)
                job = latest
                error = nil
                consecutiveFailures = 0
                if latest.isFinished {
                    didFinish = true
                    return
                }
            } catch {
                if error.isCancellation { return }
                consecutiveFailures += 1
                // Keep showing the last good snapshot; only surface the error
                // once retrying has clearly stopped helping.
                if consecutiveFailures >= 3 || job == nil {
                    self.error = error
                }
            }
            tick += 1
            do {
                try await clock.sleep(for: interval(tick: tick))
            } catch {
                return
            }
        }
    }

    /// Ask the server for an AI diagnosis. The answer arrives on a later poll,
    /// which is why this does not return one.
    func requestDiagnosis() async {
        do {
            _ = try await api.diagnoseJob(app: app, id: jobID)
            // Re-read straight away so a server that answered synchronously
            // shows its result without waiting for the next tick.
            if let latest = try? await api.job(app: app, id: jobID) { job = latest }
        } catch {
            self.error = error
        }
    }

    /// One more read, for pull-to-refresh on a finished job.
    func refreshOnce() async {
        guard let latest = try? await api.job(app: app, id: jobID) else { return }
        job = latest
    }

    /// The delay before the next poll.
    ///
    /// Exposed for tests: the backoff is the part most likely to be got wrong,
    /// and the part that costs a user battery when it is.
    func interval(tick: Int) -> Duration {
        if consecutiveFailures > 0 {
            // 2s, 4s, 8s, capped.
            let backoff = base * Double(1 << min(consecutiveFailures, 3))
            return min(backoff, maximum)
        }
        // Stay at ~1s for the first 30 polls, then ease off.
        guard tick > 30 else { return base }
        let stretched = base * (1 + Double(tick - 30) * 0.25)
        return min(stretched, maximum)
    }
}
