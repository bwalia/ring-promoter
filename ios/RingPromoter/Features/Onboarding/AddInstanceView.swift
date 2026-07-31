import SwiftUI

/// Enter a server URL and token, prove they work, then save.
///
/// The token only reaches the Keychain after the server has accepted it, so the
/// app never stores a credential it has never seen work.
struct AddInstanceView: View {
    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss

    /// Pre-filled when editing an existing record's token.
    var editing: Instance?

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
                    TextField("Name (e.g. Production)", text: $name)
                        .focused($focus, equals: .name)
                        .textInputAutocapitalization(.words)
                    TextField("https://ring-promoter.example.com", text: $urlText)
                        .focused($focus, equals: .url)
                        .textContentType(.URL)
                        .keyboardType(.URL)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                } header: {
                    Text("Control plane")
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
                    SecureField("Bearer token", text: $token)
                        .focused($focus, equals: .token)
                        .textContentType(.password)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                } header: {
                    Text("API token")
                } footer: {
                    Text(
                        "Stored in the iOS Keychain, on this device only, and never written to "
                            + "a log or shared with the widget."
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
            .navigationTitle(editing == nil ? "Add control plane" : "Update token")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    if state == .checking {
                        ProgressView()
                    } else {
                        Button("Connect") { Task { await connect() } }
                            .disabled(!canSubmit)
                    }
                }
            }
            .onAppear(perform: prefill)
        }
    }

    private var canSubmit: Bool {
        Instance.normalise(urlText: urlText) != nil
            && !token.trimmingCharacters(in: .whitespaces).isEmpty
            && !name.trimmingCharacters(in: .whitespaces).isEmpty
    }

    private func prefill() {
        guard let editing else {
            focus = .name
            return
        }
        name = editing.name
        urlText = editing.baseURL.absoluteString
        tint = editing.tint
        requireBiometrics = editing.requireBiometricsForProduction
        focus = .token
    }

    private func connect() async {
        guard let url = Instance.normalise(urlText: urlText) else {
            state = .failed(.invalidURL)
            return
        }
        let trimmedToken = token.trimmingCharacters(in: .whitespacesAndNewlines)
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
        let instance = Instance(
            id: existing?.id ?? UUID(),
            name: name.trimmingCharacters(in: .whitespacesAndNewlines),
            baseURL: url,
            tint: tint,
            requireBiometricsForProduction: requireBiometrics,
            addedAt: existing?.addedAt ?? .now
        )
        do {
            try session.instances.save(instance, token: token)
            session.connect(to: instance, token: token)
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
