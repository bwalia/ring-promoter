package org.fictionally.ringpromoter.ui.overview

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import org.fictionally.ringpromoter.ui.components.EmptyState
import org.fictionally.ringpromoter.ui.components.ErrorRow
import org.fictionally.ringpromoter.ui.components.GateBadge
import org.fictionally.ringpromoter.ui.components.GateTone
import org.fictionally.ringpromoter.ui.components.RingChip
import org.fictionally.ringpromoter.ui.theme.rpPalette

/**
 * The first screen an operator sees.
 *
 * Applications in trouble are pinned above healthy ones, because that is the
 * question this screen exists to answer: is anything wrong right now?
 */
@Composable
fun OverviewScreen(
    state: OverviewState,
    onSearch: (String) -> Unit,
    onSelectGroup: (String?) -> Unit,
    onOpenApp: (String) -> Unit,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val palette = rpPalette

    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        item {
            OutlinedTextField(
                value = state.searchText,
                onValueChange = onSearch,
                label = { Text("Search applications") },
                singleLine = true,
                leadingIcon = { Icon(Icons.Filled.Search, contentDescription = null) },
                modifier = Modifier.fillMaxWidth(),
            )
        }

        if (state.groups.isNotEmpty()) {
            item {
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    FilterChip(
                        selected = state.selectedGroupId == null,
                        onClick = { onSelectGroup(null) },
                        label = { Text("All") },
                    )
                    state.groups.forEach { group ->
                        FilterChip(
                            selected = state.selectedGroupId == group.id,
                            onClick = {
                                onSelectGroup(if (state.selectedGroupId == group.id) null else group.id)
                            },
                            label = { Text(group.name) },
                        )
                    }
                }
            }
        }

        state.error?.let { error ->
            item { ErrorRow(error = error, onRetry = onRetry) }
        }

        if (state.isStale) {
            item {
                Text(
                    "Showing the last successful refresh.",
                    style = MaterialTheme.typography.labelMedium,
                    color = palette.gate,
                )
            }
        }

        if (state.isAllClear && state.searchText.isEmpty()) {
            item {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier.padding(vertical = 4.dp),
                ) {
                    Icon(
                        Icons.Filled.CheckCircle,
                        contentDescription = null,
                        tint = palette.healthy,
                        modifier = Modifier.size(18.dp),
                    )
                    Text(
                        "All applications healthy",
                        style = MaterialTheme.typography.titleSmall,
                        color = palette.healthy,
                    )
                }
            }
        }

        items(state.orderedSummaries, key = { it.name }) { summary ->
            AppRow(summary = summary, onClick = { onOpenApp(summary.name) })
        }

        if (state.orderedSummaries.isEmpty() && state.error == null) {
            item {
                if (state.isLoading && state.summaries.isEmpty()) {
                    Row(
                        Modifier.fillMaxWidth().padding(32.dp),
                        horizontalArrangement = Arrangement.Center,
                    ) { CircularProgressIndicator() }
                } else {
                    EmptyState(
                        title = if (state.summaries.isEmpty()) "No applications"
                        else "Nothing matches",
                        detail = if (state.summaries.isEmpty()) {
                            "This control plane has no applications configured yet."
                        } else {
                            "No application matches that search or group."
                        },
                    )
                }
            }
        }
    }
}

/**
 * One application: its name, why it is in trouble if it is, and the state of
 * every ring in its pipeline.
 */
@Composable
private fun AppRow(summary: AppSummary, onClick: () -> Unit) {
    val palette = rpPalette
    Card(
        onClick = onClick,
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = if (summary.isInTrouble) {
                palette.unhealthy.copy(alpha = 0.07f)
            } else {
                MaterialTheme.colorScheme.surfaceContainerLow
            },
        ),
    ) {
        Column(
            Modifier.padding(14.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Row(
                Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    summary.title,
                    style = MaterialTheme.typography.titleMedium,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f, fill = false),
                )
                summary.busyAction?.let { action ->
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(6.dp),
                    ) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(13.dp),
                            strokeWidth = 2.dp,
                            color = palette.inFlight,
                        )
                        Text(
                            action,
                            style = MaterialTheme.typography.labelMedium,
                            color = palette.inFlight,
                        )
                    }
                }
            }

            summary.troubleSummary?.let { trouble ->
                Text(
                    trouble,
                    style = MaterialTheme.typography.bodySmall,
                    color = palette.unhealthy,
                    maxLines = 3,
                    overflow = TextOverflow.Ellipsis,
                )
            }

            FlowRow(
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                verticalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                summary.rings.forEach { ring ->
                    RingChip(ring = ring, isBusy = summary.isBusy)
                }
            }

            val gated = summary.rings.filter { it.gates.isGated }
            if (gated.isNotEmpty()) {
                FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    gated.forEach { ring ->
                        // Informational only. Whether the CR code or sign-off
                        // is satisfied depends on the version being moved, which
                        // this screen does not know, so it must not claim green.
                        GateBadge(
                            label = "${ring.ring.label} gated",
                            tone = if (ring.gates.isBlockedByClosedWindow) {
                                GateTone.BLOCKING
                            } else {
                                GateTone.PRESENT
                            },
                        )
                    }
                }
            }
        }
    }
}
