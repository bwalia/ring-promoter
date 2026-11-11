package org.fictionally.ringpromoter.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.time.Instant

/**
 * A permanent recurring window declared in the server's config file.
 *
 * Read-only from the app's point of view — it lives in git, not in the API.
 *
 * The Go struct carries no JSON tags, so this object is serialised with
 * **PascalCase** keys ("Days", "Start"), unlike everything else in the API.
 * Verified against a captured response; do not "tidy" these to snake_case.
 */
@Serializable
data class RecurringWindow(
    /** Day abbreviations ("Sat", "Sun"). Empty means every day. */
    @SerialName("Days") val days: List<String> = emptyList(),
    /** "HH:MM" in [timezone]. */
    @SerialName("Start") val start: String = "",
    /** "HH:MM"; an end before the start crosses midnight. */
    @SerialName("End") val end: String = "",
    /** IANA name; empty means UTC. */
    @SerialName("Timezone") val timezone: String = "",
) {
    val id: String get() = "${days.joinToString(",")}-$start-$end-$timezone"

    /** "Sat, Sun 02:00–04:00 Europe/London" */
    val summary: String
        get() {
            val whenPart = if (days.isEmpty()) "Every day" else days.joinToString(", ")
            val zone = timezone.ifEmpty { "UTC" }
            return "$whenPart $start–$end $zone"
        }
}

/**
 * An operator-created ad-hoc window during which deploys into a ring are
 * permitted. Mirrors `store.MaintenanceWindow`.
 */
@Serializable
data class MaintenanceWindow(
    val id: String,
    val app: String,
    /** Empty means "all guarded rings". */
    val ring: String = "",
    @SerialName("starts_at")
    @Serializable(with = InstantSerializer::class)
    val startsAt: Instant,
    @SerialName("ends_at")
    @Serializable(with = InstantSerializer::class)
    val endsAt: Instant,
    val reason: String = "",
    @SerialName("created_by") val createdBy: String = "",
    @SerialName("created_at")
    @Serializable(with = InstantSerializer::class)
    val createdAt: Instant,
) {
    /**
     * Mirrors `MaintenanceWindow.Active` on the server: start inclusive, end
     * exclusive.
     */
    fun isActive(at: Instant): Boolean = !at.isBefore(startsAt) && at.isBefore(endsAt)

    fun covers(ring: String): Boolean = this.ring.isEmpty() || this.ring == ring

    fun hasExpired(now: Instant = Instant.now()): Boolean = !endsAt.isAfter(now)

    /** "All guarded rings" when the window is not ring-specific. */
    val ringLabel: String get() = ring.ifEmpty { "All guarded rings" }
}

/**
 * The aggregate maintenance read model for an app.
 *
 * Mirrors `promoter.MaintenanceView` from
 * `GET /api/apps/{app}/maintenance-windows`.
 */
@Serializable
data class MaintenanceStatus(
    /** The app gates at least one ring behind a maintenance window. */
    val gated: Boolean = false,
    /**
     * The rings the gate guards, in pipeline order. Null for an ungated app.
     */
    @SerialName("gated_rings") val gatedRings: List<String> = emptyList(),
    /** Permanent windows from config (read-only). */
    val recurring: List<RecurringWindow> = emptyList(),
    /** Operator-created ad-hoc windows, newest first. */
    val windows: List<MaintenanceWindow> = emptyList(),
    /** Guarded ring → whether a window is open right now. */
    @SerialName("open_rings") val openRings: Map<String, Boolean> = emptyMap(),
) {
    fun isOpen(forRing: String): Boolean = openRings[forRing] ?: false

    /** Ad-hoc windows that have not yet ended, newest first. */
    fun activeAndUpcomingWindows(now: Instant = Instant.now()): List<MaintenanceWindow> =
        windows.filter { !it.hasExpired(now) }

    companion object {
        val UNGATED = MaintenanceStatus()
    }
}

/** Body of `POST /api/apps/{app}/maintenance-windows`. */
@Serializable
data class NewMaintenanceWindow(
    /** Empty string = all guarded rings. */
    val ring: String,
    @SerialName("starts_at")
    @Serializable(with = InstantSerializer::class)
    val startsAt: Instant,
    @SerialName("ends_at")
    @Serializable(with = InstantSerializer::class)
    val endsAt: Instant,
    val reason: String,
    @SerialName("created_by") val createdBy: String,
)
