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
    /// Repairs `TokenHygiene` made to what was pasted, shown on success so a
    /// token that works here but not in curl is never a mystery.
    @State private var tokenRepairs: [String] = []
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
                Section("Cluster") {
                    // Label above the field rather than beside it.
                    //
                    // These used to carry no label at all: the placeholder was
                    // doing the job ("Name (optional, e.g. Production)"). That
                    // reads acceptably on a phone and falls apart on a Mac,
                    // where SwiftUI puts a TextField's title in a leading
                    // label column — so the placeholder text became the label,
                    // the field sat empty beside it, and the example URL was
                    // detected as a link and drawn in blue.
                    //
                    // A label above the field has no column to be placed in,
                    // so both platforms render the same thing, and the
                    // placeholder goes back to being an example.
                    FormField("Name") {
                        TextField("Name", text: $name, prompt: Text(namePlaceholder))
                            .focused($focus, equals: .name)
                            .textInputAutocapitalization(.words)
                    }
                    FormField("Server URL", footnote: urlFootnote, tint: urlFootnoteTint) {
                        TextField(
                            "Server URL", text: $urlText,
                            prompt: Text("https://ring-promoter.example.com")
                        )
                        .focused($focus, equals: .url)
                        .textContentType(.URL)
                        .keyboardType(.URL)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                    }
                }

                Section {
                    FormField(
                        "API token", footnote: tokenFootnote, tint: tokenFootnoteTint
                    ) {
                        SecureField(
                            "API token", text: $token,
                            prompt: Text(editing == nil ? "Paste the bearer token" : "Unchanged")
                        )
                        .focused($focus, equals: .token)
                        // Deliberately NOT .textContentType(.password). A
                        // bearer token is not a password, and declaring it one
                        // puts iOS AutoFill's saved-credential suggestion in
                        // the QuickType bar directly above the field — where
                        // one mis-tap silently replaces the token, and the
                        // server answers that with the same opaque 401 as a
                        // typo.
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                    }
                } header: {
                    Text("Authentication")
                } footer: {
                    Text(
                        (editing == nil ? "" : "Leave blank to keep the saved token. ")
                            + "Stored in the Keychain, on this device only, and never "
                            + "written to a log or shared with the widget."
                    )
                }

                Section("Colour tag") {
                    // Swatches rather than a menu. Six options whose whole
                    // point is the colour, so showing them is both faster and
                    // more honest than a list of their names.
                    InstanceTintPicker(selection: $tint)
                }

                Section {
                    Toggle(
                        "Confirm production actions with \(biometryName)",
                        isOn: $requireBiometrics
                    )
                    .disabled(!biometricsAvailable)
                } footer: {
                    Text(
                        biometricsAvailable
                            ? "Anything that deploys into the last ring will ask you to prove "
                                + "it is you. Rollbacks are never blocked."
                            : "This device has no biometrics or passcode set up, so this "
                                + "cannot be switched on."
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
                        if !tokenRepairs.isEmpty {
                            Label(
                                "Before connecting, the app "
                                    + tokenRepairs.joined(separator: " and ") + ".",
                                systemImage: "wand.and.sparkles"
                            )
                            .font(.subheadline)
                            .foregroundStyle(Color.rpGate)
                        }
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
            .rpSheetForm(width: 580, height: 640)
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

    /// What the device actually offers, rather than "Face ID" for everyone.
    /// A Mac with Touch ID, an iPhone SE and an iPad all got the wrong word.
    private var biometryName: String {
        let availability = session.biometrics.availability
        return availability.isAvailable ? availability.label : "biometrics"
    }

    private var biometricsAvailable: Bool { session.biometrics.availability.isAvailable }

    /// Says what the app will actually connect to.
    ///
    /// Worth showing because `Instance.normalise` does real work: it adds a
    /// missing scheme and trims trailing slashes, and it keeps a query string,
    /// so pasting a console URL straight from the address bar gives a base
    /// that is not quite what you meant.
    private var urlFootnote: String? {
        let trimmed = urlText.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty else { return nil }
        guard let url = Instance.normalise(urlText: trimmed) else {
            return "Not a usable address yet."
        }
        if url.scheme == "http" {
            return "Not HTTPS — your token would cross the network in clear text. "
                + "Only do this for a local development server."
        }
        if url.query() != nil {
            return "Will connect to \(url.absoluteString) — that still carries a query "
                + "string. You probably want just the scheme and host."
        }
        return "Will connect to \(url.absoluteString)"
    }

    private var urlFootnoteTint: Color? {
        guard let url = Instance.normalise(urlText: urlText) else {
            return urlText.trimmingCharacters(in: .whitespaces).isEmpty ? nil : .rpGate
        }
        return url.scheme == "http" || url.query() != nil ? .rpGate : .rpHealthy
    }

    /// Counted as it is typed or pasted. A token truncated by a partial copy
    /// looks exactly like a whole one, and this is the only place that
    /// difference is visible before the server answers 401.
    private var tokenFootnote: String? {
        guard !token.isEmpty else { return nil }
        let cleaned = TokenHygiene.clean(token)
        return cleaned.defect?.message ?? lengthNote(cleaned)
    }

    private var tokenFootnoteTint: Color? {
        guard !token.isEmpty else { return nil }
        return TokenHygiene.clean(token).isUsable ? nil : .rpGate
    }

    /// "64 characters." — plus what was repaired, when anything was, so a
    /// token that works here but not in `curl` is never a mystery.
    private func lengthNote(_ cleaned: TokenHygiene.Cleaned) -> String {
        let count = "\(cleaned.token.count) character\(cleaned.token.count == 1 ? "" : "s")"
        guard !cleaned.repairs.isEmpty else { return "\(count)." }
        return "\(count), after the app \(cleaned.repairs.joined(separator: " and "))."
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
    ///
    /// What was typed is cleaned first. A pasted token routinely arrives
    /// wrapped in a shell's quotes or carrying the `Bearer ` prefix from a
    /// curl example, and both come back from the server as an indistinguishable
    /// 401. See `TokenHygiene`.
    private func resolvedToken() -> TokenHygiene.Cleaned? {
        let typed = token.trimmingCharacters(in: .whitespacesAndNewlines)
        if !typed.isEmpty { return TokenHygiene.clean(typed) }
        guard let editing,
              let saved = (try? Keychain.token(for: editing.keychainAccount)) ?? nil
        else { return nil }
        return TokenHygiene.Cleaned(token: saved, repairs: [], defect: nil)
    }

    private func connect() async {
        guard let url = Instance.normalise(urlText: urlText) else {
            state = .failed(.invalidURL)
            return
        }
        guard let cleaned = resolvedToken() else {
            state = .failed(.tokenRejected("No token is saved for this cluster. Paste one."))
            focus = .token
            return
        }
        // Named before any request: the server would answer 401 to all of
        // these and the operator would be none the wiser.
        if let defect = cleaned.defect {
            state = .failed(.tokenRejected(defect.message))
            focus = .token
            return
        }
        let trimmedToken = cleaned.token
        tokenRepairs = cleaned.repairs
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

/// One labelled field: caption above, control below, and a line of feedback
/// under it when there is something worth saying.
///
/// The label sits above rather than beside because SwiftUI places a
/// `TextField`'s title in a leading label column on macOS and inside the field
/// as a placeholder on iOS — one declaration, two different meanings. A label
/// above has no column to be placed in, so both platforms draw the same thing.
private struct FormField<Content: View>: View {
    let label: String
    var footnote: String?
    var tint: Color?
    @ViewBuilder var content: Content

    init(
        _ label: String,
        footnote: String? = nil,
        tint: Color? = nil,
        @ViewBuilder content: () -> Content
    ) {
        self.label = label
        self.footnote = footnote
        self.tint = tint
        self.content = content()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(label)
                .font(.caption.weight(.medium))
                .foregroundStyle(.secondary)
            content
                // Without this macOS reserves a leading column for the
                // field's own title and the row splits in two.
                .labelsHidden()
            if let footnote {
                Text(footnote)
                    .font(.caption)
                    .foregroundStyle(tint ?? Color.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                    .transition(.opacity)
            }
        }
        .padding(.vertical, 3)
        .animation(.easeInOut(duration: 0.15), value: footnote)
    }
}

/// The colour tag, as swatches.
///
/// Was a menu listing six colour *names*, which is a list of words standing in
/// for the only thing that matters. The whole purpose of the tag is to tell
/// production from staging at a glance, so the choice should look like the
/// result.
private struct InstanceTintPicker: View {
    @Binding var selection: InstanceTint

    var body: some View {
        HStack(spacing: 10) {
            ForEach(InstanceTint.allCases) { option in
                Button {
                    selection = option
                } label: {
                    Circle()
                        .fill(option.color)
                        .frame(width: 24, height: 24)
                        .padding(3)
                        .overlay {
                            // A halo rather than a tick: a checkmark has to be
                            // drawn in a colour that reads on all six, and
                            // there is not one.
                            Circle()
                                .strokeBorder(
                                    selection == option ? option.color : .clear,
                                    lineWidth: 2
                                )
                        }
                        .contentShape(.circle)
                }
                .buttonStyle(.plain)
                .accessibilityLabel(option.label)
                .accessibilityAddTraits(
                    selection == option ? [.isButton, .isSelected] : .isButton
                )
            }
            Spacer(minLength: 0)
        }
        .padding(.vertical, 2)
        .animation(.easeInOut(duration: 0.15), value: selection)
        .accessibilityElement(children: .contain)
        .accessibilityLabel("Colour tag")
    }
}

#Preview("Add instance") {
    PreviewHost { AddInstanceView() }
}
