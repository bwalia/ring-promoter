package org.fictionally.ringpromoter

import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.net.RingPromoterClient
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * The HTTP layer's contract.
 *
 * Ported from the iOS `ClientTests`. The status mapping is the part that must
 * not drift: each code sends the operator somewhere different in the UI, so
 * collapsing any of them into "something went wrong" is a real regression.
 */
class ClientTest {

    private lateinit var server: MockWebServer

    @Before
    fun start() {
        server = MockWebServer()
        server.start()
    }

    @After
    fun stop() {
        server.shutdown()
    }

    private fun client(token: String = "tok-123") =
        RingPromoterClient(server.url("/").toString(), token)

    private fun enqueue(code: Int, body: String = "") {
        server.enqueue(MockResponse().setResponseCode(code).setBody(body))
    }

    // Status contract

    @Test
    fun `401 sends the operator back to the token screen`() {
        enqueue(401, """{"error":"invalid token"}""")
        val error = assertThrows(ApiError.Unauthorized::class.java) {
            runBlocking { client().apps() }
        }
        assertEquals("invalid token", error.detail)
        assertTrue(error.requiresReauthentication)
        assertFalse(error.requiresProductionPassword)
    }

    @Test
    fun `403 re-opens the production password field`() {
        enqueue(403, """{"error":"production password required"}""")
        val error = assertThrows(ApiError.ProductionPasswordRequired::class.java) {
            runBlocking { client().promote("a", "int", null, null) }
        }
        assertTrue(error.requiresProductionPassword)
        assertFalse(error.requiresReauthentication)
    }

    @Test
    fun `409 explains a gate`() {
        enqueue(409, """{"error":"no active maintenance window for prod"}""")
        val error = assertThrows(ApiError.Conflict::class.java) {
            runBlocking { client().promote("a", "acc", null, null) }
        }
        assertEquals("no active maintenance window for prod", error.detail)
        assertEquals("Blocked", error.title)
    }

    @Test
    fun `400, 404, 501 and 5xx each keep their own case`() {
        enqueue(400, """{"error":"version must not be empty"}""")
        assertThrows(ApiError.BadRequest::class.java) {
            runBlocking { client().seed("a", "int", "v", null, null) }
        }

        enqueue(404, """{"error":"application not found"}""")
        assertThrows(ApiError.NotFound::class.java) { runBlocking { client().rings("a") } }

        enqueue(501, """{"error":"AI diagnosis is not configured"}""")
        assertThrows(ApiError.NotImplemented::class.java) {
            runBlocking { client().diagnoseJob("a", "job-1") }
        }

        enqueue(503, """{"error":"upstream unavailable"}""")
        val server5xx = assertThrows(ApiError.Server::class.java) {
            runBlocking { client().apps() }
        }
        assertEquals(503, server5xx.code)
        assertTrue("a 5xx is worth retrying", server5xx.isRetryable)
    }

    @Test
    fun `an empty error body still produces a usable message`() {
        enqueue(401, "")
        val error = assertThrows(ApiError.Unauthorized::class.java) {
            runBlocking { client().apps() }
        }
        assertEquals("The API token was rejected.", error.detail)
    }

    @Test
    fun `a proxy's HTML error page is truncated, not shown whole`() {
        val html = "<html><body>" + "x".repeat(2_000) + "</body></html>"
        enqueue(502, html)
        val error = assertThrows(ApiError.Server::class.java) {
            runBlocking { client().apps() }
        }
        assertTrue("an error alert must not become a wall of markup", error.detail.length <= 301)
        assertTrue(error.detail.endsWith("…"))
    }

    @Test
    fun `a 200 that is not the expected shape is a decoding error, not a crash`() {
        enqueue(200, """{"unexpected":true}""")
        assertThrows(ApiError.Decoding::class.java) { runBlocking { client().job("a", "1") } }
    }

    // Authentication

