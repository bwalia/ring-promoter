import Foundation
import Testing

@testable import RingPromoter

/// The client's wire behaviour, driven by a stubbed `URLProtocol`.
///
/// No network: the stub answers from canned bytes, so these run offline and
/// deterministically.
///
/// Serialized because `URLProtocol` subclasses are registered process-wide:
/// running these in parallel would have them swapping each other's handlers.
@Suite("RingPromoterClient", .serialized)
struct ClientTests {
    private let baseURL = URL(string: "https://ring-promoter.example.com")!

    private func makeClient(
        _ handler: @escaping @Sendable (URLRequest) throws -> (HTTPURLResponse, Data)
    ) -> RingPromoterClient {
        StubURLProtocol.setHandler(handler)
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [StubURLProtocol.self]
        return RingPromoterClient(
            baseURL: baseURL, token: "test-token", session: URLSession(configuration: config)
        )
    }

    private func response(_ status: Int, url: URL) -> HTTPURLResponse {
        HTTPURLResponse(url: url, statusCode: status, httpVersion: nil, headerFields: nil)!
    }

    // MARK: - Requests

    @Test("every /api call carries the bearer token")
    func bearerToken() async throws {
        let captured = RequestRecorder()
        let client = makeClient { request in
            captured.record(request)
            return (self.response(200, url: request.url!), try FixtureLoader.data("apps"))
        }
        _ = try await client.apps()

        let request = try #require(captured.last)
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer test-token")
    }

