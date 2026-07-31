import SwiftUI

/// Watch a running deploy: ordered steps, live logs, and an explicit terminal
/// state.
struct JobView: View {
    let app: String
    let jobID: String

    @Environment(AppSession.self) private var session
    @Environment(\.scenePhase) private var scenePhase
    @State private var poller: JobPoller?
    @State private var isFollowingTail = true
    @State private var hasAnnouncedResult = false

    var body: some View {
        Group {
            if let poller {
                content(poller: poller)
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .navigationTitle(title)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            if let job = poller?.job {
                ToolbarItem(placement: .topBarTrailing) {
                    ShareLink(item: job.transcript) {
                        Label("Share log", systemImage: "square.and.arrow.up")
                    }
                }
            }
        }
        // `.task` owns the polling task: it is cancelled automatically when
        // this view disappears, and restarted if the identity changes.
        .task(id: jobID) {
            let poller = JobPoller(app: app, jobID: jobID, api: session.api)
            self.poller = poller
            await poller.run()
            await finish(poller)
        }
        // Drive the Live Activity from the same polling that feeds this screen,
        // so the Lock Screen can never disagree with the app.
        .onChange(of: poller?.job) { _, job in
            guard let job else { return }
            Task { await mirrorToLiveActivity(job) }
        }
        .onChange(of: scenePhase) { _, phase in
            // Polling stops with the task when the app backgrounds; coming
            // back, take one immediate reading so the screen is never stale.
            if phase == .active { Task { await poller?.refreshOnce() } }
        }
    }

    private var title: String {
        guard let job = poller?.job else { return "Deployment" }
        return job.promotionAction?.title ?? job.action.capitalized
    }

    @ViewBuilder
    private func content(poller: JobPoller) -> some View {
        List {
            if let job = poller.job {
                Section { JobOutcomeBanner(job: job) }

                Section("Steps") {
                    ForEach(job.steps) { step in
                        StepRow(step: step)
                    }
                    if job.steps.isEmpty {
                        Text("Waiting for the server to start the first step…")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                    }
                }

                Section("Log") {
                    LogConsole(job: job, isFollowingTail: $isFollowingTail)
                        .listRowInsets(EdgeInsets())
                }

                if job.status == .failed {
                    DiagnosisSection(job: job, poller: poller)
                }
            } else if let error = poller.error {
                Section { ErrorRow(error: error) }
            } else {
                Section {
                    HStack {
                        ProgressView()
                        Text("Starting…").foregroundStyle(.secondary)
                    }
                }
            }
        }
        .refreshable { await poller.refreshOnce() }
    }

    /// Start the activity on the first snapshot, then keep it in step.
    ///
    /// Started here rather than at the moment the action is fired, because only
    /// the job says which ring and version it actually ended up targeting.
    private func mirrorToLiveActivity(_ job: Job) async {
        guard !job.isFinished else { return }
        let controller = LiveActivityController.shared
        controller.start(
            job: job,
            appTitle: session.capabilities?.title(for: app) ?? app,
            targetRing: job.result?.ring ?? "",
            version: job.result?.version ?? ""
        )
        await controller.update(with: job)
    }

    private func finish(_ poller: JobPoller) async {
        guard poller.didFinish, !hasAnnouncedResult, let job = poller.job else { return }
        hasAnnouncedResult = true
        switch job.outcome {
        case .succeeded: Haptics.success()
        case .failed, .failedAndRolledBack: Haptics.failure()
        case .running: break
        }
        // The pipeline changed, so the widget's cached snapshot is now wrong.
        WidgetRefresher.reloadNow()
        await LiveActivityController.shared.end(with: job)
    }
}

/// The terminal state, stated plainly. Succeeded, failed, and **rolled back**
/// are three different things and are never merged.
struct JobOutcomeBanner: View {
    let job: Job

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

