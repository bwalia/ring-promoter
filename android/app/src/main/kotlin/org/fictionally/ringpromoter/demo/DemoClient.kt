package org.fictionally.ringpromoter.demo

import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.fictionally.ringpromoter.model.AppGroup
import org.fictionally.ringpromoter.model.AppsResponse
import org.fictionally.ringpromoter.model.DiagnosisResponse
import org.fictionally.ringpromoter.model.DiagnosisStatus
import org.fictionally.ringpromoter.model.HistoryEntry
import org.fictionally.ringpromoter.model.Job
import org.fictionally.ringpromoter.model.JobStatus
import org.fictionally.ringpromoter.model.MaintenanceStatus
import org.fictionally.ringpromoter.model.MaintenanceWindow
import org.fictionally.ringpromoter.model.NewMaintenanceWindow
import org.fictionally.ringpromoter.model.NewSignoff
import org.fictionally.ringpromoter.model.PromotionAction
import org.fictionally.ringpromoter.model.RingGates
import org.fictionally.ringpromoter.model.RingStatus
import org.fictionally.ringpromoter.model.ServerVersion
import org.fictionally.ringpromoter.model.Signoff
import org.fictionally.ringpromoter.model.TestKit
import org.fictionally.ringpromoter.model.TestLink
import org.fictionally.ringpromoter.model.TestPlan
import org.fictionally.ringpromoter.model.VersionsResponse
import org.fictionally.ringpromoter.model.signoff
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.net.RingPromoterApi
import java.time.Instant

/**
 * Demo mode: the whole app driven from the bundled fixtures, with no server.
 *
 * Used for screenshots, store review and offline demos. It is a real simulation
 * rather than a stub — seeding, promoting and rolling back mutate state, write
 * history, and produce a job whose steps advance over time — so the live job
 * view, the gates and the failure paths can all be exercised.
 *
 * Rules that matter are simulated faithfully, **including the ones that say
 * "no"**: a shut maintenance window, a missing sign-off and a wrong production
 * password all fail here exactly as the server would fail them.
 *
 * A [Mutex] guards the mutable state, because several screens poll it at once.
 */
