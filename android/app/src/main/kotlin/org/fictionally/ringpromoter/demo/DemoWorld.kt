package org.fictionally.ringpromoter.demo

import org.fictionally.ringpromoter.model.ActionResult
import org.fictionally.ringpromoter.model.AppGroup
import org.fictionally.ringpromoter.model.AppVersion
import org.fictionally.ringpromoter.model.AppsResponse
import org.fictionally.ringpromoter.model.DISTANT_PAST
import org.fictionally.ringpromoter.model.GroupsResponse
import org.fictionally.ringpromoter.model.HistoryEntry
import org.fictionally.ringpromoter.model.HistoryResponse
import org.fictionally.ringpromoter.model.MaintenanceStatus
import org.fictionally.ringpromoter.model.MaintenanceWindow
import org.fictionally.ringpromoter.model.NewMaintenanceWindow
import org.fictionally.ringpromoter.model.NewSignoff
import org.fictionally.ringpromoter.model.RecurringWindow
import org.fictionally.ringpromoter.model.Ring
import org.fictionally.ringpromoter.model.RingPipeline
import org.fictionally.ringpromoter.model.RingStatus
import org.fictionally.ringpromoter.model.RingsResponse
import org.fictionally.ringpromoter.model.ServerVersion
import org.fictionally.ringpromoter.model.Signoff
import org.fictionally.ringpromoter.model.SignoffsResponse
import org.fictionally.ringpromoter.model.VersionsResponse
import org.fictionally.ringpromoter.net.RpJson
import java.time.Instant
import kotlin.random.Random

/**
 * The mutable state behind demo mode.
 *
 * Seeded from the bundled fixtures — real captured API responses — with
 * timestamps rebased onto "now" so relative times read sensibly during a demo
 * rather than showing a date from whenever the fixtures were recorded.
 */
