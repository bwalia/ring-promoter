import SwiftUI

/// One application's full pipeline, with the actions it allows.
struct AppDetailView: View {
    let app: String

    @Environment(AppSession.self) private var session
    @Environment(Router.self) private var router
    @State private var store: AppDetailStore?
    @State private var sheet: ActionSheetRequest?

    var body: some View {
        Group {
            if let store {
                content(store: store)
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .navigationTitle(store?.title ?? app)
        .navigationBarTitleDisplayMode(.large)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    NavigationLink(value: Route.history(app: app)) {
                        Label("History", systemImage: "clock.arrow.circlepath")
                    }
                    NavigationLink(value: Route.maintenance(app: app)) {
                        Label("Maintenance windows", systemImage: "wrench.and.screwdriver")
                    }
                } label: {
                    Label("More", systemImage: "ellipsis.circle")
                }
            }
        }
        .task {
            if store == nil { store = AppDetailStore(app: app, session: session) }
            await store?.refresh()
        }
        .sheet(item: $sheet) { request in
            if let store {
                ActionSheetView(store: store, request: request) { jobID in
                    router.showJob(app: app, id: jobID)
                }
            }
        }
    }

    @ViewBuilder
    private func content(store: AppDetailStore) -> some View {
        List {
            if let error = store.error, store.rings.isEmpty {
                Section { ErrorRow(error: error) { Task { await store.refresh() } } }
            }

            if !store.unhealthyRings.isEmpty {
                Section {
                    ForEach(store.unhealthyRings) { ring in
                        UnhealthyRingCallout(ring: ring)
                    }
                }
            }

            Section {
                ForEach(store.rings) { ring in
                    RingCard(
                        ring: ring,
                        store: store,
                        onAction: { sheet = $0 }
                    )
                }
            } header: {
                Text("Pipeline")
            } footer: {
                if let updated = store.lastUpdated {
                    RelativeTimestamp(date: updated, prefix: "Updated")
                }
            }
        }
        .refreshable { await store.refresh() }
    }
}

