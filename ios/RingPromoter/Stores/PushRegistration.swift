import Foundation
import UIKit
import UserNotifications

/// The client half of push notifications: permission, APNs registration, and
/// turning a delivered payload into a deep link.
///
/// **The backend has no push sender.** Ring Promoter's Go service exposes no
/// endpoint to register a device token with, and nothing that would call APNs
/// when a job finishes. Rather than fake it, this type does the part that is
/// genuinely the client's job and stops at the boundary: `deviceToken` is
/// captured and surfaced, and `registerWithBackend` is the single function that
/// would post it once the server can accept one.
///
/// `docs/API-GAPS.md` specifies the minimal Go-side addition required.
@MainActor
@Observable
final class PushRegistration: NSObject {
    enum Status: Equatable {
        case notDetermined
        case denied
        case authorised
        /// Permission granted, APNs registration failed.
        case failed(String)
    }

    private(set) var status: Status = .notDetermined
    /// The APNs token, hex-encoded. Kept only in memory: there is nowhere to
    /// send it yet, and storing a token nobody consumes serves no purpose.
    private(set) var deviceToken: String?

    /// Ask for permission, then register with APNs.
    ///
    /// Alerts and sounds only — no badge. A number on the icon that nothing
    /// ever clears would be noise, not information.
    func requestAuthorisation() async {
        let centre = UNUserNotificationCenter.current()
        do {
            let granted = try await centre.requestAuthorization(options: [.alert, .sound])
            guard granted else {
                status = .denied
                return
            }
            status = .authorised
            UIApplication.shared.registerForRemoteNotifications()
        } catch {
            status = .failed(error.localizedDescription)
        }
    }

    func refreshStatus() async {
        let settings = await UNUserNotificationCenter.current().notificationSettings()
        switch settings.authorizationStatus {
        case .authorized, .provisional, .ephemeral: status = .authorised
        case .denied: status = .denied
        default: status = .notDetermined
        }
    }

    func didRegister(with token: Data) {
        deviceToken = token.map { String(format: "%02x", $0) }.joined()
    }

    func didFailToRegister(_ error: any Error) {
        status = .failed(error.localizedDescription)
    }

    /// Post the device token to the control plane.
    ///
    /// Deliberately unimplemented: there is no endpoint to post to. See
    /// `docs/API-GAPS.md` for the proposed
    /// `POST /api/devices {token, platform}` and the job-completion hook that
    /// would make it useful.
    func registerWithBackend(api: any RingPromoterAPI) async {
        assertionFailure(
            "No push endpoint exists on the backend yet — see docs/API-GAPS.md before calling this."
        )
    }

    /// Turn a notification payload into a route.
    ///
    /// The payload shape is the one proposed in the API-gap document, so the
    /// client side is ready the day the server can send one.
    nonisolated static func route(from userInfo: [AnyHashable: Any]) -> Route? {
        guard let app = userInfo["app"] as? String, !app.isEmpty else { return nil }
        if let job = userInfo["job_id"] as? String, !job.isEmpty {
            return .job(app: app, id: job)
        }
        return .app(app)
    }
}
