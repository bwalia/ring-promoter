import SwiftUI

/// The quick links a ring card shows once a version is deployed: Open (the web
/// UI) first, then the other things a tester opens directly, then "Test".
struct TestLinksRow: View {
    let ring: RingStatus
    let onShowKit: () -> Void

    private var links: [TestLink] {
        (ring.links ?? []).filter { !$0.isFromLogs && $0.destination != nil }
    }

    var body: some View {
        let quick = Array(links.filter(\.isTryable).prefix(2))
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(quick) { link in
                    if let url = link.destination {
                        Link(destination: url) {
                            Label(link.shortLabel, systemImage: link.systemImage)
                            .lineLimit(1)
                        }
                        .buttonStyle(.bordered)
                        .controlSize(.small)
                        .accessibilityHint("\(link.label), \(link.shortURL)")
                    }
                }
                Button(action: onShowKit) {
                    Label("Test", systemImage: "checklist")
                }
                .buttonStyle(.bordered)
                .controlSize(.small)
                .accessibilityIdentifier("test-kit-\(ring.ring.name)")
            }
        }
    }
}

/// "Test this version": every link the ring's current version has, grouped by
/// what a tester does with it, and the AI test plan.
struct TestKitSheet: View {
    let app: String
    let ring: RingStatus

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var kit: TestKit?
    @State private var loadError: String?
    @State private var isRequesting = false
    @State private var done: Set<Int> = []

    var body: some View {
        NavigationStack {
            List {
                if let kit {
                    linkSections(kit)
                    if kit.aiEnabled { planSection(kit) }
                } else if let loadError {
                    Label(loadError, systemImage: "exclamationmark.triangle")
                        .foregroundStyle(Color.rpGate)
                } else {
                    ProgressView()
                }
            }
            .navigationTitle("Test \(ring.currentVersion)")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                }
            }
            .task { await load() }
            .refreshable { await load() }
        }
    }

    @ViewBuilder private func linkSections(_ kit: TestKit) -> some View {
        let usable = kit.links.filter { $0.destination != nil }
        let tryIt = usable.filter { !$0.isFromLogs && $0.isTryable }
        let outputs = usable.filter { !$0.isFromLogs && !$0.isTryable }
        let fromLogs = usable.filter(\.isFromLogs)
        if usable.isEmpty {
            Section {
                Text("No links yet. Add `links` to this app's config, or give the ring a public health URL.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
        }
        if !tryIt.isEmpty {
            Section("Try it in \(ring.ring.label)") { ForEach(tryIt) { LinkRow(link: $0) } }
        }
        if !outputs.isEmpty {
            Section("Build outputs") { ForEach(outputs) { LinkRow(link: $0) } }
        }
        if !fromLogs.isEmpty {
            Section("Found in deploy logs") { ForEach(fromLogs) { LinkRow(link: $0) } }
        }
    }

    @ViewBuilder private func planSection(_ kit: TestKit) -> some View {
        Section("AI test plan") {
            if let plan = kit.plan, kit.planStatus != .running, !isRequesting {
                if !plan.summary.isEmpty {
                    Text(plan.summary)
                        .font(.subheadline)
                        .fixedSize(horizontal: false, vertical: true)
                }
                ForEach(Array(plan.checklist.enumerated()), id: \.offset) { index, item in
                    Button {
                        if done.contains(index) { done.remove(index) } else { done.insert(index) }
                    } label: {
                        Label {
                            Text(item)
                                .strikethrough(done.contains(index))
                                .foregroundStyle(done.contains(index) ? .secondary : .primary)
                        } icon: {
                            Image(systemName: done.contains(index) ? "checkmark.circle.fill" : "circle")
                                .foregroundStyle(done.contains(index) ? Color.rpHealthy : .secondary)
                        }
                    }
                    .buttonStyle(.plain)
                }
                ForEach(plan.links) { LinkRow(link: $0) }
                Button {
                    Task { await requestPlan(refresh: true) }
                } label: {
                    Label("Regenerate", systemImage: "arrow.clockwise")
                }
            } else if kit.planStatus == .running || isRequesting {
                HStack(spacing: 8) {
                    ProgressView()
                    Text("Reading the deploy's links and logs…")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
            } else {
                if kit.planStatus == .failed, let error = kit.planError {
                    Label(error, systemImage: "exclamationmark.triangle")
                        .font(.subheadline)
                        .foregroundStyle(Color.rpGate)
                }
                Button {
                    Task { await requestPlan(refresh: false) }
                } label: {
                    Label(
                        kit.planStatus == .failed ? "Try again" : "Suggest what to test",
                        systemImage: "sparkles"
                    )
                }
            }
        }
    }

    private func load() async {
        do {
            kit = try await session.api.testKit(app: app, ring: ring.ring.name)
            loadError = nil
        } catch {
            loadError = error.userMessage
        }
    }

    /// Starts (or regenerates) the plan, then polls until the server settles.
    private func requestPlan(refresh: Bool) async {
        isRequesting = true
        defer { isRequesting = false }
        done = []
        do {
            var current = try await session.api.planTestKit(
                app: app, ring: ring.ring.name, refresh: refresh
            )
            kit = current
            var attempts = 0
            while current.planStatus == .running, attempts < 120 {
                try? await Task.sleep(for: .seconds(2))
                if Task.isCancelled { return }
                current = try await session.api.testKit(app: app, ring: ring.ring.name)
                kit = current
                attempts += 1
            }
        } catch {
            loadError = error.userMessage
            session.note(error)
        }
    }
}

private struct LinkRow: View {
    let link: TestLink

    var body: some View {
        if let url = link.destination {
            Link(destination: url) {
                HStack(spacing: 12) {
                    Image(systemName: link.systemImage)
                        .frame(width: 22)
                        .foregroundStyle(.secondary)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(link.label)
                            .font(.subheadline.weight(.medium))
                            .foregroundStyle(.primary)
                        Text(link.why ?? link.shortURL)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(2)
                    }
                    Spacer(minLength: 4)
                    Image(systemName: "arrow.up.right")
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                }
            }
            .contextMenu {
                Button {
                    UIPasteboard.general.url = url
                } label: {
                    Label("Copy link", systemImage: "doc.on.doc")
                }
                ShareLink(item: url)
            }
        }
    }
}
