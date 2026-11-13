package org.fictionally.ringpromoter.widget

import android.content.Context
import kotlinx.serialization.Serializable
import org.fictionally.ringpromoter.net.RpJson
import org.fictionally.ringpromoter.ui.overview.AppSummary
import java.time.Instant

/**
 * The minimum the widget needs to draw a pipeline, written by the app after a
 * successful refresh.
 *
 * Deliberately **contains no token, no URL and no secret** — only what is
 * already visible on the lock screen anyway: app names, versions and health.
 * A widget cannot prompt for a password or handle a 401, so giving it
 * credentials would add risk with nothing to show for it. That is why it reads
 * this cache rather than the network.
 */
@Serializable
data class WidgetSnapshot(
    /**
     * A short label for which control plane this came from, so a staging
     * widget is never mistaken for production.
     */
    val instanceName: String,
    /** The instance's colour tag, as a stable name the widget maps to a colour. */
    val instanceTint: String,
    val apps: List<AppCell>,
    val capturedAtEpochSeconds: Long,
    /** True when the snapshot came from demo mode, so the widget can say so. */
    val isDemo: Boolean,
) {
    @Serializable
    data class RingCell(
        val name: String,
        val label: String,
        val version: String,
        val healthy: Boolean,
        /** The app declares this ring; an unconfigured one renders as a gap. */
        val configured: Boolean,
        /** Nothing has ever been deployed here. */
        val empty: Boolean,
    )

    @Serializable
    data class AppCell(val name: String, val title: String, val rings: List<RingCell>) {
        val unhealthyCount: Int
            get() = rings.count { it.configured && !it.empty && !it.healthy }
    }

    val capturedAt: Instant get() = Instant.ofEpochSecond(capturedAtEpochSeconds)

    val unhealthyCount: Int get() = apps.sumOf { it.unhealthyCount }

    /** Whichever app is in trouble; a healthy system shows the first. */
    val featured: AppCell?
        get() = apps.firstOrNull { it.unhealthyCount > 0 } ?: apps.firstOrNull()

    companion object {
        fun of(
            instanceName: String,
            instanceTint: String,
            isDemo: Boolean,
            summaries: List<AppSummary>,
            capturedAt: Instant = Instant.now(),
        ): WidgetSnapshot = WidgetSnapshot(
            instanceName = instanceName,
            instanceTint = instanceTint,
            isDemo = isDemo,
            capturedAtEpochSeconds = capturedAt.epochSecond,
            apps = summaries.map { summary ->
                AppCell(
                    name = summary.name,
                    title = summary.title,
                    rings = summary.rings.map { ring ->
                        RingCell(
                            name = ring.ring.name,
                            label = ring.ring.label,
                            version = ring.currentVersion,
                            healthy = ring.isHealthy,
                            configured = ring.configured,
                            empty = ring.isEmpty,
                        )
                    },
                )
            },
        )
    }
}

/**
 * Where the snapshot lives: this app's own preferences, in its own sandbox.
 *
 * Its own file, separate from `rp.prefs`, so it is obvious at a glance that
 * nothing from the credential side of the app is in here.
 */
object WidgetSnapshotStore {
    private const val FILE = "rp.widget"
    private const val KEY = "snapshot"

    fun write(context: Context, snapshot: WidgetSnapshot) {
        val encoded = runCatching { RpJson.encodeToString(snapshot) }.getOrNull() ?: return
        prefs(context).edit().putString(KEY, encoded).apply()
    }

    fun read(context: Context): WidgetSnapshot? {
        val raw = prefs(context).getString(KEY, null) ?: return null
        return runCatching { RpJson.decodeFromString<WidgetSnapshot>(raw) }.getOrNull()
    }

    /** Called on sign-out: a widget must not outlive the session it describes. */
    fun clear(context: Context) {
        prefs(context).edit().remove(KEY).apply()
    }

    private fun prefs(context: Context) =
        context.applicationContext.getSharedPreferences(FILE, Context.MODE_PRIVATE)
}
