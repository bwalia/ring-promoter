package org.fictionally.ringpromoter

import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.fictionally.ringpromoter.model.HistoryEntry
import org.fictionally.ringpromoter.store.Instance
import org.fictionally.ringpromoter.store.ProbeStatus
import org.fictionally.ringpromoter.ui.DeepLink
import org.fictionally.ringpromoter.ui.history.DayGroup
import org.fictionally.ringpromoter.ui.history.HistoryViewModel
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId

/**
 * Deep links, the history grouping, and the cluster probe.
 *
 * Ported from the iOS `NavigationAndStoreTests`. The common thread is that each
 * of these quietly sends the operator, or their reading of a timeline, to the
 * wrong place when it breaks — and none of them is visible in a screenshot.
 */
class NavigationTest {

    // Deep links

    @Test
    fun `an app link opens that app`() {
        assertEquals(DeepLink.App("web-frontend"), DeepLink.parse("ringpromoter://app/web-frontend"))
    }

    @Test
    fun `a job link opens the job`() {
        assertEquals(
            DeepLink.Job("payments-api", "job-77"),
            DeepLink.parse("ringpromoter://app/payments-api/job/job-77"),
        )
    }

    @Test
    fun `a history link opens the history`() {
        assertEquals(
            DeepLink.History("payments-api"),
            DeepLink.parse("ringpromoter://app/payments-api/history"),
        )
    }

    @Test
    fun `an escaped slash in the app name is not a path separator`() {
        // Decoding before splitting would cut this in two and open an app
        // called "team" — or nothing at all.
        assertEquals(DeepLink.App("team/app"), DeepLink.parse("ringpromoter://app/team%2Fapp"))
    }

    @Test
    fun `an escaped slash survives a round trip`() {
        val url = DeepLink.url("team/app")
        assertEquals(DeepLink.App("team/app"), DeepLink.parse(url))
    }

    @Test
    fun `a job link survives a round trip`() {
        val url = DeepLink.url("payments-api", "job 9")
        assertEquals(DeepLink.Job("payments-api", "job 9"), DeepLink.parse(url))
    }

    @Test
    fun `links that are not ours are ignored`() {
        // Sending an operator to the wrong application is worse than doing
        // nothing at all, so every unrecognised shape returns null.
        assertNull(DeepLink.parse("https://example.com/app/web-frontend"))
        assertNull(DeepLink.parse("ringpromoter://settings"))
        assertNull(DeepLink.parse("ringpromoter://app"))
        assertNull(DeepLink.parse("ringpromoter://app/"))
        assertNull(DeepLink.parse("ringpromoter://app/web-frontend/unknown"))
        assertNull(DeepLink.parse(""))
        assertNull(DeepLink.parse("not a url at all"))
    }

    @Test
    fun `the scheme is matched case-insensitively`() {
        assertEquals(DeepLink.App("api"), DeepLink.parse("RingPromoter://app/api"))
    }

    // History grouping

    private val zone = ZoneId.of("Europe/London")
    private val today = LocalDate.of(2026, 11, 11)

    private fun entry(id: Long, at: Instant) = HistoryEntry(
        id = id,
        app = "payments-api",
        ring = "prod",
        action = "promote",
        result = "success",
        createdAt = at,
    )

    private fun at(day: Int, hour: Int): Instant =
        LocalDate.of(2026, 11, day).atTime(hour, 0).atZone(zone).toInstant()

    @Test
    fun `entries are grouped by local day, newest day first`() {
        val groups = HistoryViewModel.groupByDay(
            listOf(entry(1, at(9, 10)), entry(2, at(11, 9)), entry(3, at(10, 23))),
            zone,
            today,
        )
        assertEquals(listOf(11, 10, 9), groups.map { it.day.dayOfMonth })
    }

    @Test
    fun `today and yesterday are named, earlier days are not`() {
        val groups = HistoryViewModel.groupByDay(
            listOf(entry(1, at(11, 8)), entry(2, at(10, 8)), entry(3, at(4, 8))),
            zone,
            today,
        )
        assertEquals(
            listOf(DayGroup.Relative.TODAY, DayGroup.Relative.YESTERDAY, DayGroup.Relative.EARLIER),
            groups.map { it.relative },
        )
    }

    @Test
    fun `within a day the newest entry is first`() {
        val groups = HistoryViewModel.groupByDay(
            listOf(entry(1, at(11, 2)), entry(2, at(11, 18)), entry(3, at(11, 9))),
            zone,
            today,
        )
        assertEquals(listOf(2L, 3L, 1L), groups.single().entries.map { it.id })
    }

