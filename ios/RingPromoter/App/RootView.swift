import SwiftUI

/// Decides what the operator sees: onboarding, the lock screen, or the app.
struct RootView: View {
    @Environment(AppSession.self) private var session
    @Environment(Router.self) private var router

    var body: some View {
        @Bindable var router = router

        Group {
            switch session.mode {
            case .disconnected:
                OnboardingView()
            case .live, .demo:
                // The `Tab` builder syntax is iOS 18+; this app targets iOS 17,
                // so the classic `.tabItem` form is used.
                TabView(selection: $router.tab) {
                    ForEach(Router.Tab.allCases) { tab in
                        content(for: tab)
                            .tabItem {
                                Label(tab.label, systemImage: tab.systemImage)
                                    // Keep the Rings icon as two thin outline
                                    // circles; the tab bar would otherwise fill
                                    // the inner one.
                                    .environment(\.symbolVariants, tab == .rings ? .none : .fill)
                            }
                            .tag(tab)
                    }
                }
            }
        }
        // The token was rejected mid-session: take the operator straight to the
        // screen that can fix it rather than showing an error they can dismiss.
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

/// Covers the app whenever it is not in the foreground and the operator has
/// asked for a lock.
struct LockScreen: View {
    @Environment(AppSession.self) private var session
    @State private var isAuthenticating = false
    @State private var failed = false

    var body: some View {
        ZStack {
            Rectangle()
                .fill(.background)
                .ignoresSafeArea()
            VStack(spacing: 18) {
                Image(systemName: "lock.shield")
                    .font(.system(size: 46, weight: .light))
                    .foregroundStyle(.tint)
                Text("Ring Promoter is locked")
                    .font(.headline)
                if failed {
                    Text("Authentication failed. Try again.")
                        .font(.subheadline)
                        .foregroundStyle(Color.rpUnhealthy)
                }
                Button {
                    Task { await unlock() }
                } label: {
                    Label("Unlock", systemImage: "faceid")
                        .frame(maxWidth: 220)
                }
                .buttonStyle(.borderedProminent)
                .disabled(isAuthenticating)
            }
        }
        .task { await unlock() }
        .accessibilityAddTraits(.isModal)
    }

    private func unlock() async {
        guard !isAuthenticating else { return }
        isAuthenticating = true
        defer { isAuthenticating = false }
        failed = !(await session.biometrics.unlock())
    }
}

/// Shown when the server rejects the stored token mid-session.
struct ReauthenticationView: View {
    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var token = ""
    @State private var isSaving = false
    @State private var error: String?

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text(
                        "\(session.instanceName) rejected the saved API token. Paste a current one to carry on."
                    )
                    .font(.subheadline)
                }
                Section("API token") {
                    SecureField("Bearer token", text: $token)
                        .textContentType(.password)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                }
                if let error {
                    Section {
                        Label(error, systemImage: "exclamationmark.triangle.fill")
                            .foregroundStyle(Color.rpUnhealthy)
                            .font(.subheadline)
                    }
                }
            }
            .navigationTitle("Token expired")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Sign out") {
                        session.disconnect()
                        dismiss()
                    }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") { Task { await save() } }
                        .disabled(token.trimmingCharacters(in: .whitespaces).isEmpty || isSaving)
                }
            }
        }
        .interactiveDismissDisabled()
    }

    private func save() async {
        guard let instance = session.activeInstance else { return }
        isSaving = true
        defer { isSaving = false }
        let trimmed = token.trimmingCharacters(in: .whitespacesAndNewlines)
        do {
            try session.instances.save(instance, token: trimmed)
            session.connect(to: instance, token: trimmed)
            _ = try await session.loadCapabilities()
            dismiss()
        } catch let failure as Keychain.Failure {
            error = failure.message
        } catch let failure as APIError {
            error = failure.userMessage
        } catch {
            self.error = error.localizedDescription
        }
    }
}
