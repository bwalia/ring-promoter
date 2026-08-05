import Foundation
import Security

/// Keychain storage for API tokens.
///
/// Tokens never go in `UserDefaults`, are never written to a log, and are never
/// included in the widget snapshot. Items are stored with
/// `kSecAttrAccessibleWhenUnlockedThisDeviceOnly`, which means:
///
/// - unreadable while the device is locked, so a running background refresh
///   cannot leak one from a locked phone;
/// - excluded from encrypted backups and from device-to-device migration, so a
///   restored backup on someone else's phone carries no credentials.
///
/// The trade-off is deliberate: an operator re-pastes a token after restoring a
/// device, which is a far better outcome than a control-plane token travelling
/// inside a backup.
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
        // fails with errSecDuplicateItem.
        SecItemDelete(query as CFDictionary)
        query[kSecValueData as String] = data
        query[kSecAttrAccessible as String] = kSecAttrAccessibleWhenUnlockedThisDeviceOnly
        let status = SecItemAdd(query as CFDictionary, nil)
        guard status == errSecSuccess else { throw Failure.unhandled(status) }
    }

    /// Returns nil when no token is stored — a missing token is an ordinary
    /// state (a newly added instance), not an error.
    static func token(for instanceID: String) throws -> String? {
        var query = baseQuery(for: instanceID)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne

        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess else { throw Failure.unhandled(status) }
        guard let data = item as? Data, let token = String(data: data, encoding: .utf8) else {
            throw Failure.unexpectedData
        }
        return token
    }

    static func delete(for instanceID: String) throws {
        let status = SecItemDelete(baseQuery(for: instanceID) as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw Failure.unhandled(status)
        }
    }

    /// Removes every token this app stored. Used by "Forget everything" in
    /// Settings.
    static func deleteAll() throws {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
        ]
        let status = SecItemDelete(query as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw Failure.unhandled(status)
        }
    }

    private static func baseQuery(for instanceID: String) -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: instanceID,
        ]
    }
}
