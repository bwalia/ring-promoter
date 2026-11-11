package org.fictionally.ringpromoter

import kotlinx.coroutines.test.runTest
import org.fictionally.ringpromoter.demo.DemoClient
import org.fictionally.ringpromoter.model.JobsResponse
import org.fictionally.ringpromoter.demo.DemoWorld
import org.fictionally.ringpromoter.demo.FixtureSource
import org.fictionally.ringpromoter.model.Job
import org.fictionally.ringpromoter.model.NewSignoff
import org.fictionally.ringpromoter.model.Signoff
import org.fictionally.ringpromoter.net.ApiError
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Duration
import java.time.Instant
import kotlin.random.Random

/**
 * Demo mode as a simulation, not a stub.
 *
 * Ported from the iOS `DemoModeTests`. The gates that say **no** matter most
 * here: a reviewer walks this path, and so does anyone evaluating the app
 * offline.
 */
class DemoModeTest {

    private val source = FixtureSource { name -> Fixtures.text(name) }
    private val start: Instant = Instant.parse("2026-07-29T12:00:00Z")

    /** A clock the test drives by hand, so job progress is deterministic. */
    private class TestClock(var now: Instant) {
        fun advance(seconds: Long) {
            now = now.plusSeconds(seconds)
        }
    }

    private fun world() = DemoWorld.loadThrowing(source, start, Random(1))

    private fun clientAndClock(): Pair<DemoClient, TestClock> {
        val clock = TestClock(start)
        return DemoClient(world()) { clock.now } to clock
    }

    private fun client() = clientAndClock().first

    /** Run a job to completion and return its final snapshot. */
    private suspend fun finish(client: DemoClient, clock: TestClock, app: String, id: String): Job {
        // Four steps at 2.2s each, with headroom.
        clock.advance(30)
        return client.job(app, id)
    }

    @Test
    fun `the world loads from the real fixtures`() = runTest {
        val c = client()
        val apps = c.apps()
        assertEquals(listOf("web-frontend", "payments-api", "batch-worker"), apps.apps)
        assertTrue("demo mode turns AI on so the feature is demonstrable", apps.aiEnabled)
        assertTrue(apps.prodProtected)
        assertEquals(listOf("int", "test", "acc", "prod"), apps.pipeline.names)
    }

    @Test
    fun `an unknown application is not found`() = runTest {
        assertThrows(ApiError.NotFound::class.java) {
            kotlinx.coroutines.runBlocking { client().rings("nope") }
        }
    }

    @Test
    fun `payments-api has something genuinely wrong to pin to the top`() = runTest {
        val rings = client().rings("payments-api")
        assertTrue(
            "the Overview needs a real failure to demonstrate trouble pinning",
            rings.any { it.needsAttention },
        )
    }

    @Test
    fun `a seed runs to completion and moves the ring`() = runTest {
        val (c, clock) = clientAndClock()
        val before = c.rings("web-frontend").first { it.ring.name == "int" }
        val id = c.seed("web-frontend", "int", "2.8.0", null, null)

        val job = finish(c, clock, "web-frontend", id)
        assertTrue(job.isFinished)
        assertEquals(Job.Outcome.SUCCEEDED, job.outcome)

        val after = c.rings("web-frontend").first { it.ring.name == "int" }
        assertEquals("2.8.0", after.currentVersion)
        assertEquals(before.currentVersion, after.previousVersion)
        assertTrue(after.isHealthy)
    }

    @Test
    fun `a job's steps appear as time passes`() = runTest {
        val (c, clock) = clientAndClock()
        val id = c.seed("web-frontend", "int", "2.8.0", null, null)

        val atStart = c.job("web-frontend", id)
        assertFalse(atStart.isFinished)
        assertEquals(1, atStart.steps.size)

        clock.advance(3)
        val later = c.job("web-frontend", id)
        assertTrue("a second step should have appeared", later.steps.size > atStart.steps.size)
    }

    @Test
    fun `a version containing bad fails its health check and rolls back`() = runTest {
        val (c, clock) = clientAndClock()
        // int already holds a version, so there is something to roll back to.
        val id = c.seed("web-frontend", "int", "2.8.0-bad", null, null)
        val job = finish(c, clock, "web-frontend", id)

        assertEquals(Job.Outcome.FAILED_AND_ROLLED_BACK, job.outcome)
        assertTrue(job.canRequestDiagnosis)
        assertTrue(
            "the rollback step must be visible, not just implied",
            job.steps.any { it.id == "rollback" },
        )
        val health = job.steps.first { it.id == "health" }
        assertTrue(health.logs.any { it.contains("503") })
    }

