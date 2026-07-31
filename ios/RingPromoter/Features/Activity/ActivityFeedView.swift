import SwiftUI

/// What everyone is doing across the whole control plane.
///
/// `GET /api/jobs` returns the newest job per application, which is what makes
/// this feed genuinely shared: a promotion someone started on the web UI shows
/// up here too.
struct ActivityFeedView: View {
    @Environment(AppSession.self) private var session
    @Environment(Router.self) private var router

    @State private var jobs: [Job] = []
    @State private var isLoading = false
    @State private var error: APIError?
    @State private var lastUpdated: Date?

    var body: some View {
        NavigationStack {
            List {
                if let error, jobs.isEmpty {
                    Section { ErrorRow(error: error) { Task { await load() } } }
                }

                if !running.isEmpty {
                    Section {
                        ForEach(running) { job in
                            JobRow(job: job)
                        }
                    } header: {
                        Label("Running now", systemImage: "arrow.triangle.2.circlepath")
                            .foregroundStyle(Color.rpInFlight)
                    }
                }

                if !finished.isEmpty {
                    Section("Recent") {
                        ForEach(finished) { job in
                            JobRow(job: job)
                        }
                    }
                }

                if jobs.isEmpty, !isLoading, error == nil {
                    Section {
                        CalmEmptyState(
                            title: "Nothing has run yet",
                            message: "Seeds, promotions and rollbacks show up here — yours and "
                                + "everyone else's.",
                            systemImage: "clock.arrow.circlepath",
                            tint: .rpNeutral
                        )
                        .listRowBackground(Color.clear)
                    }
                }

                if let lastUpdated {
                    Section {
                        RelativeTimestamp(date: lastUpdated, prefix: "Updated")
                            .frame(maxWidth: .infinity, alignment: .center)
                            .listRowBackground(Color.clear)
                    }
                }
            }
            .navigationTitle("Activity")
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    InstanceBanner(
                        name: session.instanceName, tint: session.instanceTint,
                        isDemo: session.isDemo
                    )
                }
            }
            .refreshable { await load() }
            .task { await load() }
        }
    }

    private var running: [Job] {
        jobs.filter { !$0.isFinished }.sorted { $0.startedAt > $1.startedAt }
    }

    private var finished: [Job] {
        jobs.filter(\.isFinished).sorted { $0.startedAt > $1.startedAt }
    }

    private func load() async {
        isLoading = true
        defer { isLoading = false }
        do {
            jobs = try await session.api.recentJobs()
            error = nil
            lastUpdated = .now
        } catch {
            self.error = error
            session.note(error)
        }
    }
}

/// One job in the feed, tappable through to its live view.
private struct JobRow: View {
    let job: Job
    @Environment(AppSession.self) private var session
    @Environment(Router.self) private var router

    private var tint: Color {
        switch job.outcome {
        case .running: .rpInFlight
        case .succeeded: .rpHealthy
        case .failed, .failedAndRolledBack: .rpUnhealthy
        }
    }

    private var systemImage: String {
        switch job.outcome {
        case .running: "arrow.triangle.2.circlepath"
        case .succeeded: "checkmark.circle.fill"
        case .failedAndRolledBack: "arrow.uturn.backward.circle.fill"
        case .failed: "xmark.octagon.fill"
        }
    }

    var body: some View {
        Button {
            router.showJob(app: job.app, id: job.id)
        } label: {
            HStack(alignment: .top, spacing: 10) {
                Image(systemName: systemImage)
                    .foregroundStyle(tint)
                    .symbolEffect(.pulse, isActive: !job.isFinished)
                VStack(alignment: .leading, spacing: 3) {
                    HStack(spacing: 6) {
                        Text(session.capabilities?.title(for: job.app) ?? job.app)
                            .font(.subheadline.weight(.semibold))
                            .lineLimit(1)
                        Text(job.promotionAction?.title ?? job.action.capitalized)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    if let result = job.result {
                        Text(
                            result.fromRing.map { "\(result.version): \($0) → \(result.ring)" }
                                ?? "\(result.version) → \(result.ring)"
                        )
                        .font(.caption.monospaced())
                        .foregroundStyle(.secondary)
                    }
                    if let message = job.summaryMessage {
                        Text(message)
                            .font(.caption)
                            .foregroundStyle(job.outcome == .succeeded ? .secondary : Color.rpUnhealthy)
                            .lineLimit(2)
                    }
                }
                Spacer(minLength: 0)
                RelativeTimestamp(date: job.startedAt)
            }
            .contentShape(.rect)
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .combine)
        .accessibilityHint("Opens the deployment log")
    }
}
