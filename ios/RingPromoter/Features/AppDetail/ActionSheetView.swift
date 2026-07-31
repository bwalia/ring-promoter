import SwiftUI

/// What the operator asked to do. Carried into the sheet so one screen handles
/// every action with the same confirmation rules.
struct ActionSheetRequest: Identifiable, Hashable {
    enum Kind: Hashable {
        case promote(from: RingStatus)
        case seed(ring: RingStatus)
        case rollback(ring: RingStatus)
        case autoPromote(ring: RingStatus, enabled: Bool)
    }

    let kind: Kind
    let id = UUID()

    var action: PromotionAction {
        switch kind {
        case .promote: .promote
        case .seed: .seed
        case .rollback: .rollback
        case .autoPromote: .promote
        }
    }
}

/// Collects whatever the action needs, confirms it, and starts it.
///
/// The submit button stays disabled until every requirement is met, so the app
/// never sends a request it knows the server will refuse. What the requirements
/// *are* comes from `PromotionRules`, not from this view.
struct ActionSheetView: View {
    let store: AppDetailStore
    let request: ActionSheetRequest
    /// Called with the new job's id once the server accepts the action.
    let onStarted: (String) -> Void

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss

    @State private var crCode = ""
    @State private var password = ""
    @State private var seedVersion = ""
    @State private var typedAppName = ""
    @State private var isSubmitting = false
    @State private var error: APIError?
    @State private var showingSignoffSheet = false
    @State private var showingWindowSheet = false

    // MARK: - Derived

    private var legality: PromotionLegality {
        switch request.kind {
        case .promote(let ring): store.rules.promote(from: ring, rings: store.rings)
        case .seed(let ring): store.rules.seed(into: ring)
        case .rollback(let ring): store.rules.rollback(ring)
        case .autoPromote(let ring, let enabled):
            store.rules.toggleAutoPromote(ring, enabled: enabled)
        }
    }

    private var requirements: PromotionLegality.Requirements { legality.requirements }

    private var sourceRing: RingStatus? {
        if case .promote(let ring) = request.kind { return ring }
        return nil
    }

    private var targetRing: RingStatus? {
        legality.targetRing.flatMap { store.ring(named: $0) }
    }

    /// The version this action would put into the target ring — the exact
    /// string the QA sign-off gate matches on.
    private var versionInFlight: String {
        switch request.kind {
        case .promote(let ring): ring.currentVersion
        case .seed: seedVersion.trimmingCharacters(in: .whitespacesAndNewlines)
        case .rollback(let ring): ring.previousVersion
        case .autoPromote: ""
        }
    }

    private var targetsProduction: Bool {
        guard let target = legality.targetRing else { return false }
        return store.pipeline.isProduction(target)
    }

    /// Typing the app's name is the deliberate, un-fat-fingerable confirmation
    /// for a production deploy. Rollback is deliberately exempt.
    private var requiresTypedConfirmation: Bool {
        targetsProduction && request.action != .rollback
    }

    private var existingSignoff: Signoff? {
        guard requirements.qaSignoff, let target = legality.targetRing,
              !versionInFlight.isEmpty
        else { return nil }
        return store.signoff(ring: target, version: versionInFlight)
    }

    private var signoffSatisfied: Bool {
        guard requirements.qaSignoff else { return true }
        return existingSignoff?.isGo == true
    }

    private var windowSatisfied: Bool {
        guard requirements.maintenanceWindow else { return true }
        return requirements.maintenanceWindowOpen
    }

    private var canSubmit: Bool {
        guard legality.isAllowed, !isSubmitting else { return false }
        if requirements.changeRequestCode, crCode.trimmingCharacters(in: .whitespaces).isEmpty {
            return false
        }
        if requirements.productionPassword, password.isEmpty { return false }
        if case .seed = request.kind, versionInFlight.isEmpty { return false }
        if requiresTypedConfirmation, typedAppName.trimmingCharacters(in: .whitespaces) != store.app {
            return false
        }
        return signoffSatisfied && windowSatisfied
    }

    // MARK: - Body

