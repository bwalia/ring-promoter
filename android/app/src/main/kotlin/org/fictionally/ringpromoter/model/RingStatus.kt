package org.fictionally.ringpromoter.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.time.Instant

/**
 * The promotion-policy gates guarding entry INTO a ring.
 *
 * Mirrors `promoter.RingGates`. The app uses this to decide what a promote or
 * seed sheet must collect before it is allowed to submit.
 */
@Serializable
data class RingGates(
    /** A window must be open before anything may enter this ring. */
    @SerialName("maintenance_window") val maintenanceWindow: Boolean = false,
    /** A release engineer must have recorded a GO for the exact version. */
    @SerialName("qa_signoff") val qaSignoff: Boolean = false,
    /** The operation must carry a valid change-request code. */
    @SerialName("change_request") val changeRequest: Boolean = false,
    /** The CR validation backend ("test", "jira"). Informational only. */
    @SerialName("change_request_provider") val changeRequestProvider: String? = null,
    /**
     * Whether a window is open right now. Only meaningful when
     * [maintenanceWindow] is true.
     */
    @SerialName("maintenance_window_open") val maintenanceWindowOpen: Boolean = false,
    /**
     * A Grafana dashboard's go/no-go verdict is required to enter this ring.
     * Optional: servers that predate the Grafana gate omit it.
     */
    val grafana: Boolean? = null,
    /** The live verdict, present only when [grafana] is true. */
    @SerialName("grafana_status") val grafanaStatus: GrafanaVerdict? = null,
) {
    /** Whether any gate guards this ring at all. */
    val isGated: Boolean get() = maintenanceWindow || qaSignoff || changeRequest

    /**
     * The maintenance gate is guarding and currently shut. A promotion into
     * this ring would be refused with 409 until a window opens.
     */
    val isBlockedByClosedWindow: Boolean get() = maintenanceWindow && !maintenanceWindowOpen

    companion object {
        val NONE = RingGates()
    }
}

/**
 * One evaluation of a ring's Grafana go/no-go gate.
 *
 * Mirrors `grafana.Result`. Only the fields the app reads are decoded.
 * [verdict] is "go", "check", "no_go" or "unknown"; only "no_go" blocks.
 */
@Serializable
data class GrafanaVerdict(
    val verdict: String,
    val dashboard: String? = null,
    @SerialName("dashboard_url") val dashboardUrl: String? = null,
    val error: String? = null,
)

/**
 * The tracked deploy state of one app in one ring.
 *
 * Mirrors `store.RingState`, carried inside an [ActionResult].
 */
@Serializable
data class RingState(
    val app: String = "",
    val ring: String = "",
    @SerialName("current_version") val currentVersion: String = "",
    @SerialName("previous_version") val previousVersion: String = "",
    val healthy: Boolean = false,
    @SerialName("auto_promote") val autoPromote: Boolean = false,
    @SerialName("updated_at")
    @Serializable(with = InstantSerializer::class)
    val updatedAt: Instant = DISTANT_PAST,
)

/**
 * The read model for one ring of an application.
 *
 * Mirrors `promoter.RingView` from `GET /api/apps/{app}/rings`.
 */
@Serializable
data class RingStatus(
    val ring: Ring,
    /**
     * The app declares this ring in config. An unconfigured ring is shown as
     * "not part of this app's pipeline", never as an empty deployable ring.
     */
    val configured: Boolean = false,
    @SerialName("current_version") val currentVersion: String = "",
    @SerialName("previous_version") val previousVersion: String = "",
    /**
     * What the ring's health endpoint reports it is actually running, which can
     * differ from [currentVersion] when a deploy has drifted.
     */
    @SerialName("live_version") val liveVersion: String = "",
    /** Health as last stored by a promotion. */
    val healthy: Boolean = false,
    /** Health as freshly checked when this response was built. */
    @SerialName("live_healthy") val liveHealthy: Boolean = false,
    /** Why the live check failed, when it did. */
    @SerialName("live_health_error") val liveHealthError: String? = null,
    /**
     * Round-trip time of the live health check, in milliseconds. Absent when
     * the ring has no health endpoint or the check never completed. Drives the
     * orbit radius on the Rings of Applications screen.
     */
    @SerialName("latency_ms") val latencyMs: Int? = null,
    /** Time to first byte of the live health check, when reported. */
    @SerialName("ttfb_ms") val ttfbMs: Int? = null,
    @SerialName("auto_promote") val autoPromote: Boolean = false,
    /**
     * Config owns this ring's auto-promote switch: the API toggle returns 409,
     * so the control must be rendered disabled with an explanation.
     */
    @SerialName("auto_promote_managed") val autoPromoteManaged: Boolean = false,
    @SerialName("updated_at")
    @Serializable(with = InstantSerializer::class)
    val updatedAt: Instant = DISTANT_PAST,
    /**
     * The server will accept a promotion out of this ring. The app must not
     * offer Promote when this is false.
     */
    @SerialName("can_promote_from") val canPromoteFrom: Boolean = false,
    /** Gates guarding entry into THIS ring. */
    val gates: RingGates = RingGates.NONE,
    /**
     * Where to test the current version: config links, the ring's public host,
     * and what its deploy produced. Absent on older servers.
     */
    val links: List<TestLink>? = null,
    /** An AI test plan is stored for the current version. */
    @SerialName("has_test_plan") val hasTestPlan: Boolean? = null,
) {
    /** A ring holding no version has never been deployed to. */
    val isEmpty: Boolean get() = currentVersion.isEmpty()

    /** Rolling back is only legal when a previous version exists to return to. */
    val canRollBack: Boolean get() = previousVersion.isNotEmpty()

    /**
     * [updatedAt] is Go's zero time for a ring nothing has ever touched; the UI
     * must show "never", not "1 Jan 0001".
     */
    val hasBeenDeployedTo: Boolean get() = !isEmpty && updatedAt.epochSecond > 0

    /**
     * The stored health and the fresh check disagree — worth surfacing, because
     * it usually means the ring degraded since the last promotion.
     */
    val healthIsDrifting: Boolean get() = configured && !isEmpty && healthy != liveHealthy

    /** The endpoint is serving a different version than the one recorded here. */
    val versionIsDrifting: Boolean
        get() = configured && !isEmpty && liveVersion.isNotEmpty() && liveVersion != currentVersion

    /**
     * The single health verdict the UI leads with. The fresh check wins: it is
     * what is true right now.
     */
    val isHealthy: Boolean get() = liveHealthy

    /** Rings needing an operator's attention: deployed, but not healthy. */
    val needsAttention: Boolean get() = configured && !isEmpty && !liveHealthy
}

/** `GET /api/apps/{app}/rings` */
@Serializable
data class RingsResponse(val rings: List<RingStatus> = emptyList())
