import SwiftUI
import UIKit
import UserNotifications

/// The small amount of UIKit the app still needs: APNs registration callbacks
/// and notification taps, neither of which SwiftUI exposes.
///
/// The backend cannot send a push yet — see `docs/API-GAPS.md` — so this is the
/// client half only. It is wired up rather than stubbed out so that the day a
/// sender exists, nothing on this side has to be written or debugged under
/// pressure.
@MainActor
final class AppDelegate: NSObject, UIApplicationDelegate {
    /// Set by the app at launch so callbacks can reach the app's state.
    static weak var push: PushRegistration?
    static weak var router: Router?

    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        UNUserNotificationCenter.current().delegate = self
        return true
    }

    nonisolated func application(
        _ application: UIApplication,
        didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data
    ) {
        Task { @MainActor in Self.push?.didRegister(with: deviceToken) }
    }

    nonisolated func application(
        _ application: UIApplication,
        didFailToRegisterForRemoteNotificationsWithError error: any Error
    ) {
        Task { @MainActor in Self.push?.didFailToRegister(error) }
    }
}

extension AppDelegate: UNUserNotificationCenterDelegate {
    /// A notification that arrives while the app is open is still worth showing:
    /// the operator may be on a different application's screen.
    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        [.banner, .sound]
    }

    /// Tapping a notification deep-links to the job it is about.
    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse
    ) async {
        let userInfo = response.notification.request.content.userInfo
        guard let route = PushRegistration.route(from: userInfo) else { return }
        await MainActor.run {
            switch route {
            case .app(let name): Self.router?.show(app: name)
            case .job(let app, let id): Self.router?.showJob(app: app, id: id)
            case .history(let app), .maintenance(let app): Self.router?.show(app: app)
            case .group(let id): Self.router?.show(group: id)
            }
        }
    }
}
