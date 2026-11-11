package org.fictionally.ringpromoter.ui.rings

import android.content.Context
import android.provider.Settings
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.BlurCircular
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.TableRows
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import org.fictionally.ringpromoter.ui.components.EmptyState
import org.fictionally.ringpromoter.ui.components.ErrorRow
import org.fictionally.ringpromoter.ui.overview.OverviewState
import org.fictionally.ringpromoter.ui.theme.rpPalette

/** Orbit, or the lanes that carry the same facts in a list. */
enum class DescentMode { ORBIT, LANES }

/**
 * Rings of Applications: the whole fleet on one stage.
 *
 * The promotion rings are the orbits and Earth is live users, so "how far has
 * this version got?" is a distance rather than a table to read. The lanes
 * fallback is not a lesser view — on a narrow screen or a large fleet it is the
 * readable one, and it is offered as a choice as well as a fallback.
 */
@Composable
fun RingsScreen(
    state: OverviewState,
    ringNames: List<String>,
    onOpenApp: (String) -> Unit,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val palette = rpPalette
    val density = LocalDensity.current
    val reduceMotion = rememberReduceMotion()

    var preferredMode by remember { mutableStateOf(DescentMode.ORBIT) }
    var filterGroupId by remember { mutableStateOf<String?>(null) }
    var selectedId by remember { mutableStateOf<String?>(null) }

    val summaries = state.summaries.filter { summary ->
        filterGroupId == null || state.groups.firstOrNull { it.id == filterGroupId }
            ?.let { summary.name in it } == true
    }
    val (apps, spans) = DescentApp.ordered(
        summaries = summaries,
        groups = if (filterGroupId == null) state.groups else emptyList(),
        order = ringNames,
    )
    val selected = apps.firstOrNull { it.id == selectedId }

    Box(modifier.fillMaxSize().background(EarthPalette.space)) {
        if (state.error != null && state.summaries.isEmpty()) {
            ErrorRow(error = state.error, onRetry = onRetry, modifier = Modifier.padding(16.dp))
            return@Box
        }
        if (apps.isEmpty()) {
            EmptyState(
                title = if (filterGroupId != null) "No apps in this group" else "No applications",
                detail = "Applications appear here once the control plane reports them.",
            )
            return@Box
        }

        BoxWithConstraints(Modifier.fillMaxSize()) {
            val widthDp = maxWidth.value.toDouble()
            val fits = DescentLayout.descentFits(widthDp, apps.size)
            val mode = if (fits) preferredMode else DescentMode.LANES

            Column(Modifier.fillMaxSize()) {
                Row(
                    Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 6.dp),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    if (fits) {
                        ModeChip(
                            label = "Orbit",
                            icon = Icons.Filled.BlurCircular,
                            selected = mode == DescentMode.ORBIT,
                            onClick = { preferredMode = DescentMode.ORBIT },
                        )
                        ModeChip(
                            label = "Lanes",
                            icon = Icons.Filled.TableRows,
                            selected = mode == DescentMode.LANES,
                            onClick = { preferredMode = DescentMode.LANES },
                        )
                    } else {
                        // Say why, rather than silently showing a different
                        // screen than the one the operator last chose.
                        Text(
                            if (apps.size > DescentLayout.MAX_SPOKES) {
                                "${apps.size} apps — too many spokes to read as an orbit."
                            } else {
                                "The screen is too narrow for the orbit."
                            },
                            style = MaterialTheme.typography.labelSmall,
                            color = Color.White.copy(alpha = 0.5f),
                        )
                    }
                    Spacer(Modifier.weight(1f))
                    filterGroupId?.let { id ->
                        val name = state.groups.firstOrNull { it.id == id }?.name ?: "Group"
                        TextButton(onClick = { filterGroupId = null }) {
                            Icon(
                                Icons.Filled.Close,
                                contentDescription = null,
                                modifier = Modifier.size(14.dp),
                            )
                            Spacer(Modifier.width(4.dp))
                            Text(name)
                        }
                    }
                }

                Box(Modifier.weight(1f)) {
                    when (mode) {
                        DescentMode.ORBIT -> DescentStage(
                            apps = apps,
                            spans = spans,
                            ringNames = ringNames,
                            selectedId = selectedId,
                            reduceMotion = reduceMotion,
                            onSelect = { selectedId = it },
                            onDeselect = { selectedId = null },
                            onOpenGroup = { filterGroupId = it; selectedId = null },
                        )

                        DescentMode.LANES -> LanesView(
                            apps = apps,
                            spans = spans,
                            ringNames = ringNames,
                            selectedId = selectedId,
                            filtering = filterGroupId != null,
                            onSelect = { selectedId = it },
                            onOpenGroup = { filterGroupId = it; selectedId = null },
                        )
                    }
                }

                selected?.let { app ->
                    DetailCard(
                        app = app,
                        onOpen = { onOpenApp(app.id) },
                        onClose = { selectedId = null },
                    )
                }
            }
        }
    }
}

