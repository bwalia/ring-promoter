import Foundation
import Security

/// Keychain storage for API tokens.
///
/// Tokens never go in `UserDefaults`, are never written to a log, and are
/// never included in the widget snapshot. Items ask for
/// `kSecAttrAccessibleWhenUnlockedThisDeviceOnly`, which means:
///
/// - unreadable while the device is locked, so a running background refresh
///   cannot leak one from a locked device;
/// - excluded from encrypted backups and from device-to-device migration, so a
///   restored backup on someone else's device carries no credentials.
///
/// The trade-off is deliberate: an operator re-pastes a token after restoring
/// a device, which is a far better outcome than a control-plane token
/// travelling inside a backup.
///
/// ## The two macOS keychains
///
/// iOS has one keychain. macOS has two, and which one the app gets decides
/// whether the paragraph above is true there at all:
///
/// - the **data-protection keychain** is the iOS one. It honours
///   `kSecAttrAccessible`, and it grants access by the app's team identifier,
///   so the app is never asked to prove who it is.
/// - the **legacy file-based keychain** is what macOS uses when
///   `kSecUseDataProtectionKeychain` is not set. It ignores
///   `kSecAttrAccessible` completely — so every promise above is quietly not
///   kept — and it grants access through an ACL naming the exact code
///   signature that wrote the item. A rebuilt binary has a different
///   signature, so macOS stops and asks the operator for their login password
///   on the app's behalf. That prompt is not a sign of anything wrong; it is
///   the legacy keychain working as designed on a binary it has not met.
///
/// So every operation asks for the data-protection keychain first. A build
/// with no team identifier — an ad-hoc signed local build — is refused with
/// `errSecMissingEntitlement` and falls back to the legacy keychain, which is
/// what keeps a development build usable. A properly signed build never takes
/// that path: it gets the accessibility guarantees, and it never prompts.
enum Keychain {
    /// Every failure the caller can act on. `unhandled` carries the raw
    /// `OSStatus` for a bug report.
    enum Failure: Error, Equatable {
        case unhandled(OSStatus)
        case unexpectedData

        var message: String {
            switch self {
            case .unhandled(let status):
                let text = SecCopyErrorMessageString(status, nil) as String?
                return text ?? "Keychain error \(status)."
            case .unexpectedData:
                return "The stored credential could not be read."
            }
        }
    }

    /// The service under which all Ring Promoter tokens live.
    static let service = "org.fictionally.ringpromoter.token"

    static func save(token: String, for instanceID: String) throws {
        guard let data = token.data(using: .utf8) else { throw Failure.unexpectedData }
        var query = baseQuery(for: instanceID)
        // Replace rather than duplicate: SecItemAdd on an existing account
        // fails with errSecDuplicateItem. Cleared from both keychains, so a
        // token written before this build had a team identifier cannot linger
        // as an older second copy that a later read might find first.
        _ = run(query) { SecItemDelete($0) }
        _ = SecItemDelete(query as CFDictionary)
        query[kSecValueData as String] = data
        query[kSecAttrAccessible as String] = kSecAttrAccessibleWhenUnlockedThisDeviceOnly
        let status = run(query) { SecItemAdd($0, nil) }
        guard status == errSecSuccess else { throw Failure.unhandled(status) }
    }

    /// Returns nil when no token is stored — a missing token is an ordinary
    /// state (a newly added instance), not an error.
    static func token(for instanceID: String) throws -> String? {
        var query = baseQuery(for: instanceID)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne

        var item: CFTypeRef?
        var status = run(query) { SecItemCopyMatching($0, &item) }
        if status == errSecItemNotFound {
            // Absent from the modern keychain, but it may be in the legacy one
            // from before this build was signed. Read it rather than silently
            // losing a token the operator saved.
            status = SecItemCopyMatching(query as CFDictionary, &item)
        }
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess else { throw Failure.unhandled(status) }
        guard let data = item as? Data, let token = String(data: data, encoding: .utf8) else {
            throw Failure.unexpectedData
        }
        return token
    }

    static func delete(for instanceID: String) throws {
        try deleteEverywhere(baseQuery(for: instanceID))
    }

    /// Removes every token this app stored. Used by "Forget everything" in
    /// Settings.
    static func deleteAll() throws {
        try deleteEverywhere([
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
        ])
    }

    /// Gone from both keychains counts as gone. Only a real error — not
    /// "nothing there", and not "this build may not use that keychain" — is
    /// worth reporting to the operator.
    private static func deleteEverywhere(_ query: [String: Any]) throws {
        let results = [
            run(query) { SecItemDelete($0) },
            SecItemDelete(query as CFDictionary),
        ]
        let ignorable: Set<OSStatus> = [
            errSecSuccess, errSecItemNotFound, errSecMissingEntitlement,
        ]
        if let failure = results.first(where: { !ignorable.contains($0) }) {
            throw Failure.unhandled(failure)
        }
    }

    /// Runs `operation` against the data-protection keychain, falling back to
    /// the legacy one when this build is not entitled to it. See the type's
    /// documentation for why that fallback exists.
    private static func run(
        _ query: [String: Any],
        _ operation: (CFDictionary) -> OSStatus
    ) -> OSStatus {
        var modern = query
        modern[kSecUseDataProtectionKeychain as String] = true
        let status = operation(modern as CFDictionary)
        guard status == errSecMissingEntitlement else { return status }
        return operation(query as CFDictionary)
    }

    private static func baseQuery(for instanceID: String) -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: instanceID,
        ]
    }
}
