import SwiftUI

/// Every saved control plane in one place: how each one is doing right now,
/// one tap to open its rings, and add / edit / remove without leaving the list.
///
/// Each instance is probed with its own token on a short-timeout session, one
/// after another (see `OverviewStore` for why there is no task group). The
/// probe is `GET /api/apps` plus `GET /api/jobs` — two cheap calls, never the
/// per-app ring health checks the Overview runs.
struct ControlPlanesView: View {
    @Environment(AppSession.self) private var session
    @Environment(Router.self) private var router
    @Environment(\.dismiss) private var dismiss

    /// Presented as a sheet (from the instance switcher) rather than pushed
    /// from Settings, so it needs its own navigation stack and a Done button.
    var isSheet = false

    @State private var statuses: [UUID: ProbeStatus] = [:]
    @State private var isAdding = false
    @State private var editing: Instance?
    @State private var pendingRemoval: Instance?

    var body: some View {
        if isSheet {
            NavigationStack { list }
        } else {
            list
        }
    }

    private var list: some View {
        List {
            if session.instances.isEmpty {
                ContentUnavailableView {
                    Label("No control planes yet", systemImage: "server.rack")
                } description: {
                    Text("Add the URL and API token of each Ring Promoter you look after.")
                } actions: {
                    Button("Add a control plane") { isAdding = true }
                        .buttonStyle(.borderedProminent)
                }
            } else {
                Section {
                    ForEach(session.instances.instances) { instance in
                        Button {
                            open(instance)
                        } label: {
                            ControlPlaneRow(
                                instance: instance,
                                isActive: instance.id == session.activeInstance?.id,
                                status: statuses[instance.id] ?? .checking
                            )
                        }
                        .buttonStyle(.plain)
                        .swipeActions {
                            Button("Remove", role: .destructive) { pendingRemoval = instance }
                            Button("Edit") { editing = instance }
                                .tint(.accentColor)
                        }
                        .contextMenu {
                            Button {
                                open(instance)
                            } label: {
                                Label("Open rings", systemImage: "circle.circle")
                            }
                            Button {
                                editing = instance
                            } label: {
                                Label("Edit name, colour or token", systemImage: "pencil")
                            }
                            Button(role: .destructive) {
                                pendingRemoval = instance
                            } label: {
                                Label("Remove", systemImage: "trash")
                            }
                        }
                    }
                } footer: {
                    Text(
                        "Tap a control plane to open its applications and rings. Swipe to "
                            + "edit or remove. Each one keeps its own token in the Keychain."
                    )
                }
            }
        }
        .navigationTitle("Control planes")
        .toolbar {
            if isSheet {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Done") { dismiss() }
                }
            }
            ToolbarItem(placement: .primaryAction) {
                Button {
                    isAdding = true
                } label: {
                    Label("Add a control plane", systemImage: "plus")
                }
            }
        }
        .refreshable { await probeAll() }
        // Re-probe whenever the set of instances changes (one added, edited or
        // removed), not just on first appearance.
        .task(id: session.instances.instances) { await probeAll() }
        .sheet(isPresented: $isAdding) {
            // An operator adding several control planes should not be switched
            // to each one as it is saved.
            AddInstanceView(switchesOnSave: session.instances.isEmpty)
        }
        .sheet(item: $editing) { AddInstanceView(editing: $0, switchesOnSave: false) }
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
                let wasActive = session.activeInstance?.id == instance.id
                session.instances.remove(instance)
                if wasActive { session.disconnect() }
            }
        } message: { _ in
            Text("Its API token is deleted from the Keychain too.")
        }
    }

    /// Switch to the instance (if needed) and land on its Overview.
    private func open(_ instance: Instance) {
        if instance.id == session.activeInstance?.id {
            router.resetForNewConnection()
            dismiss()
            return
        }
        if session.activate(instance) {
            // The tab tree is rebuilt for the new instance and RootView resets
            // navigation; dismissing tidies up when shown as a sheet.
            dismiss()
        } else {
            editing = instance
        }
    }

    private func probeAll() async {
        for instance in session.instances.instances {
            if Task.isCancelled { return }
            statuses[instance.id] = await ProbeStatus.probe(instance)
        }
    }
}

/// What a quick look at one control plane found.
enum ProbeStatus: Equatable, Sendable {
    case checking
    case noToken
    case reachable(apps: Int, rings: Int, failing: Int, running: Int)
    case tokenRejected
    case unreachable(String)

    /// Short-timeout client per instance, so one dead host does not hold up
    /// the rest of the list for a full minute.
    static func probe(_ instance: Instance) async -> ProbeStatus {
        guard let token = (try? Keychain.token(for: instance.keychainAccount)) ?? nil,
              !token.isEmpty
        else { return .noToken }
        let client = RingPromoterClient(
            baseURL: instance.baseURL, token: token,
            session: RingPromoterClient.makeSession(timeout: 8)
        )
        let apps: AppsResponse
        do {
            apps = try await client.apps()
        } catch {
            if case .unauthorized = error { return .tokenRejected }
            return .unreachable(error.userMessage)
        }
        let jobs = (try? await client.recentJobs()) ?? []
        // The newest job per app says whether its last deploy held.
        let latest = Dictionary(jobs.map { ($0.app, $0) }, uniquingKeysWith: { first, _ in first })
        let failing = latest.values.filter {
            $0.outcome == .failed || $0.outcome == .failedAndRolledBack
        }.count
        let running = latest.values.filter { !$0.isFinished }.count
        return .reachable(
            apps: apps.apps.count, rings: apps.rings.count, failing: failing, running: running
        )
    }
}

/// One control plane with its live status line.
private struct ControlPlaneRow: View {
    let instance: Instance
    let isActive: Bool
    let status: ProbeStatus

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            InstanceRow(instance: instance, isActive: isActive)
            statusLine
                .font(.caption)
                .padding(.leading, 24)
        }
        .padding(.vertical, 2)
        .contentShape(.rect)
        .accessibilityElement(children: .combine)
        .accessibilityHint(isActive ? "Open its applications" : "Switch to this control plane")
    }

    @ViewBuilder
    private var statusLine: some View {
        switch status {
        case .checking:
            HStack(spacing: 6) {
                ProgressView().controlSize(.mini)
                Text("Checking…").foregroundStyle(.secondary)
            }
        case .noToken:
            Label("No token saved — tap to add one", systemImage: "key")
                .foregroundStyle(Color.rpGate)
        case .tokenRejected:
            Label("Token rejected — swipe to edit", systemImage: "key.slash")
                .foregroundStyle(Color.rpUnhealthy)
        case .unreachable(let message):
            Label(message, systemImage: "wifi.exclamationmark")
                .foregroundStyle(Color.rpUnhealthy)
                .lineLimit(2)
        case let .reachable(apps, rings, failing, running):
            HStack(spacing: 10) {
                Label(
                    "\(apps) app\(apps == 1 ? "" : "s") · \(rings) rings",
                    systemImage: "checkmark.circle.fill"
                )
                .foregroundStyle(Color.rpHealthy)
                if failing > 0 {
                    Label("\(failing) failed", systemImage: "exclamationmark.triangle.fill")
                        .foregroundStyle(Color.rpUnhealthy)
                }
                if running > 0 {
                    Label("\(running) deploying", systemImage: "arrow.triangle.2.circlepath")
                        .foregroundStyle(Color.rpInFlight)
                }
            }
        }
    }
}

#Preview("Control planes") {
    PreviewHost { NavigationStack { ControlPlanesView() } }
}