    @Test
    fun `the day is the operator's own, not UTC`() {
        // 23:30 in Los Angeles is already the next day in UTC. An operator
        // asking "was that yesterday?" means their own midnight.
        val la = ZoneId.of("America/Los_Angeles")
        val lateEvening = LocalDate.of(2026, 11, 10).atTime(23, 30).atZone(la).toInstant()
        val groups = HistoryViewModel.groupByDay(listOf(entry(1, lateEvening)), la, today)
        assertEquals(LocalDate.of(2026, 11, 10), groups.single().day)
        assertEquals(DayGroup.Relative.YESTERDAY, groups.single().relative)
    }

    @Test
    fun `no entries means no groups`() {
        assertTrue(HistoryViewModel.groupByDay(emptyList(), zone, today).isEmpty())
    }

    @Test
    fun `the real history fixture groups without loss`() {
        val entries: List<HistoryEntry> =
            Fixtures.decode<org.fictionally.ringpromoter.model.HistoryResponse>("history").history
        val groups = HistoryViewModel.groupByDay(entries, zone, today)
        assertEquals(entries.size, groups.sumOf { it.entries.size })
    }

    // The cluster probe

    private var server: MockWebServer? = null

    @Before
    fun startServer() {
        server = MockWebServer().also { it.start() }
    }

    @After
    fun stopServer() {
        server?.shutdown()
    }

    private fun instance() = Instance(name = "Test", baseUrl = server!!.url("/").toString())

    private fun serve(appsBody: String, jobsBody: String?, appsCode: Int = 200) {
        server!!.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                val path = request.path.orEmpty()
                return when {
                    path.startsWith("/api/jobs") && jobsBody != null ->
                        MockResponse().setResponseCode(200).setBody(jobsBody)

                    path.startsWith("/api/jobs") -> MockResponse().setResponseCode(500)
                    else -> MockResponse().setResponseCode(appsCode).setBody(appsBody)
                }
            }
        }
    }

    @Test
    fun `no stored token is reported as such, not as unreachable`() = runBlocking {
        // The fix is different: paste a token, not check the network.
        assertEquals(ProbeStatus.NoToken, ProbeStatus.probe(instance(), null))
        assertEquals(ProbeStatus.NoToken, ProbeStatus.probe(instance(), ""))
    }

    @Test
    fun `a rejected token is told apart from a dead host`() = runBlocking {
        serve("""{"error":"invalid token"}""", null, appsCode = 401)
        assertEquals(ProbeStatus.TokenRejected, ProbeStatus.probe(instance(), "tok"))
    }

    @Test
    fun `an error from the server is reported in its own words`() = runBlocking {
        serve("""{"error":"database is on fire"}""", null, appsCode = 500)
        val status = ProbeStatus.probe(instance(), "tok")
        assertTrue(status is ProbeStatus.Unreachable)
        assertTrue((status as ProbeStatus.Unreachable).message.contains("database is on fire"))
    }

    @Test
    fun `a reachable cluster counts its apps and rings`() = runBlocking {
        serve(Fixtures.text("apps"), "null")
        val status = ProbeStatus.probe(instance(), "tok") as ProbeStatus.Reachable
        val apps = Fixtures.decode<org.fictionally.ringpromoter.model.AppsResponse>("apps")
        assertEquals(apps.apps.size, status.apps)
        assertEquals(apps.rings.size, status.rings)
    }

    @Test
    fun `only the newest job per app counts towards failing and running`() = runBlocking {
        // An app whose last deploy failed but has since been fixed is not
        // failing. Counting every job would keep it red forever.
        val jobs = """
            {"jobs":[
              {"id":"1","app":"a","action":"promote","status":"failed",
               "started_at":"2026-11-11T09:00:00Z","finished_at":"2026-11-11T09:01:00Z"},
              {"id":"2","app":"a","action":"promote","status":"success",
               "started_at":"2026-11-11T10:00:00Z","finished_at":"2026-11-11T10:01:00Z"},
              {"id":"3","app":"b","action":"promote","status":"running",
               "started_at":"2026-11-11T10:05:00Z"},
              {"id":"4","app":"c","action":"promote","status":"failed",
               "started_at":"2026-11-11T08:00:00Z","finished_at":"2026-11-11T08:01:00Z"}
            ]}
        """.trimIndent()
        serve(Fixtures.text("apps"), jobs)
        val status = ProbeStatus.probe(instance(), "tok") as ProbeStatus.Reachable
        assertEquals(1, status.failing)
        assertEquals(1, status.running)
    }

    @Test
    fun `a cluster whose job list is unavailable is still reachable`() = runBlocking {
        // /api/jobs is a bonus. Losing it must not make a working cluster look
        // dead, which would send an operator chasing a network problem.
        serve(Fixtures.text("apps"), null)
        val status = ProbeStatus.probe(instance(), "tok")
        assertTrue(status is ProbeStatus.Reachable)
        assertEquals(0, (status as ProbeStatus.Reachable).failing)
    }
}
