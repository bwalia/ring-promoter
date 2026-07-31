import Foundation
import Testing

@testable import RingPromoter

/// Every status the backend can return maps to exactly one `APIError` case, and
/// the server's own message survives the trip.
///
/// The messages here are the real ones, taken from the captured error corpus —
/// so if the Go side rewords an error, the app still shows what it said.
@Suite("HTTP status → APIError mapping")
struct APIErrorMappingTests {

    @Test(
        "each status maps to its own case",
        arguments: [
            (400, "version must not be empty"),
            (401, "missing or invalid bearer token"),
            (403, "production password required"),
            (404, "application not found"),
            (409, "source ring has no version to promote"),
            (501, "AI diagnosis is not configured on this server"),
        ]
    )
    func statusMapping(status: Int, message: String) {
        let error = APIError.from(status: status, message: message)
        // The server's words are never replaced by a generic string.
        #expect(error.userMessage == message)

        switch (status, error) {
        case (400, .badRequest), (401, .unauthorized), (403, .productionPasswordRequired),
             (404, .notFound), (409, .conflict), (501, .notImplemented):
            break
        default:
            Issue.record("HTTP \(status) mapped to the wrong case: \(error)")
        }
    }

    @Test("only 401 sends the operator back to the token screen")
    func reauthentication() {
        #expect(APIError.from(status: 401, message: "bad token").requiresReauthentication)
        for status in [400, 403, 404, 409, 422, 500, 501] {
            #expect(
                !APIError.from(status: status, message: "x").requiresReauthentication,
                "HTTP \(status) must not trigger re-authentication"
            )
        }
    }

    @Test("only 403 asks for the production password")
    func productionPassword() {
        #expect(APIError.from(status: 403, message: "x").requiresProductionPassword)
        #expect(!APIError.from(status: 409, message: "x").requiresProductionPassword)
    }

    @Test("5xx becomes a server error carrying its code")
    func serverErrors() {
        let error = APIError.from(status: 503, message: "")
        guard case .server(let code, _) = error else {
            Issue.record("503 should map to .server, got \(error)")
            return
        }
        #expect(code == 503)
        #expect(error.isRetryable)
    }

    @Test("an unmapped status still surfaces its code rather than nothing")
    func unexpectedStatus() {
        let error = APIError.from(status: 418, message: nil)
        guard case .unexpectedStatus(let code, _) = error else {
            Issue.record("418 should map to .unexpectedStatus, got \(error)")
            return
        }
        #expect(code == 418)
        #expect(error.userMessage.contains("418"))
    }

    @Test("an empty error body still produces something a person can read")
    func emptyBody() {
        for status in [400, 401, 403, 404, 409, 501] {
            let message = APIError.from(status: status, message: "").userMessage
            #expect(!message.isEmpty, "HTTP \(status) produced an empty message")
            // Never a bare status code with nothing else.
            #expect(message != "\(status)")
        }
    }

    @Test(
        "URLError maps to the transport failure that explains it",
        arguments: [
            (URLError.Code.cannotFindHost, "reach"),
            (URLError.Code.timedOut, "did not respond"),
            (URLError.Code.notConnectedToInternet, "offline"),
            (URLError.Code.secureConnectionFailed, "TLS"),
        ]
    )
    func transportMapping(code: URLError.Code, expectedFragment: String) {
        let error = APIError.from(urlError: URLError(code), host: "ring-promoter.example.com")
        #expect(
            error.userMessage.localizedCaseInsensitiveContains(expectedFragment),
            "\(code) produced: \(error.userMessage)"
        )
    }

    @Test("a cancelled request is recognised so it is not reported as a failure")
    func cancellation() {
        let error = APIError.from(urlError: URLError(.cancelled), host: "example.com")
        #expect(error.isCancellation)
        #expect(!error.isRetryable)
    }

    @Test("only failures a retry could fix are marked retryable")
    func retryability() {
        #expect(APIError.from(urlError: URLError(.timedOut), host: "h").isRetryable)
        #expect(APIError.from(status: 500, message: "").isRetryable)
        // A gate, a bad request or a bad token will not fix itself.
        #expect(!APIError.from(status: 409, message: "gate shut").isRetryable)
        #expect(!APIError.from(status: 400, message: "bad").isRetryable)
        #expect(!APIError.from(status: 401, message: "bad").isRetryable)
    }

    @Test("the captured error bodies each map to the case the UI expects")
    func capturedCorpus() throws {
        // (fixture, status, predicate on the mapped error)
        let cases: [(String, Int, (APIError) -> Bool)] = [
            ("error-401", 401, { $0.requiresReauthentication }),
            ("error-403-prod-password", 403, { $0.requiresProductionPassword }),
            ("error-403-wrong-password", 403, { $0.requiresProductionPassword }),
            ("error-404-app", 404, { if case .notFound = $0 { true } else { false } }),
            ("error-409-nothing-to-promote", 409, { if case .conflict = $0 { true } else { false } }),
            ("error-409-window-closed", 409, { if case .conflict = $0 { true } else { false } }),
            ("error-409-signoff-required", 409, { if case .conflict = $0 { true } else { false } }),
            ("error-400-cr-required", 400, { if case .badRequest = $0 { true } else { false } }),
            ("error-400-empty-version", 400, { if case .badRequest = $0 { true } else { false } }),
            ("error-501-diagnose", 501, { if case .notImplemented = $0 { true } else { false } }),
            ("autopromote-409-managed", 409, { if case .conflict = $0 { true } else { false } }),
        ]

        for (fixture, status, matches) in cases {
            let body = try FixtureLoader.decode(APIErrorBody.self, from: fixture)
            let error = APIError.from(status: status, message: body.error)
            #expect(matches(error), "\(fixture) mapped unexpectedly: \(error)")
            #expect(error.userMessage == body.error, "\(fixture) lost the server's message")
        }
    }
}
