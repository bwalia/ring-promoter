package org.fictionally.ringpromoter.net

import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import okhttp3.Call
import okhttp3.Callback
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import org.fictionally.ringpromoter.model.AppGroup
import org.fictionally.ringpromoter.model.AppsResponse
import org.fictionally.ringpromoter.model.DiagnosisResponse
import org.fictionally.ringpromoter.model.GroupBody
import org.fictionally.ringpromoter.model.GroupsResponse
import org.fictionally.ringpromoter.model.HistoryEntry
import org.fictionally.ringpromoter.model.HistoryResponse
import org.fictionally.ringpromoter.model.Job
import org.fictionally.ringpromoter.model.JobHandle
import org.fictionally.ringpromoter.model.JobsResponse
import org.fictionally.ringpromoter.model.MaintenanceStatus
import org.fictionally.ringpromoter.model.MaintenanceWindow
import org.fictionally.ringpromoter.model.NewMaintenanceWindow
import org.fictionally.ringpromoter.model.NewSignoff
import org.fictionally.ringpromoter.model.RingStatus
import org.fictionally.ringpromoter.model.RingsResponse
import org.fictionally.ringpromoter.model.ServerVersion
import org.fictionally.ringpromoter.model.Signoff
import org.fictionally.ringpromoter.model.SignoffsResponse
import org.fictionally.ringpromoter.model.TestKit
import org.fictionally.ringpromoter.model.VersionsResponse
import java.io.IOException
import java.net.SocketTimeoutException
import java.net.UnknownHostException
import java.util.concurrent.TimeUnit
import javax.net.ssl.SSLException
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/**
 * The live API client.
 *
 * The single place that knows about HTTP: token injection, JSON coding, and
 * the status-code → [ApiError] mapping all live here, so no screen ever sees a
 * status code.
 *
 * Nothing here logs a token, a production password, or a request body.
 */