class DemoClient(
    private val world: DemoWorld,
    private val clock: () -> Instant = Instant::now,
) : RingPromoterApi {

    private val lock = Mutex()
    private val jobs = mutableMapOf<String, DemoJob>()
    private var jobSequence = 0
    /** AI test plans written in this demo session, keyed by app/ring/version. */
    private val testPlans = mutableMapOf<String, TestPlan>()

    companion object {
        /**
         * The production password demo mode accepts. Shown on the demo banner
         * so the gated path is reachable during review.
         */
        const val PRODUCTION_PASSWORD = "demo"

        /**
         * The change-request code the backend always accepts, whatever the
         * provider — the real server does the same.
         */
        const val DEMO_CHANGE_REQUEST_CODE = "test"

        /**
         * Example links in the shape a real server returns. example.com hosts
         * keep demo mode from pointing anywhere real.
         */
        fun demoLinks(app: String, ring: String, version: String) = listOf(
            TestLink("Web UI", "https://$ring.$app.example.com/", "web", "config"),
            TestLink(
                "iOS beta (TestFlight)",
                "https://example.com/testflight/DEMO0000",
                "ios",
                "config",
            ),
            TestLink("Release $version", "https://example.com/releases/$version", "release", "run"),
        )

        private fun planKey(app: String, ring: String, version: String) = "$app/$ring/$version"
    }

    // Unauthenticated

    override suspend fun checkHealth() = Unit

    override suspend fun serverVersion(): ServerVersion = lock.withLock { world.serverVersion }

    // Reads

    override suspend fun apps(): AppsResponse = lock.withLock { world.apps }

    override suspend fun rings(app: String): List<RingStatus> = lock.withLock {
        requireApp(app)
        val maintenance = world.maintenance(app, clock())
        world.rings(app).map { ring ->
            // The real server computes `maintenance_window_open` live when it
            // builds this response. Demo mode must do the same, or the badge on
            // the ring says "window shut" while a promotion sails through —
            // the gates are the thing this app is trusted for, so they cannot
            // be allowed to disagree with themselves.
            var patched = if (ring.gates.maintenanceWindow) {
                ring.copy(
                    gates = ring.gates.copy(
                        maintenanceWindowOpen = maintenance.isOpen(ring.ring.name),
                    ),
                )
            } else {
                ring
            }
            if (patched.configured && !patched.isEmpty) {
                patched = patched.copy(
                    links = demoLinks(app, ring.ring.name, ring.currentVersion),
                    hasTestPlan =
                        testPlans.containsKey(planKey(app, ring.ring.name, ring.currentVersion)),
                )
            }
            patched
        }
    }

    override suspend fun history(app: String): List<HistoryEntry> = lock.withLock {
        requireApp(app)
        world.history.filter { it.app == app }.sortedByDescending { it.createdAt }
    }

    override suspend fun versions(app: String): VersionsResponse = lock.withLock {
        requireApp(app)
        world.versions[app] ?: VersionsResponse.UNSUPPORTED
    }

    override suspend fun job(app: String, id: String): Job = lock.withLock {
        val demo = jobs[id]?.takeIf { it.app == app }
        if (demo != null) {
            // Advance on read: the poller sees steps appear as time passes,
            // with no background work to leak.
            val advanced = demo.snapshot(clock())
            applyCompletion(demo, advanced)
            advanced
        } else {
            // A seeded job from the captured corpus is already finished, so it
            // is returned as it stands rather than advanced.
            world.seededJobs.firstOrNull { it.id == id && it.app == app }
                ?: throw ApiError.NotFound("job not found")
        }
    }

    override suspend fun recentJobs(): List<Job> = lock.withLock {
        val now = clock()
        // The server returns the newest job per application, so a job started
        // in this session replaces the seeded one for that app rather than
        // appearing alongside it.
        (world.seededJobs + jobs.values.map { it.snapshot(now) })
            .groupBy { it.app }
            .mapNotNull { (_, snapshots) -> snapshots.maxByOrNull { it.startedAt } }
            .sortedByDescending { it.startedAt }
    }

    // Actions

    override suspend fun seed(
        app: String,
        ring: String,
        version: String,
        crCode: String?,
        password: String?,
    ): String = lock.withLock {
        requireApp(app)
        val trimmed = version.trim()
        if (trimmed.isEmpty()) throw ApiError.BadRequest("version must not be empty")
        val target = world.ring(app, ring)?.takeIf { it.configured }
            ?: throw ApiError.NotFound("ring not configured for application")
        checkProductionPassword(target.ring.name, password)
        checkGates(app, ring, trimmed, crCode)
        startJob(app, PromotionAction.SEED, ring, null, trimmed)
    }

    override suspend fun promote(
        app: String,
        fromRing: String,
        crCode: String?,
        password: String?,
    ): String = lock.withLock {
        requireApp(app)
        val source = world.ring(app, fromRing)?.takeIf { it.configured }
            ?: throw ApiError.NotFound("ring not configured for application")
        val next = world.pipeline.next(fromRing)
            ?: throw ApiError.BadRequest("no next ring: already at the last ring")
        if (source.currentVersion.isEmpty()) {
            throw ApiError.Conflict("source ring has no version to promote")
        }
        if (!source.liveHealthy) {
            throw ApiError.Conflict("source ring $fromRing is unhealthy, promotion aborted")
        }
        checkProductionPassword(next.name, password)
        checkGates(app, next.name, source.currentVersion, crCode)
        startJob(app, PromotionAction.PROMOTE, next.name, fromRing, source.currentVersion)
    }

    override suspend fun rollback(app: String, ring: String): String = lock.withLock {
        requireApp(app)
        val state = world.ring(app, ring)?.takeIf { it.configured }
            ?: throw ApiError.NotFound("ring not configured for application")
        if (state.previousVersion.isEmpty()) {
            throw ApiError.Conflict("ring has no previous version to roll back to")
        }
        // Deliberately no password and no gate check: rollback is exempt on the
        // real server too, so incident response is never blocked.
        startJob(app, PromotionAction.ROLLBACK, ring, null, state.previousVersion)
    }

    override suspend fun setAutoPromote(
        app: String,
        ring: String,
        enabled: Boolean,
        password: String?,
    ) = lock.withLock {
        requireApp(app)
        val state = world.ring(app, ring)
            ?: throw ApiError.NotFound("ring not configured for application")
        if (state.autoPromoteManaged) {
            throw ApiError.Conflict(
                "auto-promote is managed by config: auto-promote for $app/$ring " +
                    "is declared in config — change it there",
            )
        }
        val next = world.pipeline.next(ring)
        if (enabled && next != null && world.pipeline.isProduction(next.name)) {
            checkPassword(password)
        }
        world.setAutoPromote(app, ring, enabled)
    }

    // Gates

    override suspend fun maintenance(app: String): MaintenanceStatus = lock.withLock {
        requireApp(app)
        world.maintenance(app, clock())
    }

    override suspend fun openMaintenanceWindow(
        app: String,
        window: NewMaintenanceWindow,
    ): MaintenanceWindow = lock.withLock {
        requireApp(app)
        if (!window.endsAt.isAfter(window.startsAt)) {
            throw ApiError.BadRequest("invalid maintenance window: end must be after start")
        }
        world.addWindow(app, window)
    }

    override suspend fun closeMaintenanceWindow(app: String, id: String) = lock.withLock {
        requireApp(app)
        if (!world.removeWindow(app, id)) throw ApiError.NotFound("maintenance window not found")
    }

    override suspend fun signoffs(app: String): List<Signoff> = lock.withLock {
        requireApp(app)
        world.signoffs.filter { it.app == app }.sortedByDescending { it.updatedAt }
    }

    override suspend fun recordSignoff(app: String, signoff: NewSignoff): Signoff =
        lock.withLock {
            requireApp(app)
            if (signoff.version.isBlank()) {
                throw ApiError.BadRequest("invalid sign-off: version is required")
            }
            if (signoff.engineer.isBlank()) {
                throw ApiError.BadRequest(
                    "invalid sign-off: the release engineer's name is required",
                )
            }
            world.upsertSignoff(app, signoff)
        }

    // Groups

    override suspend fun groups(): List<AppGroup> = lock.withLock { world.groupsSorted() }

    override suspend fun createGroup(name: String, apps: List<String>): AppGroup =
        lock.withLock {
            val trimmed = name.trim()
            if (trimmed.isEmpty()) throw ApiError.BadRequest("group name must not be empty")
            apps.firstOrNull { it !in world.apps.apps }?.let {
                throw ApiError.BadRequest("unknown application: \"$it\"")
            }
            world.createGroup(trimmed, apps)
        }

    override suspend fun updateGroup(id: String, name: String, apps: List<String>): AppGroup =
        lock.withLock {
            world.updateGroup(id, name, apps) ?: throw ApiError.NotFound("group not found")
        }

    override suspend fun deleteGroup(id: String) = lock.withLock {
        if (!world.deleteGroup(id)) throw ApiError.NotFound("group not found")
    }

    // AI diagnosis

    override suspend fun diagnoseJob(app: String, id: String): DiagnosisResponse = lock.withLock {
        requireAi()
        // Only a job started in this session can be diagnosed; the seeded one
        // succeeded, and the server refuses to diagnose a success anyway.
        val demo = jobs[id]?.takeIf { it.app == app } ?: throw ApiError.NotFound("job not found")
        if (demo.snapshot(clock()).status != JobStatus.FAILED) {
            throw ApiError.Conflict("only failed jobs can be diagnosed")
        }
        jobs[id] = demo.copy(diagnosis = DemoWorld.SAMPLE_DIAGNOSIS)
        DiagnosisResponse(DiagnosisStatus.RUNNING, null)
    }

    override suspend fun diagnoseHistoryEntry(app: String, id: Long): DiagnosisResponse =
        lock.withLock {
            requireAi()
            val entry = world.history.firstOrNull { it.id == id && it.app == app }
                ?: throw ApiError.NotFound("history entry not found")
            if (entry.succeeded) throw ApiError.Conflict("only failed entries can be diagnosed")
            world.setHistoryDiagnosis(id, DemoWorld.SAMPLE_DIAGNOSIS)
            DiagnosisResponse(DiagnosisStatus.DONE, DemoWorld.SAMPLE_DIAGNOSIS)
        }

    override suspend fun historyDiagnosis(app: String, id: Long): DiagnosisResponse =
        lock.withLock {
            val entry = world.history.firstOrNull { it.id == id && it.app == app }
                ?: throw ApiError.NotFound("history entry not found")
            val diagnosis = entry.diagnosis
            if (diagnosis.isNullOrEmpty()) DiagnosisResponse(DiagnosisStatus.NONE, null)
            else DiagnosisResponse(DiagnosisStatus.DONE, diagnosis)
        }

    // Test kits

    override suspend fun testKit(app: String, ring: String): TestKit = lock.withLock {
        unlockedTestKit(app, ring)
    }

    override suspend fun planTestKit(app: String, ring: String, refresh: Boolean): TestKit =
        lock.withLock {
            requireAi("AI test plans are not configured on this server")
            val kit = unlockedTestKit(app, ring)
            testPlans[planKey(app, ring, kit.version)] = TestPlan(
                summary = "Demo: ${kit.version} is live in $ring. " +
                    "Check the web UI first, then the iOS beta.",
                checklist = listOf(
                    "Open the web UI and sign in",
                    "Run through the main flow end to end",
                    "Install the beta build and launch the app",
                    "Skim the release notes for anything risky",
                ),
                links = kit.links.take(2).map { link ->
                    link.copy(
                        why = if (link.kind == "ios") "Install the beta build"
                        else "Try the main flow",
                    )
                },
                generatedAt = clock(),
            )
            unlockedTestKit(app, ring)
        }

    /** Already holding [lock]; [planTestKit] needs the kit twice. */
    private fun unlockedTestKit(app: String, ring: String): TestKit {
        requireApp(app)
        val status = world.rings(app).firstOrNull { it.ring.name == ring }?.takeIf { it.configured }
            ?: throw ApiError.NotFound("ring not configured for this application")
        if (status.isEmpty) throw ApiError.Conflict("no version deployed in this ring")
        val plan = testPlans[planKey(app, ring, status.currentVersion)]
        return TestKit(
            app = app,
            ring = ring,
            version = status.currentVersion,
            links = demoLinks(app, ring, status.currentVersion),
            plan = plan,
            capturedAt = status.updatedAt,
            aiEnabled = world.apps.aiEnabled,
            planStatus = if (plan == null) DiagnosisStatus.NONE else DiagnosisStatus.DONE,
        )
    }

    // Rule checks (mirroring the server's)

    private fun requireApp(app: String) {
        if (app !in world.apps.apps) throw ApiError.NotFound("application not found")
    }

    private fun requireAi(message: String = "AI diagnosis is not configured on this server") {
        if (!world.apps.aiEnabled) throw ApiError.NotImplemented(message)
    }

    private fun checkProductionPassword(targetRing: String, password: String?) {
        if (!world.apps.prodProtected || !world.pipeline.isProduction(targetRing)) return
        checkPassword(password)
    }

    private fun checkPassword(password: String?) {
        val supplied = password?.trim().orEmpty()
        if (supplied.isEmpty()) {
            throw ApiError.ProductionPasswordRequired("production password required")
        }
        if (supplied != PRODUCTION_PASSWORD) {
            throw ApiError.ProductionPasswordRequired("incorrect production password")
        }
    }

    private fun checkGates(app: String, targetRing: String, version: String, crCode: String?) {
        val gates = world.ring(app, targetRing)?.gates ?: RingGates.NONE

        if (gates.changeRequest) {
            val code = crCode?.trim().orEmpty()
            if (code.isEmpty()) {
                throw ApiError.BadRequest(
                    "change-request code required: promotion to $targetRing " +
                        "requires a valid change-request code",
                )
            }
            if (code != DEMO_CHANGE_REQUEST_CODE) {
                throw ApiError.BadRequest(
                    "change-request code invalid: invalid change request code\n" +
                        "no change-request system configured; " +
                        "only the demo code \"test\" is accepted",
                )
            }
        }

        if (gates.maintenanceWindow && !world.maintenance(app, clock()).isOpen(targetRing)) {
            throw ApiError.Conflict(
                "maintenance window closed: no active maintenance window for $targetRing " +
                    "(open one, or wait for a scheduled window)",
            )
        }

        if (gates.qaSignoff) {
            val signoff = world.signoffs.filter { it.app == app }.signoff(targetRing, version)
                ?: throw ApiError.Conflict(
                    "qa/release sign-off required: $version needs a release-engineer " +
                        "sign-off for $targetRing before it can be promoted",
                )
            if (!signoff.isGo) {
                throw ApiError.Conflict(
                    "qa/release sign-off is NO-GO: $version was signed off NO-GO for $targetRing",
                )
            }
        }
    }

    // Job simulation

    private fun startJob(
        app: String,
        action: PromotionAction,
        target: String,
        from: String?,
        version: String,
    ): String {
        jobSequence += 1
        val id = "job-$jobSequence"
        // Demo mode deliberately fails one specific case so the failure and
        // rollback paths are reachable without breaking a real cluster: any
        // version containing "bad".
        val shouldFail = version.contains("bad", ignoreCase = true)
        jobs[id] = DemoJob(
            id = id,
            app = app,
            action = action,
            targetRing = target,
            sourceRing = from,
            version = version,
            previousVersion = world.ring(app, target)?.currentVersion.orEmpty(),
            startedAt = clock(),
            fails = shouldFail,
        )
        return id
    }

    /**
     * When a simulated job reaches its terminal state, commit its effect to the
     * world exactly once so rings and history reflect it afterwards.
     */
    private fun applyCompletion(demo: DemoJob, snapshot: Job) {
        if (!snapshot.isFinished || demo.applied) return
        val result = snapshot.result ?: return
        jobs[demo.id] = demo.copy(applied = true)
        world.apply(result, snapshot.finishedAt ?: clock())
    }
}
