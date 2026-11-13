package org.fictionally.ringpromoter.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.net.URI
import java.time.Instant

/**
 * One place to try or inspect a deployed version: the web UI, a store build, an
 * artifact, the workflow run.
 *
 * Mirrors `store.TestLink`.
 */
@Serializable
data class TestLink(
    val label: String,
    val url: String,
    /** web, ios, android, api, docs, dashboard, release, artifact, ci or other. */
    val kind: String,
    /**
     * config, health (the ring's health host), run (deploy outputs) or log
     * (found in the deploy logs).
     */
    val source: String,
    /** The AI's reason for recommending the link (AI picks only). */
    val why: String? = null,
) {
    /** Found in the deploy logs: shown, but behind the curated links. */
    val isFromLogs: Boolean get() = source == "log"

    /** Something a tester opens directly, as opposed to a build output. */
    val isTryable: Boolean
        get() = kind in setOf("web", "ios", "android", "api", "docs", "dashboard")

    /**
     * The URL, but only when it is http(s); anything else is ignored.
     *
     * These links arrive over the network, and handing an arbitrary scheme to
     * an Intent is how an app ends up launching something it never meant to.
     * Returned as a string so this model stays free of Android types and can be
     * unit-tested on the JVM; the UI parses it at the point of launch.
     */
    val destination: String?
        get() {
            val scheme = runCatching { URI(url).scheme }.getOrNull()?.lowercase()
            return if (scheme == "http" || scheme == "https") url else null
        }

    /**
     * A one-word label for a compact button; the full label is the content
     * description's job.
     */
    val shortLabel: String
        get() = when (kind) {
            "web" -> "Open"
            "ios" -> "iOS"
            "android" -> "Android"
            "api" -> "API"
            "docs" -> "Docs"
            "dashboard" -> "Dashboard"
            else -> label
        }

    /** Host and path, for a compact second line. */
    val shortUrl: String
        get() {
            val parsed = runCatching { URI(url) }.getOrNull() ?: return url
            val host = parsed.host ?: return url
            val path = parsed.path.orEmpty()
            return if (path.isEmpty() || path == "/") host else host + path
        }
}

/**
 * The AI's suggestion of how to test a deployed version.
 *
 * Mirrors `store.TestPlan`. Its links are always chosen from the kit's own
 * links by the server — never invented.
 */
@Serializable
data class TestPlan(
    val summary: String = "",
    val checklist: List<String> = emptyList(),
    val links: List<TestLink> = emptyList(),
    @SerialName("generated_at")
    @Serializable(with = InstantSerializer::class)
    val generatedAt: Instant,
)

/**
 * `GET /api/apps/{app}/rings/{ring}/test-kit` — where to test a ring's current
 * version, and where its AI test plan stands.
 */
@Serializable
data class TestKit(
    val app: String,
    val ring: String,
    val version: String = "",
    val links: List<TestLink> = emptyList(),
    val plan: TestPlan? = null,
    /**
     * When the deploy's links were captured; absent for versions deployed
     * before the server had test kits.
     */
    @SerialName("captured_at")
    @Serializable(with = InstantSerializer::class)
    val capturedAt: Instant? = null,
    @SerialName("ai_enabled") val aiEnabled: Boolean = false,
    @SerialName("plan_status") val planStatus: DiagnosisStatus = DiagnosisStatus.NONE,
    @SerialName("plan_error") val planError: String? = null,
)
