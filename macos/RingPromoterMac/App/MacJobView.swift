import SwiftUI

/// Watching a deploy, laid out for a window.
///
/// The shared `JobView` is a grouped `List` — right on a phone, and on macOS
/// the plain table that prompted "this is very basic": three bare sections,
/// no elapsed time, no step numbers, no durations, everything flush to the
/// edges.
///
/// This is the same `JobPoller`, the same `StepRow`, `LogConsole` and
/// `DiagnosisSection`. What it adds is what a window has room for and the
/// operator actually wants while a promotion runs: how long it has been going,
/// which step of how many, and how long each one took.
///
/// Not here yet: the web console's **Flow** tab, the node graph of request →
/// policy gate → verify → build → deploy. That is a real piece of work — a
/// custom layout with edges and gate nodes — and a thin version of it would be
/// worse than the honest absence.
struct MacJobView: View {
    let app: String
    let jobID: String

    @Environment(AppSession.self) private var session
    @State private var poller: JobPoller?
    @State private var isFollowingTail = true
    /// Ticks once a second so the elapsed time is live while a job runs.
    @State private var now = Date.now

    private let tick = Timer.publish(every: 1, on: .main, in: .common).autoconnect()

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                if let job = poller?.job {
                    summary(job)
                    steps(job)
                    logPanel(job)
                    if let poller, job.outcome != .succeeded {
                        DiagnosisSection(job: job, poller: poller)
                            .padding(14)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(Color.rpCard, in: .rect(cornerRadius: RPRadius.lg))
                            .overlay {
                                RoundedRectangle(cornerRadius: RPRadius.lg)
                                    .strokeBorder(Color.rpBorder)
                            }
                    }
                } else {
                    loading
                }
            }
            .padding(.horizontal, 22)
            .padding(.vertical, 20)
        }
        .navigationTitle(title)
        .toolbar {
            if let job = poller?.job {
                ToolbarItem(placement: .primaryAction) {
                    ShareLink(item: job.transcript) {
                        Image(systemName: "square.and.arrow.up")
                    }
                    .help("Share the full log")
                }
            }
        }
        .task(id: jobID) {
            let created = JobPoller(app: app, jobID: jobID, api: session.api)
            poller = created
            await created.run()
        }
        .onReceive(tick) { now = $0 }
        .consoleSurface()
    }

    private var title: String {
        guard let job = poller?.job else { return "Deployment" }
        return job.promotionAction?.title ?? job.action.capitalized
    }

    // MARK: - Summary

    private func summary(_ job: Job) -> some View {
        HStack(alignment: .top, spacing: 44) {
            ConsoleStat(label: "Status") {
                ConsoleStatValue(
                    text: statusText(job), tint: tint(for: job),
                    systemImage: symbol(for: job)
                )
            }
            ConsoleStat(label: "Elapsed") {
                ConsoleStatValue(text: elapsed(job), isMonospaced: true)
            }
            if let result = job.result, !result.version.isEmpty {
                ConsoleStat(label: "Target") {
                    ConsoleStatValue(
                        text: result.fromRing.map { "\($0) → \(result.ring)" } ?? result.ring,
                        isMonospaced: true
                    )
                }
            }
            if !job.steps.isEmpty {
                ConsoleStat(label: "Steps") {
                    ConsoleStatValue(
                        text: "\(job.steps.filter { $0.status == .success }.count)"
                            + "/\(job.steps.count)",
                        isMonospaced: true
                    )
                }
            }
            Spacer(minLength: 0)
        }
    }

    private func statusText(_ job: Job) -> String {
        switch job.outcome {
        case .succeeded: "Succeeded"
        case .failed: "Failed"
        case .failedAndRolledBack: "Rolled back"
        case .running: "Running"
        }
    }

    private func tint(for job: Job) -> Color {
        switch job.outcome {
        case .succeeded: .rpHealthy
        case .failed, .failedAndRolledBack: .rpUnhealthy
        case .running: .rpInFlight
        }
    }

    private func symbol(for job: Job) -> String {
        switch job.outcome {
        case .succeeded: "checkmark.circle.fill"
        case .failed, .failedAndRolledBack: "xmark.circle.fill"
        case .running: "arrow.triangle.2.circlepath"
        }
    }

    /// "1m 40s". Counts from `now`, which ticks each second, so a running job
    /// shows time passing rather than a number frozen at the last poll.
    private func elapsed(_ job: Job) -> String {
        let end = job.finishedAt ?? now
        let seconds = Int(max(0, end.timeIntervalSince(job.startedAt)).rounded())
        return seconds < 60 ? "\(seconds)s" : "\(seconds / 60)m \(seconds % 60)s"
    }

    // MARK: - Steps

    private func steps(_ job: Job) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            ConsoleSectionLabel("Steps")
            ConsoleCard(padding: 0) {
                VStack(spacing: 0) {
                    if job.steps.isEmpty {
                        Text("Waiting for the server to start the first step…")
                            .font(.system(size: 12))
                            .foregroundStyle(Color.rpMutedForeground)
                            .padding(.horizontal, 14)
                            .padding(.vertical, 11)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    } else {
                        ForEach(Array(job.steps.enumerated()), id: \.element.id) { index, step in
                            if index > 0 {
                                Rectangle().fill(Color.rpBorder).frame(height: 1)
                            }
                            HStack(alignment: .firstTextBaseline, spacing: 10) {
                                // Numbered, as the web does: "step 8 of 9" is
                                // the question being asked while it runs.
                                Text("\(index + 1)")
                                    .font(.system(size: 11, weight: .medium, design: .monospaced))
                                    .foregroundStyle(Color.rpMutedForeground)
                                    .frame(width: 18, alignment: .trailing)
                                StepRow(step: step)
                            }
                            .padding(.horizontal, 14)
                            .padding(.vertical, 10)
                        }
                    }
                }
            }
        }
    }

    // MARK: - Log

    private func logPanel(_ job: Job) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            ConsoleSectionLabel("Log")
            ConsoleCard(padding: 0) {
                LogConsole(job: job, isFollowingTail: $isFollowingTail)
                    .frame(minHeight: 220)
            }
        }
    }

    private var loading: some View {
        VStack(alignment: .leading, spacing: 18) {
            HStack(spacing: 44) {
                ForEach(0..<3, id: \.self) { _ in
                    VStack(alignment: .leading, spacing: 7) {
                        ConsoleSkeleton(width: 70, height: 9)
                        ConsoleSkeleton(width: 100, height: 20)
                    }
                }
                Spacer(minLength: 0)
            }
            ConsoleSkeleton(width: 60, height: 9)
            ConsoleCard { ConsoleSkeleton(height: 90) }
        }
        .accessibilityLabel("Loading the deployment")
    }
}
