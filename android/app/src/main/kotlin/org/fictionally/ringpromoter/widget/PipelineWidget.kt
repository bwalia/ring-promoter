package org.fictionally.ringpromoter.widget

import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.glance.GlanceId
import androidx.glance.GlanceModifier
import androidx.glance.GlanceTheme
import androidx.glance.action.clickable
import androidx.glance.appwidget.GlanceAppWidget
import androidx.glance.appwidget.GlanceAppWidgetReceiver
import androidx.glance.appwidget.cornerRadius
import androidx.glance.appwidget.provideContent
import androidx.glance.background
import androidx.glance.layout.Alignment
import androidx.glance.layout.Column
import androidx.glance.layout.Row
import androidx.glance.layout.Spacer
import androidx.glance.layout.fillMaxSize
import androidx.glance.layout.fillMaxWidth
import androidx.glance.layout.padding
import androidx.glance.layout.size
import androidx.glance.layout.width
import androidx.glance.text.FontWeight
import androidx.glance.text.Text
import androidx.glance.text.TextStyle
import androidx.glance.unit.ColorProvider
import org.fictionally.ringpromoter.ui.DeepLink
import java.time.Duration
import java.time.Instant

/**
 * One application's pipeline at a glance: the one that most needs attention.
 *
 * Reads only the snapshot the app cached — never the network, and never a
 * token. Tapping it opens that application in the app.
 */
class PipelineWidget : GlanceAppWidget() {

    override suspend fun provideGlance(context: Context, id: GlanceId) {
        val snapshot = WidgetSnapshotStore.read(context)
        provideContent {
            GlanceTheme {
                if (snapshot == null) {
                    Empty()
                } else {
                    Pipeline(context, snapshot)
                }
            }
        }
    }
}

class PipelineWidgetReceiver : GlanceAppWidgetReceiver() {
    override val glanceAppWidget: GlanceAppWidget = PipelineWidget()
}

private object WidgetColours {
    val healthy = Color(0.36f, 0.80f, 0.53f)
    val unhealthy = Color(1.00f, 0.45f, 0.42f)
    val neutral = Color(0.62f, 0.65f, 0.70f)

    /** The instance tints, by the name the app stores. */
    fun instance(name: String): Color = when (name.lowercase()) {
        "red" -> Color(0.90f, 0.35f, 0.33f)
        "green" -> Color(0.36f, 0.80f, 0.53f)
        "orange" -> Color(0.95f, 0.65f, 0.25f)
        "purple" -> Color(0.78f, 0.55f, 0.95f)
        "graphite" -> Color(0.55f, 0.57f, 0.60f)
        else -> Color(0.44f, 0.68f, 1.00f)
    }
}

@androidx.compose.runtime.Composable
private fun Empty() {
    Column(
        modifier = GlanceModifier
            .fillMaxSize()
            .background(GlanceTheme.colors.widgetBackground)
            .padding(12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(
            "Ring Promoter",
            style = TextStyle(fontWeight = FontWeight.Medium, color = GlanceTheme.colors.onSurface),
        )
        Text(
            "Open the app to connect.",
            style = TextStyle(
                fontSize = 11.sp,
                color = GlanceTheme.colors.onSurfaceVariant,
            ),
        )
    }
}

@androidx.compose.runtime.Composable
private fun Pipeline(context: Context, snapshot: WidgetSnapshot) {
    val app = snapshot.featured
    Column(
        modifier = GlanceModifier
            .fillMaxSize()
            .background(GlanceTheme.colors.widgetBackground)
            .padding(12.dp)
            .let { base ->
                if (app == null) base else base.clickable(openApp(context, app.name))
            },
    ) {
        Row(
            modifier = GlanceModifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Dot(WidgetColours.instance(snapshot.instanceTint))
            Spacer(GlanceModifier.width(5.dp))
            Text(
                if (snapshot.isDemo) "Demo" else snapshot.instanceName,
                style = TextStyle(
                    fontSize = 11.sp,
                    fontWeight = FontWeight.Medium,
                    color = GlanceTheme.colors.onSurfaceVariant,
                ),
                maxLines = 1,
            )
            Spacer(GlanceModifier.defaultWeight())
            if (snapshot.unhealthyCount > 0) {
                Text(
                    "${snapshot.unhealthyCount} unhealthy",
                    style = TextStyle(
                        fontSize = 11.sp,
                        color = ColorProvider(WidgetColours.unhealthy),
                    ),
                )
            }
        }

        if (app == null) {
            Text(
                "No applications.",
                style = TextStyle(
                    fontSize = 12.sp,
                    color = GlanceTheme.colors.onSurfaceVariant,
                ),
            )
            return@Column
        }

        Spacer(GlanceModifier.size(6.dp))
        Text(
            app.title,
            style = TextStyle(
                fontWeight = FontWeight.Bold,
                color = GlanceTheme.colors.onSurface,
            ),
            maxLines = 1,
        )
        Spacer(GlanceModifier.size(4.dp))

        app.rings.filter { it.configured }.forEach { ring ->
            Row(
                modifier = GlanceModifier.fillMaxWidth().padding(vertical = 2.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Dot(
                    when {
                        ring.empty -> WidgetColours.neutral
                        ring.healthy -> WidgetColours.healthy
                        else -> WidgetColours.unhealthy
                    },
                )
                Spacer(GlanceModifier.width(6.dp))
                Text(
                    ring.name,
                    style = TextStyle(
                        fontSize = 11.sp,
                        color = GlanceTheme.colors.onSurfaceVariant,
                    ),
                    maxLines = 1,
                )
                Spacer(GlanceModifier.defaultWeight())
                Text(
                    ring.version.ifEmpty { "—" },
                    style = TextStyle(
                        fontSize = 11.sp,
                        color = GlanceTheme.colors.onSurface,
                    ),
                    maxLines = 1,
                )
            }
        }

        Spacer(GlanceModifier.defaultWeight())
        Text(
            age(snapshot.capturedAt),
            style = TextStyle(
                fontSize = 10.sp,
                color = GlanceTheme.colors.onSurfaceVariant,
            ),
        )
    }
}

@androidx.compose.runtime.Composable
private fun Dot(color: Color) {
    Spacer(
        GlanceModifier
            .size(7.dp)
            .background(ColorProvider(color))
            .cornerRadius(3.dp),
    )
}

/**
 * How old the snapshot is, stated plainly.
 *
 * A widget that cannot say when it last heard from the server is a widget an
 * operator can be misled by: a stale "all healthy" is worse than no widget.
 */
private fun age(capturedAt: Instant, now: Instant = Instant.now()): String {
    val minutes = Duration.between(capturedAt, now).toMinutes()
    return when {
        minutes < 1 -> "just now"
        minutes < 60 -> "$minutes min ago"
        minutes < 60 * 24 -> "${minutes / 60} h ago"
        else -> "${minutes / (60 * 24)} d ago"
    }
}

/** Opens the application this widget is showing. */
private fun openApp(context: Context, app: String) =
    androidx.glance.appwidget.action.actionStartActivity(
        Intent(Intent.ACTION_VIEW, Uri.parse(DeepLink.url(app))).apply {
            setPackage(context.packageName)
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        },
    )