class RingPromoterClient(
    baseUrl: String,
    private val token: String,
    private val http: OkHttpClient = defaultHttpClient(),
) : RingPromoterApi {

    private val base: HttpUrl = baseUrl.toHttpUrlOrNull()
        ?: throw IllegalArgumentException("$baseUrl is not a usable server URL.")

    private val host: String get() = base.host

    companion object {
        private val JSON_MEDIA = "application/json".toMediaType()

        /**
         * Timeouts that suit a phone: fail fast enough that an operator on a
         * bad connection learns something is wrong, but not so fast that a slow
         * control plane looks broken.
         */
        fun defaultHttpClient(timeoutSeconds: Long = 20): OkHttpClient =
            OkHttpClient.Builder()
                .connectTimeout(timeoutSeconds, TimeUnit.SECONDS)
                .readTimeout(timeoutSeconds, TimeUnit.SECONDS)
                .writeTimeout(timeoutSeconds, TimeUnit.SECONDS)
                .callTimeout(60, TimeUnit.SECONDS)
                .retryOnConnectionFailure(false)
                .build()
    }

    // Unauthenticated

    override suspend fun checkHealth() {
        send(Spec(listOf("healthz"), authenticated = false))
    }

    override suspend fun serverVersion(): ServerVersion =
        get(Spec(listOf("version"), authenticated = false))

    // Reads

    override suspend fun apps(): AppsResponse = get(Spec(listOf("api", "apps")))

    override suspend fun rings(app: String): List<RingStatus> =
        get<RingsResponse>(Spec(listOf("api", "apps", app, "rings"))).rings

    override suspend fun history(app: String): List<HistoryEntry> =
        get<HistoryResponse>(Spec(listOf("api", "apps", app, "history"))).history

    override suspend fun versions(app: String): VersionsResponse =
        get(Spec(listOf("api", "apps", app, "versions")))

    override suspend fun job(app: String, id: String): Job =
        get(Spec(listOf("api", "apps", app, "jobs", id)))

    override suspend fun recentJobs(): List<Job> =
        get<JobsResponse>(Spec(listOf("api", "jobs"))).jobs

    // Actions

    /**
     * Bodies are built as explicit maps so an optional that is null is
     * *omitted* rather than sent as null — the server decodes with
     * `DisallowUnknownFields`, and an explicit null for `password` would be a
     * different thing from not sending one.
     */
    override suspend fun seed(
        app: String,
        ring: String,
        version: String,
        crCode: String?,
        password: String?,
    ): String {
        val body = buildMap {
            put("ring", ring)
            put("version", version)
            nonEmpty(crCode)?.let { put("cr_code", it) }
            nonEmpty(password)?.let { put("password", it) }
        }
        return startJob(listOf("api", "apps", app, "seed"), body)
    }

    override suspend fun promote(
        app: String,
        fromRing: String,
        crCode: String?,
        password: String?,
    ): String {
        val body = buildMap {
            put("from_ring", fromRing)
            nonEmpty(crCode)?.let { put("cr_code", it) }
            nonEmpty(password)?.let { put("password", it) }
        }
        return startJob(listOf("api", "apps", app, "promote"), body)
    }

    /**
     * No password parameter exists by design: the server exempts rollback, and
     * offering one here would imply incident response can be gated.
     */
    override suspend fun rollback(app: String, ring: String): String =
        startJob(listOf("api", "apps", app, "rollback"), mapOf("ring" to ring))

    override suspend fun setAutoPromote(
        app: String,
        ring: String,
        enabled: Boolean,
        password: String?,
    ) {
        val fields = buildMap<String, JsonElement> {
            put("enabled", JsonPrimitive(enabled))
            nonEmpty(password)?.let { put("password", JsonPrimitive(it)) }
        }
        send(
            Spec(
                segments = listOf("api", "apps", app, "rings", ring, "auto-promote"),
                method = "PUT",
                body = jsonBody(JsonObject(fields)),
            ),
        )
    }

    // Gates

    override suspend fun maintenance(app: String): MaintenanceStatus =
        get(Spec(listOf("api", "apps", app, "maintenance-windows")))

    override suspend fun openMaintenanceWindow(
        app: String,
        window: NewMaintenanceWindow,
    ): MaintenanceWindow = post(
        listOf("api", "apps", app, "maintenance-windows"),
        encode(NewMaintenanceWindow.serializer(), window),
    )

    override suspend fun closeMaintenanceWindow(app: String, id: String) {
        send(
            Spec(
                segments = listOf("api", "apps", app, "maintenance-windows", id),
                method = "DELETE",
            ),
        )
    }

    override suspend fun signoffs(app: String): List<Signoff> =
        get<SignoffsResponse>(Spec(listOf("api", "apps", app, "signoffs"))).signoffs

    override suspend fun recordSignoff(app: String, signoff: NewSignoff): Signoff =
        post(
            listOf("api", "apps", app, "signoffs"),
            encode(NewSignoff.serializer(), signoff),
        )

    // Groups

    override suspend fun groups(): List<AppGroup> =
        get<GroupsResponse>(Spec(listOf("api", "groups"))).groups

    override suspend fun createGroup(name: String, apps: List<String>): AppGroup =
        post(listOf("api", "groups"), encode(GroupBody.serializer(), GroupBody(name, apps)))

    override suspend fun updateGroup(id: String, name: String, apps: List<String>): AppGroup {
        val data = send(
            Spec(
                segments = listOf("api", "groups", id),
                method = "PUT",
                body = encode(GroupBody.serializer(), GroupBody(name, apps)),
            ),
        )
        return decode(data)
    }

    override suspend fun deleteGroup(id: String) {
        send(Spec(segments = listOf("api", "groups", id), method = "DELETE"))
    }

    // AI diagnosis

    override suspend fun diagnoseJob(app: String, id: String): DiagnosisResponse =
        post(listOf("api", "apps", app, "jobs", id, "diagnose"), null)

    override suspend fun diagnoseHistoryEntry(app: String, id: Long): DiagnosisResponse =
        post(listOf("api", "apps", app, "history", id.toString(), "diagnose"), null)

    override suspend fun historyDiagnosis(app: String, id: Long): DiagnosisResponse =
        get(Spec(listOf("api", "apps", app, "history", id.toString(), "diagnose")))

    // Test kits

    override suspend fun testKit(app: String, ring: String): TestKit =
        get(Spec(listOf("api", "apps", app, "rings", ring, "test-kit")))

    override suspend fun planTestKit(app: String, ring: String, refresh: Boolean): TestKit {
        // 202 (running) and 200 (stored plan) both carry the test kit.
        val data = send(
            Spec(
                segments = listOf("api", "apps", app, "rings", ring, "test-kit", "plan"),
                method = "POST",
                query = if (refresh) mapOf("refresh" to "1") else emptyMap(),
            ),
        )
        return decode(data)
    }

    // Plumbing

    private data class Spec(
        val segments: List<String>,
        val method: String = "GET",
        val body: RequestBody? = null,
        val query: Map<String, String> = emptyMap(),
        val authenticated: Boolean = true,
    )

    /**
     * Fires a mutating operation on the **async** path and returns the job id.
     *
     * The server answers 202 with `{"job_id": …}`. A 200 would mean the server
     * ran it synchronously, which it only does when `?async=1` is absent — so
     * that is treated as a protocol mismatch rather than silently swallowed.
     */
    private suspend fun startJob(segments: List<String>, body: Map<String, String>): String {
        val fields = body.mapValues { JsonPrimitive(it.value) as JsonElement }
        val data = send(
            Spec(
                segments = segments,
                method = "POST",
                body = jsonBody(JsonObject(fields)),
                query = mapOf("async" to "1"),
            ),
        )
        return decode<JobHandle>(data).jobId
    }

    private suspend inline fun <reified T> get(spec: Spec): T = decode(send(spec))

    private suspend inline fun <reified T> post(
        segments: List<String>,
        body: RequestBody?,
    ): T = decode(send(Spec(segments = segments, method = "POST", body = body)))

    private fun jsonBody(element: JsonElement): RequestBody =
        RpJson.encodeToString(JsonElement.serializer(), element).toRequestBody(JSON_MEDIA)

    private fun <T> encode(
        serializer: kotlinx.serialization.SerializationStrategy<T>,
        value: T,
    ): RequestBody = try {
        RpJson.encodeToString(serializer, value).toRequestBody(JSON_MEDIA)
    } catch (e: Exception) {
        throw ApiError.Decoding("Could not encode the request: ${e.message}")
    }

    private inline fun <reified T> decode(data: String): T = try {
        RpJson.decodeFromString<T>(data)
    } catch (e: Exception) {
        throw ApiError.Decoding(describe(e))
    }

    /**
     * Performs the request and maps the outcome. Returns the body text for any
     * 2xx; throws the mapped [ApiError] for everything else.
     */
    private suspend fun send(spec: Spec): String {
        val url = base.newBuilder().apply {
            spec.segments.forEach { addPathSegment(it) }
            spec.query.forEach { (k, v) -> addQueryParameter(k, v) }
        }.build()

        val request = Request.Builder()
            .url(url)
            .method(spec.method, spec.body ?: emptyBodyFor(spec.method))
            .header("Accept", "application/json")
            .apply { if (spec.authenticated) header("Authorization", "Bearer $token") }
            .build()

        val response = try {
            http.newCall(request).await()
        } catch (e: IOException) {
            throw ApiError.Transport(transportFailure(e))
        }

        response.use {
            val text = it.body?.string().orEmpty()
            if (!it.isSuccessful) {
                throw ApiError.from(it.code, serverMessage(text), host)
            }
            return text
        }
    }

    /** POST/PUT with no body still needs an empty one; GET/DELETE must have none. */
    private fun emptyBodyFor(method: String): RequestBody? =
        if (method == "POST" || method == "PUT" || method == "PATCH") {
            ByteArray(0).toRequestBody(null)
        } else {
            null
        }

    /** Translate an [IOException] into the transport case that best explains it. */
    private fun transportFailure(e: IOException): ApiError.TransportFailure = when (e) {
        is SocketTimeoutException -> ApiError.TransportFailure.TimedOut
        is UnknownHostException -> ApiError.TransportFailure.CannotReachHost(host)
        is SSLException -> ApiError.TransportFailure.TlsFailure(e.message ?: "unknown reason")
        else -> {
            // OkHttp reports a cancelled call as a plain IOException.
            if (e.message?.contains("Canceled", ignoreCase = true) == true) {
                ApiError.TransportFailure.Cancelled
            } else {
                ApiError.TransportFailure.CannotReachHost(host)
            }
        }
    }

    /**
     * Pull `{"error": "..."}` out of a failure body. Falls back to the raw text
     * when the body is not the expected shape (a proxy's HTML 502, say),
     * truncated so an error alert cannot become a wall of markup.
     */
    private fun serverMessage(body: String): String? {
        runCatching { RpJson.decodeFromString<ApiErrorBody>(body) }
            .getOrNull()
            ?.let { return it.error }
        val text = body.trim()
        if (text.isEmpty()) return null
        return if (text.length > 300) text.take(300) + "…" else text
    }

    /** Describe a decoding failure without echoing the payload. */
    private fun describe(e: Exception): String =
        e.message?.substringBefore("\nJSON input")?.take(300)
            ?: "The response did not match the expected format."

    /**
     * Treat whitespace-only input as absent, so an untouched password field is
     * never sent as `""` (which the server reads as "wrong").
     */
    private fun nonEmpty(value: String?): String? = value?.trim()?.takeIf { it.isNotEmpty() }
}

/**
 * Await an OkHttp call from a coroutine.
 *
 * Enqueued rather than executed so cancelling the coroutine cancels the call:
 * switching control plane must drop every request still in flight for the
 * previous one.
 */
private suspend fun Call.await(): Response = suspendCancellableCoroutine { cont ->
    enqueue(object : Callback {
        override fun onResponse(call: Call, response: Response) = cont.resume(response)
        override fun onFailure(call: Call, e: IOException) {
            if (!cont.isCancelled) cont.resumeWithException(e)
        }
    })
    cont.invokeOnCancellation { runCatching { cancel() } }
}
