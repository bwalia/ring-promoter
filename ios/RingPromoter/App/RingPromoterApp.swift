import SwiftUI

@main
struct RingPromoterApp: App {
    @State private var session = AppSession()
    @State private var router = Router()
    @State private var push = PushRegistration()
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(session)
                .environment(router)
                .environment(push)
                .preferredColorScheme(session.settings.theme.colorScheme)
                .task {
                    // The delegate exists only to receive UIKit callbacks; it
                    // holds no state of its own.
                    AppDelegate.push = push
                    AppDelegate.router = router
                    session.restoreLastSession()
                    await push.refreshStatus()
                    // Do not touch ActivityKit here. Cleaning orphans at launch
                    // SIGSEGV'd on TestFlight Mac (MacFamily) builds inside
                    // Swift concurrency; orphans are cleared the next time a
                    // Live Activity is started instead.
                    consumePendingIntent()
                }
                .onOpenURL { router.open($0) }
                .onChange(of: scenePhase) { _, phase in
                    // Re-lock as soon as the app leaves the foreground, so the
                    // app switcher never shows a live pipeline on a device
                    // handed to someone else.
                    if phase != .active, session.settings.lockOnOpen {
                        session.biometrics.lock()
                    }
                    // An App Intent runs before the UI exists and leaves its
                    // request behind, so pick it up on the way back in.
                    if phase == .active { consumePendingIntent() }
                }
        }
    }

    /// Act on whatever a Shortcut asked for.
    ///
    /// "Prepare a rollback" navigates to the ring's screen rather than rolling
    /// anything back — the confirmation still happens by hand.
    @MainActor
    private func consumePendingIntent() {
        switch IntentDeepLink.take() {
        case .app(let name):
            router.show(app: name)
        case .rollback(let name, _):
            router.show(app: name)
        case nil:
            break
        }
    }
}
