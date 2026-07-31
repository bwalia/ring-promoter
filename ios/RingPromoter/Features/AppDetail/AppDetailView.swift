import SwiftUI

/// One application's pipeline.
///
/// Deliberately spare: four rows, one per ring, each showing only what an
/// operator scans for — state, name, version. Everything else (previous
/// version, drift, health detail, gates, auto-promote, and the actions) lives
/// one tap away in the ring's own screen, because repeating seven rows of
/// detail four times buries the one line that matters.
struct AppDetailView: View {
    let app: String

    @Environment(AppSession.self) private var session
    @Environment(Router.self) private var router
    @State private var store: AppDetailStore?
    @State private var selectedRing: RingStatus?
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
        .sheet(item: $selectedRing) { ring in
            if let store {
                RingDetailView(store: store, ringName: ring.ring.name) { request in
                    selectedRing = nil
                    // Let the ring sheet finish dismissing before the action
                    // sheet arrives, or the second presentation is dropped.
                    Task { @MainActor in
                        try? await Task.sleep(for: .milliseconds(350))
                        sheet = request
                    }
                }
            }
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

            // One line, only when something is actually wrong.
            if let ring = store.unhealthyRings.first {
                Section {
                    Label {
                        Text(ring.liveHealthError ?? "\(ring.ring.label) is unhealthy.")
                            .font(.subheadline)
                    } icon: {
                        Image(systemName: "exclamationmark.triangle.fill")
                    }
                    .foregroundStyle(Color.rpUnhealthy)
                }
            }

            Section {
                ForEach(store.rings) { ring in
                    Button {
                        selectedRing = ring
                    } label: {
                        RingRow(
                            ring: ring,
                            isProduction: store.pipeline.isProduction(ring.ring.name)
                        )
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("ring-\(ring.ring.name)")
                }
            } footer: {
                if let updated = store.lastUpdated {
                    RelativeTimestamp(date: updated, prefix: "Updated")
                }
            }
        }
        .refreshable { await store.refresh() }
    }
}

/// One ring, reduced to what is scannable: state, name, version.
private struct RingRow: View {
    let ring: RingStatus
    let isProduction: Bool

    private var presentation: HealthPresentation { HealthPresentation(ring) }

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: presentation.systemImage)
                .font(.body)
                .foregroundStyle(presentation.tint)
                .frame(width: 22)

            Text(ring.ring.label)
                .font(.body)
                .foregroundStyle(ring.configured ? .primary : .secondary)

            if isProduction {
                Image(systemName: "lock.fill")
                    .font(.caption2)
                    .foregroundStyle(Color.rpProduction)
                    .accessibilityLabel("Production ring")
            }
            // A single dot stands in for "this ring is gated"; which gates, and
            // whether they are satisfied, is on the ring's own screen.
            if ring.gates.isGated {
                Image(systemName: "circle.fill")
                    .font(.system(size: 5))
                    .foregroundStyle(Color.rpGate)
                    .accessibilityLabel("Gated")
            }

            Spacer(minLength: 8)

            Text(versionText)
                .font(.subheadline.monospaced())
                .foregroundStyle(ring.isEmpty || !ring.configured ? .secondary : .primary)
                .lineLimit(1)
                .truncationMode(.middle)

            Image(systemName: "chevron.right")
                .font(.caption2.weight(.semibold))
                .foregroundStyle(.tertiary)
        }
        .padding(.vertical, 3)
        .contentShape(.rect)
        .accessibilityElement(children: .combine)
        .accessibilityLabel(accessibilityLabel)
        .accessibilityHint("Opens \(ring.ring.label)")
    }

    private var versionText: String {
        ring.configured && !ring.isEmpty ? ring.currentVersion : "—"
    }

    private var accessibilityLabel: String {
        var parts = [ring.ring.label]
        if !ring.configured {
            parts.append("not in this pipeline")
            return parts.joined(separator: ", ")
        }
        parts.append(ring.isEmpty ? "nothing deployed" : ring.currentVersion)
        parts.append(presentation.label)
        if isProduction { parts.append("production") }
        if ring.gates.isGated { parts.append("gated") }
        return parts.joined(separator: ", ")
    }
}

#Preview("App detail") {
    PreviewHost {
        NavigationStack { AppDetailView(app: "payments-api") }
    }
}