    @Test
    fun `an empty version is rejected`() = runTest {
        assertThrows(ApiError.BadRequest::class.java) {
            kotlinx.coroutines.runBlocking { client().seed("web-frontend", "int", "  ", null, null) }
        }
    }

    // Production password

    @Test
    fun `seeding production without a password is refused`() = runTest {
        assertThrows(ApiError.ProductionPasswordRequired::class.java) {
            kotlinx.coroutines.runBlocking {
                client().seed("web-frontend", "prod", "2.8.0", null, null)
            }
        }
    }

    @Test
    fun `seeding production with the wrong password is refused`() = runTest {
        assertThrows(ApiError.ProductionPasswordRequired::class.java) {
            kotlinx.coroutines.runBlocking {
                client().seed("web-frontend", "prod", "2.8.0", null, "hunter2")
            }
        }
    }

    @Test
    fun `seeding production with the demo password is allowed`() = runTest {
        val id = client().seed(
            "web-frontend", "prod", "2.8.0", null, DemoClient.PRODUCTION_PASSWORD,
        )
        assertTrue(id.isNotEmpty())
    }

    // Gates

    /**
     * `acc` → `prod` is the gated hop to exercise.
     *
     * Not `test` → `acc`, which is also CR-gated: payments-api's `test` ring is
     * deliberately the unhealthy one from the failure capture, so a promotion
     * out of it is refused for health before any gate is reached.
     */
    @Test
    fun `promoting into a change-request gated ring needs the code`() = runTest {
        val c = client()
        val prod = c.rings("payments-api").first { it.ring.name == "prod" }
        assertTrue("the gated fixture must gate prod on a CR code", prod.gates.changeRequest)

        val error = assertThrows(ApiError.BadRequest::class.java) {
            kotlinx.coroutines.runBlocking {
                c.promote("payments-api", "acc", null, DemoClient.PRODUCTION_PASSWORD)
            }
        }
        assertTrue(error.detail.contains("change-request"))
    }

    @Test
    fun `an invalid change-request code is refused`() = runTest {
        val error = assertThrows(ApiError.BadRequest::class.java) {
            kotlinx.coroutines.runBlocking {
                client().promote("payments-api", "acc", "NOPE-1", DemoClient.PRODUCTION_PASSWORD)
            }
        }
        assertTrue(error.detail.contains("invalid"))
    }

    @Test
    fun `the fully gated promotion into production is walkable`() = runTest {
        // This is the path a reviewer takes. prod gates on all three: a CR
        // code, an open maintenance window and a QA sign-off. The fixtures are
        // arranged so all three can be satisfied — the window is rebased open,
        // and there is a GO sign-off for exactly the version acc is holding.
        val c = client()
        val id = c.promote(
            "payments-api",
            "acc",
            DemoClient.DEMO_CHANGE_REQUEST_CODE,
            DemoClient.PRODUCTION_PASSWORD,
        )
        assertTrue(id.isNotEmpty())
    }

    @Test
    fun `the ring's window gate agrees with what a promotion would actually do`() = runTest {
        // The badge on a ring and the gate the server enforces must never
        // disagree: the fixtures rebase prod's window to be open now, so the
        // ring must report it open rather than echoing the captured false.
        val c = client()
        val prod = c.rings("payments-api").first { it.ring.name == "prod" }
        assertTrue("the fixture gates prod on a window", prod.gates.maintenanceWindow)
        assertTrue(
            "the ring says the window is shut, but a promotion succeeds — pick one",
            prod.gates.maintenanceWindowOpen,
        )
        assertFalse(prod.gates.isBlockedByClosedWindow)
    }

    @Test
    fun `the sign-off gate is version-specific`() = runTest {
        // The fixture signs off v5.0.0 for prod, and nothing else. Seeding a
        // different version must still be refused, or a GO for one release
        // would silently authorise the next one.
        val error = assertThrows(ApiError.Conflict::class.java) {
            kotlinx.coroutines.runBlocking {
                client().seed(
                    "payments-api",
                    "prod",
                    "v9.9.9",
                    DemoClient.DEMO_CHANGE_REQUEST_CODE,
                    DemoClient.PRODUCTION_PASSWORD,
                )
            }
        }
        assertTrue("got: ${error.detail}", error.detail.contains("sign-off"))
    }

    @Test
    fun `promoting out of an unhealthy ring is refused`() = runTest {
        val c = client()
        val unhealthy = c.rings("payments-api").firstOrNull { !it.isHealthy && !it.isEmpty }
        assertNotNull("the fixture must have an unhealthy ring", unhealthy)
        val error = assertThrows(ApiError.Conflict::class.java) {
            kotlinx.coroutines.runBlocking {
                c.promote("payments-api", unhealthy!!.ring.name, "test", DemoClient.PRODUCTION_PASSWORD)
            }
        }
        assertTrue(error.detail.contains("unhealthy"))
    }