    @Test
    fun `authenticated calls carry the bearer token`() = runTest {
        enqueue(200, """{"apps":[],"titles":{},"rings":[],"prod_protected":false,"ai_enabled":false}""")
        client().apps()
        val request = server.takeRequest()
        assertEquals("Bearer tok-123", request.getHeader("Authorization"))
        assertEquals("application/json", request.getHeader("Accept"))
    }

    @Test
    fun `healthz and version are sent unauthenticated`() = runTest {
        enqueue(200, "ok")
        client().checkHealth()
        assertNull(
            "the health probe must work before a token exists",
            server.takeRequest().getHeader("Authorization"),
        )

        enqueue(200, """{"version":"1.0","commit":"abc","built_at":"-"}""")
        client().serverVersion()
        assertNull(server.takeRequest().getHeader("Authorization"))
    }

    // Request shape

    @Test
    fun `mutations take the async path and return the job id`() = runTest {
        enqueue(202, """{"job_id":"job-7"}""")
        val id = client().promote("web-frontend", "int", null, null)
        assertEquals("job-7", id)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("1", request.requestUrl?.queryParameter("async"))
        assertTrue(request.path!!.startsWith("/api/apps/web-frontend/promote"))
    }

    @Test
    fun `an absent password is omitted, never sent as null`() = runTest {
        enqueue(202, """{"job_id":"job-1"}""")
        client().promote("a", "int", null, null)
        val body = server.takeRequest().body.readUtf8()
        assertFalse(
            "the server decodes with DisallowUnknownFields; a null password is not the " +
                "same as no password",
            body.contains("password"),
        )
        assertFalse(body.contains("cr_code"))
        assertTrue(body.contains("\"from_ring\":\"int\""))
    }

    @Test
    fun `a whitespace-only password counts as absent`() = runTest {
        enqueue(202, """{"job_id":"job-1"}""")
        client().promote("a", "int", "   ", "  ")
        val body = server.takeRequest().body.readUtf8()
        assertFalse(
            "an untouched field must not be sent as \"\", which the server reads as wrong",
            body.contains("password"),
        )
    }

    @Test
    fun `a supplied password and CR code are sent trimmed`() = runTest {
        enqueue(202, """{"job_id":"job-1"}""")
        client().promote("a", "acc", " CR-42 ", " demo ")
        val body = server.takeRequest().body.readUtf8()
        assertTrue(body.contains("\"password\":\"demo\""))
        assertTrue(body.contains("\"cr_code\":\"CR-42\""))
    }

    @Test
    fun `rollback never sends a password`() = runTest {
        enqueue(202, """{"job_id":"job-1"}""")
        client().rollback("a", "prod")
        val body = server.takeRequest().body.readUtf8()
        assertFalse(
            "offering one would imply incident response can be gated",
            body.contains("password"),
        )
        assertTrue(body.contains("\"ring\":\"prod\""))
    }

    @Test
    fun `an application name containing a slash is escaped, not split`() = runTest {
        enqueue(200, """{"rings":[]}""")
        client().rings("team/app")
        val path = server.takeRequest().path!!
        assertTrue(
            "a decoded slash would address a different application entirely",
            path.startsWith("/api/apps/team%2Fapp/rings"),
        )
    }

    @Test
    fun `a base URL with a path prefix keeps that prefix`() = runTest {
        enqueue(200, """{"apps":[],"titles":{},"rings":[],"prod_protected":false,"ai_enabled":false}""")
        RingPromoterClient(server.url("/rp").toString(), "tok").apps()
        assertEquals("/rp/api/apps", server.takeRequest().path)
    }

    @Test
    fun `a base URL with a trailing slash does not double it`() = runTest {
        enqueue(200, """{"apps":[],"titles":{},"rings":[],"prod_protected":false,"ai_enabled":false}""")
        RingPromoterClient(server.url("/rp/").toString(), "tok").apps()
        assertEquals("/rp/api/apps", server.takeRequest().path)
    }

    @Test
    fun `an unusable base URL is rejected at construction`() {
        assertThrows(IllegalArgumentException::class.java) {
            RingPromoterClient("not a url", "tok")
        }
    }
}