    var body: some View {
        NavigationStack {
            Form {
                summarySection
                if case .seed = request.kind { versionSection }
                if requirements.maintenanceWindow { maintenanceSection }
                if requirements.qaSignoff { signoffSection }
                if requirements.changeRequestCode { changeRequestSection }
                if requirements.productionPassword { passwordSection }
                if requiresTypedConfirmation { typedConfirmationSection }
                if let error { errorSection(error) }
            }
            .navigationTitle(navigationTitle)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                        .accessibilityIdentifier("action-cancel")
                }
                ToolbarItem(placement: .confirmationAction) {
                    if isSubmitting {
                        ProgressView()
                    } else {
                        Button(submitTitle) { Task { await submit() } }
                            .disabled(!canSubmit)
                            .fontWeight(.semibold)
                            .accessibilityIdentifier("action-submit")
                    }
                }
            }
            .sheet(isPresented: $showingSignoffSheet) {
                if let target = legality.targetRing {
                    RecordSignoffView(
                        store: store, ring: target, version: versionInFlight
                    )
                }
            }
            .sheet(isPresented: $showingWindowSheet) {
                if let target = legality.targetRing {
                    OpenWindowView(store: store, ring: target)
                }
            }
            .onAppear(perform: prefill)
        }
        .presentationDetents([.large])
    }

    // MARK: - Sections

    private var summarySection: some View {
        Section {
            switch request.kind {
            case .promote(let ring):
                SummaryLine(label: "From", value: "\(ring.ring.label) · \(ring.currentVersion)")
                if let target = targetRing {
                    SummaryLine(
                        label: "To", value: target.ring.label,
                        isProduction: store.pipeline.isProduction(target.ring.name)
                    )
                }
            case .seed(let ring):
                SummaryLine(
                    label: "Ring", value: ring.ring.label,
                    isProduction: store.pipeline.isProduction(ring.ring.name)
                )
                if !ring.isEmpty {
                    SummaryLine(label: "Replaces", value: ring.currentVersion)
                }
            case .rollback(let ring):
                SummaryLine(label: "Ring", value: ring.ring.label)
                SummaryLine(label: "From", value: ring.currentVersion)
                SummaryLine(label: "Back to", value: ring.previousVersion)
            case .autoPromote(let ring, let enabled):
                SummaryLine(label: "Ring", value: ring.ring.label)
                SummaryLine(label: "Auto-promote", value: enabled ? "On" : "Off")
            }
        } header: {
            Text(store.title)
        } footer: {
            if case .rollback = request.kind {
                Text(
                    "Rollbacks are never gated: no window, no sign-off and no production "
                        + "password is required."
                )
            } else if targetsProduction {
                Label(
                    "This deploys to production.", systemImage: "exclamationmark.shield.fill"
                )
                .foregroundStyle(Color.rpProduction)
                .font(.footnote.weight(.medium))
            }
        }
    }

    private var versionSection: some View {
        Section {
            if store.versions.offersPicker {
                Picker("Version", selection: $seedVersion) {
                    Text("Choose…").tag("")
                    ForEach(store.versions.branches) { version in
                        Label(version.name, systemImage: version.systemImage).tag(version.name)
                    }
                    ForEach(store.versions.tags) { version in
                        Label(version.name, systemImage: version.systemImage).tag(version.name)
                    }
                }
                .pickerStyle(.navigationLink)
            } else {
                TextField("Version, branch or tag", text: $seedVersion)
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)
                    .font(.body.monospaced())
            }
        } header: {
            Text("Version")
        } footer: {
            Text(
                store.versions.offersPicker
                    ? "Only versions that exist in the application's source repository are offered."
                    : "This application's deployer cannot list versions, so the value is not "
                        + "checked until the server tries it."
            )
        }
    }

    private var maintenanceSection: some View {
        Section {
            HStack {
                Label(
                    requirements.maintenanceWindowOpen ? "A window is open" : "No window is open",
                    systemImage: requirements.maintenanceWindowOpen ? "lock.open.fill" : "lock.fill"
                )
                .foregroundStyle(
                    requirements.maintenanceWindowOpen ? Color.rpHealthy : Color.rpGate
                )
                .font(.subheadline)
                Spacer()
                if !requirements.maintenanceWindowOpen {
                    Button("Open one") { showingWindowSheet = true }
                        .buttonStyle(.bordered)
                        .controlSize(.small)
                }
            }
            ForEach(store.maintenance.recurring) { window in
                Label(window.summary, systemImage: "calendar")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        } header: {
            Text("Maintenance window")
        } footer: {
            if !requirements.maintenanceWindowOpen {
                Text(
                    "The server will refuse this promotion until a window is open — either a "
                        + "scheduled one, or an ad-hoc one you open here."
                )
            }
        }
    }

    private var signoffSection: some View {
        Section {
            if let signoff = existingSignoff {
                HStack {
                    Label(
                        signoff.isGo ? "GO recorded" : "NO-GO recorded",
                        systemImage: signoff.isGo ? "checkmark.seal.fill" : "xmark.seal.fill"
                    )
                    .foregroundStyle(signoff.isGo ? Color.rpHealthy : Color.rpUnhealthy)
                    .font(.subheadline.weight(.medium))
                    Spacer()
                    Button("Change") { showingSignoffSheet = true }
                        .buttonStyle(.bordered)
                        .controlSize(.small)
                }
                VStack(alignment: .leading, spacing: 2) {
                    Text("\(signoff.engineer) · \(signoff.qaStatus)")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    if let note = signoff.note, !note.isEmpty {
                        Text(note).font(.caption).foregroundStyle(.secondary)
                    }
                }
            } else {
                HStack {
                    Label("No sign-off for this version", systemImage: "seal")
                        .foregroundStyle(Color.rpGate)
                        .font(.subheadline)
                    Spacer()
                    Button("Record") { showingSignoffSheet = true }
                        .buttonStyle(.bordered)
                        .controlSize(.small)
                        .disabled(versionInFlight.isEmpty)
                }
            }
        } header: {
            Text("QA sign-off")
        } footer: {
            Text(
                versionInFlight.isEmpty
                    ? "Choose a version first — sign-offs are recorded against one exact version."
                    : "Sign-offs are version-specific. This one covers \(versionInFlight) only."
            )
        }
    }

    private var changeRequestSection: some View {
        Section {
            TextField("Change-request code", text: $crCode)
                .autocorrectionDisabled()
                .textInputAutocapitalization(.characters)
                .font(.body.monospaced())
        } header: {
            Text("Change request")
        } footer: {
            if let provider = requirements.changeRequestProvider, !provider.isEmpty {
                Text("Validated against \(provider).")
            } else {
                Text("The server validates this code before anything is deployed.")
            }
        }
    }

    private var passwordSection: some View {
        Section {
            SecureField("Production password", text: $password)
                .textContentType(.password)
                .autocorrectionDisabled()
        } header: {
            Text("Production password")
        } footer: {
            Text(
                "This server was started with a production password. It is sent only with this "
                    + "request and is never stored on the device."
            )
        }
    }

    private var typedConfirmationSection: some View {
        Section {
            TextField("Type \(store.app) to confirm", text: $typedAppName)
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
                .font(.body.monospaced())
        } header: {
            Text("Confirm")
        } footer: {
            Text("Type the application's name exactly to enable \(submitTitle).")
        }
    }

    private func errorSection(_ error: APIError) -> some View {
        Section {
            VStack(alignment: .leading, spacing: 6) {
                Label(error.title, systemImage: "exclamationmark.triangle.fill")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(Color.rpUnhealthy)
                Text(error.userMessage)
                    .font(.subheadline)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    // MARK: - Behaviour

    private var navigationTitle: String {
        switch request.kind {
        case .promote: "Promote"
        case .seed: "Seed a version"
        case .rollback: "Roll back"
        case .autoPromote(_, let enabled): enabled ? "Enable auto-promote" : "Disable auto-promote"
        }
    }

    private var submitTitle: String {
        switch request.kind {
        case .promote: "Promote"
        case .seed: "Seed"
        case .rollback: "Roll back"
        case .autoPromote: "Save"
        }
    }

    private func prefill() {
        // The demo code the backend always accepts; pre-filling it in demo mode
        // keeps the gated path walkable during a review without a JIRA server.
        if session.isDemo, requirements.changeRequestCode {
            crCode = DemoClient.demoChangeRequestCode
        }
        if case .seed(let ring) = request.kind, !ring.currentVersion.isEmpty {
            seedVersion = ring.currentVersion
        }
    }

    private func submit() async {
        guard canSubmit else { return }
        error = nil

        // Biometrics are the last gate before a production deploy, and are
        // asked for fresh every time — a successful unlock earlier in the
        // session must not authorise this.
        if targetsProduction, request.action != .rollback,
           session.requiresBiometricsForProduction {
            let reason = "Confirm deploying \(store.app) to production"
            guard await session.biometrics.authenticate(reason: reason) else {
                error = .productionPasswordRequired(
                    "Biometric confirmation was not completed, so nothing was sent."
                )
                return
            }
        }

        isSubmitting = true
        defer { isSubmitting = false }

        let code = crCode.trimmingCharacters(in: .whitespacesAndNewlines)
        let secret = password.isEmpty ? nil : password

        do {
            let jobID: String
            switch request.kind {
            case .promote(let ring):
                jobID = try await store.promote(
                    from: ring.ring.name, crCode: code.isEmpty ? nil : code, password: secret
                )
            case .seed(let ring):
                jobID = try await store.seed(
                    ring: ring.ring.name, version: versionInFlight,
                    crCode: code.isEmpty ? nil : code, password: secret
                )
            case .rollback(let ring):
                jobID = try await store.rollback(ring: ring.ring.name)
            case .autoPromote(let ring, let enabled):
                try await store.setAutoPromote(
                    ring: ring.ring.name, enabled: enabled, password: secret
                )
                Haptics.success()
                dismiss()
                return
            }
            Haptics.actionStarted()
            dismiss()
            onStarted(jobID)
        } catch {
            self.error = error
            session.note(error)
            Haptics.failure()
        }
    }
}

/// A label/value line in the sheet's summary.
private struct SummaryLine: View {
    let label: String
    let value: String
    var isProduction: Bool = false

    var body: some View {
        HStack {
            Text(label)
                .foregroundStyle(.secondary)
            Spacer()
            HStack(spacing: 5) {
                if isProduction {
                    Image(systemName: "lock.fill")
                        .font(.caption)
                        .foregroundStyle(Color.rpProduction)
                }
                Text(value)
                    .fontWeight(.medium)
                    .multilineTextAlignment(.trailing)
            }
        }
        .accessibilityElement(children: .combine)
    }
}
