package org.fictionally.ringpromoter.net

import org.fictionally.ringpromoter.model.AppGroup
import org.fictionally.ringpromoter.model.AppsResponse
import org.fictionally.ringpromoter.model.DiagnosisResponse
import org.fictionally.ringpromoter.model.HistoryEntry
import org.fictionally.ringpromoter.model.Job
import org.fictionally.ringpromoter.model.MaintenanceStatus
import org.fictionally.ringpromoter.model.MaintenanceWindow
import org.fictionally.ringpromoter.model.NewMaintenanceWindow
import org.fictionally.ringpromoter.model.NewSignoff
import org.fictionally.ringpromoter.model.RingStatus
import org.fictionally.ringpromoter.model.ServerVersion
import org.fictionally.ringpromoter.model.Signoff
import org.fictionally.ringpromoter.model.TestKit
import org.fictionally.ringpromoter.model.VersionsResponse

/**
 * The Ring Promoter REST API, as the app uses it.
 *
 * An interface so previews, unit tests and demo mode run against a fixture
 * client with no network at all.
 *
 * Every mutating call here takes the **async** path (`?async=1`): the server
 * answers 202 with a job id and the app polls for progress. A phone on a flaky
 * network must never hold a long synchronous request open.
 *
 * Every method throws [ApiError] and nothing else. No caller ever sees a status
 * code or an OkHttp type.
 */
interface RingPromoterApi {

    // Unauthenticated

    /**
     * `GET /healthz`. Used to tell "wrong URL" apart from "wrong token" during
     * onboarding.
     */
    suspend fun checkHealth()

    /** `GET /version` — build metadata for Settings. */
    suspend fun serverVersion(): ServerVersion

    // Reads

    suspend fun apps(): AppsResponse

    suspend fun rings(app: String): List<RingStatus>

    suspend fun history(app: String): List<HistoryEntry>

    suspend fun versions(app: String): VersionsResponse

    suspend fun job(app: String, id: String): Job

    /** `GET /api/jobs` — the newest job per app, shared by all operators. */
    suspend fun recentJobs(): List<Job>

    // Actions (always async on the wire)

    /** `POST /api/apps/{app}/seed?async=1` → the job id. */
    suspend fun seed(
        app: String,
        ring: String,
        version: String,
        crCode: String?,
        password: String?,
    ): String

    /** `POST /api/apps/{app}/promote?async=1` → the job id. */
    suspend fun promote(
        app: String,
        fromRing: String,
        crCode: String?,
        password: String?,
    ): String

    /**
     * `POST /api/apps/{app}/rollback?async=1`. Never carries a password — the
     * server exempts rollback so incident response is not gated.
     */
    suspend fun rollback(app: String, ring: String): String

    /** `PUT /api/apps/{app}/rings/{ring}/auto-promote` */
    suspend fun setAutoPromote(app: String, ring: String, enabled: Boolean, password: String?)

    // Gates

    suspend fun maintenance(app: String): MaintenanceStatus

    suspend fun openMaintenanceWindow(
        app: String,
        window: NewMaintenanceWindow,
    ): MaintenanceWindow

    suspend fun closeMaintenanceWindow(app: String, id: String)

    suspend fun signoffs(app: String): List<Signoff>

    suspend fun recordSignoff(app: String, signoff: NewSignoff): Signoff

    // Groups

    suspend fun groups(): List<AppGroup>

    suspend fun createGroup(name: String, apps: List<String>): AppGroup

    suspend fun updateGroup(id: String, name: String, apps: List<String>): AppGroup

    suspend fun deleteGroup(id: String)

    // AI diagnosis

    /**
     * `POST /api/apps/{app}/jobs/{id}/diagnose`. Returns immediately; the
     * answer arrives on the polled job.
     */
    suspend fun diagnoseJob(app: String, id: String): DiagnosisResponse

    suspend fun diagnoseHistoryEntry(app: String, id: Long): DiagnosisResponse

    suspend fun historyDiagnosis(app: String, id: Long): DiagnosisResponse

    // Test kits

    /** Where to test a ring's current version, plus its AI test plan. */
    suspend fun testKit(app: String, ring: String): TestKit

    /**
     * Ask the AI what to test. Runs server-side: a `running` status means poll
     * [testKit] until it resolves. [refresh] regenerates a stored plan.
     */
    suspend fun planTestKit(app: String, ring: String, refresh: Boolean): TestKit
}
