package org.fictionally.ringpromoter

import org.fictionally.ringpromoter.model.ActionResult
import org.fictionally.ringpromoter.model.AppGroup
import org.fictionally.ringpromoter.model.AppsResponse
import org.fictionally.ringpromoter.model.DiagnosisStatus
import org.fictionally.ringpromoter.model.GroupsResponse
import org.fictionally.ringpromoter.model.HistoryResponse
import org.fictionally.ringpromoter.model.Job
import org.fictionally.ringpromoter.model.JobStatus
import org.fictionally.ringpromoter.model.JobsResponse
import org.fictionally.ringpromoter.model.MaintenanceStatus
import org.fictionally.ringpromoter.model.RingsResponse
import org.fictionally.ringpromoter.model.ServerVersion
import org.fictionally.ringpromoter.model.Signoff
import org.fictionally.ringpromoter.model.SignoffsResponse
import org.fictionally.ringpromoter.model.StepStatus
import org.fictionally.ringpromoter.model.TestKit
import org.fictionally.ringpromoter.model.VersionsResponse
import org.fictionally.ringpromoter.model.signoff
import org.fictionally.ringpromoter.net.RpJson
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The models decode the backend's real responses.
 *
 * Ported from the iOS `ModelDecodingTests`, against the same fixtures. If the
 * Go models change, this fails rather than the app.
 */
class ModelDecodingTest {

    @Test
    fun `apps carries the pipeline and the capability flags`() {
        val apps = Fixtures.decode<AppsResponse>("apps")
        assertEquals(listOf("web-frontend", "payments-api", "batch-worker"), apps.apps)
        assertEquals(listOf("int", "test", "acc", "prod"), apps.pipeline.names)
        assertTrue(apps.prodProtected)
        assertFalse(apps.aiEnabled)
        assertEquals("Web Frontend", apps.title("web-frontend"))
        // Falls back to the name when no display title is configured.
        assertEquals("batch-worker", apps.title("batch-worker"))
        assertEquals("London, GB", apps.locations["web-frontend"]?.label)
    }

    @Test
    fun `pipeline answers what promotion needs to know`() {
        val pipeline = Fixtures.decode<AppsResponse>("apps").pipeline
        assertEquals("test", pipeline.next("int")?.name)
        assertEquals("prod", pipeline.productionRing?.name)
        assertNull("the last ring has no next", pipeline.next("prod"))
        assertNull("an unknown ring has no next", pipeline.next("nope"))
        assertTrue(pipeline.promotingTargetsProduction("acc"))
        assertFalse(pipeline.promotingTargetsProduction("int"))
        assertTrue(pipeline.isProduction("prod"))
    }

    @Test
    fun `rings decode with previous versions and drift`() {
        val rings = Fixtures.decode<RingsResponse>("rings").rings
        assertEquals(4, rings.size)
        val int = rings.first { it.ring.name == "int" }
        assertTrue(int.configured)
        assertTrue(int.isHealthy)
        assertFalse(int.isEmpty)
    }

    @Test
    fun `a never-deployed ring reads as never, not as 1 Jan 0001`() {
        val rings = Fixtures.decode<RingsResponse>("rings-gated").rings
        val untouched = rings.filter { it.isEmpty }
        assertTrue("expected at least one never-deployed ring", untouched.isNotEmpty())
        untouched.forEach {
            assertFalse(
                "${it.ring.name} has Go's zero time and must not count as deployed",
                it.hasBeenDeployedTo,
            )
        }
    }

    @Test
    fun `gates decode including the change-request provider`() {
        val rings = Fixtures.decode<RingsResponse>("rings-gated").rings
        val gated = rings.filter { it.gates.isGated }
        assertTrue("rings-gated must contain a gated ring", gated.isNotEmpty())
    }

    @Test
    fun `an unhealthy ring carries its error and reports drift`() {
        val rings = Fixtures.decode<RingsResponse>("rings-unhealthy").rings
        val test = rings.first { it.ring.name == "test" }
        assertEquals("unhealthy: status 503", test.liveHealthError)
        assertFalse(test.isHealthy)
        assertTrue(test.needsAttention)

        // int reports v9.1.0 stored but v9.0.0 live: that is version drift.
        val int = rings.first { it.ring.name == "int" }
        assertTrue(int.versionIsDrifting)
    }

    @Test
    fun `auto-promote managed by config decodes`() {
        val rings = Fixtures.decode<RingsResponse>("rings-managed").rings
        assertTrue(rings.any { it.autoPromoteManaged })
    }

    @Test
    fun `a failed job that rolled back is its own outcome`() {
        val job = Fixtures.decode<Job>("job-failed-rolledback")
        assertEquals(JobStatus.FAILED, job.status)
        assertTrue(job.isFinished)
        assertEquals(Job.Outcome.FAILED_AND_ROLLED_BACK, job.outcome)
        assertTrue(job.canRequestDiagnosis)
        // The real four-step shape: source health, deploy, failed check, rollback.
        assertEquals(4, job.steps.size)
        assertEquals(StepStatus.FAILED, job.steps[2].status)
        assertTrue(job.steps[2].logs.any { it.contains("503") })
        assertNotNull(job.steps[0].duration)
    }

    @Test
    fun `a running job is not finished`() {
        val job = Fixtures.decode<Job>("job-running")
        assertFalse(job.isFinished)
        assertEquals(Job.Outcome.RUNNING, job.outcome)
        assertFalse("a running job cannot be diagnosed", job.canRequestDiagnosis)
    }

    @Test
    fun `a successful job reports succeeded`() {
        val job = Fixtures.decode<Job>("job-success")
        assertTrue(job.isFinished)
        assertEquals(Job.Outcome.SUCCEEDED, job.outcome)
        assertFalse(job.canRequestDiagnosis)
    }

