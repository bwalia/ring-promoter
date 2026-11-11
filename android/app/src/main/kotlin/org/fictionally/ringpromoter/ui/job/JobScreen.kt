package org.fictionally.ringpromoter.ui.job

import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
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
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Undo
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import org.fictionally.ringpromoter.model.Job
import org.fictionally.ringpromoter.model.JobStep
import org.fictionally.ringpromoter.model.StepStatus
import org.fictionally.ringpromoter.ui.components.ErrorRow
import org.fictionally.ringpromoter.ui.theme.RpPalette
import org.fictionally.ringpromoter.ui.theme.rpPalette

/**
 * A running operation, step by step.
 *
 * Steps appear as they run, each with its own duration and its own log lines —
 * a live console rather than a progress bar, because a progress bar tells an
 * operator nothing about what is actually happening to their cluster.
 */
@Composable
fun JobScreen(
    state: JobState,
    aiEnabled: Boolean,
    onDiagnose: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val palette = rpPalette
    val job = state.job

    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        if (job == null) {
            item {
                state.error?.let { ErrorRow(error = it) } ?: Row(
                    Modifier.fillMaxWidth().padding(32.dp),
                    horizontalArrangement = Arrangement.Center,
                ) { CircularProgressIndicator() }
            }
            return@LazyColumn
        }

        item { OutcomeBanner(job, palette) }

        state.error?.let { item { ErrorRow(error = it) } }

        items(job.steps, key = { it.id }) { step -> StepCard(step, palette) }

        if (job.canRequestDiagnosis && aiEnabled) {
            item {
                OutlinedButton(onClick = onDiagnose, modifier = Modifier.fillMaxWidth()) {
                    Text("Diagnose with AI")
                }
            }
        }

        job.diagnosis?.takeIf { it.isNotEmpty() }?.let { diagnosis ->
            item {
                Card(Modifier.fillMaxWidth()) {
                    Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        Text("Diagnosis", style = MaterialTheme.typography.titleSmall)
                        Text(diagnosis, style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
        }
    }
}

/**
 * The terminal state, stated as its own thing.
 *
 * "Failed, and rolled back" is deliberately distinct from "failed": the ring is
 * running its previous version, which is a materially different situation to be
 * woken up for.
 */
@Composable
private fun OutcomeBanner(job: Job, palette: RpPalette) {
    val (label, color, icon) = when (job.outcome) {
        Job.Outcome.RUNNING -> Triple("Running", palette.inFlight, null)
        Job.Outcome.SUCCEEDED -> Triple("Succeeded", palette.healthy, Icons.Filled.CheckCircle)
        Job.Outcome.FAILED_AND_ROLLED_BACK ->
            Triple("Failed — rolled back", palette.unhealthy, Icons.Filled.Undo)

        Job.Outcome.FAILED -> Triple("Failed", palette.unhealthy, Icons.Filled.Warning)
    }

    Card(
        Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = color.copy(alpha = 0.10f)),
    ) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (icon != null) {
                    Icon(icon, contentDescription = null, tint = color, modifier = Modifier.size(18.dp))
                } else {
                    CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp, color = color)
                }
                Text(label, style = MaterialTheme.typography.titleMedium, color = color)
            }
            Text(
                "${job.promotionAction?.title ?: job.action} · ${job.app}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            job.summaryMessage?.let {
                Text(it, style = MaterialTheme.typography.bodyMedium)
            }
        }
    }
}

@Composable
private fun StepCard(step: JobStep, palette: RpPalette) {
    val color = when (step.status) {
        StepStatus.SUCCESS -> palette.healthy
        StepStatus.FAILED -> palette.unhealthy
        StepStatus.RUNNING -> palette.inFlight
        StepStatus.UNKNOWN -> palette.neutral
    }

    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(
                Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Row(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    modifier = Modifier.weight(1f),
                ) {
                    when (step.status) {
                        StepStatus.RUNNING -> CircularProgressIndicator(
                            Modifier.size(14.dp), strokeWidth = 2.dp, color = color,
                        )

                        StepStatus.SUCCESS -> Icon(
                            Icons.Filled.CheckCircle, null, Modifier.size(16.dp), tint = color,
                        )

                        else -> Icon(
                            Icons.Filled.Warning, null, Modifier.size(16.dp), tint = color,
                        )
                    }
                    Text(step.title, style = MaterialTheme.typography.titleSmall)
                }
                step.duration?.let {
                    Text(
                        "${it.toMillis() / 1000.0}s",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }

            if (step.logs.isNotEmpty()) {
                Column(
                    Modifier
                        .fillMaxWidth()
                        .background(
                            MaterialTheme.colorScheme.surfaceContainerHighest,
                            RoundedCornerShape(8.dp),
                        )
                        .padding(10.dp),
                    verticalArrangement = Arrangement.spacedBy(2.dp),
                ) {
                    step.logs.forEach { line ->
                        Text(
                            line,
                            style = MaterialTheme.typography.bodySmall,
                            fontFamily = FontFamily.Monospace,
                            modifier = Modifier.horizontalScroll(rememberScrollState()),
                            maxLines = 1,
                        )
                    }
                }
            }
        }
    }
}
