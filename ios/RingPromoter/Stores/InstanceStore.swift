import Foundation

/// The saved control planes and which one is active.
///
/// Records live in `UserDefaults`; their tokens live in the Keychain, keyed by
/// the record's id. Deleting a record deletes its token — an orphaned token is
/// a credential nobody is managing.
@MainActor
@Observable
final class InstanceStore {
    private enum Key {
        static let instances = "rp.instances"
        static let activeID = "rp.activeInstanceID"
    }

    private(set) var instances: [Instance] = []
    private(set) var activeID: UUID?

    private let defaults: UserDefaults

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        load()
    }

    var active: Instance? {
        guard let activeID else { return nil }
        return instances.first { $0.id == activeID }
    }

    var isEmpty: Bool { instances.isEmpty }

    /// Add or replace a record and store its token. The token is written first:
    /// if the Keychain refuses, no record is saved, so the app never lists an
    /// instance it cannot authenticate to.
    func save(_ instance: Instance, token: String) throws {
        try Keychain.save(token: token, for: instance.keychainAccount)
        if let index = instances.firstIndex(where: { $0.id == instance.id }) {
            instances[index] = instance
        } else {
            instances.append(instance)
        }
        persist()
    }

    /// Update a record's metadata (name, colour, biometric preference) without
    /// touching its token.
    func update(_ instance: Instance) {
        guard let index = instances.firstIndex(where: { $0.id == instance.id }) else { return }
        instances[index] = instance
        persist()
    }

    func setActive(_ instance: Instance) {
        activeID = instance.id
        defaults.set(instance.id.uuidString, forKey: Key.activeID)
    }

    /// Remove a record and its token together.
    func remove(_ instance: Instance) {
        try? Keychain.delete(for: instance.keychainAccount)
        instances.removeAll { $0.id == instance.id }
        if activeID == instance.id {
            activeID = nil
            defaults.removeObject(forKey: Key.activeID)
        }
        persist()
    }

    /// Wipe every record and every token. Offered in Settings so an operator
    /// handing a device on has one action that leaves nothing behind.
    func removeAll() {
        try? Keychain.deleteAll()
        instances = []
        activeID = nil
        defaults.removeObject(forKey: Key.activeID)
        defaults.removeObject(forKey: Key.instances)
        WidgetSnapshotStore.clear()
    }

    /// Does a record for this URL already exist? Used so adding the same server
    /// twice updates its token rather than creating a confusing duplicate.
    func instance(withBaseURL url: URL) -> Instance? {
        instances.first { $0.baseURL == url }
    }

    private func load() {
        if let data = defaults.data(forKey: Key.instances),
           let decoded = try? JSONDecoder().decode([Instance].self, from: data) {
            instances = decoded
        }
        if let raw = defaults.string(forKey: Key.activeID) {
            activeID = UUID(uuidString: raw)
        }
    }

    private func persist() {
        guard let data = try? JSONEncoder().encode(instances) else { return }
        defaults.set(data, forKey: Key.instances)
    }
}
