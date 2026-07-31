import SwiftUI

/// Everything about one ring, and the actions it allows.
///
/// This is where the detail the pipeline list omits lives, so the list can stay
/// scannable. Reads the ring back out of the store by name rather than holding
/// a copy, so it updates in place after an auto-promote toggle or a refresh.
struct RingDetailView: View {
    let store: AppDetailStore
    let ringName: String
    /// Called with the action the operator chose; the caller presents it.
    let onAction: (ActionSheetRequest) -> Void

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var autoPromoteError: String?
    @State private var isTogglingAutoPromote = false

    private var ring: RingStatus? { store.ring(named: ringName) }

    var body: some View {
        NavigationStack {
            Group {
                if let ring {
                    content(ring)
                } else {
                    ContentUnavailableView(
                        "Ring not found", systemImage: "questionmark.circle"
                    )
                }
            }
            .navigationTitle(ring?.ring.label ?? ringName)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                }
            }
        }
        .presentationDetents([.medium, .large])
    }

    @ViewBuilder
    private func content(_ ring: RingStatus) -> some View {
        List {
            Section {
                LabeledContent("State") {
                    Label(
                        HealthPresentation(ring).label,
                        systemImage: HealthPresentation(ring).systemImage
                    )
                    .foregroundStyle(HealthPresentation(ring).tint)
                    .labelStyle(.titleAndIcon)
                }
                LabeledContent("Version") {
                    Text(ring.isEmpty ? "not deployed" : ring.currentVersion)
                        .monospaced()
                        .textSelection(.enabled)
                }
                if !ring.previousVersion.isEmpty {
                    LabeledContent("Previous") {
                        Text(ring.previousVersion).monospaced()
                    }
                }
                // Only when it disagrees with what was deployed — otherwise it
                // is a row that always says the same thing as the one above.
                if ring.versionIsDrifting {
                    LabeledContent("Live") {
                        Text(ring.liveVersion).monospaced()
                    }
                }
                LabeledContent("Updated") {
                    RelativeTimestamp(date: ring.updatedAt)
                }
            } footer: {
                if ring.versionIsDrifting {
                    Text("The endpoint is reporting a different version than the one deployed.")
                }
            }

            if let detail = ring.liveHealthError, !detail.isEmpty {
                Section("Health") {
                    Text(detail)
                        .font(.subheadline)
                        .foregroundStyle(Color.rpUnhealthy)
                        .textSelection(.enabled)
                }
            }

            if ring.gates.isGated {
                Section {
                    GateBadges(gates: ring.gates)
                } header: {
                    Text("Gates")
                } footer: {
                    Text("These must be satisfied before anything enters this ring.")
                }
            }

            Section {
                Toggle(isOn: autoPromoteBinding(ring)) {
                    Text("Auto-promote onward")
                }
                .disabled(ring.autoPromoteManaged || isTogglingAutoPromote)
            } footer: {
                if ring.autoPromoteManaged {
                    Text("Managed by config — change it in the server's config file.")
                } else if let autoPromoteError {
                    Text(autoPromoteError).foregroundStyle(Color.rpUnhealthy)
                } else {
                    Text("A healthy version here is carried to the next ring automatically.")
                }
            }

            actions(ring)
        }
    }

    @ViewBuilder
    private func actions(_ ring: RingStatus) -> some View {
        let promote = store.rules.promote(from: ring, rings: store.rings)
        let rollback = store.rules.rollback(ring)
        let seed = store.rules.seed(into: ring)

        Section {
            ActionRow(
                title: promoteTitle(ring),
                systemImage: PromotionAction.promote.systemImage,
                legality: promote,
                identifier: "promote-\(ring.ring.name)"
            ) {
                onAction(ActionSheetRequest(kind: .promote(from: ring)))
            }
            ActionRow(
                title: "Roll back to \(ring.previousVersion.isEmpty ? "previous" : ring.previousVersion)",
                systemImage: PromotionAction.rollback.systemImage,
                legality: rollback,
                tint: .rpUnhealthy,
                identifier: "rollback-\(ring.ring.name)"
            ) {
                onAction(ActionSheetRequest(kind: .rollback(ring: ring)))
            }
            ActionRow(
                title: "Seed a version",
                systemImage: PromotionAction.seed.systemImage,
                legality: seed,
                identifier: "seed-\(ring.ring.name)"
            ) {
                onAction(ActionSheetRequest(kind: .seed(ring: ring)))
            }
        } footer: {
            // Say why, rather than leaving a disabled row unexplained.
            if let blocker = promote.blocker, blocker != .noNextRing {
                Text(blocker.explanation)
            }
        }
    }

    private func promoteTitle(_ ring: RingStatus) -> String {
        guard let next = store.pipeline.next(after: ring.ring.name) else {
            return "Promote"
        }
        return "Promote to \(next.name)"
    }

    private func autoPromoteBinding(_ ring: RingStatus) -> Binding<Bool> {
        Binding(
            get: { ring.autoPromote },
            set: { newValue in
                let legality = store.rules.toggleAutoPromote(ring, enabled: newValue)
                guard legality.isAllowed else {
                    autoPromoteError = legality.blocker?.explanation
                    return
                }
                if legality.requirements.productionPassword {
                    // Enabling the hands-free path into production needs the
                    // password, so it goes through the same confirmed sheet as
                    // any other production action.
                    onAction(
                        ActionSheetRequest(kind: .autoPromote(ring: ring, enabled: newValue))
                    )
                    return
                }
                Task { await toggle(ring, to: newValue) }
            }
        )
    }

    private func toggle(_ ring: RingStatus, to enabled: Bool) async {
        isTogglingAutoPromote = true
        defer { isTogglingAutoPromote = false }
        autoPromoteError = nil
        do {
            try await store.setAutoPromote(ring: ring.ring.name, enabled: enabled, password: nil)
        } catch {
            autoPromoteError = error.userMessage
            session.note(error)
        }
    }
}

/// A full-width action row, enabled only when the action is legal.
private struct ActionRow: View {
    let title: String
    let systemImage: String
    let legality: PromotionLegality
    var tint: Color = .accentColor
    let identifier: String
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Label(title, systemImage: systemImage)
                .foregroundStyle(legality.isAllowed ? tint : Color.secondary)
        }
        .disabled(!legality.isAllowed)
        .accessibilityIdentifier(identifier)
        .accessibilityHint(legality.isAllowed ? "" : (legality.blocker?.explanation ?? ""))
    }
}