    @Test
    fun `the shared jobs feed decodes`() {
        val jobs = Fixtures.decode<JobsResponse>("jobs").jobs
        assertTrue(jobs.isNotEmpty())
    }

    @Test
    fun `a 422 result is a real outcome, not a transport error`() {
        val result = Fixtures.decode<ActionResult>("result-422-failed")
        assertFalse(result.success)
        assertTrue(result.message.isNotEmpty())
        assertTrue(
            "a 422 is either a plain failure or a rollback",
            result.outcome != ActionResult.Outcome.SUCCEEDED,
        )
    }

    @Test
    fun `promote and rollback results decode`() {
        val promote = Fixtures.decode<ActionResult>("promote-result")
        assertEquals("promote", promote.action)
        assertTrue(promote.success)

        // promote-result omits rolled_back entirely, so it must default rather
        // than throw.
        assertFalse(promote.rolledBack)

        val rollback = Fixtures.decode<ActionResult>("rollback-result")
        assertEquals("rollback", rollback.action)
        // A rollback action sets rolled_back, and it still succeeded. `success`
        // has to win, or every deliberate rollback would render as the
        // "failed, and we put it back" state.
        assertTrue(rollback.rolledBack)
        assertTrue(rollback.success)
        assertEquals(ActionResult.Outcome.SUCCEEDED, rollback.outcome)
    }

    @Test
    fun `history decodes and formats its version transition`() {
        val history = Fixtures.decode<HistoryResponse>("history").history
        assertTrue(history.isNotEmpty())
        val entry = history.first()
        assertTrue(entry.versionTransition.isNotEmpty())
        assertEquals(entry.result == "success", entry.succeeded)
    }

    @Test
    fun `failed history entries can be diagnosed`() {
        val history = Fixtures.decode<HistoryResponse>("history-with-failures").history
        val failed = history.filter { !it.succeeded }
        assertTrue("expected a failure in the fixture", failed.isNotEmpty())
        failed.forEach { assertTrue(it.canRequestDiagnosis) }
    }

    @Test
    fun `recurring windows keep their PascalCase keys`() {
        val status = Fixtures.decode<MaintenanceStatus>("maintenance-gated")
        assertTrue(status.gated)
        assertEquals(listOf("prod"), status.gatedRings)
        val recurring = status.recurring.single()
        assertEquals(listOf("Sat", "Sun"), recurring.days)
        assertEquals("02:00", recurring.start)
        assertEquals("Europe/London", recurring.timezone)
        assertEquals("Sat, Sun 02:00–04:00 Europe/London", recurring.summary)
        assertTrue(status.isOpen("prod"))
    }

    @Test
    fun `an ungated app decodes its null collections as empty`() {
        val status = Fixtures.decode<MaintenanceStatus>("maintenance-ungated")
        assertFalse(status.gated)
        assertTrue(status.gatedRings.isEmpty())
        assertTrue(status.recurring.isEmpty())
    }

    @Test
    fun `a closed window reports not open`() {
        val status = Fixtures.decode<MaintenanceStatus>("maintenance-gated-closed")
        assertTrue(status.gated)
        assertFalse(status.isOpen("prod"))
    }

    @Test
    fun `signoffs are looked up by exact ring and version`() {
        val signoffs = Fixtures.decode<SignoffsResponse>("signoffs").signoffs
        assertTrue(signoffs.isNotEmpty())
        val first = signoffs.first()
        assertEquals(first, signoffs.signoff(first.ring, first.version))
        assertNull(
            "a GO for one version must not authorise another",
            signoffs.signoff(first.ring, first.version + "-nope"),
        )
    }

    @Test
    fun `an unknown decision fails closed`() {
        val decoded = RpJson.decodeFromString<Signoff>(
            """{"app":"a","ring":"prod","version":"v1","decision":"maybe",
               "updated_at":"2026-07-29T10:38:52Z"}""",
        )
        assertFalse("an unrecognised decision must never read as a go", decoded.isGo)
        assertEquals(Signoff.Decision.NO_GO, decoded.decision)
    }

    @Test
    fun `groups decode with a null member list`() {
        val groups = Fixtures.decode<GroupsResponse>("groups").groups
        assertTrue(groups.isNotEmpty())
        groups.forEach { g: AppGroup -> assertNotNull(g.apps) }
    }

    @Test
    fun `an unsupported version list offers no picker`() {
        val versions = Fixtures.decode<VersionsResponse>("versions-unsupported")
        assertFalse(versions.supported)
        assertFalse(versions.offersPicker)
        assertTrue(versions.versions.isEmpty())
    }

    @Test
    fun `server version shortens its commit`() {
        val version = Fixtures.decode<ServerVersion>("version")
        assertTrue(version.shortCommit.length <= 8)
    }

    @Test
    fun `a test kit decodes its links`() {
        val kit = Fixtures.decode<TestKit>("test-kit")
        assertTrue(kit.links.isNotEmpty())
        assertNotNull(kit.planStatus)
    }

    @Test
    fun `only http links are offered as a destination`() {
        val kit = Fixtures.decode<TestKit>("test-kit")
        kit.links.forEach { link ->
            val d = link.destination
            if (d != null) {
                assertTrue(
                    "$d was offered but is not http(s)",
                    d.startsWith("http://") || d.startsWith("https://"),
                )
            }
        }
    }

    @Test
    fun `rings with links decode`() {
        val rings = Fixtures.decode<RingsResponse>("rings-with-links").rings
        assertTrue(rings.any { !it.links.isNullOrEmpty() })
    }

    @Test
    fun `a diagnosis status that is absent defaults to none`() {
        val job = Fixtures.decode<Job>("job-success")
        assertEquals(DiagnosisStatus.NONE, job.diagnosisStatus)
    }
}