    @Test
    fun `promoting out of the last ring is refused`() = runTest {
        val error = assertThrows(ApiError.BadRequest::class.java) {
            kotlinx.coroutines.runBlocking { client().promote("web-frontend", "prod", null, null) }
        }
        assertTrue(error.detail.contains("no next ring"))
    }

    // Rollback is never gated

    @Test
    fun `rollback needs no password even into production`() = runTest {
        val c = client()
        // Seed prod twice so it has a previous version to return to.
        val (client2, clock) = clientAndClock()
        val first = client2.seed(
            "web-frontend", "prod", "2.7.1", null, DemoClient.PRODUCTION_PASSWORD,
        )
        finish(client2, clock, "web-frontend", first)
        val second = client2.seed(
            "web-frontend", "prod", "2.8.0", null, DemoClient.PRODUCTION_PASSWORD,
        )
        finish(client2, clock, "web-frontend", second)

        // No password argument exists on rollback at all — that is the point.
        val id = client2.rollback("web-frontend", "prod")
        assertTrue(id.isNotEmpty())
        assertNotNull(c)
    }

    @Test
    fun `rollback with nothing to return to is refused`() = runTest {
        val c = client()
        val empty = c.rings("web-frontend").firstOrNull { it.previousVersion.isEmpty() }
        assertNotNull(empty)
        assertThrows(ApiError.Conflict::class.java) {
            kotlinx.coroutines.runBlocking { c.rollback("web-frontend", empty!!.ring.name) }
        }
    }

    // Sign-offs

    @Test
    fun `a sign-off requires an engineer and a version`() = runTest {
        val c = client()
        assertThrows(ApiError.BadRequest::class.java) {
            kotlinx.coroutines.runBlocking {
                c.recordSignoff(
                    "web-frontend",
                    NewSignoff("prod", "", Signoff.Decision.GO, "a.patel", "passed"),
                )
            }
        }
        assertThrows(ApiError.BadRequest::class.java) {
            kotlinx.coroutines.runBlocking {
                c.recordSignoff(
                    "web-frontend",
                    NewSignoff("prod", "2.8.0", Signoff.Decision.GO, "  ", "passed"),
                )
            }
        }
    }

    @Test
    fun `recording a sign-off replaces the one for the same version`() = runTest {
        val c = client()
        c.recordSignoff(
            "web-frontend",
            NewSignoff("prod", "2.8.0", Signoff.Decision.NO_GO, "a.patel", "failed"),
        )
        c.recordSignoff(
            "web-frontend",
            NewSignoff("prod", "2.8.0", Signoff.Decision.GO, "a.patel", "passed"),
        )
        val matching = c.signoffs("web-frontend").filter {
            it.ring == "prod" && it.version == "2.8.0"
        }
        assertEquals(1, matching.size)
        assertTrue(matching.single().isGo)
    }

    // Groups

    @Test
    fun `a group cannot be created with an unknown application`() = runTest {
        assertThrows(ApiError.BadRequest::class.java) {
            kotlinx.coroutines.runBlocking { client().createGroup("Core", listOf("nope")) }
        }
    }

    @Test
    fun `groups round-trip through create, update and delete`() = runTest {
        val c = client()
        val created = c.createGroup("Core", listOf("web-frontend"))
        assertEquals("Core", created.name)

        val updated = c.updateGroup(created.id, "Core services", listOf("web-frontend", "payments-api"))
        assertEquals("Core services", updated.name)
        assertEquals(2, updated.apps.size)

        c.deleteGroup(created.id)
        assertThrows(ApiError.NotFound::class.java) {
            kotlinx.coroutines.runBlocking { c.deleteGroup(created.id) }
        }
    }

    // Diagnosis

    @Test
    fun `only a failed job can be diagnosed`() = runTest {
        val (c, clock) = clientAndClock()
        val ok = c.seed("web-frontend", "int", "2.8.0", null, null)
        finish(c, clock, "web-frontend", ok)
        assertThrows(ApiError.Conflict::class.java) {
            kotlinx.coroutines.runBlocking { c.diagnoseJob("web-frontend", ok) }
        }

        val bad = c.seed("web-frontend", "int", "2.9.0-bad", null, null)
        finish(c, clock, "web-frontend", bad)
        val response = c.diagnoseJob("web-frontend", bad)
        assertNotNull(response.diagnosisStatus)
        // The answer arrives on a later poll, exactly as on a real server.
        val polled = c.job("web-frontend", bad)
        assertTrue(polled.diagnosis.orEmpty().isNotEmpty())
    }

