import Foundation

/// A user-defined collection of applications, stored server-side and shared by
/// every operator of this control plane.
///
/// Mirrors `store.Group`. Named `AppGroup` to avoid colliding with SwiftUI's
/// `Group`.
struct AppGroup: Codable, Hashable, Sendable, Identifiable {
    let id: String
    let name: String
    let apps: [String]
    let updatedAt: Date

    enum CodingKeys: String, CodingKey {
        case id, name, apps
        case updatedAt = "updated_at"
    }

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        name = try c.decode(String.self, forKey: .name)
        // The server omits an empty member list as null.
        apps = try c.decodeIfPresent([String].self, forKey: .apps) ?? []
        updatedAt = try c.decode(Date.self, forKey: .updatedAt)
    }

    init(id: String, name: String, apps: [String], updatedAt: Date) {
        self.id = id
        self.name = name
        self.apps = apps
        self.updatedAt = updatedAt
    }

    func contains(_ app: String) -> Bool { apps.contains(app) }
}

/// `GET /api/groups`
struct GroupsResponse: Codable, Hashable, Sendable {
    let groups: [AppGroup]

    init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        groups = try c.decodeIfPresent([AppGroup].self, forKey: .groups) ?? []
    }

    init(groups: [AppGroup]) { self.groups = groups }

    enum CodingKeys: String, CodingKey { case groups }
}
