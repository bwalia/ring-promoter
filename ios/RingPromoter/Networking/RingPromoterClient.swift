import Foundation

/// The live API client.
///
/// An actor because it owns the mutable pieces — base URL, bearer token,
/// `URLSession` — that every screen touches concurrently. It is the single
/// place that knows about HTTP: token injection, JSON coding, and the
/// status-code → `APIError` mapping all live here, so no view or view model
/// ever sees a status code.
///
/// Nothing here logs a token, a production password, or a request body.
actor RingPromoterClient: RingPromoterAPI {
    private let baseURL: URL
    private let token: String
    private let session: URLSession
    private let decoder = JSONCoding.makeDecoder()
    private let encoder = JSONCoding.makeEncoder()

    /// - Parameters:
    ///   - baseURL: the operator-supplied control-plane URL.
    ///   - token: the bearer token. Held in memory only; it is read from the
    ///     Keychain by the caller and never written to disk here.
    ///   - session: injectable so tests can drive a stub protocol.
    init(baseURL: URL, token: String, session: URLSession = .shared) {
        self.baseURL = baseURL
        self.token = token
        self.session = session
    }

    /// A session with timeouts that suit a phone: fail fast enough that an
    /// operator on a bad connection learns something is wrong, but not so fast
    /// that a slow control plane looks broken.
    static func makeSession(timeout: TimeInterval = 20) -> URLSession {
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = timeout
        config.timeoutIntervalForResource = 60
        config.waitsForConnectivity = false
        config.httpAdditionalHeaders = ["Accept": "application/json"]
        return URLSession(configuration: config)
    }

    private var host: String { baseURL.host() ?? baseURL.absoluteString }

    // MARK: - Unauthenticated

    func checkHealth() async throws(APIError) {
        _ = try await send(Request(path: "/healthz", authenticated: false))
    }

    func serverVersion() async throws(APIError) -> ServerVersion {
        try await get(ServerVersion.self, path: "/version", authenticated: false)
    }

    // MARK: - Reads

    func apps() async throws(APIError) -> AppsResponse {
        try await get(AppsResponse.self, path: "/api/apps")
    }

    func rings(app: String) async throws(APIError) -> [RingStatus] {
        try await get(RingsResponse.self, path: "/api/apps/\(esc(app))/rings").rings
    }

    func history(app: String) async throws(APIError) -> [HistoryEntry] {
        try await get(HistoryResponse.self, path: "/api/apps/\(esc(app))/history").history
    }

    func versions(app: String) async throws(APIError) -> VersionsResponse {
        try await get(VersionsResponse.self, path: "/api/apps/\(esc(app))/versions")
    }

    func job(app: String, id: String) async throws(APIError) -> Job {
        try await get(Job.self, path: "/api/apps/\(esc(app))/jobs/\(esc(id))")
    }

    func recentJobs() async throws(APIError) -> [Job] {
        try await get(JobsResponse.self, path: "/api/jobs").jobs
    }

    // MARK: - Actions

    /// Bodies are built as explicit dictionaries so an optional that is nil is
    /// *omitted* rather than sent as null — the server decodes with
    /// `DisallowUnknownFields`, and an explicit null for `password` would be a
    /// different thing from not sending one.
    func seed(
        app: String, ring: String, version: String, crCode: String?, password: String?
    ) async throws(APIError) -> String {
        var body: [String: String] = ["ring": ring, "version": version]
        body["cr_code"] = nonEmpty(crCode)
        body["password"] = nonEmpty(password)
        return try await startJob(path: "/api/apps/\(esc(app))/seed", body: body)
    }

    func promote(
        app: String, fromRing: String, crCode: String?, password: String?
    ) async throws(APIError) -> String {
        var body: [String: String] = ["from_ring": fromRing]
        body["cr_code"] = nonEmpty(crCode)
        body["password"] = nonEmpty(password)
        return try await startJob(path: "/api/apps/\(esc(app))/promote", body: body)
    }

    /// No password parameter exists by design: the server exempts rollback, and
    /// offering one here would imply incident response can be gated.
    func rollback(app: String, ring: String) async throws(APIError) -> String {
        try await startJob(path: "/api/apps/\(esc(app))/rollback", body: ["ring": ring])
    }

    func setAutoPromote(
        app: String, ring: String, enabled: Bool, password: String?
    ) async throws(APIError) {
        var body: [String: AnyEncodableValue] = ["enabled": .bool(enabled)]
        if let password = nonEmpty(password) { body["password"] = .string(password) }
        _ = try await send(
            Request(
                path: "/api/apps/\(esc(app))/rings/\(esc(ring))/auto-promote",
                method: "PUT",
                body: try encodeBody(body)
            )
        )
    }

    // MARK: - Gates

    func maintenance(app: String) async throws(APIError) -> MaintenanceStatus {
        try await get(MaintenanceStatus.self, path: "/api/apps/\(esc(app))/maintenance-windows")
    }

    func openMaintenanceWindow(
        app: String, window: NewMaintenanceWindow
    ) async throws(APIError) -> MaintenanceWindow {
        try await post(
            MaintenanceWindow.self,
            path: "/api/apps/\(esc(app))/maintenance-windows",
            body: try encodeBody(window)
        )
    }

    func closeMaintenanceWindow(app: String, id: String) async throws(APIError) {
        _ = try await send(
            Request(
                path: "/api/apps/\(esc(app))/maintenance-windows/\(esc(id))",
                method: "DELETE"
            )
        )
    }

    func signoffs(app: String) async throws(APIError) -> [Signoff] {
        try await get(SignoffsResponse.self, path: "/api/apps/\(esc(app))/signoffs").signoffs
    }

    func recordSignoff(app: String, signoff: NewSignoff) async throws(APIError) -> Signoff {
        try await post(
            Signoff.self,
            path: "/api/apps/\(esc(app))/signoffs",
            body: try encodeBody(signoff)
        )
    }

    // MARK: - Groups

    func groups() async throws(APIError) -> [AppGroup] {
        try await get(GroupsResponse.self, path: "/api/groups").groups
    }

    func createGroup(name: String, apps: [String]) async throws(APIError) -> AppGroup {
        try await post(
            AppGroup.self, path: "/api/groups",
            body: try encodeBody(GroupBody(name: name, apps: apps))
        )
    }

    func updateGroup(id: String, name: String, apps: [String]) async throws(APIError) -> AppGroup {
        let data = try await send(
            Request(
                path: "/api/groups/\(esc(id))", method: "PUT",
                body: try encodeBody(GroupBody(name: name, apps: apps))
            )
        )
        return try decode(AppGroup.self, from: data)
    }

    func deleteGroup(id: String) async throws(APIError) {
        _ = try await send(Request(path: "/api/groups/\(esc(id))", method: "DELETE"))
    }

    // MARK: - AI diagnosis

    func diagnoseJob(app: String, id: String) async throws(APIError) -> DiagnosisResponse {
        try await post(
            DiagnosisResponse.self,
            path: "/api/apps/\(esc(app))/jobs/\(esc(id))/diagnose", body: nil
        )
    }

    func diagnoseHistoryEntry(app: String, id: Int64) async throws(APIError) -> DiagnosisResponse {
        try await post(
            DiagnosisResponse.self,
            path: "/api/apps/\(esc(app))/history/\(id)/diagnose", body: nil
        )
    }

    func historyDiagnosis(app: String, id: Int64) async throws(APIError) -> DiagnosisResponse {
        try await get(
            DiagnosisResponse.self, path: "/api/apps/\(esc(app))/history/\(id)/diagnose"
        )
    }

    // MARK: - Plumbing

    private struct Request {
        var path: String
        var method: String = "GET"
        var body: Data?
        var query: [URLQueryItem] = []
        var authenticated: Bool = true
    }

    private struct GroupBody: Encodable {
        let name: String
        let apps: [String]
    }

    /// A tiny encodable box so a heterogeneous body (bool + string) can be
    /// built without dropping to `JSONSerialization`.
    private enum AnyEncodableValue: Encodable {
        case bool(Bool)
        case string(String)

        func encode(to encoder: any Encoder) throws {
            var container = encoder.singleValueContainer()
            switch self {
            case .bool(let value): try container.encode(value)
            case .string(let value): try container.encode(value)
            }
        }
    }

    /// Fires a mutating operation on the **async** path and returns the job id.
    ///
    /// The server answers 202 with `{"job_id": …}`. A 200 would mean the server
    /// ran it synchronously, which it only does when `?async=1` is absent — so
    /// that is treated as a protocol mismatch rather than silently swallowed.
    private func startJob(path: String, body: [String: String]) async throws(APIError) -> String {
        let data = try await send(
            Request(
                path: path, method: "POST",
                body: try encodeBody(body),
                query: [URLQueryItem(name: "async", value: "1")]
            )
        )
        return try decode(JobHandle.self, from: data).jobID
    }

    private func get<T: Decodable>(
        _ type: T.Type, path: String, authenticated: Bool = true
    ) async throws(APIError) -> T {
        let data = try await send(Request(path: path, authenticated: authenticated))
        return try decode(type, from: data)
    }

    private func post<T: Decodable>(
        _ type: T.Type, path: String, body: Data?
    ) async throws(APIError) -> T {
        let data = try await send(Request(path: path, method: "POST", body: body))
        return try decode(type, from: data)
    }

    private func encodeBody(_ value: some Encodable) throws(APIError) -> Data {
        do {
            return try encoder.encode(value)
        } catch {
            throw .decoding("Could not encode the request: \(error.localizedDescription)")
        }
    }

    private func decode<T: Decodable>(_ type: T.Type, from data: Data) throws(APIError) -> T {
        // A 200/202 with an empty body is valid for the endpoints that return
        // only a status object; callers that ignore the value pass through
        // `send` instead, so reaching here with no bytes is a real mismatch.
        do {
            return try decoder.decode(type, from: data)
        } catch let error as DecodingError {
            throw .decoding(Self.describe(error))
        } catch {
            throw .decoding(error.localizedDescription)
        }
    }

    /// Performs the request and maps the outcome. Returns the body bytes for
    /// any 2xx; throws the mapped `APIError` for everything else.
    private func send(_ request: Request) async throws(APIError) -> Data {
        let urlRequest = try makeURLRequest(request)
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: urlRequest)
        } catch let error as URLError {
            throw .from(urlError: error, host: host)
        } catch is CancellationError {
            throw .transport(.cancelled)
        } catch {
            throw .transport(.other(error.localizedDescription))
        }
        guard let http = response as? HTTPURLResponse else {
            throw .decoding("The server did not send an HTTP response.")
        }
        guard (200..<300).contains(http.statusCode) else {
            throw .from(
                status: http.statusCode, message: Self.serverMessage(in: data), host: host
            )
        }
        return data
    }

    private func makeURLRequest(_ request: Request) throws(APIError) -> URLRequest {
        guard
            var components = URLComponents(
                url: baseURL.appending(path: request.path), resolvingAgainstBaseURL: false
            )
        else {
            throw .transport(.other("\(baseURL.absoluteString) is not a usable server URL."))
        }
        if !request.query.isEmpty { components.queryItems = request.query }
        guard let url = components.url else {
            throw .transport(.other("Could not build a request URL for \(request.path)."))
        }
        var urlRequest = URLRequest(url: url)
        urlRequest.httpMethod = request.method
        urlRequest.httpBody = request.body
        if request.body != nil {
            urlRequest.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        if request.authenticated {
            urlRequest.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        return urlRequest
    }

    /// Pull `{"error": "..."}` out of a failure body. Falls back to the raw
    /// text when the body is not the expected shape (a proxy's HTML 502, say),
    /// truncated so an error alert cannot become a wall of markup.
    private static func serverMessage(in data: Data) -> String? {
        if let body = try? JSONDecoder().decode(APIErrorBody.self, from: data) {
            return body.error
        }
        guard
            let text = String(data: data, encoding: .utf8)?
                .trimmingCharacters(in: .whitespacesAndNewlines),
            !text.isEmpty
        else { return nil }
        return text.count > 300 ? String(text.prefix(300)) + "…" : text
    }

    /// Turn a `DecodingError` into something a bug report can act on, without
    /// echoing the payload.
    private static func describe(_ error: DecodingError) -> String {
        func path(_ context: DecodingError.Context) -> String {
            context.codingPath.map(\.stringValue).joined(separator: ".")
        }
        switch error {
        case .keyNotFound(let key, let context):
            return "Missing field \"\(key.stringValue)\" at \(path(context))."
        case .typeMismatch(let type, let context):
            return "Field \(path(context)) was not the expected \(type)."
        case .valueNotFound(let type, let context):
            return "Field \(path(context)) was null but a \(type) was required."
        case .dataCorrupted(let context):
            return context.debugDescription
        @unknown default:
            return "The response did not match the expected format."
        }
    }

    private func esc(_ component: String) -> String {
        component.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? component
    }

    /// Treat whitespace-only optional input as absent, so an untouched password
    /// field is never sent as `""` (which the server reads as "wrong").
    private func nonEmpty(_ value: String?) -> String? {
        guard let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines),
              !trimmed.isEmpty
        else { return nil }
        return trimmed
    }
}
