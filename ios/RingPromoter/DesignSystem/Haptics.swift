import UIKit

/// Haptic feedback for the moments that matter.
///
/// Deliberately sparse: an operator glancing at a phone mid-incident should
/// feel *started*, *succeeded* and *failed*, and nothing else. Anything more
/// and the meaningful ones stop registering.
@MainActor
enum Haptics {
    /// An action was accepted by the server and a job is now running.
    static func actionStarted() {
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
    }

    /// A job finished successfully.
    static func success() {
        UINotificationFeedbackGenerator().notificationOccurred(.success)
    }

    /// A job failed, or was rolled back.
    static func failure() {
        UINotificationFeedbackGenerator().notificationOccurred(.error)
    }

    /// Something was refused before it was sent.
    static func warning() {
        UINotificationFeedbackGenerator().notificationOccurred(.warning)
    }
}
