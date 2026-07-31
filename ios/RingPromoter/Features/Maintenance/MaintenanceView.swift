import SwiftUI

/// One application's maintenance windows: what is scheduled, what is open, and
/// what an operator has opened by hand.
struct MaintenanceView: View {
    let app: String

    @Environment(AppSession.self) private var session
    @State private var status: MaintenanceStatus = .ungated
    @State private var isLoading = false
    @State private var error: APIError?
    @State private var pendingClose: MaintenanceWindow?
    @State private var showingOpenSheet = false
    @State private var store: AppDetailStore?

    var body: some View {
        List {
            if let error, !status.gated {
                Section { ErrorRow(error: error) { Task { await load() } } }
            }

            if status.gated {
                Section {
                    ForEach(status.gatedRings, id: \.self) { ring in
                        HStack {
                            Text(ring)
                                .font(.subheadline.weight(.medium))
                            Spacer()
                            StatusBadge(
                                text: status.isOpen(for: ring) ? "Open now" : "Shut",
                                systemImage: status.isOpen(for: ring)
                                    ? "lock.open.fill" : "lock.fill",
                                tint: status.isOpen(for: ring) ? .rpHealthy : .rpGate
                            )
                        }
                    }
                } header: {
                    Text("Guarded rings").accessibilityIdentifier("guarded-rings-header")
                } footer: {
                    Text(
                        "Nothing may be promoted into these rings unless a window is open — "
                            + "either a scheduled one or one opened here."
                    )
                }

                if !status.recurring.isEmpty {
                    Section {
                        ForEach(status.recurring) { window in
                            Label(window.summary, systemImage: "calendar")
                                .font(.subheadline)
                        }
                    } header: {
                        Text("Scheduled")
                    } footer: {
                        Text("Defined in the server's config file and read-only from here.")
                    }
                }

                Section {
                    if status.windows.isEmpty {
                        Text("None open.")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                    }
                    ForEach(status.windows) { window in
                        WindowRow(window: window) { pendingClose = window }
                    }
                    Button {
                        showingOpenSheet = true
                    } label: {
                        Label("Open a window", systemImage: "plus.circle")
                    }
                } header: {
                    Text("Ad-hoc windows")
                }
            } else if !isLoading {
                Section {
                    CalmEmptyState(
                        title: "No maintenance gate",
                        message: "This application can be promoted at any time.",
                        systemImage: "clock.badge.checkmark",
                        tint: .rpHealthy
                    )
                    .listRowBackground(Color.clear)
                }
            }
        }
        .navigationTitle("Maintenance")
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await load() }
        .task {
            if store == nil { store = AppDetailStore(app: app, session: session) }
            await load()
        }
        .overlay { if isLoading, !status.gated { ProgressView() } }
        .sheet(isPresented: $showingOpenSheet) {
            if let store {
                OpenWindowView(store: store, ring: status.gatedRings.first ?? "")
                    .onDisappear { Task { await load() } }
            }
        }
        .confirmationDialog(
            "Close this window?",
            isPresented: Binding(
                get: { pendingClose != nil },
                set: { if !$0 { pendingClose = nil } }
            ),
            titleVisibility: .visible,
            presenting: pendingClose
        ) { window in
            Button("Close window", role: .destructive) {
                Task { await close(window) }
            }
        } message: { window in
            Text(
                "Promotions into \(window.ringLabel) will be blocked again until another "
                    + "window opens."
            )
        }
    }

    private func load() async {
        isLoading = true
        defer { isLoading = false }
        do {
            status = try await session.api.maintenance(app: app)
            error = nil
        } catch {
            self.error = error
            session.note(error)
        }
    }

    private func close(_ window: MaintenanceWindow) async {
        do {
            try await session.api.closeMaintenanceWindow(app: app, id: window.id)
            Haptics.success()
            await load()
        } catch {
            self.error = error
            session.note(error)
        }
    }
}

/// One ad-hoc window, with its live open/closed state.
private struct WindowRow: View {
    let window: MaintenanceWindow
    let onClose: () -> Void

    private var isActive: Bool { window.isActive(at: .now) }

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 6) {
                Text(window.ringLabel)
                    .font(.subheadline.weight(.medium))
                Spacer()
                StatusBadge(
                    text: isActive ? "Open" : (window.hasExpired ? "Expired" : "Scheduled"),
                    systemImage: isActive ? "lock.open.fill" : "clock",
                    tint: isActive ? .rpHealthy : .rpNeutral
                )
            }
            Text(
                "\(window.startsAt.formatted(date: .abbreviated, time: .shortened)) – "
                    + "\(window.endsAt.formatted(date: .omitted, time: .shortened))"
            )
            .font(.caption)
            .foregroundStyle(.secondary)
            if !window.reason.isEmpty {
                Text(window.reason).font(.caption)
            }
            if !window.createdBy.isEmpty {
                Text("Opened by \(window.createdBy)")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .swipeActions {
            Button("Close", role: .destructive, action: onClose)
        }
        .accessibilityElement(children: .combine)
    }
}
