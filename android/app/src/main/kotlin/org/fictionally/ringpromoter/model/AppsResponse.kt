package org.fictionally.ringpromoter.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.util.Locale

/** Geographic pin from config `location`. Apps without one are omitted. */
@Serializable
data class AppLocation(
    val lat: Double,
    val lng: Double,
    val city: String? = null,
    val region: String? = null,
) {
    val label: String
        get() {
            val c = city?.takeIf { it.isNotEmpty() }
            val r = region?.takeIf { it.isNotEmpty() }
            return when {
                c != null && r != null -> "$c, $r"
                c != null -> c
                r != null -> r
                else -> String.format(Locale.US, "%.1f°, %.1f°", lat, lng)
            }
        }
}

/**
 * `GET /api/apps` — the control plane's shape: which apps exist, the ring
 * pipeline they all share, and two capability flags the UI must respect.
 */
@Serializable
data class AppsResponse(
    /** Application names. Every API path uses these, never the display titles. */
    val apps: List<String> = emptyList(),
    /**
     * Display titles per app (config `display_name`, falling back to the name).
     * Purely cosmetic.
     */
    val titles: Map<String, String> = emptyMap(),
    /** Config `location` pins. A missing key means the app is unplaced. */
    val locations: Map<String, AppLocation> = emptyMap(),
    /** The ordered ring pipeline, lowest environment first. */
    val rings: List<Ring> = emptyList(),
    /**
     * The server was started with a production password, so any action that
     * deploys into the last ring must carry it.
     */
    @SerialName("prod_protected") val prodProtected: Boolean = false,
    /** AI failure diagnosis is configured, so "Diagnose with AI" is offered. */
    @SerialName("ai_enabled") val aiEnabled: Boolean = false,
    /**
     * The server can suggest what to test on a deployed ring (AI test plan).
     * Absent on servers that predate test kits.
     */
    @SerialName("ai_test_plans") val aiTestPlans: Boolean? = null,
) {
    /** Display title for an app, falling back to its name. */
    fun title(forApp: String): String = titles[forApp] ?: forApp

    val pipeline: RingPipeline get() = RingPipeline(rings)
}