    @Test("healthz is sent unauthenticated")
    func healthzIsUnauthenticated() async throws {
        let captured = RequestRecorder()
        let client = makeClient { request in
            captured.record(request)
            return (self.response(200, url: request.url!), Data(#"{"status":"ok"}"#.utf8))
        }
        try await client.checkHealth()

        let request = try #require(captured.last)
        #expect(request.url?.path == "/healthz")
        // The token must not leak to an endpoint that does not need it.
        #expect(request.value(forHTTPHeaderField: "Authorization") == nil)
    }

    @Test("every mutating call takes the async path")
    func actionsAreAsync() async throws {
        let captured = RequestRecorder()
        let client = makeClient { request in
            captured.record(request)
            return (self.response(202, url: request.url!), Data(#"{"job_id":"job-7"}"#.utf8))
        }

        _ = try await client.seed(
            app: "web-frontend", ring: "int", version: "1.0", crCode: nil, password: nil
        )
        #expect(captured.last?.url?.query == "async=1")

        _ = try await client.promote(
            app: "web-frontend", fromRing: "int", crCode: nil, password: nil
        )
        #expect(captured.last?.url?.query == "async=1")

        _ = try await client.rollback(app: "web-frontend", ring: "int")
        #expect(captured.last?.url?.query == "async=1")
    }

    @Test("a 202 yields the job id the app then polls")
    func jobHandle() async throws {
        let client = makeClient { request in
            (self.response(202, url: request.url!), Data(#"{"job_id":"job-42"}"#.utf8))
        }
        let id = try await client.promote(
            app: "web-frontend", fromRing: "int", crCode: nil, password: nil
        )
        #expect(id == "job-42")
    }

    @Test("omitted optionals are absent from the body, not sent as null")
    func optionalsAreOmitted() async throws {
        // The server decodes with DisallowUnknownFields and reads "" as a wrong
        // password, so an untouched field must not be sent at all.
        let captured = RequestRecorder()
        let client = makeClient { request in
            captured.record(request)
            return (self.response(202, url: request.url!), Data(#"{"job_id":"j"}"#.utf8))
        }

        _ = try await client.promote(
            app: "app", fromRing: "int", crCode: nil, password: nil
        )
        let body = try #require(captured.lastBody)
        let json = try #require(
            try JSONSerialization.jsonObject(with: body) as? [String: Any]
        )
        #expect(json["from_ring"] as? String == "int")
        #expect(json["password"] == nil)
        #expect(json["cr_code"] == nil)
        #expect(json.count == 1)
    }

    @Test("a whitespace-only password is treated as absent")
    func blankPasswordOmitted() async throws {
        let captured = RequestRecorder()
        let client = makeClient { request in
            captured.record(request)
            return (self.response(202, url: request.url!), Data(#"{"job_id":"j"}"#.utf8))
        }
        _ = try await client.promote(
            app: "app", fromRing: "acc", crCode: "  ", password: "   "
        )
        let body = try #require(captured.lastBody)
        let json = try #require(
            try JSONSerialization.jsonObject(with: body) as? [String: Any]
        )
        #expect(json["password"] == nil)
        #expect(json["cr_code"] == nil)
    }

    @Test("rollback never sends a password")
    func rollbackSendsNoPassword() async throws {
        // There is no parameter for one, and there must not be: the server
        // exempts rollback so incident response is never gated.
        let captured = RequestRecorder()
        let client = makeClient { request in
            captured.record(request)
            return (self.response(202, url: request.url!), Data(#"{"job_id":"j"}"#.utf8))
        }
        _ = try await client.rollback(app: "app", ring: "prod")
        let body = try #require(captured.lastBody)
        let json = try #require(
            try JSONSerialization.jsonObject(with: body) as? [String: Any]
        )
        #expect(json["ring"] as? String == "prod")
        #expect(json.count == 1)
    }

    @Test("app names with awkward characters are escaped into the path")
    func pathEscaping() async throws {
        let captured = RequestRecorder()
        let client = makeClient { request in
            captured.record(request)
            return (self.response(200, url: request.url!), Data(#"{"rings":[]}"#.utf8))
        }
        _ = try await client.rings(app: "team/app name")
        let path = try #require(captured.last?.url?.absoluteString)
        #expect(!path.contains(" "))
        #expect(path.contains("/api/apps/"))
    }

    // MARK: - Responses

    @Test("the server's error message is decoded and surfaced")
    func errorBodyIsDecoded() async throws {
        let client = makeClient { request in
            (
                self.response(409, url: request.url!),
                try FixtureLoader.data("error-409-window-closed")
            )
        }
        await #expect(throws: APIError.self) {
            _ = try await client.promote(
                app: "payments-api", fromRing: "acc", crCode: "test", password: "x"
            )
        }
        do {
            _ = try await client.promote(
                app: "payments-api", fromRing: "acc", crCode: "test", password: "x"
            )
            Issue.record("expected a conflict")
        } catch {
            guard case .conflict(let message) = error else {
                Issue.record("expected .conflict, got \(error)")
                return
            }
            #expect(message.contains("maintenance window closed"))
        }
    }

    @Test("a non-JSON failure body is still shown, truncated")
    func nonJSONErrorBody() async throws {
        // A proxy in front of the control plane can return HTML.
        let html = String(repeating: "<html>502 Bad Gateway</html>", count: 40)
        let client = makeClient { request in
            (self.response(502, url: request.url!), Data(html.utf8))
        }
        do {
            _ = try await client.apps()
            Issue.record("expected a server error")
        } catch {
            #expect(error.userMessage.contains("502 Bad Gateway"))
            // Never a wall of markup in an alert.
            #expect(error.userMessage.count <= 301)
        }
    }

    @Test("a malformed success body becomes a decoding error, not a crash")
    func malformedBody() async throws {
        let client = makeClient { request in
            (self.response(200, url: request.url!), Data(#"{"apps": "not an array"}"#.utf8))
        }
        do {
            _ = try await client.apps()
            Issue.record("expected a decoding error")
        } catch {
            guard case .decoding(let detail) = error else {
                Issue.record("expected .decoding, got \(error)")
                return
            }
            #expect(!detail.isEmpty)
        }
    }

    @Test("a transport failure becomes a transport error naming the host")
    func transportFailure() async throws {
        let client = makeClient { _ in throw URLError(.cannotFindHost) }
        do {
            _ = try await client.apps()
            Issue.record("expected a transport error")
        } catch {
            #expect(error.userMessage.contains("ring-promoter.example.com"))
        }
    }

    @Test("captured fixtures round-trip through the client")
    func endToEndDecoding() async throws {
        let client = makeClient { request in
            let path = request.url!.path
            let fixture: String =
                if path.hasSuffix("/rings") { "rings" }
                else if path.hasSuffix("/history") { "history" }
                else if path.hasSuffix("/signoffs") { "signoffs" }
                else if path.hasSuffix("/maintenance-windows") { "maintenance-gated" }
                else if path == "/api/groups" { "groups" }
                else if path == "/api/jobs" { "jobs" }
                else { "apps" }
            return (self.response(200, url: request.url!), try FixtureLoader.data(fixture))
        }

        #expect(try await client.rings(app: "web-frontend").count == 4)
        #expect(try await !client.history(app: "web-frontend").isEmpty)
        #expect(try await client.signoffs(app: "payments-api").count == 4)
        #expect(try await client.maintenance(app: "payments-api").gated)
        #expect(try await client.groups().count == 2)
        #expect(try await !client.recentJobs().isEmpty)
    }
}

// MARK: - Test doubles

/// Collects the requests the client made, for assertions about the wire format.
///
/// A class with a lock because `URLProtocol` hands work to its own queue.
final class RequestRecorder: @unchecked Sendable {
    private let lock = NSLock()
    private var requests: [(URLRequest, Data?)] = []

    func record(_ request: URLRequest) {
        lock.lock()
        defer { lock.unlock() }
        // URLProtocol strips httpBody into a stream, so read it back.
        requests.append((request, request.httpBody ?? request.bodyStreamData))
    }

    var last: URLRequest? {
        lock.lock()
        defer { lock.unlock() }
        return requests.last?.0
    }

    var lastBody: Data? {
        lock.lock()
        defer { lock.unlock() }
        return requests.last?.1
    }
}

extension URLRequest {
    /// `URLSession` converts `httpBody` to `httpBodyStream` before a protocol
    /// sees it, so the bytes have to be read back off the stream.
    var bodyStreamData: Data? {
        guard let stream = httpBodyStream else { return nil }
        stream.open()
        defer { stream.close() }
        var data = Data()
        let size = 4_096
        let buffer = UnsafeMutablePointer<UInt8>.allocate(capacity: size)
        defer { buffer.deallocate() }
        while stream.hasBytesAvailable {
            let read = stream.read(buffer, maxLength: size)
            if read <= 0 { break }
            data.append(buffer, count: read)
        }
        return data
    }
}

/// Answers requests from a handler instead of the network.
final class StubURLProtocol: URLProtocol {
    private nonisolated(unsafe) static var handler:
        (@Sendable (URLRequest) throws -> (HTTPURLResponse, Data))?
    private static let lock = NSLock()

    static func setHandler(
        _ handler: @escaping @Sendable (URLRequest) throws -> (HTTPURLResponse, Data)
    ) {
        lock.lock()
        defer { lock.unlock() }
        Self.handler = handler
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        Self.lock.lock()
        let handler = Self.handler
        Self.lock.unlock()

        guard let handler else {
            client?.urlProtocol(self, didFailWithError: URLError(.unknown))
            return
        }
        do {
            let (response, data) = try handler(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}
