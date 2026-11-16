import SwiftUI

/// The Mac shell. Mirrors `App/RootView.swift` decision for decision, with one
/// substitution: a sidebar in place of the tab bar.
///
/// That substitution is the whole of the Mac-specific UI. A `TabView` does
/// render on macOS, so sharing `RootView` would have compiled — but it puts an
/// iPhone tab strip across a 1180pt window and leaves the sidebar area, which
/// is where a Mac operator looks for navigation, empty. The four destinations,
/// their labels and their SF Symbols all come from `Router.Tab`, so the two
/// platforms cannot drift apart: adding a tab on iOS adds a row here.
///
/// `LockScreen` and `ReauthenticationView` are reused from `RootView.swift`
/// unchanged.
struct MacRootView: View {
    @Environment(AppSession.self) private var session
    @Environment(Router.self) private var router

    var body: some View {
        @Bindable var router = router

        Group {
            switch session.mode {
            case .disconnected:
                OnboardingView()
            case .live, .demo:
                NavigationSplitView {
                    List(Router.Tab.allCases, selection: $router.tab) { tab in
                        NavigationLink(value: tab) {
                            Label(tab.label, systemImage: tab.systemImage)
                                // Matches the iOS tab bar: the Rings glyph stays
                                // two thin outline circles, because the filled
                                // variant closes up the inner one.
                                .environment(\.symbolVariants, tab == .rings ? .none : .fill)
                        }
                    }
                    .navigationSplitViewColumnWidth(min: 180, ideal: 210, max: 280)
                    .safeAreaInset(edge: .bottom) { ConnectionFooter() }
                } detail: {
                    content(for: router.tab)
                        // Each destination owns its own NavigationStack on iOS;
                        // that works unchanged in the detail column.
                        .frame(minWidth: 620)
                }
                // One view tree per control plane, exactly as on iOS: every
                // screen and store is rebuilt on a switch, so nothing from the
                // previous server survives on screen or in a cached store.
                .id(session.connectionKey)
            }
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
    private func content(for tab: Router.Tab) -> some View {
        switch tab {
        case .overview:
            OverviewView()
        case .rings:
            RingsUniverseView()
        case .activity:
            ActivityFeedView()
        case .settings:
            SettingsView()
        }
    }

    private var reauthenticationBinding: Binding<Bool> {
        Binding(
            get: { session.needsReauthentication && session.activeInstance != nil },
            set: { if !$0 { session.clearReauthenticationFlag() } }
        )
    }
}

/// Which control plane the window is pointed at, pinned under the sidebar.
///
/// On iOS this lives in the Settings tab, which is one tap away. A Mac window
/// stays open for hours across several planes, so the answer to "which server
/// am I about to promote on" belongs permanently on screen.
private struct ConnectionFooter: View {
    @Environment(AppSession.self) private var session

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Divider()
                .padding(.bottom, 6)
            HStack(spacing: 6) {
                Circle()
                    .fill(session.mode == .demo ? Color.rpGate : Color.rpHealthy)
                    .frame(width: 7, height: 7)
                Text(session.mode == .demo ? "Demo mode" : session.instanceName)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.middle)
            }
        }
        .padding(.horizontal, 12)
        .padding(.bottom, 10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
        .accessibilityLabel(
            session.mode == .demo
                ? "Demo mode, no server connected"
                : "Connected to \(session.instanceName)"
        )
    }
}
