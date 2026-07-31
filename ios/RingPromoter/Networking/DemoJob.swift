import Foundation

/// A simulated job whose steps advance with wall-clock time.
///
/// Deriving the snapshot from elapsed time — rather than mutating on a timer —
/// keeps demo mode free of background tasks and makes the progression
/// reproducible: poll at t+3s and you always get the same third step.
struct DemoJob: Sendable {
    let id: String
    let app: String
    let action: PromotionAction
    let targetRing: String
    let sourceRing: String?
    let version: String
    /// What the target ring was running before this operation.
    let previousVersion: String
    let startedAt: Date
    /// When true the health check fails and the ring is rolled back, so the
    /// failure path is demonstrable.
    let fails: Bool

    /// Set once the finished job's effect has been written into the world.
    var applied = false
    /// Set when someone asks for an AI diagnosis.
    var diagnosis: String?

    /// How long each step takes. Slow enough to watch, short enough that a demo
    /// does not stall.
    private static let stepDuration: TimeInterval = 2.2

    private struct Plan: Sendable {
        let id: String
        let title: String
        let logs: [String]
        let fails: Bool
    }

    private var plan: [Plan] {
        switch action {
        case .promote:
            var steps: [Plan] = [
                Plan(
                    id: "source-health",
                    title: "Verify \(sourceRing ?? "source") (\(version)) is healthy",
                    logs: ["source healthy"], fails: false
                ),
                Plan(
                    id: "deploy", title: "Deploy \(version) to \(targetRing)",
                    logs: ["image set to \(version)"], fails: false
                ),
                healthStep,
            ]
            if fails, !previousVersion.isEmpty { steps.append(rollbackStep) }
            return steps
        case .seed:
            var steps: [Plan] = [
                Plan(
                    id: "deploy", title: "Deploy \(version) to \(targetRing)",
                    logs: ["image set to \(version)"], fails: false
                ),
                healthStep,
            ]
            if fails, !previousVersion.isEmpty { steps.append(rollbackStep) }
            return steps
        case .rollback:
            return [
                Plan(
                    id: "deploy", title: "Roll \(targetRing) back to \(version)",
                    logs: ["image set to \(version)"], fails: false
                ),
                Plan(
                    id: "health", title: "Health check \(targetRing)",
                    logs: ["attempt 1/3 succeeded"], fails: false
                ),
            ]
        }
    }

    private var healthStep: Plan {
        Plan(
            id: "health", title: "Health check \(targetRing)",
            logs: fails
                ? [
                    "attempt 1/3 failed: unhealthy: status 503",
                    "waiting 5s before retry",
                    "attempt 2/3 failed: unhealthy: status 503",
                    "waiting 5s before retry",
                    "attempt 3/3 failed: unhealthy: status 503",
                    "health check failed after retries: unhealthy: status 503",
                ]
                : ["attempt 1/3 succeeded"],
            fails: fails
        )
    }

    private var rollbackStep: Plan {
        Plan(
            id: "rollback",
            title: "Auto-rollback \(targetRing) to \(previousVersion)",
            logs: [
                "deploying previous version \(previousVersion)",
                "attempt 1/3 succeeded",
                "rolled back to \(previousVersion)",
            ],
            fails: false
        )
    }

    /// The job as it appears at `instant`.
    func snapshot(at instant: Date) -> Job {
        let plan = plan
        let elapsed = max(0, instant.timeIntervalSince(startedAt))
        let completedCount = min(plan.count, Int(elapsed / Self.stepDuration))
        let isFinished = completedCount >= plan.count

        var steps: [JobStep] = []
        for (index, item) in plan.enumerated() where index <= completedCount {
            let stepStart = startedAt.addingTimeInterval(Double(index) * Self.stepDuration)
            let done = index < completedCount
            steps.append(
                JobStep(
                    id: item.id, title: item.title,
                    status: done ? (item.fails ? .failed : .success) : .running,
                    // A running step reveals its log lines gradually.
                    logs: done ? item.logs : revealedLogs(item, at: instant, stepStart: stepStart),
                    startedAt: stepStart,
                    finishedAt: done
                        ? stepStart.addingTimeInterval(Self.stepDuration) : nil
                )
            )
        }

        let finishedAt = isFinished
            ? startedAt.addingTimeInterval(Double(plan.count) * Self.stepDuration) : nil

        return Job(
            id: id, app: app, action: action.rawValue,
            status: isFinished ? (fails ? .failed : .success) : .running,
            steps: steps,
            result: isFinished ? result() : nil,
            error: nil,
            diagnosis: diagnosis,
            diagnosisStatus: diagnosis == nil ? .none : .done,
            diagnosisError: nil,
            startedAt: startedAt,
            finishedAt: finishedAt
        )
    }

    private func revealedLogs(_ item: Plan, at instant: Date, stepStart: Date) -> [String] {
        guard !item.logs.isEmpty else { return [] }
        let progress = instant.timeIntervalSince(stepStart) / Self.stepDuration
        let shown = max(1, min(item.logs.count, Int(progress * Double(item.logs.count)) + 1))
        return Array(item.logs.prefix(shown))
    }

    private func result() -> ActionResult {
        let rolledBack = fails && !previousVersion.isEmpty
        let message: String
        if !fails {
            switch action {
            case .seed: message = "seeded \(version) and healthy"
            case .promote:
                message = "promoted \(version) from \(sourceRing ?? "?") to \(targetRing) and healthy"
            case .rollback: message = "rolled back to \(version)"
            }
        } else if rolledBack {
            message = "\(action.rawValue) of \(version) failed health check; rolled back to \(previousVersion)"
        } else {
            message = "\(action.rawValue) of \(version) failed health check: unhealthy: status 503"
        }

        return ActionResult(
            app: app, action: action.rawValue, ring: targetRing, fromRing: sourceRing,
            version: version, success: !fails, rolledBack: rolledBack, message: message,
            state: RingState(
                app: app, ring: targetRing,
                currentVersion: fails ? previousVersion : version,
                previousVersion: fails ? version : previousVersion,
                healthy: !fails, autoPromote: false,
                updatedAt: startedAt.addingTimeInterval(Double(plan.count) * Self.stepDuration)
            )
        )
    }
}
