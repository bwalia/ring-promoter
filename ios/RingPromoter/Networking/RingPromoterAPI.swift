import Foundation

/// The Ring Promoter REST API, as the app uses it.
///
/// Protocol-based so previews, unit tests and demo mode run against
/// `FixtureClient` with no network at all.
///
/// Every mutating call here takes the **async** path (`?async=1`): the server
/// answers 202 with a job id and the app polls for progress. A phone on a flaky
/// network must never hold a long synchronous request open.
protocol RingPromoterAPI: Sendable {
    // MARK: Unauthenticated

    /// `GET /healthz`. Used to tell "wrong URL" apart from "wrong token" during
    /// onboarding.
    func checkHealth() async throws(APIError)

    /// `GET /version` — build metadata for Settings.
    func serverVersion() async throws(APIError) -> ServerVersion

    // MARK: Reads

    func apps() async throws(APIError) -> AppsResponse
    func rings(app: String) async throws(APIError) -> [RingStatus]
    func history(app: String) async throws(APIError) -> [HistoryEntry]
    func versions(app: String) async throws(APIError) -> VersionsResponse
    func job(app: String, id: String) async throws(APIError) -> Job
    /// `GET /api/jobs` — the newest job per app, shared by all operators.
    func recentJobs() async throws(APIError) -> [Job]

    // MARK: Actions (always async on the wire)

    /// `POST /api/apps/{app}/seed?async=1`
    func seed(
        app: String, ring: String, version: String, crCode: String?, password: String?
    ) async throws(APIError) -> String

    /// `POST /api/apps/{app}/promote?async=1`
    func promote(
        app: String, fromRing: String, crCode: String?, password: String?
    ) async throws(APIError) -> String

    /// `POST /api/apps/{app}/rollback?async=1`. Never carries a password —
    /// the server exempts rollback so incident response is not gated.
    func rollback(app: String, ring: String) async throws(APIError) -> String

    /// `PUT /api/apps/{app}/rings/{ring}/auto-promote`
    func setAutoPromote(
        app: String, ring: String, enabled: Bool, password: String?
    ) async throws(APIError)

    // MARK: Gates

    func maintenance(app: String) async throws(APIError) -> MaintenanceStatus
    func openMaintenanceWindow(
        app: String, window: NewMaintenanceWindow
    ) async throws(APIError) -> MaintenanceWindow
    func closeMaintenanceWindow(app: String, id: String) async throws(APIError)

    func signoffs(app: String) async throws(APIError) -> [Signoff]
    func recordSignoff(app: String, signoff: NewSignoff) async throws(APIError) -> Signoff

    // MARK: Groups

    func groups() async throws(APIError) -> [AppGroup]
    func createGroup(name: String, apps: [String]) async throws(APIError) -> AppGroup
    func updateGroup(id: String, name: String, apps: [String]) async throws(APIError) -> AppGroup
    func deleteGroup(id: String) async throws(APIError)

    // MARK: AI diagnosis

    /// `POST /api/apps/{app}/jobs/{id}/diagnose`. Returns immediately; the
    /// answer arrives on the polled job.
    func diagnoseJob(app: String, id: String) async throws(APIError) -> DiagnosisResponse
    func diagnoseHistoryEntry(app: String, id: Int64) async throws(APIError) -> DiagnosisResponse
    func historyDiagnosis(app: String, id: Int64) async throws(APIError) -> DiagnosisResponse
}
