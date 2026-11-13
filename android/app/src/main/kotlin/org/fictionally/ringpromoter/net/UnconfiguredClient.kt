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
 * The client used before an operator has connected to anything.
 *
 * It refuses every call with [ApiError.Unauthorized], which is both cheap and
 * honest: the alternative — defaulting to a demo client — would load the whole
 * fixture corpus at every launch, and would quietly serve *demo data* to any
 * screen that asked before a real connection existed. A screen that renders
 * invented pipelines while claiming to be connected to production is precisely
 * the failure this app cannot afford.
 */
object UnconfiguredClient : RingPromoterApi {

    private val notConnected get() = ApiError.Unauthorized("Not connected to a cluster yet.")

    override suspend fun checkHealth(): Nothing = throw notConnected
    override suspend fun serverVersion(): ServerVersion = throw notConnected
    override suspend fun apps(): AppsResponse = throw notConnected
    override suspend fun rings(app: String): List<RingStatus> = throw notConnected
    override suspend fun history(app: String): List<HistoryEntry> = throw notConnected
    override suspend fun versions(app: String): VersionsResponse = throw notConnected
    override suspend fun job(app: String, id: String): Job = throw notConnected
    override suspend fun recentJobs(): List<Job> = throw notConnected

    override suspend fun seed(
        app: String,
        ring: String,
        version: String,
        crCode: String?,
        password: String?,
    ): String = throw notConnected

    override suspend fun promote(
        app: String,
        fromRing: String,
        crCode: String?,
        password: String?,
    ): String = throw notConnected

    override suspend fun rollback(app: String, ring: String): String = throw notConnected

    override suspend fun setAutoPromote(
        app: String,
        ring: String,
        enabled: Boolean,
        password: String?,
    ): Nothing = throw notConnected

    override suspend fun maintenance(app: String): MaintenanceStatus = throw notConnected

    override suspend fun openMaintenanceWindow(
        app: String,
        window: NewMaintenanceWindow,
    ): MaintenanceWindow = throw notConnected

    override suspend fun closeMaintenanceWindow(app: String, id: String): Nothing =
        throw notConnected

    override suspend fun signoffs(app: String): List<Signoff> = throw notConnected

    override suspend fun recordSignoff(app: String, signoff: NewSignoff): Signoff =
        throw notConnected

    override suspend fun groups(): List<AppGroup> = throw notConnected

    override suspend fun createGroup(name: String, apps: List<String>): AppGroup =
        throw notConnected

    override suspend fun updateGroup(id: String, name: String, apps: List<String>): AppGroup =
        throw notConnected

    override suspend fun deleteGroup(id: String): Nothing = throw notConnected

    override suspend fun diagnoseJob(app: String, id: String): DiagnosisResponse =
        throw notConnected

    override suspend fun diagnoseHistoryEntry(app: String, id: Long): DiagnosisResponse =
        throw notConnected

    override suspend fun historyDiagnosis(app: String, id: Long): DiagnosisResponse =
        throw notConnected

    override suspend fun testKit(app: String, ring: String): TestKit = throw notConnected

    override suspend fun planTestKit(app: String, ring: String, refresh: Boolean): TestKit =
        throw notConnected
}
