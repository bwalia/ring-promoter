import SwiftUI

/// The Mac entry point. Replaces `App/RingPromoterApp.swift`, which cannot be
/// shared: `@main` may exist once per target, and
/// `@UIApplicationDelegateAdaptor` has no macOS counterpart.
///
/// Everything below the scene — the session, the router, every screen — is the
/// same code the iOS app runs.
@main
struct RingPromoterMacApp: App {
    @State private var session = AppSession()
    @State private var router = Router()
    @State private var push = PushRegistration()
    @Environment(\.openWindow) private var openWindow

    var body: some Scene {
        WindowGroup {
            MacRootView()
                .environment(session)
                .environment(router)
                .environment(push)
                .preferredColorScheme(session.settings.theme.colorScheme)
                // A control plane is a dense table of rings; below this the
                // columns collapse into something unreadable.
                .frame(minWidth: 860, minHeight: 560)
                .task {
                    session.restoreLastSession()
                    await push.refreshStatus()
                }
                .onOpenURL { router.open($0) }
        }
        .defaultSize(width: 1180, height: 760)
        // Mac windows persist position and size per scene, and the operator can
        // open two at once onto different control planes.
        .commands { RingPromoterCommands(session: session, router: router) }
    }
}

/// Menu-bar commands.
///
/// A Mac app is expected to be drivable from the keyboard, and the four
/// destinations in the sidebar are the same four the iOS tab bar offers — so
/// ⌘1…⌘4 mirror them exactly rather than inventing a second navigation model.
struct RingPromoterCommands: Commands {
    let session: AppSession
    let router: Router

    var body: some Commands {
        // Replaces the default "New Window" placement with navigation, which is
        // what this app actually does.
        CommandGroup(after: .sidebar) {
            Divider()
            ForEach(Array(Router.Tab.allCases.enumerated()), id: \.element) { index, tab in
                Button(tab.label) { router.tab = tab }
                    .keyboardShortcut(
                        KeyEquivalent(Character("\(index + 1)")),
                        modifiers: .command
                    )
                    .disabled(session.mode == .disconnected)
            }
        }

        CommandGroup(replacing: .help) {
            Link("Ring Promoter Documentation", destination: URL(string: "https://ringpromoter.com")!)
        }
    }
}
