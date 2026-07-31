import SwiftUI

/// Server-side application groups, shared by every operator of this control
/// plane — so renaming one here renames it for the whole team.
struct GroupsView: View {
    /// The Overview's already-loaded ring data, so a group can be drawn as what
    /// it actually is — its applications and their pipelines — without a second
    /// round of requests.
    var summaries: [AppSummary] = []
    /// Called with a group id when the operator asks for its ring page.
    var onOpenRing: ((String) -> Void)?

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss

    @State private var groups: [AppGroup] = []
    @State private var isLoading = false
    @State private var error: APIError?
    @State private var editing: EditRequest?
    @State private var pendingDeletion: AppGroup?

    /// Nil `group` means "create a new one".
    private struct EditRequest: Identifiable {
        let group: AppGroup?
        let id = UUID()
    }

    var body: some View {
        NavigationStack {
            List {
                if let error {
                    Section { ErrorRow(error: error) { Task { await load() } } }
                }
                ForEach(groups) { group in
                    Section {
                        let members = summaries.filter { group.contains($0.name) }
                        if members.isEmpty {
                            Text(
                                group.apps.isEmpty
                                    ? "No applications yet." : memberSummary(group)
                            )
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        }
                        Button {
                            onOpenRing?(group.id)
                            dismiss()
                        } label: {
                            Label("Open the deployment ring", systemImage: "circle.hexagonpath")
                                .font(.subheadline)
                        }
                        ForEach(members) { summary in
                            GroupMemberRow(summary: summary, pipeline: session.pipeline)
                        }
                    } header: {
                        HStack {
                            Text(group.name)
                            Spacer()
                            Button("Edit") { editing = EditRequest(group: group) }
                                .font(.caption.weight(.semibold))
                                .textCase(nil)
                        }
                    }
                    .swipeActions {
                        Button("Delete", role: .destructive) { pendingDeletion = group }
                    }
                }
                if groups.isEmpty, !isLoading, error == nil {
                    Section {
                        CalmEmptyState(
                            title: "No groups yet",
                            message: "Group related applications and the Overview organises "
                                + "itself around the ones you own.",
                            systemImage: "square.stack.3d.down.right",
                            tint: .rpNeutral
                        )
                        .listRowBackground(Color.clear)
                    }
                }
            }
            .navigationTitle("Groups")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Done") { dismiss() }
                }
                ToolbarItem(placement: .primaryAction) {
                    Button {
                        editing = EditRequest(group: nil)
                    } label: {
                        Label("New group", systemImage: "plus")
                    }
                }
            }
            .refreshable { await load() }
            .task { await load() }
            .sheet(item: $editing) { request in
                EditGroupView(group: request.group) { await load() }
            }
            .confirmationDialog(
                "Delete this group?",
                isPresented: Binding(
                    get: { pendingDeletion != nil },
                    set: { if !$0 { pendingDeletion = nil } }
                ),
                titleVisibility: .visible,
                presenting: pendingDeletion
            ) { group in
                Button("Delete \(group.name)", role: .destructive) {
                    Task { await delete(group) }
                }
            } message: { _ in
                Text("Groups are shared, so this removes it for everyone using this server.")
            }
        }
    }

    private func memberSummary(_ group: AppGroup) -> String {
        guard !group.apps.isEmpty else { return "No applications" }
        return group.apps
            .map { session.capabilities?.title(for: $0) ?? $0 }
            .joined(separator: ", ")
    }

    private func load() async {
        isLoading = true
        defer { isLoading = false }
        do {
            groups = try await session.api.groups()
            error = nil
        } catch {
            self.error = error
            session.note(error)
        }
    }

    private func delete(_ group: AppGroup) async {
        do {
            try await session.api.deleteGroup(id: group.id)
            await load()
        } catch {
            self.error = error
            session.note(error)
        }
    }
}

/// One application inside a group: its name and its live ring pipeline, with
/// the in-flight ring animating exactly as it does on the Overview.
private struct GroupMemberRow: View {
    let summary: AppSummary
    let pipeline: RingPipeline

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 6) {
                Text(summary.title)
                    .font(.subheadline.weight(.medium))
                    .lineLimit(1)
                if summary.busyAction != nil {
                    SpinningGlyph()
                        .font(.caption2)
                        .foregroundStyle(Color.rpInFlight)
                        .accessibilityLabel("Deploy running")
                }
                Spacer(minLength: 0)
            }
            PipelineStrip(
                rings: summary.rings, pipeline: pipeline,
                busyRing: summary.busyRing, isCompact: true
            )
        }
        .padding(.vertical, 4)
    }
}

/// Create or rename a group and choose its members.
private struct EditGroupView: View {
    let group: AppGroup?
    let onSaved: () async -> Void

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss

    @State private var name = ""
    @State private var selected: Set<String> = []
    @State private var isSaving = false
    @State private var error: APIError?

    var body: some View {
        NavigationStack {
            Form {
                Section("Name") {
                    TextField("Group name", text: $name)
                        .textInputAutocapitalization(.words)
                }
                Section("Applications") {
                    ForEach(session.capabilities?.apps ?? [], id: \.self) { app in
                        Button {
                            if selected.contains(app) {
                                selected.remove(app)
                            } else {
                                selected.insert(app)
                            }
                        } label: {
                            HStack {
                                Text(session.capabilities?.title(for: app) ?? app)
                                Spacer()
                                if selected.contains(app) {
                                    Image(systemName: "checkmark")
                                        .foregroundStyle(.tint)
                                }
                            }
                            .contentShape(.rect)
                        }
                        .buttonStyle(.plain)
                        .accessibilityAddTraits(selected.contains(app) ? .isSelected : [])
                    }
                }
                if let error {
                    Section {
                        Label(error.userMessage, systemImage: "exclamationmark.triangle.fill")
                            .font(.subheadline)
                            .foregroundStyle(Color.rpUnhealthy)
                    }
                }
            }
            .navigationTitle(group == nil ? "New group" : "Edit group")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    if isSaving {
                        ProgressView()
                    } else {
                        Button("Save") { Task { await save() } }
                            .disabled(name.trimmingCharacters(in: .whitespaces).isEmpty)
                    }
                }
            }
            .onAppear {
                name = group?.name ?? ""
                selected = Set(group?.apps ?? [])
            }
        }
    }

    private func save() async {
        isSaving = true
        defer { isSaving = false }
        error = nil
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        // Preserve the server's app ordering rather than a Set's arbitrary one.
        let members = (session.capabilities?.apps ?? []).filter { selected.contains($0) }
        do {
            if let group {
                _ = try await session.api.updateGroup(id: group.id, name: trimmed, apps: members)
            } else {
                _ = try await session.api.createGroup(name: trimmed, apps: members)
            }
            await onSaved()
            dismiss()
        } catch {
            self.error = error
            session.note(error)
        }
    }
}
