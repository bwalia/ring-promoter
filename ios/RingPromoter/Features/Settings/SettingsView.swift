import SwiftUI

/// Instances, security, refresh, appearance, and what the server says it is.
struct SettingsView: View {
    @Environment(AppSession.self) private var session
    @State private var serverVersion: ServerVersion?
    @State private var isAddingInstance = false
    @State private var editingInstance: Instance?
    @State private var pendingRemoval: Instance?
    @State private var showingForgetEverything = false
    @State private var showingGroups = false

    var body: some View {
        @Bindable var settings = session.settings

        NavigationStack {
            Form {
                Section {
                    ForEach(session.instances.instances) { instance in
                        Button {
                            _ = session.activate(instance)
                        } label: {
                            InstanceRow(
                                instance: instance,
                                isActive: instance.id == session.activeInstance?.id
                            )
                        }
                        .buttonStyle(.plain)
                        .swipeActions {
                            Button("Remove", role: .destructive) { pendingRemoval = instance }
                            Button("Token") { editingInstance = instance }
                                .tint(.accentColor)
                        }
                    }
                    Button {
                        isAddingInstance = true
                    } label: {
                        Label("Add a control plane", systemImage: "plus.circle")
                    }
                    if !session.isDemo {
                        Button {
                            session.enterDemoMode()
                        } label: {
                            Label("Switch to demo mode", systemImage: "theatermasks")
                        }
                    }
                } header: {
                    Text("Control planes")
                } footer: {
                    Text(
                        session.isDemo
                            ? "Demo mode is active. Nothing you do here reaches a real server."
                            : "Tokens are stored in the Keychain, on this device only."
                    )
                }

                if !session.isDemo, session.activeInstance != nil {
                    Section("Groups") {
                        Button {
                            showingGroups = true
                        } label: {
                            Label("Application groups", systemImage: "square.stack.3d.down.right")
                        }
                    }
                }

                Section {
                    Toggle("Lock the app when it closes", isOn: $settings.lockOnOpen)
                    if var instance = session.activeInstance {
                        Toggle(
                            "Confirm production actions",
                            isOn: Binding(
                                get: { instance.requireBiometricsForProduction },
                                set: { newValue in
                                    instance.requireBiometricsForProduction = newValue
                                    session.updateActiveInstance(instance)
                                }
                            )
                        )
                    }
                } header: {
                    Text("Security")
                } footer: {
                    Text(
                        biometricFooter
                            + " Rollbacks are never blocked, so incident response is always available."
                    )
                }

                Section {
                    NotificationsRow()
                } header: {
                    Text("Notifications")
                } footer: {
                    Text(
                        "This control plane cannot send notifications yet — it has no push "
                            + "sender. Granting permission now means the app is ready if that "
                            + "changes. See docs/API-GAPS.md."
                    )
                }

                Section {
                    Picker("Refresh", selection: $settings.refreshInterval) {
                        ForEach(AppSettings.refreshOptions, id: \.self) { interval in
                            Text(settings.label(forRefresh: interval)).tag(interval)
                        }
                    }
                    Picker("Appearance", selection: $settings.theme) {
                        ForEach(AppSettings.Theme.allCases) { theme in
                            Text(theme.label).tag(theme)
                        }
                    }
                } header: {
                    Text("Display")
                } footer: {
                    Text(
                        "The Overview refreshes itself at this interval while it is on screen. "
                            + "Choose “Manual only” on a metered connection."
                    )
                }

                Section("Server") {
                    if let serverVersion {
                        LabeledContent("Version", value: serverVersion.version)
                        LabeledContent("Commit", value: serverVersion.shortCommit)
                        LabeledContent("Built", value: serverVersion.builtAt)
                        if let started = serverVersion.startedAt {
                            LabeledContent("Started") {
                                Text(started, style: .relative)
                            }
                        }
                    } else {
                        HStack {
                            ProgressView().controlSize(.small)
                            Text("Reading…").foregroundStyle(.secondary)
                        }
                    }
                    if let host = session.activeInstance?.displayHost {
                        LabeledContent("Host", value: host)
                    }
                    LabeledContent(
                        "Production password",
                        value: session.prodProtected ? "Required" : "Not configured"
                    )
                    LabeledContent(
                        "AI diagnosis", value: session.aiEnabled ? "Available" : "Not configured"
                    )
                }

                Section {
                    Button("Sign out", role: .destructive) { session.disconnect() }
                    Button("Forget every control plane", role: .destructive) {
                        showingForgetEverything = true
                    }
                } footer: {
                    Text(
                        "Forgetting removes every saved server and deletes its token from the "
                            + "Keychain. Nothing is left on this device."
                    )
                }
            }
            .navigationTitle("Settings")
            .sheet(isPresented: $isAddingInstance) { AddInstanceView() }
            .sheet(item: $editingInstance) { AddInstanceView(editing: $0) }
            .sheet(isPresented: $showingGroups) { GroupsView() }
            .task { await loadVersion() }
            .confirmationDialog(
                "Remove this control plane?",
                isPresented: Binding(
                    get: { pendingRemoval != nil },
                    set: { if !$0 { pendingRemoval = nil } }
                ),
                titleVisibility: .visible,
                presenting: pendingRemoval
            ) { instance in
                Button("Remove \(instance.name)", role: .destructive) {
                    session.instances.remove(instance)
                    if session.activeInstance?.id == instance.id { session.disconnect() }
                }
            } message: { _ in
                Text("Its API token is deleted from the Keychain too.")
            }
            .confirmationDialog(
                "Forget every control plane?",
                isPresented: $showingForgetEverything,
                titleVisibility: .visible
            ) {
                Button("Forget everything", role: .destructive) {
                    session.instances.removeAll()
                    session.disconnect()
                }
            } message: {
                Text("Every saved server and every stored token is deleted.")
            }
        }
    }

    private var biometricFooter: String {
        switch session.biometrics.availability {
        case .unavailable(let reason):
            return "Biometric confirmation is unavailable on this device: \(reason)"
        case let available:
            return "Actions that deploy into the last ring will ask for \(available.label)."
        }
    }

    private func loadVersion() async {
        serverVersion = try? await session.api.serverVersion()
    }
}

/// Permission state for push notifications, and the honest caveat that the
/// backend has nothing to send one with.
private struct NotificationsRow: View {
    @Environment(PushRegistration.self) private var push

    var body: some View {
        switch push.status {
        case .notDetermined:
            Button {
                Task { await push.requestAuthorisation() }
            } label: {
                Label("Allow notifications", systemImage: "bell.badge")
            }
        case .authorised:
            LabeledContent("Notifications") {
                Label(
                    push.deviceToken == nil ? "Registering…" : "Allowed",
                    systemImage: "checkmark.circle.fill"
                )
                .foregroundStyle(Color.rpHealthy)
                .labelStyle(.titleAndIcon)
            }
        case .denied:
            LabeledContent("Notifications", value: "Denied in iOS Settings")
        case .failed(let reason):
            LabeledContent("Notifications", value: reason)
        }
    }
}

#Preview("Settings") {
    PreviewHost { SettingsView() }
}