class DemoWorld(
    var apps: AppsResponse,
    private val ringsByApp: MutableMap<String, List<RingStatus>>,
    var history: MutableList<HistoryEntry>,
    private val groups: MutableList<AppGroup>,
    val signoffs: MutableList<Signoff>,
    private val windowsByApp: MutableMap<String, MutableList<MaintenanceWindow>>,
    private val recurringByApp: Map<String, List<RecurringWindow>>,
    private val gatedRingsByApp: Map<String, List<String>>,
    val versions: Map<String, VersionsResponse>,
    val serverVersion: ServerVersion,
    private var nextHistoryId: Long,
) {
    private var groupSequence = 0
    private var windowSequence = 0

    val pipeline: RingPipeline get() = apps.pipeline

    // Lookups

    fun rings(forApp: String): List<RingStatus> = ringsByApp[forApp].orEmpty()

    fun ring(app: String, ring: String): RingStatus? =
        ringsByApp[app]?.firstOrNull { it.ring.name == ring }

    fun groupsSorted(): List<AppGroup> = groups.sortedBy { it.name }

    fun maintenance(forApp: String, now: Instant): MaintenanceStatus {
        val gatedRings = gatedRingsByApp[forApp].orEmpty()
        val windows = windowsByApp[forApp].orEmpty()
        val recurring = recurringByApp[forApp].orEmpty()
        val open = gatedRings.associateWith { ring ->
            windows.any { it.covers(ring) && it.isActive(now) }
        }
        return MaintenanceStatus(
            gated = gatedRings.isNotEmpty(),
            gatedRings = gatedRings,
            recurring = recurring,
            windows = windows.sortedByDescending { it.createdAt },
            openRings = open,
        )
    }

    // Mutations

    fun setAutoPromote(app: String, ring: String, enabled: Boolean) {
        updateRing(app, ring) { it.copy(autoPromote = enabled) }
    }

    /** Commit a finished operation: move versions, record health, write history. */
    fun apply(result: ActionResult, at: Instant) {
        updateRing(result.app, result.ring) { it.applying(result, at) }
        appendHistory(result, at)
    }

    fun addWindow(app: String, from: NewMaintenanceWindow): MaintenanceWindow {
        windowSequence += 1
        val window = MaintenanceWindow(
            id = "mw-demo$windowSequence",
            app = app,
            ring = from.ring,
            startsAt = from.startsAt,
            endsAt = from.endsAt,
            reason = from.reason,
            createdBy = from.createdBy,
            createdAt = Instant.now(),
        )
        windowsByApp.getOrPut(app) { mutableListOf() }.add(0, window)
        return window
    }

    fun removeWindow(app: String, id: String): Boolean =
        windowsByApp[app]?.removeAll { it.id == id } ?: false

    fun upsertSignoff(app: String, from: NewSignoff): Signoff {
        val signoff = Signoff(
            app = app,
            ring = from.ring,
            version = from.version,
            decision = from.decision,
            engineer = from.engineer,
            qaStatus = from.qaStatus,
            note = from.note.ifEmpty { null },
            updatedAt = Instant.now(),
        )
        signoffs.removeAll { it.app == app && it.ring == from.ring && it.version == from.version }
        signoffs.add(0, signoff)
        return signoff
    }

    fun createGroup(name: String, apps: List<String>): AppGroup {
        groupSequence += 1
        val group = AppGroup("g-demo$groupSequence", name, apps, Instant.now())
        groups.add(group)
        return group
    }

    fun updateGroup(id: String, name: String, apps: List<String>): AppGroup? {
        val index = groups.indexOfFirst { it.id == id }
        if (index < 0) return null
        val updated = AppGroup(id, name, apps, Instant.now())
        groups[index] = updated
        return updated
    }

    fun deleteGroup(id: String): Boolean = groups.removeAll { it.id == id }

    fun setHistoryDiagnosis(id: Long, text: String) {
        val index = history.indexOfFirst { it.id == id }
        if (index >= 0) history[index] = history[index].copy(diagnosis = text)
    }

    private fun updateRing(app: String, ring: String, transform: (RingStatus) -> RingStatus) {
        val rings = ringsByApp[app] ?: return
        val index = rings.indexOfFirst { it.ring.name == ring }
        if (index < 0) return
        ringsByApp[app] = rings.toMutableList().also { it[index] = transform(it[index]) }
        recomputePromotability(app)
    }

    /**
     * `canPromoteFrom` is derived server-side; keep the demo consistent with
     * the same rule so the UI's enablement stays honest.
     */
    private fun recomputePromotability(app: String) {
        val rings = ringsByApp[app] ?: return
        ringsByApp[app] = rings.map { r ->
            val hasNext = pipeline.next(r.ring.name) != null
            r.copy(
                canPromoteFrom = r.configured && hasNext &&
                    r.currentVersion.isNotEmpty() && r.liveHealthy,
            )
        }
    }

    private fun appendHistory(result: ActionResult, at: Instant) {
        history.add(
            0,
            HistoryEntry(
                id = nextHistoryId,
                app = result.app,
                ring = result.ring,
                action = result.action,
                fromVersion = result.state.previousVersion,
                toVersion = result.version,
                result = if (result.success) "success" else "failure",
                message = result.message,
                diagnosis = null,
                createdAt = at,
            ),
        )
        nextHistoryId += 1
    }

    companion object {
        /** A version list so the Seed sheet's picker path is reachable. */
        val DEMO_VERSIONS = VersionsResponse(
            supported = true,
            versions = listOf(
                AppVersion("main", "branch"),
                AppVersion("release/2.8", "branch"),
                AppVersion("hotfix/checkout-timeout", "branch"),
                AppVersion("2.8.0", "tag"),
                AppVersion("2.7.2", "tag"),
                AppVersion("2.7.1", "tag"),
            ),
        )

        val SAMPLE_DIAGNOSIS = """
            **What happened**

            The deploy to `test` completed, but every health check that followed came back `503`. After three attempts Ring Promoter rolled the ring back to `v9.0.0` — and that version answered `503` as well, so the ring is still unhealthy.

            **Most likely cause**

            Because both the new and the previous version fail identically, this is very unlikely to be the release itself. The pattern points at something the ring depends on: a database or upstream service that is refusing connections for every pod in that namespace.

            **What to check first**

            1. `kubectl -n test get pods` — are the pods running, or crash-looping?
            2. The dependency the readiness probe touches. A `503` on both versions usually means the probe cannot reach it.
            3. Whether anything else changed in `test` around the same time — config, secrets, or a network policy.

            **Note**

            This diagnosis is generated text shown for the demo. On a real server it comes from the configured model, using the job's own step logs.
        """.trimIndent()

        /**
         * Build the demo world from the bundled fixtures, falling back to a
         * small built-in world if a fixture is missing — demo mode must never
         * open to a blank screen.
         */
        fun load(
            source: FixtureSource,
            now: Instant = Instant.now(),
            random: Random = Random.Default,
        ): DemoWorld = runCatching { loadThrowing(source, now, random) }.getOrElse { fallback(now) }

        fun loadThrowing(
            source: FixtureSource,
            now: Instant = Instant.now(),
            random: Random = Random.Default,
        ): DemoWorld {
            fun <T> decode(name: String, deserializer: kotlinx.serialization.DeserializationStrategy<T>): T =
                RpJson.decodeFromString(deserializer, source.read(name))

            // The fixtures were captured from a server without an AI backend;
            // demo mode turns the feature on so "Diagnose with AI" is
            // demonstrable.
            val apps = decode(FixtureName.APPS, AppsResponse.serializer())
                .copy(aiEnabled = true)

            val webRings = decode(FixtureName.RINGS, RingsResponse.serializer()).rings
            val gatedRings = decode(FixtureName.RINGS_GATED, RingsResponse.serializer()).rings
            val managedRings = decode(FixtureName.RINGS_MANAGED, RingsResponse.serializer()).rings
            val unhealthyRings =
                decode(FixtureName.RINGS_UNHEALTHY, RingsResponse.serializer()).rings

            // payments-api gets the unhealthy `test` ring from the failure
            // capture so the Overview has something genuinely wrong to pin to
            // the top.
            val paymentsRings = gatedRings.map { ring ->
                val unhealthy = unhealthyRings.firstOrNull { it.ring.name == ring.ring.name }
                if (ring.ring.name == "test" && unhealthy != null) {
                    unhealthy.copy(gates = ring.gates).rebased(now, random)
                } else {
                    ring.rebased(now, random)
                }
            }

            val history = (
                decode(FixtureName.HISTORY, HistoryResponse.serializer()).history +
                    decode(
                        FixtureName.HISTORY_WITH_FAILURES,
                        HistoryResponse.serializer(),
                    ).history
                )
                .mapIndexed { index, entry ->
                    entry.copy(createdAt = now.minusSeconds(index * 2_400L))
                }

            val groups = decode(FixtureName.GROUPS, GroupsResponse.serializer()).groups
                .map { it.copy(updatedAt = now) }
            val signoffs = decode(FixtureName.SIGNOFFS, SignoffsResponse.serializer()).signoffs
                .map { it.copy(updatedAt = now) }
            val gatedMaintenance =
                decode(FixtureName.MAINTENANCE_GATED, MaintenanceStatus.serializer())

            // Rebase the captured ad-hoc window so it is open *now*, which is
            // what makes the gated promote path walkable in a demo.
            val windowStart = now.minusSeconds(1_800)
            val windows = gatedMaintenance.windows.map {
                it.copy(
                    startsAt = windowStart,
                    endsAt = windowStart.plusSeconds(7_200),
                    createdAt = windowStart,
                )
            }

            return DemoWorld(
                apps = apps,
                ringsByApp = mutableMapOf(
                    "web-frontend" to webRings.map { it.rebased(now, random) },
                    "payments-api" to paymentsRings,
                    "batch-worker" to managedRings.map { it.rebased(now, random) },
                ),
                history = history.toMutableList(),
                groups = groups.toMutableList(),
                signoffs = signoffs.toMutableList(),
                windowsByApp = mutableMapOf("payments-api" to windows.toMutableList()),
                recurringByApp = mapOf("payments-api" to gatedMaintenance.recurring),
                gatedRingsByApp = mapOf("payments-api" to gatedMaintenance.gatedRings),
                versions = mapOf("web-frontend" to DEMO_VERSIONS),
                serverVersion = ServerVersion(
                    version = "demo",
                    commit = "demo0000",
                    builtAt = "—",
                    startedAt = now.minusSeconds(86_400),
                ),
                nextHistoryId = (history.maxOfOrNull { it.id } ?: 0L) + 1,
            )
        }

        /** A minimal world used only if the bundled fixtures cannot be read. */
        fun fallback(now: Instant): DemoWorld {
            val rings = listOf(
                Ring("int", "Integration"),
                Ring("test", "Test"),
                Ring("acc", "Acceptance"),
                Ring("prod", "Production"),
            )
            val apps = AppsResponse(
                apps = listOf("web-frontend"),
                titles = mapOf("web-frontend" to "Web Frontend"),
                rings = rings,
                prodProtected = true,
                aiEnabled = true,
            )
            val states = rings.mapIndexed { index, ring ->
                val live = index < 2
                RingStatus(
                    ring = ring,
                    configured = true,
                    currentVersion = if (live) "2.7.1" else "",
                    liveVersion = if (live) "2.7.1" else "",
                    healthy = live,
                    liveHealthy = live,
                    updatedAt = if (live) now else DISTANT_PAST,
                    canPromoteFrom = live,
                )
            }
            return DemoWorld(
                apps = apps,
                ringsByApp = mutableMapOf("web-frontend" to states),
                history = mutableListOf(),
                groups = mutableListOf(),
                signoffs = mutableListOf(),
                windowsByApp = mutableMapOf(),
                recurringByApp = emptyMap(),
                gatedRingsByApp = emptyMap(),
                versions = mapOf("web-frontend" to DEMO_VERSIONS),
                serverVersion = ServerVersion("demo", "demo0000", "—", now),
                nextHistoryId = 1,
            )
        }
    }
}

/**
 * Move a never-deployed ring's zero timestamp aside and pull real ones close to
 * [now], so demo mode shows "12 minutes ago", not a stale date.
 */
private fun RingStatus.rebased(now: Instant, random: Random): RingStatus =
    if (!hasBeenDeployedTo) this
    else copy(updatedAt = now.minusSeconds(random.nextLong(300, 7_200)))

/** Apply a finished operation's effect to this ring. */
private fun RingStatus.applying(result: ActionResult, at: Instant): RingStatus = when {
    result.success -> copy(
        currentVersion = result.version,
        previousVersion = currentVersion,
        liveVersion = result.version,
        healthy = true,
        liveHealthy = true,
        liveHealthError = null,
        updatedAt = at,
    )

    // The target was returned to what it was running before.
    result.rolledBack -> copy(
        liveVersion = currentVersion,
        healthy = false,
        liveHealthy = false,
        liveHealthError = "unhealthy: status 503",
        updatedAt = at,
    )

    else -> copy(
        healthy = false,
        liveHealthy = false,
        liveHealthError = "unhealthy: status 503",
        updatedAt = at,
    )
}
