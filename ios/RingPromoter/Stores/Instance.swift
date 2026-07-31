import Foundation

/// A saved control plane.
///
/// Operators routinely have more than one — a staging control plane and the
/// production one — and promoting on the wrong cluster is the mistake this
/// whole type exists to prevent. Hence the required name and colour tag, and
/// hence the fact that the active instance is shown on every screen that can
/// change something.
///
/// The token is **not** stored here. Only its Keychain account id lives in this
/// record, which is what makes this struct safe to keep in `UserDefaults`.
struct Instance: Codable, Hashable, Sendable, Identifiable {
    let id: UUID
    /// Operator-supplied label ("Production", "Staging").
    var name: String
    var baseURL: URL
    var tint: InstanceTint
    /// Require Face ID / Touch ID before any action that targets the last ring
    /// on this instance.
    var requireBiometricsForProduction: Bool
    var addedAt: Date

    init(
        id: UUID = UUID(), name: String, baseURL: URL, tint: InstanceTint = .blue,
        requireBiometricsForProduction: Bool = true, addedAt: Date = .now
    ) {
        self.id = id
        self.name = name
        self.baseURL = baseURL
        self.tint = tint
        self.requireBiometricsForProduction = requireBiometricsForProduction
        self.addedAt = addedAt
    }

    /// The Keychain account under which this instance's token is filed.
    var keychainAccount: String { id.uuidString }

    /// Host and port, for the subtitle in the instance list.
    var displayHost: String {
        guard let host = baseURL.host() else { return baseURL.absoluteString }
        if let port = baseURL.port { return "\(host):\(port)" }
        return host
    }

    /// The scheme is not HTTPS, so the token would cross the network in clear.
    /// Surfaced as a warning rather than a hard block, because a local
    /// `http://localhost:8080` is a legitimate development target.
    var isInsecure: Bool { baseURL.scheme?.lowercased() != "https" }

    /// Normalises what an operator is likely to type: bare hosts get `https://`,
    /// trailing slashes and stray whitespace are dropped.
    static func normalise(urlText: String) -> URL? {
        var text = urlText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return nil }
        if !text.contains("://") { text = "https://" + text }
        while text.hasSuffix("/") { text.removeLast() }
        guard let url = URL(string: text), url.host() != nil,
              let scheme = url.scheme?.lowercased(), scheme == "https" || scheme == "http"
        else { return nil }
        return url
    }
}
