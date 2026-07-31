import SwiftUI

/// One application's history, newest first, grouped by day.
struct HistoryView: View {
    let app: String

    @Environment(AppSession.self) private var session
    @State private var entries: [HistoryEntry] = []
    @State private var isLoading = false
    @State private var error: APIError?
    @State private var expanded: Set<Int64> = []

    var body: some View {
        List {
            if let error, entries.isEmpty {
                Section { ErrorRow(error: error) { Task { await load() } } }
            }
            ForEach(groupedByDay, id: \.day) { group in
                Section(group.title) {
                    ForEach(group.entries) { entry in
                        HistoryRow(
                            app: app,
                            entry: entry,
                            isExpanded: expanded.contains(entry.id),
                            onToggle: { toggle(entry) }
                        )
                    }
                }
            }
            if entries.isEmpty, !isLoading, error == nil {
                Section {
                    CalmEmptyState(
                        title: "No history yet",
                        message: "Seeds, promotions and rollbacks appear here as they happen.",
                        systemImage: "clock",
                        tint: .rpNeutral
                    )
                    .listRowBackground(Color.clear)
                }
            }
        }
        .navigationTitle("History")
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await load() }
        .task { await load() }
        .overlay {
            if isLoading, entries.isEmpty { ProgressView() }
        }
    }

    private struct DayGroup {
        let day: Date
        let title: String
        let entries: [HistoryEntry]
    }

    /// Grouped by calendar day in the operator's own time zone, because "was
    /// that yesterday?" is asked in local time.
    private var groupedByDay: [DayGroup] {
        let calendar = Calendar.current
        let buckets: [Date: [HistoryEntry]] = Dictionary(grouping: entries) { entry in
            calendar.startOfDay(for: entry.createdAt)
        }
        let days: [Date] = buckets.keys.sorted(by: >)
        var groups: [DayGroup] = []
        groups.reserveCapacity(days.count)
        for day in days {
            let sorted = (buckets[day] ?? []).sorted { $0.createdAt > $1.createdAt }
            groups.append(DayGroup(day: day, title: Self.title(for: day), entries: sorted))
        }
        return groups
    }

    /// "Today", "Yesterday", then the full date — how an operator actually
    /// refers to when something happened.
    private static func title(for day: Date) -> String {
        let calendar = Calendar.current
        if calendar.isDateInToday(day) { return "Today" }
        if calendar.isDateInYesterday(day) { return "Yesterday" }
        return day.formatted(date: .complete, time: .omitted)
    }

    private func toggle(_ entry: HistoryEntry) {
        if expanded.contains(entry.id) {
            expanded.remove(entry.id)
        } else {
            expanded.insert(entry.id)
        }
    }

    private func load() async {
        isLoading = true
        defer { isLoading = false }
        do {
            entries = try await session.api.history(app: app)
            error = nil
        } catch {
            self.error = error
            session.note(error)
        }
    }
}

/// One history entry, expanding to show its diagnosis.
struct HistoryRow: View {
    let app: String
    let entry: HistoryEntry
    var isExpanded: Bool = false
    var onToggle: (() -> Void)?
    /// Shown on the cross-app feed, where the app name is not implied.
    var showsAppName: Bool = false

    @Environment(AppSession.self) private var session
    @State private var diagnosis: String?
    @State private var isDiagnosing = false
    @State private var diagnosisError: String?

    private var tint: Color { entry.succeeded ? .rpHealthy : .rpUnhealthy }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Button {
                onToggle?()
            } label: {
                header
            }
            .buttonStyle(.plain)
            .disabled(onToggle == nil)

            if isExpanded {
                detail
            }
        }
        .padding(.vertical, 2)
    }

    private var header: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: entry.succeeded ? "checkmark.circle.fill" : "xmark.circle.fill")
                .foregroundStyle(tint)
                .font(.subheadline)
            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 6) {
                    Text(entry.promotionAction?.pastTense ?? entry.action.capitalized)
                        .font(.subheadline.weight(.semibold))
                    Text(entry.ring)
                        .font(.caption.weight(.medium))
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(.quaternary, in: .capsule)
                    if showsAppName {
                        Text(session.capabilities?.title(for: entry.app) ?? entry.app)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                }
                Text(entry.versionTransition)
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
                if !entry.message.isEmpty {
                    Text(entry.message)
                        .font(.caption)
                        .foregroundStyle(entry.succeeded ? .secondary : Color.rpUnhealthy)
                        .lineLimit(isExpanded ? nil : 2)
                        .fixedSize(horizontal: false, vertical: isExpanded)
                }
            }
            Spacer(minLength: 0)
            Text(entry.createdAt, style: .time)
                .font(.caption.monospacedDigit())
                .foregroundStyle(.secondary)
        }
        .contentShape(.rect)
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder private var detail: some View {
        if !entry.succeeded, session.aiEnabled {
            VStack(alignment: .leading, spacing: 8) {
                if let text = diagnosis ?? entry.diagnosis, !text.isEmpty {
                    Text(LocalizedStringKey(text))
                        .font(.caption)
                        .textSelection(.enabled)
                        .fixedSize(horizontal: false, vertical: true)
                } else if isDiagnosing {
                    HStack(spacing: 8) {
                        ProgressView().controlSize(.small)
                        Text("Working out what went wrong…")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                } else {
                    Button {
                        Task { await diagnose() }
                    } label: {
                        Label("Diagnose with AI", systemImage: "sparkles")
                            .font(.caption)
                    }
                    .buttonStyle(.bordered)
                    .controlSize(.small)
                }
                if let diagnosisError {
                    Text(diagnosisError)
                        .font(.caption)
                        .foregroundStyle(Color.rpGate)
                }
            }
            .padding(.leading, 26)
            .task { await loadExistingDiagnosis() }
        }
    }

    /// A diagnosis someone else already requested is stored server-side, so
    /// check before spending another model call.
    private func loadExistingDiagnosis() async {
        guard diagnosis == nil, entry.diagnosis == nil, !entry.succeeded else { return }
        guard let response = try? await session.api.historyDiagnosis(app: app, id: entry.id),
              response.diagnosisStatus == .done
        else { return }
        diagnosis = response.diagnosis
    }

    private func diagnose() async {
        isDiagnosing = true
        defer { isDiagnosing = false }
        diagnosisError = nil
        do {
            let response = try await session.api.diagnoseHistoryEntry(app: app, id: entry.id)
            if let text = response.diagnosis, !text.isEmpty {
                diagnosis = text
                return
            }
            // The server accepted the request and is generating; poll briefly.
            for _ in 0..<20 {
                try await Task.sleep(for: .seconds(2))
                let latest = try await session.api.historyDiagnosis(app: app, id: entry.id)
                if latest.diagnosisStatus == .done, let text = latest.diagnosis {
                    diagnosis = text
                    return
                }
                if latest.diagnosisStatus == .failed { break }
            }
            diagnosisError = "The diagnosis did not finish. Try again in a moment."
        } catch let error as APIError {
            diagnosisError = error.userMessage
            session.note(error)
        } catch {
            diagnosisError = error.localizedDescription
        }
    }
}
