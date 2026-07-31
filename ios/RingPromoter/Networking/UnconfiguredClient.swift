import Foundation

/// The client used before an operator has connected to anything.
///
/// It refuses every call with `.unauthorized`, which is both cheap and honest:
/// the alternative — defaulting to `DemoClient` — would decode the whole
/// fixture corpus at every launch, and would quietly serve *demo data* to any
/// screen that asked before a real connection existed. A screen that renders
/// invented pipelines while claiming to be connected to production is precisely
/// the failure this app cannot afford.
struct UnconfiguredClient: RingPromoterAPI {
    private var notConnected: APIError {
        .unauthorized("Not connected to a control plane yet.")
    }

    func checkHealth() async throws(APIError) { throw notConnected }
    func serverVersion() async throws(APIError) -> ServerVersion { throw notConnected }
    func apps() async throws(APIError) -> AppsResponse { throw notConnected }
    func rings(app: String) async throws(APIError) -> [RingStatus] { throw notConnected }
    func history(app: String) async throws(APIError) -> [HistoryEntry] { throw notConnected }
    func versions(app: String) async throws(APIError) -> VersionsResponse { throw notConnected }
    func job(app: String, id: String) async throws(APIError) -> Job { throw notConnected }
    func recentJobs() async throws(APIError) -> [Job] { throw notConnected }

    func seed(
        app: String, ring: String, version: String, crCode: String?, password: String?
    ) async throws(APIError) -> String { throw notConnected }

    func promote(
        app: String, fromRing: String, crCode: String?, password: String?
    ) async throws(APIError) -> String { throw notConnected }

    func rollback(app: String, ring: String) async throws(APIError) -> String { throw notConnected }

    func setAutoPromote(
        app: String, ring: String, enabled: Bool, password: String?
    ) async throws(APIError) { throw notConnected }

    func maintenance(app: String) async throws(APIError) -> MaintenanceStatus { throw notConnected }

    func openMaintenanceWindow(
        app: String, window: NewMaintenanceWindow
    ) async throws(APIError) -> MaintenanceWindow { throw notConnected }

    func closeMaintenanceWindow(app: String, id: String) async throws(APIError) {
        throw notConnected
    }

    func signoffs(app: String) async throws(APIError) -> [Signoff] { throw notConnected }

    func recordSignoff(
        app: String, signoff: NewSignoff
    ) async throws(APIError) -> Signoff { throw notConnected }

    func groups() async throws(APIError) -> [AppGroup] { throw notConnected }

    func createGroup(
        name: String, apps: [String]
    ) async throws(APIError) -> AppGroup { throw notConnected }

    func updateGroup(
        id: String, name: String, apps: [String]
    ) async throws(APIError) -> AppGroup { throw notConnected }

    func deleteGroup(id: String) async throws(APIError) { throw notConnected }

    func diagnoseJob(
        app: String, id: String
    ) async throws(APIError) -> DiagnosisResponse { throw notConnected }

    func diagnoseHistoryEntry(
        app: String, id: Int64
    ) async throws(APIError) -> DiagnosisResponse { throw notConnected }

    func historyDiagnosis(
        app: String, id: Int64
    ) async throws(APIError) -> DiagnosisResponse { throw notConnected }
}
