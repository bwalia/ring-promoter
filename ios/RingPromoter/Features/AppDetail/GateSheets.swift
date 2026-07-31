import SwiftUI

/// Record a QA / release Go-No-Go for one exact version, inline, without
/// leaving the promote sheet.
struct RecordSignoffView: View {
    let store: AppDetailStore
    let ring: String
    let version: String

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss

    @State private var decision: Signoff.Decision = .go
    @State private var engineer = ""
    @State private var qaStatus = "passed"
    @State private var note = ""
    @State private var isSaving = false
    @State private var error: APIError?

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    LabeledContent("Ring", value: ring)
                    LabeledContent("Version", value: version)
                } footer: {
                    Text(
                        "This decision authorises \(version) and nothing else. A later version "
                            + "needs its own sign-off."
                    )
                }

                Section("Decision") {
                    Picker("Decision", selection: $decision) {
                        ForEach(Signoff.Decision.allCases, id: \.self) { option in
                            Text(option.title).tag(option)
                        }
                    }
                    .pickerStyle(.segmented)
                }

                Section("Release engineer") {
                    TextField("Your name", text: $engineer)
                        .textContentType(.name)
                        .autocorrectionDisabled()
                }

                Section("QA result") {
                    TextField("e.g. passed, passed-with-waivers", text: $qaStatus)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                    TextField("Note (optional)", text: $note, axis: .vertical)
                        .lineLimit(2...5)
                }

                if let error {
                    Section {
                        Label(error.userMessage, systemImage: "exclamationmark.triangle.fill")
                            .font(.subheadline)
                            .foregroundStyle(Color.rpUnhealthy)
                    }
                }
            }
            .navigationTitle("QA sign-off")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    if isSaving {
                        ProgressView()
                    } else {
                        Button("Record") { Task { await save() } }
                            .disabled(engineer.trimmingCharacters(in: .whitespaces).isEmpty)
                    }
                }
            }
            .onAppear { engineer = session.settings.signoffEngineer }
        }
    }

    private func save() async {
        isSaving = true
        defer { isSaving = false }
        error = nil
        let name = engineer.trimmingCharacters(in: .whitespacesAndNewlines)
        do {
            try await store.recordSignoff(
                ring: ring, version: version, decision: decision, engineer: name,
                qaStatus: qaStatus.trimmingCharacters(in: .whitespaces), note: note
            )
            // Remembered so the next sign-off does not need retyping. A name,
            // not a credential.
            session.settings.signoffEngineer = name
            Haptics.success()
            dismiss()
        } catch {
            self.error = error
            session.note(error)
        }
    }
}

/// Open an ad-hoc maintenance window so a gated promotion can proceed.
struct OpenWindowView: View {
    let store: AppDetailStore
    let ring: String

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss

    @State private var duration: TimeInterval = 3_600
    @State private var reason = ""
    @State private var createdBy = ""
    @State private var isSaving = false
    @State private var error: APIError?

    private static let durations: [TimeInterval] = [1_800, 3_600, 7_200, 14_400]

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    LabeledContent("Ring", value: ring)
                    Picker("Length", selection: $duration) {
                        ForEach(Self.durations, id: \.self) { seconds in
                            Text(label(for: seconds)).tag(seconds)
                        }
                    }
                    LabeledContent("Closes") {
                        Text(Date().addingTimeInterval(duration), style: .time)
                    }
                } header: {
                    Text("Window")
                } footer: {
                    Text(
                        "The window opens now and closes on its own. You can also close it "
                            + "early from the Maintenance screen."
                    )
                }

                Section("Why") {
                    TextField("Reason", text: $reason, axis: .vertical)
                        .lineLimit(2...4)
                    TextField("Opened by", text: $createdBy)
                        .textContentType(.name)
                        .autocorrectionDisabled()
                }

                if let error {
                    Section {
                        Label(error.userMessage, systemImage: "exclamationmark.triangle.fill")
                            .font(.subheadline)
                            .foregroundStyle(Color.rpUnhealthy)
                    }
                }
            }
            .navigationTitle("Open a window")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    if isSaving {
                        ProgressView()
                    } else {
                        Button("Open") { Task { await save() } }
                            .disabled(reason.trimmingCharacters(in: .whitespaces).isEmpty)
                    }
                }
            }
            .onAppear { createdBy = session.settings.signoffEngineer }
        }
    }

    private func label(for seconds: TimeInterval) -> String {
        seconds < 3_600
            ? "\(Int(seconds / 60)) minutes"
            : "\(Int(seconds / 3_600)) hour\(seconds == 3_600 ? "" : "s")"
    }

    private func save() async {
        isSaving = true
        defer { isSaving = false }
        error = nil
        // A minute of slack on the start avoids a clock-skew race where the
        // server considers the window not yet open.
        let start = Date().addingTimeInterval(-60)
        do {
            try await store.openMaintenanceWindow(
                ring: ring, from: start, to: start.addingTimeInterval(duration + 60),
                reason: reason.trimmingCharacters(in: .whitespacesAndNewlines),
                createdBy: createdBy.trimmingCharacters(in: .whitespacesAndNewlines)
            )
            Haptics.success()
            dismiss()
        } catch {
            self.error = error
            session.note(error)
        }
    }
}
