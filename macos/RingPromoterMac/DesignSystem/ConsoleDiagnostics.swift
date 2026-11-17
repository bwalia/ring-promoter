#if DEBUG
import Foundation
import Security

/// Exercises the two things a Mac build can break that a phone build cannot,
/// and prints what actually happened:
///
///     RingPromoterMac.app/Contents/MacOS/RingPromoterMac --diagnose [url]
///
/// 1. A Keychain round trip. The shared `Keychain` was written for iOS, and
///    macOS has a second, legacy keychain with different rules.
/// 2. An outbound HTTPS request, which the App Sandbox can refuse.
///
/// DEBUG-only and unreachable without the argument. It never prints a token.
@MainActor
enum ConsoleDiagnostics {
    static var requestedHost: String?? {
        let arguments = ProcessInfo.processInfo.arguments
        guard let flag = arguments.firstIndex(of: "--diagnose") else { return nil }
        let next = arguments.index(after: flag)
        return next < arguments.endIndex ? .some(arguments[next]) : .some(nil)
    }

    static func run(host: String?) async -> Never {
        print("— Keychain —")
        let probe = "diagnostic-value-not-a-real-token"
        do {
            try Keychain.save(token: probe, for: "diagnostic")
            let read = try Keychain.token(for: "diagnostic")
            try Keychain.delete(for: "diagnostic")
            print(read == probe ? "  save/read/delete: OK" : "  read back the WRONG value")
        } catch let failure as Keychain.Failure {
            if case .unhandled(let status) = failure {
                print("  FAILED: OSStatus \(status) — \(failure.message)")
            } else {
                print("  FAILED: \(failure.message)")
            }
        } catch {
            print("  FAILED: \(error)")
        }

        // Which of macOS's two keychains the app is actually using, and
        // whether the other one would even accept it. The legacy file-based
        // keychain ties each item to the exact code signature that wrote it,
        // so every rebuild of an ad-hoc binary is a stranger to yesterday's
        // item and macOS asks the operator to vouch for it.
        print("\n— Keychain kind —")
        let dpQuery: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: "org.fictionally.ringpromoter.diagnostic",
            kSecAttrAccount as String: "data-protection-probe",
            kSecValueData as String: Data("probe".utf8),
            kSecUseDataProtectionKeychain as String: true,
        ]
        SecItemDelete(dpQuery as CFDictionary)
        let status = SecItemAdd(dpQuery as CFDictionary, nil)
        if status == errSecSuccess {
            SecItemDelete(dpQuery as CFDictionary)
            print("  data-protection keychain: available (no ACL prompts)")
        } else {
            let text = SecCopyErrorMessageString(status, nil) as String? ?? "unknown"
            print("  data-protection keychain: REFUSED, OSStatus \(status) — \(text)")
            print("  so the app is on the legacy keychain, which prompts per signature.")
        }

        guard let host else {
            print("\n(no host given — skipping the network check)")
            exit(0)
        }

        // Exactly what the Add-cluster sheet does with what you typed.
        print("\n— URL —")
        guard let base = Instance.normalise(urlText: host) else {
            print("  \"\(host)\" is not a usable server URL — this is what the sheet rejects.")
            exit(0)
        }
        print("  typed:      \(host)")
        print("  normalised: \(base.absoluteString)")
        if base.query() != nil {
            print("  NOTE: that still carries a query string. Use just the scheme and host.")
        }

        print("\n— Network, from inside the sandbox —")
        for path in ["/healthz", "/api/apps"] {
            guard let url = URL(string: path, relativeTo: base) else { continue }
            do {
                var request = URLRequest(url: url)
                request.timeoutInterval = 15
                let (data, response) = try await URLSession.shared.data(for: request)
                let code = (response as? HTTPURLResponse)?.statusCode ?? -1
                let body = String(data: data.prefix(120), encoding: .utf8) ?? ""
                print("  \(path) → HTTP \(code)  \(body.replacingOccurrences(of: "\n", with: " "))")
            } catch {
                print("  \(path) → FAILED: \(error.localizedDescription)")
            }
        }
        // The real validation path, with a token supplied by the environment
        // so it is never typed into a chat, a log or this source file.
        guard let token = ProcessInfo.processInfo.environment["RP_DIAG_TOKEN"],
              !token.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        else {
            print("\n— Validation —")
            print("  skipped: set RP_DIAG_TOKEN to run the same check the sheet runs.")
            exit(0)
        }

        print("\n— Validation (the same call the Add-cluster sheet makes) —")
        switch await ConnectionValidator.live.validate(
            url: base, token: token.trimmingCharacters(in: .whitespacesAndNewlines)
        ) {
        case .success(let response):
            print("  OK — \(response.apps.count) application(s); "
                + "production password required: \(response.prodProtected)")
            print("  So the server, the token and the sandbox are all fine, and")
            print("  anything still failing is in the window's UI.")
        case .failure(let failure):
            print("  FAILED — \(failure.message)")
        }
        exit(0)
    }
}
#endif
