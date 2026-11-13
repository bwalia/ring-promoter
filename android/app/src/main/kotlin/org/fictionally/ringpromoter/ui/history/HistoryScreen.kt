package org.fictionally.ringpromoter.ui.history

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Cancel
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import org.fictionally.ringpromoter.model.HistoryEntry
import org.fictionally.ringpromoter.ui.components.EmptyState
import org.fictionally.ringpromoter.ui.components.ErrorRow
import org.fictionally.ringpromoter.ui.components.MarkdownText
import org.fictionally.ringpromoter.ui.components.Times
import org.fictionally.ringpromoter.ui.theme.rpPalette

/**
 * One application's history, newest first, grouped by day.
 *
 * A row expands rather than pushing a screen: the reason to open one is to read
 * a failure's message and its diagnosis, and both belong next to the entry they
 * explain.
 */
@Composable
fun HistoryScreen(
    state: HistoryState,
    viewModel: HistoryViewModel,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val groups = viewModel.groups(state.entries)

    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        state.error?.takeIf { state.entries.isEmpty() }?.let {
            item { ErrorRow(error = it, onRetry = onRetry) }
        }

        groups.forEach { group ->
            item(key = "day-${group.day}") {
                Text(
                    when (group.relative) {
                        DayGroup.Relative.TODAY -> "Today"
                        DayGroup.Relative.YESTERDAY -> "Yesterday"
                        DayGroup.Relative.EARLIER ->
                            Times.fullDate(group.entries.first().createdAt)
                    },
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(top = 8.dp),
                )
            }
            items(group.entries.size, key = { group.entries[it].id }) { index ->
                val entry = group.entries[index]
                HistoryRow(
                    entry = entry,
                    isExpanded = entry.id in state.expanded,
                    state = state,
                    viewModel = viewModel,
                )
            }
        }

        if (state.isEmpty) {
            item {
                EmptyState(
                    title = "No history yet",
                    detail = "Seeds, promotions and rollbacks appear here as they happen.",
                )
            }
        }
    }
}

@Composable
private fun HistoryRow(
    entry: HistoryEntry,
    isExpanded: Boolean,
    state: HistoryState,
    viewModel: HistoryViewModel,
) {
    val palette = rpPalette
    val tint = if (entry.succeeded) palette.healthy else palette.unhealthy

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clickable { viewModel.toggle(entry) }
            .padding(vertical = 6.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Row(
            Modifier.fillMaxWidth().semantics(mergeDescendants = true) { },
            horizontalArrangement = Arrangement.spacedBy(10.dp),
            verticalAlignment = Alignment.Top,
        ) {
            Icon(
                if (entry.succeeded) Icons.Filled.CheckCircle else Icons.Filled.Cancel,
                contentDescription = if (entry.succeeded) "Succeeded" else "Failed",
                tint = tint,
                modifier = Modifier.size(18.dp),
            )
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Row(
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(
                        entry.promotionAction?.pastTense ?: entry.action,
                        style = MaterialTheme.typography.titleSmall,
                    )
                    Text(
                        entry.ring,
                        style = MaterialTheme.typography.labelSmall,
                        modifier = Modifier
                            .background(
                                MaterialTheme.colorScheme.surfaceContainerHighest,
                                RoundedCornerShape(6.dp),
                            )
                            .padding(horizontal = 6.dp, vertical = 2.dp),
                    )
                }
                Text(
                    entry.versionTransition,
                    style = MaterialTheme.typography.labelSmall,
                    fontFamily = FontFamily.Monospace,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                if (entry.message.isNotEmpty()) {
                    Text(
                        entry.message,
                        style = MaterialTheme.typography.bodySmall,
                        color = if (entry.succeeded) {
                            MaterialTheme.colorScheme.onSurfaceVariant
                        } else {
                            palette.unhealthy
                        },
                        maxLines = if (isExpanded) Int.MAX_VALUE else 2,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
            Text(
                Times.time(entry.createdAt),
                style = MaterialTheme.typography.labelSmall,
                fontFamily = FontFamily.Monospace,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        if (isExpanded && !entry.succeeded && viewModel.aiEnabled) {
            Diagnosis(entry, state, viewModel)
        }
    }
}

/**
 * The AI explanation of a failure: whatever is already stored, otherwise an
 * offer to generate one.
 */
@Composable
private fun Diagnosis(
    entry: HistoryEntry,
    state: HistoryState,
    viewModel: HistoryViewModel,
) {
    val palette = rpPalette
    val text = state.diagnosis(entry)
    val isDiagnosing = entry.id in state.diagnosing

    // Checking costs one cheap call and may save a model call, since a
    // colleague may already have asked for this one.
    LaunchedEffect(entry.id) { viewModel.loadExistingDiagnosis(entry) }

    Column(
        modifier = Modifier.padding(start = 28.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        when {
            text != null -> MarkdownText(text, style = MaterialTheme.typography.bodySmall)

            isDiagnosing -> Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                CircularProgressIndicator(Modifier.size(14.dp), strokeWidth = 2.dp)
                Text(
                    "Working out what went wrong…",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            else -> OutlinedButton(onClick = { viewModel.diagnoseAsync(entry) }) {
                Icon(
                    Icons.Filled.AutoAwesome,
                    contentDescription = null,
                    modifier = Modifier.size(15.dp),
                )
                Spacer(Modifier.width(6.dp))
                Text("Diagnose with AI", style = MaterialTheme.typography.labelLarge)
            }
        }
        state.diagnosisErrors[entry.id]?.let {
            Text(it, style = MaterialTheme.typography.bodySmall, color = palette.gate)
        }
    }
}
