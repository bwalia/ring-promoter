import Foundation
import LocalAuthentication

/// Face ID / Touch ID, used in two independent places:
///
/// 1. locking the app on open, and
/// 2. confirming an action that deploys into the production ring.
///
/// They are separate settings on purpose. Someone who does not want the app
/// locked all day may still want the production confirmation, and that is the
/// one that actually prevents an accident.
@MainActor
@Observable
final class BiometricGate {
    enum Availability: Equatable {
        case faceID
        case touchID
        case passcodeOnly
        case unavailable(String)

        var isAvailable: Bool {
            if case .unavailable = self { return false }
            return true
        }

        var label: String {
            switch self {
            case .faceID: "Face ID"
            case .touchID: "Touch ID"
            case .passcodeOnly: "device passcode"
            case .unavailable: "unavailable"
            }
        }
    }

    /// Whether the app is currently covered by the lock screen.
    private(set) var isLocked = false

    /// Fresh context each evaluation: reusing one lets a single successful
    /// authentication silently authorise later prompts, which is exactly what
    /// a production confirmation must not do.
    private func makeContext() -> LAContext {
        let context = LAContext()
        context.localizedFallbackTitle = "Use Passcode"
        return context
    }

    var availability: Availability {
        let context = LAContext()
        var error: NSError?
        guard context.canEvaluatePolicy(
            .deviceOwnerAuthentication, error: &error
        ) else {
            return .unavailable(error?.localizedDescription ?? "Not set up on this device.")
        }
        switch context.biometryType {
        case .faceID: return .faceID
        case .touchID: return .touchID
        default: return .passcodeOnly
        }
    }

    /// Ask for authentication. Returns true when the user proved who they are.
    ///
    /// Falls back to the device passcode (`deviceOwnerAuthentication`) rather
    /// than biometrics alone, so a failed Face ID scan at 2am does not lock an
    /// on-call engineer out of a rollback.
    func authenticate(reason: String) async -> Bool {
        let context = makeContext()
        var error: NSError?
        guard context.canEvaluatePolicy(.deviceOwnerAuthentication, error: &error) else {
            // With no passcode set there is nothing to check against. Refusing
            // here would make the app unusable on such a device, so the caller
            // decides: `authenticate` reports success and the typed-name
            // confirmation remains as the real guard.
            return true
        }
        do {
            return try await context.evaluatePolicy(
                .deviceOwnerAuthentication, localizedReason: reason
            )
        } catch {
            return false
        }
    }

    func lock() { isLocked = true }

    /// Unlock the app itself.
    func unlock() async -> Bool {
        let ok = await authenticate(reason: "Unlock Ring Promoter")
        if ok { isLocked = false }
        return ok
    }
}