@Composable
private fun ModeChip(
    label: String,
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    selected: Boolean,
    onClick: () -> Unit,
) {
    FilterChip(
        selected = selected,
        onClick = onClick,
        leadingIcon = { Icon(icon, contentDescription = null, Modifier.size(16.dp)) },
        label = { Text(label) },
    )
}

/**
 * What the selected spoke says, in a sentence, with the way through to the
 * application itself.
 */
@Composable
private fun DetailCard(app: DescentApp, onOpen: () -> Unit, onClose: () -> Unit) {
    val palette = rpPalette
    Card(
        modifier = Modifier.fillMaxWidth().padding(12.dp),
        colors = CardDefaults.cardColors(
            containerColor = Color.White.copy(alpha = 0.06f),
        ),
    ) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(
                    app.status.icon,
                    contentDescription = null,
                    tint = app.status.tint(palette),
                    modifier = Modifier.size(16.dp),
                )
                Spacer(Modifier.width(8.dp))
                Text(
                    app.title,
                    style = MaterialTheme.typography.titleSmall,
                    color = Color.White,
                    modifier = Modifier.weight(1f),
                )
                IconButton(onClick = onClose, modifier = Modifier.size(28.dp)) {
                    Icon(
                        Icons.Filled.Close,
                        contentDescription = "Close",
                        tint = Color.White.copy(alpha = 0.6f),
                        modifier = Modifier.size(16.dp),
                    )
                }
            }
            Text(
                app.sentence,
                style = MaterialTheme.typography.bodySmall,
                color = Color.White.copy(alpha = 0.8f),
            )
            app.busyAction?.let {
                Text(
                    "$it in flight",
                    style = MaterialTheme.typography.labelSmall,
                    color = palette.inFlight,
                )
            }
            app.loadError?.let {
                Text(
                    it,
                    style = MaterialTheme.typography.labelSmall,
                    color = palette.unhealthy,
                )
            }
            Text(
                app.spoke.nodes
                    .filter { it.state != DescentLayout.NodeState.OFF }
                    .joinToString("   ") { "${it.ring} ${it.version ?: "—"}" },
                style = MaterialTheme.typography.labelSmall,
                fontFamily = FontFamily.Monospace,
                color = Color.White.copy(alpha = 0.55f),
            )
            TextButton(onClick = onOpen) { Text("Open ${app.title}") }
        }
    }
}

/**
 * Whether the device has asked for less motion.
 *
 * Android's equivalent of iOS's Reduce Motion is the animator duration scale in
 * developer options and accessibility settings; zero means "do not animate".
 * Honoured here because this screen is the only one in the app that moves on
 * its own.
 */
@Composable
private fun rememberReduceMotion(): Boolean {
    val context = LocalContext.current
    return remember(context) { animatorScale(context) == 0f }
}

private fun animatorScale(context: Context): Float = Settings.Global.getFloat(
    context.contentResolver,
    Settings.Global.ANIMATOR_DURATION_SCALE,
    1f,
)
