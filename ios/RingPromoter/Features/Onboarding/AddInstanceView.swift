import SwiftUI

/// Enter a server URL and token, prove they work, then save.
///
/// The token only reaches the Keychain after the server has accepted it, so the
/// app never stores a credential it has never seen work.
///
/// Only the URL and token are required for a new control plane: a blank name
/// is filled from the host and the colour defaults to one no other instance
/// uses. When editing, the token may be left blank to keep the saved one, so
/// renaming or recolouring never means re-pasting a credential.
struct AddInstanceView: View {
    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss

    /// Pre-filled when editing an existing record.
    var editing: Instance?
    /// Make the saved instance the active one. Off when adding from the
    /// control-planes list, so several can be added in a row without the app
    /// switching to each in turn.
    var switchesOnSave = true

    @State private var name = ""
    @State private var urlText = ""
    @State private var token = ""
    @State private var tint: InstanceTint = .blue
    @State private var requireBiometrics = true
    @State private var state: ValidationState = .idle
    @FocusState private var focus: Field?

    private enum Field: Hashable { case name, url, token }

    private enum ValidationState: Equatable {
        case idle
        case checking
        case failed(ConnectionValidator.Failure)
        case succeeded(appCount: Int, prodProtected: Bool)
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField(namePlaceholder, text: $name)
                        .focused($focus, equals: .name)
                        .textInputAutocapitalization(.words)
                    TextField("https://ring-promoter.example.com", text: $urlText)
                        .focused($focus, equals: .url)
                        .textContentType(.URL)
                        .keyboardType(.URL)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                } header: {
                    Text("Cluster")
                } footer: {
                    if let url = Instance.normalise(urlText: urlText), url.scheme == "http" {
                        Label(
                            "This address is not HTTPS. Your token will cross the network in "
                                + "clear text — only do this for a local development server.",
                            systemImage: "exclamationmark.triangle.fill"
                        )
                        .foregroundStyle(Color.rpGate)
                    }
                }

                Section {
                    SecureField(editing == nil ? "Bearer token" : "Unchanged", text: $token)
                        .focused($focus, equals: .token)
                        .textContentType(.password)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                } header: {
                    Text("API token")
                } footer: {
                    Text(
                        (editing == nil ? "" : "Leave blank to keep the saved token. ")
                            + "Stored in the iOS Keychain, on this device only, and never "
                            + "written to a log or shared with the widget."
                    )
                }

                Section("Colour tag") {
                    Picker("Colour", selection: $tint) {
                        ForEach(InstanceTint.allCases) { option in
                            Label {
                                Text(option.label)
                            } icon: {
                                Image(systemName: "circle.fill").foregroundStyle(option.color)
                            }
                            .tag(option)
                        }
                    }
                    .pickerStyle(.navigationLink)
                }

                Section {
                    Toggle("Confirm production actions with Face ID", isOn: $requireBiometrics)
                } footer: {
                    Text(
                        "Anything that deploys into the last ring will ask you to prove it is "
                            + "you. Rollbacks are never blocked."
                    )
                }

                if case .failed(let failure) = state {
                    Section {
                        Label(failure.message, systemImage: "exclamationmark.triangle.fill")
                            .foregroundStyle(Color.rpUnhealthy)
                            .font(.subheadline)
                    }
                }
                if case .succeeded(let count, let protected) = state {
                    Section {
                        Label(
                            "Connected. \(count) application\(count == 1 ? "" : "s") available.",
                            systemImage: "checkmark.circle.fill"
                        )
                        .foregroundStyle(Color.rpHealthy)
                        if protected {
                            Label(
                                "This server requires a production password.",
                                systemImage: "lock.fill"
                            )
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                        }
                    }
                }
            }
            .navigationTitle(editing == nil ? "Add cluster" : "Edit cluster")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    if state == .checking {
                        ProgressView()
                    } else {
                        Button(editing == nil ? "Connect" : "Save") { Task { await connect() } }
                            .disabled(!canSubmit)
                    }
                }
            }
            .onAppear(perform: prefill)
        }
    }

    private var canSubmit: Bool {
        Instance.normalise(urlText: urlText) != nil
            && (editing != nil || !token.trimmingCharacters(in: .whitespaces).isEmpty)
    }

    private var namePlaceholder: String {
        if let url = Instance.normalise(urlText: urlText) {
            return "Name (\(Instance.defaultName(for: url)))"
        }
        return "Name (optional, e.g. Production)"
    }

    private func prefill() {
        guard let editing else {
            tint = session.instances.nextUnusedTint()
            focus = .url
            return
        }
        name = editing.name
        urlText = editing.baseURL.absoluteString
        tint = editing.tint
        requireBiometrics = editing.requireBiometricsForProduction
    }

    /// The token to validate: what was typed, or — when editing with the field
    /// left blank — the one already in the Keychain.
    private func resolvedToken() -> String? {
        let typed = token.trimmingCharacters(in: .whitespacesAndNewlines)
        if !typed.isEmpty { return typed }
        guard let editing else { return nil }
        return (try? Keychain.token(for: editing.keychainAccount)) ?? nil
    }

    private func connect() async {
        guard let url = Instance.normalise(urlText: urlText) else {
            state = .failed(.invalidURL)
            return
        }
        guard let trimmedToken = resolvedToken(), !trimmedToken.isEmpty else {
            state = .failed(.tokenRejected("No token is saved for this cluster. Paste one."))
            focus = .token
            return
        }
        state = .checking

        switch await ConnectionValidator.live.validate(url: url, token: trimmedToken) {
        case .failure(let failure):
            state = .failed(failure)
        case .success(let response):
            state = .succeeded(
                appCount: response.apps.count, prodProtected: response.prodProtected
            )
            save(url: url, token: trimmedToken)
        }
    }

    private func save(url: URL, token: String) {
        // Reuse an existing record for the same URL so re-adding a server
        // refreshes its token instead of leaving two entries that disagree.
        let existing = editing ?? session.instances.instance(withBaseURL: url)
        let typedName = name.trimmingCharacters(in: .whitespacesAndNewlines)
        let instance = Instance(
            id: existing?.id ?? UUID(),
            name: typedName.isEmpty ? Instance.defaultName(for: url) : typedName,
            baseURL: url,
            tint: tint,
            requireBiometricsForProduction: requireBiometrics,
            addedAt: existing?.addedAt ?? .now
        )
        // Read before saving, which overwrites it.
        let previousToken = existing.flatMap { (try? Keychain.token(for: $0.keychainAccount)) ?? nil }
        do {
            try session.instances.save(instance, token: token)
            let isActive = session.activeInstance?.id == instance.id
            if isActive, existing?.baseURL == url, token == previousToken {
                // Only the name, colour or Face ID preference changed: update in
                // place rather than rebuilding every screen.
                session.updateActiveInstance(instance)
            } else if switchesOnSave || isActive {
                session.connect(to: instance, token: token)
            }
            dismiss()
        } catch let failure as Keychain.Failure {
            state = .failed(.other(failure.message))
        } catch {
            state = .failed(.other(error.localizedDescription))
        }
    }
}

#Preview("Add instance") {
    PreviewHost { AddInstanceView() }
}
