package org.fictionally.ringpromoter.widget

import android.content.Context
import androidx.glance.appwidget.updateAll
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import org.fictionally.ringpromoter.store.WidgetPublisher
import org.fictionally.ringpromoter.ui.overview.AppSummary
import java.time.Duration
import java.time.Instant

/**
 * Writes the snapshot and nudges Glance.
 *
 * Rate-limited: the Overview can poll every 15 seconds, and redrawing every
 * home-screen widget that often spends the system's update budget for no
 * visible gain. The cache is still written each time, so the next redraw —
 * whenever the system asks for one — is current.
 */
class GlanceWidgetPublisher(context: Context) : WidgetPublisher {

    private val app = context.applicationContext
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    @Volatile
    private var lastUpdate: Instant? = null

    override fun publish(
        instanceName: String,
        instanceTint: String,
        isDemo: Boolean,
        apps: List<AppSummary>,
    ) {
        WidgetSnapshotStore.write(
            app,
            WidgetSnapshot.of(instanceName, instanceTint, isDemo, apps),
        )
        val now = Instant.now()
        val last = lastUpdate
        if (last != null && Duration.between(last, now) < MINIMUM_GAP) return
        lastUpdate = now
        redraw()
    }

    override fun clear() {
        WidgetSnapshotStore.clear(app)
        lastUpdate = null
        redraw()
    }

    private fun redraw() {
        scope.launch { runCatching { PipelineWidget().updateAll(app) } }
    }

    private companion object {
        val MINIMUM_GAP: Duration = Duration.ofSeconds(60)
    }
}