    private var headline: String {
        switch job.outcome {
        case .running: "Running"
        case .succeeded: "Succeeded"
        case .failedAndRolledBack: "Failed — rolled back"
        case .failed: "Failed"
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Image(systemName: systemImage)
                    .foregroundStyle(tint)
                    .symbolEffect(.pulse, isActive: job.outcome == .running)
                Text(headline)
                    .font(.headline)
                    .foregroundStyle(tint)
                Spacer()
                if let finished = job.finishedAt {
                    Text(duration(to: finished))
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
            }
            if let result = job.result {
                Text(routeDescription(result))
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
            if let message = job.summaryMessage {
                Text(message)
                    .font(.subheadline)
                    .fixedSize(horizontal: false, vertical: true)
                    .textSelection(.enabled)
            }
            if job.outcome == .failedAndRolledBack {
                Label(
                    "The ring was returned to its previous version automatically.",
                    systemImage: "info.circle"
                )
                .font(.caption)
                .foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 4)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("job-outcome")
        .accessibilityLabel("\(headline). \(job.summaryMessage ?? "")")
        .accessibilityValue(headline)
    }

    private func routeDescription(_ result: ActionResult) -> String {
        if let from = result.fromRing, !from.isEmpty {
            return "\(result.version): \(from) → \(result.ring)"
        }
        return "\(result.version) → \(result.ring)"
    }

    private func duration(to finished: Date) -> String {
        let seconds = Int(finished.timeIntervalSince(job.startedAt).rounded())
        return seconds < 60 ? "\(seconds)s" : "\(seconds / 60)m \(seconds % 60)s"
    }
}

/// One step with its status.
private struct StepRow: View {
    let step: JobStep

    private var tint: Color {
        switch step.status {
        case .success: .rpHealthy
        case .failed: .rpUnhealthy
        case .running: .rpInFlight
        case .unknown: .rpNeutral
        }
    }

    private var systemImage: String {
        switch step.status {
        case .success: "checkmark.circle.fill"
        case .failed: "xmark.circle.fill"
        case .running: "circle.dotted"
        case .unknown: "questionmark.circle"
        }
    }

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 10) {
            Image(systemName: systemImage)
                .foregroundStyle(tint)
                .symbolEffect(.pulse, isActive: step.status == .running)
            VStack(alignment: .leading, spacing: 2) {
                Text(step.title)
                    .font(.subheadline.weight(.medium))
                    .fixedSize(horizontal: false, vertical: true)
                if let duration = step.duration, duration > 0.5 {
                    Text("\(Int(duration.rounded()))s")
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
            }
            Spacer(minLength: 0)
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(step.title), \(step.status.rawValue)")
    }
}

/// The log console: monospaced, selectable, auto-scrolling with an explicit way
/// back to the bottom once the operator has scrolled up to read something.
private struct LogConsole: View {
    let job: Job
    @Binding var isFollowingTail: Bool

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private struct Line: Identifiable {
        let id: String
        let text: String
        let isStepTitle: Bool
        let isFailure: Bool
    }

    private var lines: [Line] {
        job.steps.flatMap { step -> [Line] in
            let header = Line(
                id: "\(step.id)-title", text: step.title, isStepTitle: true,
                isFailure: step.status == .failed
            )
            let body = step.logs.enumerated().map { index, text in
                Line(
                    id: "\(step.id)-\(index)", text: text, isStepTitle: false,
                    isFailure: step.status == .failed
                )
            }
            return [header] + body
        }
    }

    var body: some View {
        ZStack(alignment: .bottomTrailing) {
            ScrollViewReader { proxy in
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 2) {
                        ForEach(lines) { line in
                            Text(line.text)
                                .font(.system(.caption, design: .monospaced))
                                .foregroundStyle(colour(for: line))
                                .fontWeight(line.isStepTitle ? .semibold : .regular)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .id(line.id)
                        }
                        Color.clear.frame(height: 1).id(Self.tailID)
                    }
                    .padding(10)
                    .textSelection(.enabled)
                }
                .frame(minHeight: 160, maxHeight: 320)
                .background(Color.primary.opacity(0.04))
                .onChange(of: lines.count) { _, _ in
                    guard isFollowingTail else { return }
                    scroll(proxy)
                }
                .onAppear { scroll(proxy) }
                .overlay(alignment: .bottomTrailing) {
                    if !isFollowingTail {
                        Button {
                            isFollowingTail = true
                            scroll(proxy)
                        } label: {
                            Label("Jump to bottom", systemImage: "arrow.down.to.line")
                                .font(.caption.weight(.semibold))
                                .padding(.horizontal, 10)
                                .padding(.vertical, 6)
                                .background(.thinMaterial, in: .capsule)
                        }
                        .padding(10)
                    }
                }
            }
        }
        .simultaneousGesture(
            DragGesture().onChanged { value in
                // Dragging downwards means the operator is scrolling back to
                // read something; stop yanking them to the bottom.
                if value.translation.height > 12 { isFollowingTail = false }
            }
        )
        .accessibilityLabel("Deployment log")
        .accessibilityValue(job.transcript)
    }

    private static let tailID = "log-tail"

    private func scroll(_ proxy: ScrollViewProxy) {
        if reduceMotion {
            proxy.scrollTo(Self.tailID, anchor: .bottom)
        } else {
            withAnimation(.easeOut(duration: 0.18)) {
                proxy.scrollTo(Self.tailID, anchor: .bottom)
            }
        }
    }

    private func colour(for line: Line) -> Color {
        if line.isStepTitle { return .primary }
        return line.isFailure ? .rpUnhealthy : .secondary
    }
}

/// "Diagnose with AI", shown only when the server says the feature exists.
private struct DiagnosisSection: View {
    let job: Job
    let poller: JobPoller

    @Environment(AppSession.self) private var session
    @State private var isRequesting = false

    var body: some View {
        if session.aiEnabled {
            Section("Diagnosis") {
                if let diagnosis = job.diagnosis, !diagnosis.isEmpty {
                    Text(LocalizedStringKey(diagnosis))
                        .font(.subheadline)
                        .textSelection(.enabled)
                        .fixedSize(horizontal: false, vertical: true)
                } else if job.diagnosisStatus == .running || isRequesting {
                    HStack(spacing: 8) {
                        ProgressView()
                        Text("Working out what went wrong…")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                    }
                } else if let failure = job.diagnosisError, !failure.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        Label(failure, systemImage: "exclamationmark.triangle")
                            .font(.subheadline)
                            .foregroundStyle(Color.rpGate)
                        Button("Try again") { Task { await request() } }
                            .buttonStyle(.bordered)
                            .controlSize(.small)
                    }
                } else if job.canRequestDiagnosis {
                    Button {
                        Task { await request() }
                    } label: {
                        Label("Diagnose with AI", systemImage: "sparkles")
                    }
                }
            }
        }
    }

    private func request() async {
        isRequesting = true
        defer { isRequesting = false }
        await poller.requestDiagnosis()
    }
}

#Preview("Live job") {
    PreviewHost {
        NavigationStack { JobView(app: "payments-api", jobID: "job-1") }
    }
}