    // Test kits

    @Test
    fun `a test kit only exists for a deployed ring`() = runTest {
        val c = client()
        val deployed = c.rings("web-frontend").first { !it.isEmpty }
        val kit = c.testKit("web-frontend", deployed.ring.name)
        assertEquals(deployed.currentVersion, kit.version)
        assertTrue(kit.links.isNotEmpty())

        val empty = c.rings("web-frontend").firstOrNull { it.isEmpty }
        if (empty != null) {
            assertThrows(ApiError.Conflict::class.java) {
                kotlinx.coroutines.runBlocking { c.testKit("web-frontend", empty.ring.name) }
            }
        }
    }

    @Test
    fun `planning a test kit stores a plan the ring then reports`() = runTest {
        val c = client()
        val ring = c.rings("web-frontend").first { !it.isEmpty }.ring.name
        assertFalse(c.rings("web-frontend").first { it.ring.name == ring }.hasTestPlan == true)

        val kit = c.planTestKit("web-frontend", ring, refresh = false)
        assertNotNull(kit.plan)
        assertTrue(kit.plan!!.checklist.isNotEmpty())
        assertTrue(c.rings("web-frontend").first { it.ring.name == ring }.hasTestPlan == true)
    }

    @Test
    fun `demo links never point at a real host`() = runTest {
        val c = client()
        c.rings("web-frontend").forEach { ring ->
            ring.links.orEmpty().forEach { link ->
                assertTrue(
                    "${link.url} must stay inside example.com",
                    link.url.contains("example.com"),
                )
            }
        }
    }

    // History

    @Test
    fun `a finished job is written into history`() = runTest {
        val (c, clock) = clientAndClock()
        val before = c.history("web-frontend").size
        val id = c.seed("web-frontend", "int", "2.8.0", null, null)
        finish(c, clock, "web-frontend", id)
        val after = c.history("web-frontend")
        assertEquals(before + 1, after.size)
        assertEquals("2.8.0", after.first().toVersion)
        assertTrue(after.first().succeeded)
    }

    @Test
    fun `a completed job is applied exactly once`() = runTest {
        val (c, clock) = clientAndClock()
        val id = c.seed("web-frontend", "int", "2.8.0", null, null)
        finish(c, clock, "web-frontend", id)
        val afterFirstPoll = c.history("web-frontend").size
        // Poll again: the job is already applied and must not write twice.
        c.job("web-frontend", id)
        c.job("web-frontend", id)
        assertEquals(afterFirstPoll, c.history("web-frontend").size)
    }

    @Test
    fun `the fallback world is usable when fixtures cannot be read`() {
        val world = DemoWorld.load(FixtureSource { error("no fixtures here") }, start)
        assertTrue("demo mode must never open to a blank screen", world.apps.apps.isNotEmpty())
        assertTrue(world.rings("web-frontend").isNotEmpty())
    }

    // The activity feed

    @Test
    fun `the activity feed is not empty before anything has been started`() = runTest {
        // A screen whose subject is "what is everyone doing" must not open on
        // "nothing has run yet" during a demo.
        val jobs = client().recentJobs()
        assertTrue(jobs.isNotEmpty())
        assertTrue(jobs.all { it.isFinished })
    }

    @Test
    fun `a seeded job keeps its recorded step durations`() = runTest {
        val job = client().recentJobs().first { it.app == "web-frontend" }
        val captured: JobsResponse = Fixtures.decode("jobs")
        val original = captured.jobs.first { it.id == job.id }
        assertEquals(
            original.steps.map { Duration.between(it.startedAt, it.finishedAt).toMillis() },
            job.steps.map { Duration.between(it.startedAt, it.finishedAt).toMillis() },
        )
    }

    @Test
    fun `a seeded job opens from the feed`() = runTest {
        val listed = client().recentJobs().first()
        val opened = client().job(listed.app, listed.id)
        assertEquals(listed.id, opened.id)
    }

    @Test
    fun `a job started now replaces the seeded one for that app`() = runTest {
        val (client, clock) = clientAndClock()
        val before = client.recentJobs().size
        val id = client.seed("web-frontend", "int", "9.9.9", null, null)
        clock.advance(30)
        val jobs = client.recentJobs()
        // The server returns the newest job per app, so the count is unchanged
        // and the new job is the one listed.
        assertEquals(before, jobs.size)
        assertEquals(id, jobs.first { it.app == "web-frontend" }.id)
    }
}
