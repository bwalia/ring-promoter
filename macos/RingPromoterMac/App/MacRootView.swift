import SwiftUI

/// The Mac shell. Mirrors `App/RootView.swift` decision for decision, with one
/// substitution: the console's sidebar in place of the iPhone's tab bar.
///
/// The sidebar is the whole of the Mac-specific navigation. A `TabView` does
/// render on macOS, so sharing `RootView` would have compiled — but it puts an
/// iPhone tab strip across a 1180pt window and leaves the sidebar area, which
/// is where a Mac operator looks for navigation, empty. The four destinations,
/// their order and their SF Symbols all come from `Router.Tab`, so the two
/// platforms cannot drift: adding a tab on iOS adds a row here.
///
/// `OverviewStore` is owned here rather than inside the overview screen so the
/// sidebar's application list is live on every tab, and so one filter box
/// narrows both it and the cards. `LockScreen` and `ReauthenticationView` are
/// reused from `RootView.swift` unchanged.
struct MacRootView: View {
    @Environment(AppSession.self) private var session
    @Environment(Router.self) private var router
    @Environment(\.scenePhase) private var scenePhase
    @State private var store: OverviewStore?

    var body: some View {
        Group {
            switch session.mode {
            case .disconnected:
                OnboardingView()
                    .consoleSurface()
            case .live, .demo:
                NavigationSplitView {
                    MacSidebar(store: store)
                        .navigationSplitViewColumnWidth(min: 232, ideal: 268, max: 340)
                } detail: {
                    content
                        .frame(minWidth: 600)
                }
                // One view tree per control plane, exactly as on iOS: every
                // screen and store is rebuilt on a switch, so nothing from the
                // previous server survives on screen or in a cached store.
                .id(session.connectionKey)
            }
        }
        .background(Color.rpBackground)
        .task(id: session.connectionKey) {
            // Rebuilt per control plane for the same reason the view tree is:
            // the previous server's applications must not survive a switch.
            store = session.mode == .disconnected ? nil : OverviewStore(session: session)
            await store?.refresh()
        }
        .task(id: session.settings.refreshInterval) {
            await autoRefreshLoop()
        }
        .onChange(of: scenePhase) { _, phase in
            // Coming back to the app is the moment an operator most wants the
            // truth, so refresh immediately rather than waiting for the timer.
            if phase == .active { Task { await store?.refresh() } }
        }
        .onChange(of: session.connectionKey) { old, _ in
            if old != nil { router.resetForNewConnection() }
        }
        .sheet(isPresented: reauthenticationBinding) {
            ReauthenticationView()
        }
        .overlay {
            if session.biometrics.isLocked, session.settings.lockOnOpen {
                LockScreen()
                    .transition(.opacity)
            }
        }
        .animation(.easeInOut(duration: 0.2), value: session.biometrics.isLocked)
    }

    @ViewBuilder
    private var content: some View {
        switch router.tab {
        case .overview:
            if let store {
                MacOverviewView(store: store)
            } else {
                ProgressView()
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .consoleSurface()
            }
        case .rings:
            // Deliberately not `consoleSurface()`: this screen paints its own
            // toolbar with `DescentPalette.space` so the stage and the strip
            // above it read as one black field.
            RingsUniverseView()
        case .activity:
            ActivityFeedView().consoleSurface()
        case .settings:
            SettingsView().consoleSurface()
        }
    }

    /// Periodic refresh. On iOS this lives in `OverviewView` and stops when
    /// that tab goes away; here the sidebar shows the same data on every tab,
    /// so it belongs to the window.
    private func autoRefreshLoop() async {
        let interval = session.settings.refreshInterval
        guard interval > 0 else { return }
        while !Task.isCancelled {
            try? await Task.sleep(for: .seconds(interval))
            guard !Task.isCancelled else { return }
            await store?.refresh()
        }
    }

    private var reauthenticationBinding: Binding<Bool> {
        Binding(
            get: { session.needsReauthentication && session.activeInstance != nil },
            set: { if !$0 { session.clearReauthenticationFlag() } }
        )
    }
}
