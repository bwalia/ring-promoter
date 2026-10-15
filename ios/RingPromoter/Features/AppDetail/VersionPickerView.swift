import SwiftUI

/// Searchable list of the branches and tags in an application's source
/// repository, pushed from the Seed sheet.
///
/// A repository can list hundreds of tags, so the list filters as you type.
/// Anything typed that is not a listed branch or tag can still be used as a
/// commit SHA: the server resolves it against the repository before deploying,
/// the same as the web console.
struct VersionPickerView: View {
    let versions: VersionsResponse
    @Binding var selection: String

    @Environment(\.dismiss) private var dismiss
    @State private var query = ""

    private var trimmedQuery: String {
        query.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var matches: [AppVersion] { versions.matching(query) }
    private var branches: [AppVersion] { matches.filter { !$0.isTag } }
    private var tags: [AppVersion] { matches.filter(\.isTag) }

    var body: some View {
        List {
            if !trimmedQuery.isEmpty, !versions.contains(name: trimmedQuery) {
                Section {
                    Button {
                        pick(trimmedQuery)
                    } label: {
                        Label {
                            VStack(alignment: .leading, spacing: 2) {
                                Text("Use \u{201C}\(trimmedQuery)\u{201D}")
                                    .font(.body.monospaced())
                                Text("As a commit SHA — checked by the server before deploying")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        } icon: {
                            Image(systemName: "number")
                        }
                    }
                    .accessibilityIdentifier("version-use-typed")
                }
            }
            if !branches.isEmpty {
                Section("Branches (\(branches.count))") {
                    ForEach(branches) { row($0) }
                }
            }
            if !tags.isEmpty {
                Section("Tags (\(tags.count))") {
                    ForEach(tags) { row($0) }
                }
            }
            if matches.isEmpty, trimmedQuery.isEmpty {
                ContentUnavailableView("No branches or tags", systemImage: "tag.slash")
            }
        }
        .navigationTitle("Version")
        .navigationBarTitleDisplayMode(.inline)
        .searchable(
            text: $query,
            placement: .navigationBarDrawer(displayMode: .always),
            prompt: "Search, or paste a commit SHA"
        )
        .autocorrectionDisabled()
        .textInputAutocapitalization(.never)
        .onSubmit(of: .search) {
            // Return picks the only match, an exact match, or the typed SHA.
            guard !trimmedQuery.isEmpty else { return }
            if let exact = matches.first(where: { $0.name == trimmedQuery }) {
                pick(exact.name)
            } else if matches.count == 1 {
                pick(matches[0].name)
            } else if matches.isEmpty {
                pick(trimmedQuery)
            }
        }
    }

    private func row(_ version: AppVersion) -> some View {
        Button {
            pick(version.name)
        } label: {
            HStack {
                Label {
                    Text(version.name)
                        .font(.body.monospaced())
                        .lineLimit(1)
                        .truncationMode(.middle)
                } icon: {
                    Image(systemName: version.systemImage)
                }
                Spacer()
                if version.name == selection {
                    Image(systemName: "checkmark")
                        .foregroundStyle(Color.accentColor)
                        .fontWeight(.semibold)
                }
            }
            .contentShape(.rect)
        }
        .foregroundStyle(.primary)
        .accessibilityIdentifier("version-\(version.id)")
        .accessibilityAddTraits(version.name == selection ? .isSelected : [])
    }

    private func pick(_ name: String) {
        selection = name
        dismiss()
    }
}

#Preview("Version picker") {
    @Previewable @State var selection = "main"
    NavigationStack {
        VersionPickerView(versions: .demoVersions, selection: $selection)
    }
}