/// Why a ring needs attention, stated once at the top rather than buried in a
/// card an operator has to scroll to.
private struct UnhealthyRingCallout: View {
    let ring: RingStatus

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Label(
                "\(ring.ring.label) is unhealthy",
                systemImage: "exclamationmark.triangle.fill"
            )
            .font(.subheadline.weight(.semibold))
            .foregroundStyle(Color.rpUnhealthy)
            if let detail = ring.liveHealthError {
                Text(detail)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if ring.canRollBack {
                Text("A rollback to \(ring.previousVersion) is available below.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 2)
        .accessibilityElement(children: .combine)
    }
}

/// One ring: state, drift, gates, auto-promote, and its legal actions.
private struct RingCard: View {
    let ring: RingStatus
    let store: AppDetailStore
    let onAction: (ActionSheetRequest) -> Void

    @Environment(AppSession.self) private var session
    @State private var autoPromoteError: String?
    @State private var isTogglingAutoPromote = false

    private var presentation: HealthPresentation { HealthPresentation(ring) }
    private var isProduction: Bool { store.pipeline.isProduction(ring.ring.name) }

    private var promoteLegality: PromotionLegality {
        store.rules.promote(from: ring, rings: store.rings)
    }
    private var rollbackLegality: PromotionLegality { store.rules.rollback(ring) }
    private var seedLegality: PromotionLegality { store.rules.seed(into: ring) }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            header
            if ring.configured {
                versions
                if ring.gates.isGated {
                    GateBadges(gates: ring.gates)
                }
                autoPromoteRow
                actions
            } else {
                Text("Not part of this application's pipeline.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 6)
        .opacity(ring.configured ? 1 : 0.55)
    }

    private var header: some View {
        HStack(spacing: 8) {
            Image(systemName: presentation.systemImage)
                .foregroundStyle(presentation.tint)
            Text(ring.ring.label)
                .font(.headline)
            if isProduction {
                // A glyph, not a badge: the ring's own label already says
                // "Production", and a second word here only competes with the
                // health state for room.
                Image(systemName: "lock.fill")
                    .font(.caption)
                    .foregroundStyle(Color.rpProduction)
                    .accessibilityLabel("Production ring")
            }
            Spacer(minLength: 4)
            if ring.configured {
                Text(presentation.label)
                    .font(.caption.weight(.medium))
                    .foregroundStyle(presentation.tint)
                    .lineLimit(1)
                    .fixedSize()
            }
        }
        .accessibilityElement(children: .combine)
        // On the header only: an identifier set on the whole card would be
        // inherited by every control inside it, masking their own.
        .accessibilityIdentifier("ring-\(ring.ring.name)")
    }

    @ViewBuilder private var versions: some View {
        VStack(alignment: .leading, spacing: 4) {
            LabeledRow(
                label: "Current", value: ring.isEmpty ? "not deployed" : ring.currentVersion,
                isMuted: ring.isEmpty
            )
            if !ring.previousVersion.isEmpty {
                LabeledRow(
                    label: "Previous", value: ring.previousVersion, isMuted: true,
                    // Worth saying out loud: the rollback button is enabled, but
                    // it would put back the version already running.
                    warning: ring.previousVersion == ring.currentVersion
                        ? "Same as current — a rollback would change nothing." : nil
                )
            }
            // Only worth the row when it disagrees with what was deployed —
            // otherwise it is noise.
            if ring.versionIsDrifting {
                LabeledRow(
                    label: "Live", value: ring.liveVersion, isMuted: false,
                    warning: "The endpoint reports a different version."
                )
            }
            if let healthError = ring.liveHealthError, !healthError.isEmpty {
                Label(healthError, systemImage: "stethoscope")
                    .font(.caption)
                    .foregroundStyle(Color.rpUnhealthy)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if ring.healthIsDrifting {
                Label(
                    ring.isHealthy
                        ? "Recovered since the last promotion."
                        : "Was healthy when last promoted; failing now.",
                    systemImage: "arrow.triangle.branch"
                )
                .font(.caption)
                .foregroundStyle(.secondary)
            }
            RelativeTimestamp(date: ring.updatedAt, prefix: "Updated")
        }
    }

    private var autoPromoteRow: some View {
        VStack(alignment: .leading, spacing: 4) {
            Toggle(isOn: autoPromoteBinding) {
                Text("Auto-promote onward")
                    .font(.subheadline)
            }
            .disabled(ring.autoPromoteManaged || isTogglingAutoPromote)
            if ring.autoPromoteManaged {
                Label(
                    "Managed by config — change it in the server's config file.",
                    systemImage: "doc.badge.gearshape"
                )
                .font(.caption)
                .foregroundStyle(.secondary)
            }
            if let autoPromoteError {
                Text(autoPromoteError)
                    .font(.caption)
                    .foregroundStyle(Color.rpUnhealthy)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    private var autoPromoteBinding: Binding<Bool> {
        Binding(
            get: { ring.autoPromote },
            set: { newValue in
                let legality = store.rules.toggleAutoPromote(ring, enabled: newValue)
                if !legality.isAllowed {
                    autoPromoteError = legality.blocker?.explanation
                    return
                }
                if legality.requirements.productionPassword {
                    // Turning on the hands-free path into production needs the
                    // password, so it goes through the same confirmed sheet as
                    // any other production action.
                    onAction(
                        ActionSheetRequest(kind: .autoPromote(ring: ring, enabled: newValue))
                    )
                    return
                }
                Task { await toggle(newValue) }
            }
        )
    }

    private func toggle(_ enabled: Bool) async {
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

    @ViewBuilder private var actions: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Promote gets its own full-width row: it is the primary action, and
            // three capsules across a phone truncates every label to "Promo…".
            ActionButton(
                title: promoteTitle,
                systemImage: PromotionAction.promote.systemImage,
                legality: promoteLegality,
                identifier: "promote-\(ring.ring.name)",
                isProminent: true,
                fillsWidth: true
            ) {
                onAction(ActionSheetRequest(kind: .promote(from: ring)))
            }
            HStack(spacing: 8) {
                ActionButton(
                    title: "Roll back",
                    systemImage: PromotionAction.rollback.systemImage,
                    legality: rollbackLegality,
                    identifier: "rollback-\(ring.ring.name)",
                    tint: .rpUnhealthy,
                    fillsWidth: true
                ) {
                    onAction(ActionSheetRequest(kind: .rollback(ring: ring)))
                }
                ActionButton(
                    title: "Seed",
                    systemImage: PromotionAction.seed.systemImage,
                    legality: seedLegality,
                    identifier: "seed-\(ring.ring.name)",
                    fillsWidth: true
                ) {
                    onAction(ActionSheetRequest(kind: .seed(ring: ring)))
                }
            }
            // Say why, rather than leaving a disabled button unexplained.
            if let blocker = promoteLegality.blocker, ring.configured,
               blocker != .noNextRing {
                Text(blocker.explanation)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    private var promoteTitle: String {
        guard let target = store.pipeline.next(after: ring.ring.name) else { return "Promote" }
        return "Promote to \(target.name)"
    }
}

/// A single labelled value line.
private struct LabeledRow: View {
    let label: String
    let value: String
    var isMuted: Bool = false
    var warning: String?

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Text(label)
                .font(.caption)
                .foregroundStyle(.secondary)
                .frame(width: 62, alignment: .leading)
            VStack(alignment: .leading, spacing: 1) {
                Text(value)
                    .font(.subheadline.weight(.medium))
                    .foregroundStyle(isMuted ? .secondary : .primary)
                    .textSelection(.enabled)
                if let warning {
                    Text(warning)
                        .font(.caption2)
                        .foregroundStyle(Color.rpGate)
                }
            }
            Spacer(minLength: 0)
        }
        .accessibilityElement(children: .combine)
    }
}

/// A button that is only tappable when the action is legal, and that explains
/// itself to VoiceOver when it is not.
private struct ActionButton: View {
    let title: String
    let systemImage: String
    let legality: PromotionLegality
    let identifier: String
    var isProminent: Bool = false
    var tint: Color?
    /// Fill the available width rather than hugging the label, so a long title
    /// like "Promote to test" is never truncated.
    var fillsWidth: Bool = false
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Label(title, systemImage: systemImage)
                .font(.caption.weight(.semibold))
                .lineLimit(1)
                .frame(maxWidth: fillsWidth ? .infinity : nil)
                .padding(.vertical, 2)
        }
        .buttonStyle(.bordered)
        .buttonBorderShape(.capsule)
        .controlSize(.small)
        .tint(tint ?? (isProminent ? .accentColor : .secondary))
        .disabled(!legality.isAllowed)
        .accessibilityIdentifier(identifier)
        .accessibilityLabel(title)
        .accessibilityHint(legality.isAllowed ? "" : (legality.blocker?.explanation ?? ""))
    }
}

#Preview("App detail") {
    PreviewHost {
        NavigationStack { AppDetailView(app: "payments-api") }
    }
}
