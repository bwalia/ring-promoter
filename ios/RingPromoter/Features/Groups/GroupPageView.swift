import SwiftUI

/// A group's own page: the deployment ring, then its members as a list.
///
/// Mirrors the web's group view — the ring answers "is my group healthy?" at a
/// glance, and the list underneath is how you actually get to an application.
struct GroupPageView: View {
    let groupID: String

    @Environment(AppSession.self) private var session
    @State private var store: OverviewStore?
    @State private var group: AppGroup?

    var body: some View {
        Group {
            if let store, let group {
                content(store: store, group: group)
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .navigationTitle(group?.name ?? "Group")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            if store == nil { store = OverviewStore(session: session) }
            await store?.refresh()
            group = store?.groups.first { $0.id == groupID }
        }
        // Keep the ring live: the orbit should show a deploy starting without
        // the operator pulling to refresh.
        .task(id: groupID) {
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(5))
                guard !Task.isCancelled else { return }
                await store?.refresh()
                group = store?.groups.first { $0.id == groupID }
            }
        }
    }

    @ViewBuilder
    private func content(store: OverviewStore, group: AppGroup) -> some View {
        let members = store.orderedSummaries.filter { group.contains($0.name) }

        List {
            Section {
                GroupRingView(group: group, members: members)
                    .listRowInsets(EdgeInsets())
                    .listRowBackground(Color.clear)
            }

            Section("Applications") {
                if members.isEmpty {
                    Text("This group has no applications yet.")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
                ForEach(members) { summary in
                    NavigationLink(value: Route.app(summary.name)) {
                        VStack(alignment: .leading, spacing: 6) {
                            HStack(spacing: 6) {
                                Circle()
                                    .fill(NodeStatus.of(summary).color)
                                    .frame(width: 8, height: 8)
                                Text(summary.title)
                                    .font(.subheadline.weight(.medium))
                                    .lineLimit(1)
                                Spacer(minLength: 0)
                                Text(RingsSummary(summary.rings).ringsLabel)
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                            PipelineStrip(
                                rings: summary.rings, pipeline: session.pipeline,
                                busyRing: summary.busyRing, isCompact: true
                            )
                        }
                        .padding(.vertical, 2)
                    }
                }
            }
        }
        .refreshable { await store.refresh() }
    }
}
