import ActivityKit
import Foundation

/// The Live Activity shown on the Lock Screen and in the Dynamic Island while a
/// promotion runs.
///
/// Defined here so the app (which starts and updates it) and the widget
/// extension (which draws it) compile against exactly the same type — a
/// mismatch between them is a silent failure at runtime.
struct PromotionActivityAttributes: ActivityAttributes {
    /// Fixed for the lifetime of the activity.
    let app: String
    let appTitle: String
    /// "seed" | "promote" | "rollback"
    let action: String
    let targetRing: String
    let version: String
    let jobID: String

    /// The parts that change as the job runs.
    struct ContentState: Codable, Hashable, Sendable {
        /// What the server is doing right now.
        var stepTitle: String
        /// 1-based, for "step 2 of 4".
        var stepIndex: Int
        var stepCount: Int
        var status: Status
        /// The terminal message, once there is one.
        var message: String?

        enum Status: String, Codable, Hashable, Sendable {
            case running
            case succeeded
            case failed
            case rolledBack

            var isTerminal: Bool { self != .running }

            var headline: String {
                switch self {
                case .running: "Running"
                case .succeeded: "Succeeded"
                case .failed: "Failed"
                case .rolledBack: "Failed — rolled back"
                }
            }

            var systemImage: String {
                switch self {
                case .running: "arrow.triangle.2.circlepath"
                case .succeeded: "checkmark.circle.fill"
                case .failed: "xmark.octagon.fill"
                case .rolledBack: "arrow.uturn.backward.circle.fill"
                }
            }
        }

        /// 0…1, for the progress bar. Falls back to indeterminate-looking zero
        /// when the step count is not yet known.
        var fraction: Double {
            guard stepCount > 0 else { return 0 }
            return min(1, Double(stepIndex) / Double(stepCount))
        }
    }
}
