import Foundation

/// Validates a URL + token pair before the app commits to them.
///
/// Two calls, deliberately in this order, because they answer two different
/// questions and an operator needs to know which one failed:
///
/// 1. `GET /healthz` — is there a Ring Promoter at this URL at all?
/// 2. `GET /api/apps` — does this token work?
///
/// Collapsing them would turn a typo'd hostname and a stale token into the same
/// unhelpful "could not connect".
struct ConnectionValidator: Sendable {
    /// What went wrong, phrased so the operator knows which field to fix.
    enum Failure: Error, Equatable {
        case invalidURL
        case unreachable(String)
        case notRingPromoter(String)
        case tokenRejected(String)
        case other(String)

        var message: String {
            switch self {
            case .invalidURL:
                "That is not a usable server address. Try something like https://ring-promoter.example.com"
            case .unreachable(let detail), .other(let detail):
                detail
            case .notRingPromoter(let detail):
                "Reached the server, but it did not answer like a Ring Promoter instance. \(detail)"
            case .tokenRejected(let detail):
                detail
            }
        }

        /// Which field the operator should correct.
        var field: Field {
            switch self {
            case .invalidURL, .unreachable, .notRingPromoter: .url
            case .tokenRejected: .token
            case .other: .url
            }
        }

        enum Field { case url, token }
    }

    /// Injected so tests validate against a fake client with no network.
    let makeClient: @Sendable (URL, String) -> any RingPromoterAPI

    init(makeClient: @escaping @Sendable (URL, String) -> any RingPromoterAPI) {
        self.makeClient = makeClient
    }

    /// Live default: a real client on a short-timeout session, because an
    /// operator typing a URL should not wait 60 seconds to learn it is wrong.
    static let live = ConnectionValidator { url, token in
        RingPromoterClient(
            baseURL: url, token: token, session: RingPromoterClient.makeSession(timeout: 12)
        )
    }

    /// Returns what the server said about itself, so onboarding can show the
    /// app count as confirmation the operator reached the right cluster.
    func validate(url: URL, token: String) async -> Result<AppsResponse, Failure> {
        let client = makeClient(url, token)
        do {
            try await client.checkHealth()
        } catch {
            return .failure(healthFailure(error))
        }
        do {
            return .success(try await client.apps())
        } catch {
            if case .unauthorized(let message) = error {
                return .failure(.tokenRejected(message))
            }
            return .failure(.other(error.userMessage))
        }
    }

    /// A reachable host that is not a Ring Promoter answers `/healthz` with
    /// something other than 200 — worth saying plainly, because pointing the
    /// app at a company homepage is an easy mistake.
    private func healthFailure(_ error: APIError) -> Failure {
        switch error {
        case .transport(let detail):
            .unreachable(detail.message)
        case .notFound, .unexpectedStatus, .decoding:
            .notRingPromoter(error.userMessage)
        default:
            .unreachable(error.userMessage)
        }
    }
}
