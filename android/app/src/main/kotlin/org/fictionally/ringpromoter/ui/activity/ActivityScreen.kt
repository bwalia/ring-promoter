package org.fictionally.ringpromoter.ui.activity

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Autorenew
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Dangerous
import androidx.compose.material.icons.filled.Undo
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import org.fictionally.ringpromoter.model.Job
import org.fictionally.ringpromoter.ui.components.EmptyState
import org.fictionally.ringpromoter.ui.components.ErrorRow
import org.fictionally.ringpromoter.ui.components.RelativeTimestamp
import org.fictionally.ringpromoter.ui.theme.RpPalette
import org.fictionally.ringpromoter.ui.theme.rpPalette

/**
 * What everyone is doing on this control plane.
 */
@Composable
fun ActivityScreen(
    state: ActivityState,
    title: (String) -> String,
    onOpenJob: (app: String, id: String) -> Unit,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val palette = rpPalette

    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        state.error?.takeIf { state.jobs.isEmpty() }?.let {
            item { ErrorRow(error = it, onRetry = onRetry) }
        }

        if (state.running.isNotEmpty()) {
            item { SectionHeader("Running now", palette.inFlight) }
            items(state.running, key = { it.id }) { job ->
                JobRow(job, title(job.app), palette) { onOpenJob(job.app, job.id) }
            }
        }

        if (state.finished.isNotEmpty()) {
            item { SectionHeader("Recent", MaterialTheme.colorScheme.onSurfaceVariant) }
            items(state.finished, key = { it.id }) { job ->
                JobRow(job, title(job.app), palette) { onOpenJob(job.app, job.id) }
            }
        }

        if (state.isEmpty) {
            item {
                EmptyState(
                    title = "Nothing has run yet",
                    detail = "Seeds, promotions and rollbacks show up here — yours and " +
                        "everyone else's.",
                )
            }
        }

        state.lastUpdated?.let {
            item {
                RelativeTimestamp(
                    instant = it,
                    prefix = "Updated",
                    modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
                )
            }
        }
    }
}

@Composable
private fun SectionHeader(text: String, tint: Color) {
    Text(
        text,
        style = MaterialTheme.typography.labelLarge,
        color = tint,
        modifier = Modifier.padding(top = 6.dp),
    )
}

/** One job, through to its live log. */
@Composable
private fun JobRow(
    job: Job,
    appTitle: String,
    palette: RpPalette,
    onOpen: () -> Unit,
) {
    val tint = when (job.outcome) {
        Job.Outcome.RUNNING -> palette.inFlight
        Job.Outcome.SUCCEEDED -> palette.healthy
        Job.Outcome.FAILED, Job.Outcome.FAILED_AND_ROLLED_BACK -> palette.unhealthy
    }
    val icon: ImageVector = when (job.outcome) {
        Job.Outcome.RUNNING -> Icons.Filled.Autorenew
        Job.Outcome.SUCCEEDED -> Icons.Filled.CheckCircle
        Job.Outcome.FAILED_AND_ROLLED_BACK -> Icons.Filled.Undo
        Job.Outcome.FAILED -> Icons.Filled.Dangerous
    }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onOpen)
            .padding(vertical = 6.dp)
            .semantics(mergeDescendants = true) { },
        horizontalArrangement = Arrangement.spacedBy(10.dp),
        verticalAlignment = Alignment.Top,
    ) {
        // The icon is the signal and the colour reinforces it, so the row still
        // reads on a monochrome display or to a colour-blind operator.
        Icon(icon, contentDescription = null, tint = tint, modifier = Modifier.size(18.dp))

        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    appTitle,
                    style = MaterialTheme.typography.titleSmall,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Text(
                    job.promotionAction?.title ?: job.action,
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            job.result?.let { result ->
                Text(
                    result.fromRing
                        ?.let { "${result.version}: $it → ${result.ring}" }
                        ?: "${result.version} → ${result.ring}",
                    style = MaterialTheme.typography.labelSmall,
                    fontFamily = FontFamily.Monospace,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            job.summaryMessage?.let {
                Text(
                    it,
                    style = MaterialTheme.typography.bodySmall,
                    color = if (job.outcome == Job.Outcome.SUCCEEDED) {
                        MaterialTheme.colorScheme.onSurfaceVariant
                    } else {
                        palette.unhealthy
                    },
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }

        RelativeTimestamp(job.startedAt)
    }
}
