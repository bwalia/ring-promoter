import Foundation

/// Every way a Ring Promoter request can fail, mapped to what the operator
/// should actually do about it.
///
/// The HTTP cases mirror the backend's status contract exactly. They are kept
/// distinct — never collapsed into "something went wrong" — because each one
/// leads somewhere different in the UI: `.unauthorized` goes to the token
/// screen, `.productionPasswordRequired` re-opens the password field,
/// `.conflict` explains a gate.
enum APIError: Error, Hashable, Sendable {
    /// The host could not be reached, TLS failed, or the request timed out.
    case transport(TransportFailure)
    /// The response was not valid JSON, or did not match the expected shape.
    /// Carries a short description; never the raw body, which may be large.
    case decoding(String)
    /// A 2xx arrived with a status the client does not handle.
    case unexpectedStatus(Int, message: String?)

    /// 400 — bad input: empty version, promoting past the last ring, a version
    /// that does not exist in the source repo, a missing or invalid CR code.
    case badRequest(String)
    /// 401 — the bearer token is missing, wrong or expired. Send the user to
    /// the token screen.
    case unauthorized(String)
    /// 403 — the production password is required, or was wrong.
    case productionPasswordRequired(String)
    /// 404 — unknown app, ring, job, window or group.
    case notFound(String)
    /// 409 — nothing to promote or roll back, a shut maintenance window, a
    /// missing QA sign-off, or auto-promote owned by config.
    case conflict(String)
    /// 501 — the server has no AI diagnosis configured.
    case notImplemented(String)
    /// 5xx.
    case server(Int, String)

    /// Transport-level failures worth telling apart during onboarding, where
    /// "wrong URL" and "wrong token" are the two mistakes people actually make.
    enum TransportFailure: Hashable, Sendable {
        case cannotReachHost(String)
        case tlsFailure(String)
        case timedOut
        case offline
        case cancelled
        case other(String)

        var message: String {
            switch self {
            case .cannotReachHost(let host):
                "Could not reach \(host). Check the server URL and that you are on the right network or VPN."
            case .tlsFailure(let detail):
                "The server's TLS certificate was rejected: \(detail)"
            case .timedOut:
                "The server did not respond in time."
            case .offline:
                "This device appears to be offline."
            case .cancelled:
                "The request was cancelled."
            case .other(let detail):
                detail
            }
        }
    }
}

extension APIError {
    /// The message to show the user. Always the server's own words when it
    /// supplied any — never a bare status code.
    var userMessage: String {
        switch self {
        case .transport(let failure): failure.message
        case .decoding(let detail): "The server sent a response this app could not read. \(detail)"
        case .unexpectedStatus(let code, let message): message ?? "Unexpected response (HTTP \(code))."
        case .badRequest(let m), .unauthorized(let m), .productionPasswordRequired(let m),
             .notFound(let m), .conflict(let m), .notImplemented(let m):
            m
        case .server(let code, let m):
            m.isEmpty ? "The server failed to handle the request (HTTP \(code))." : m
        }
    }

    /// A short heading to pair with `userMessage`.
    var title: String {
        switch self {
        case .transport: "Cannot reach the server"
        case .decoding: "Unreadable response"
        case .unexpectedStatus: "Unexpected response"
        case .badRequest: "That request was rejected"
        case .unauthorized: "Sign in again"
        case .productionPasswordRequired: "Production password"
        case .notFound: "Not found"
        case .conflict: "Blocked"
        case .notImplemented: "Not available"
        case .server: "Server error"
        }
    }

    /// The token is bad, so the UI must send the operator back to the token
    /// screen rather than showing a toast they can dismiss and forget.
    var requiresReauthentication: Bool {
        if case .unauthorized = self { return true }
        return false
    }

    /// The operator can fix this by supplying the production password.
    var requiresProductionPassword: Bool {
        if case .productionPasswordRequired = self { return true }
        return false
    }

    /// The user cancelled or navigated away; nothing to report.
    var isCancellation: Bool {
        if case .transport(.cancelled) = self { return true }
        return false
    }

    /// A retry might plausibly succeed without the user changing anything.
    var isRetryable: Bool {
        switch self {
        case .transport(.timedOut), .transport(.offline), .transport(.cannotReachHost): true
        case .server: true
        default: false
        }
    }

    /// Build the right case from a status code and the server's decoded error
    /// message. This is the single place the status contract lives.
    static func from(status: Int, message: String?, host: String = "the server") -> APIError {
        let text = message?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let fallback = text.isEmpty ? "The server returned HTTP \(status)." : text
        switch status {
        case 400: return .badRequest(fallback)
        case 401: return .unauthorized(text.isEmpty ? "The API token was rejected." : text)
        case 403: return .productionPasswordRequired(
            text.isEmpty ? "A production password is required for this action." : text
        )
        case 404: return .notFound(text.isEmpty ? "Not found on \(host)." : text)
        case 409: return .conflict(fallback)
        case 501: return .notImplemented(
            text.isEmpty ? "This server does not have that feature configured." : text
        )
        case 500...599: return .server(status, text)
        default: return .unexpectedStatus(status, message: text.isEmpty ? nil : text)
        }
    }

    /// Translate a `URLError` into the transport case that best explains it.
    static func from(urlError error: URLError, host: String) -> APIError {
        switch error.code {
        case .cancelled:
            .transport(.cancelled)
        case .notConnectedToInternet, .dataNotAllowed:
            .transport(.offline)
        case .timedOut:
            .transport(.timedOut)
        case .cannotFindHost, .cannotConnectToHost, .dnsLookupFailed, .networkConnectionLost:
            .transport(.cannotReachHost(host))
        case .secureConnectionFailed, .serverCertificateUntrusted,
             .serverCertificateHasBadDate, .serverCertificateHasUnknownRoot,
             .serverCertificateNotYetValid, .clientCertificateRejected,
             .clientCertificateRequired, .appTransportSecurityRequiresSecureConnection:
            .transport(.tlsFailure(error.localizedDescription))
        default:
            .transport(.other(error.localizedDescription))
        }
    }
}

/// The server's error body: `{"error": "..."}`.
struct APIErrorBody: Decodable, Sendable {
    let error: String
}
