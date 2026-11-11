package org.fictionally.ringpromoter.ui.detail

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowForward
import androidx.compose.material.icons.filled.Science
import androidx.compose.material.icons.filled.Undo
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import org.fictionally.ringpromoter.model.RingStatus
import org.fictionally.ringpromoter.ui.components.ErrorRow
import org.fictionally.ringpromoter.ui.components.GateBadge
import org.fictionally.ringpromoter.ui.components.GateTone
import org.fictionally.ringpromoter.ui.components.HealthDot
import org.fictionally.ringpromoter.ui.theme.HealthPresentation
import org.fictionally.ringpromoter.ui.theme.rpPalette

/**
 * One application, ring by ring: what each is running, whether it is healthy,
 * what guards it, and what may legally be done to it.
 */
@Composable
fun AppDetailScreen(
    state: AppDetailState,
    viewModel: AppDetailViewModel,
    onAction: (ActionRequest) -> Unit,
    onOpenHistory: () -> Unit,
    onOpenMaintenance: () -> Unit,
    onOpenTestKit: (ring: String) -> Unit,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        state.error?.let { item { ErrorRow(error = it, onRetry = onRetry) } }

        item {
            Row(
                Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                OutlinedButton(onClick = onOpenHistory) { Text("History") }
                if (state.maintenance.gated) {
                    OutlinedButton(onClick = onOpenMaintenance) { Text("Maintenance") }
                }
            }
        }

        items(state.rings, key = { it.ring.name }) { ring ->
            RingCard(
                ring = ring,
                state = state,
                viewModel = viewModel,
                onAction = onAction,
                onOpenTestKit = onOpenTestKit,
            )
        }
    }
}

/**
 * One action on a ring.
 *
 * The button stays tappable even when the rules disallow the action: tapping it
 * opens the sheet, which explains *why* it cannot be done. A dead control that
 * does nothing when pressed tells an operator nothing at 2am.
 */
@Composable
private fun ActionButton(
    label: String,
    icon: androidx.compose.ui.graphics.vector.ImageVector?,
    allowed: Boolean,
    onClick: () -> Unit,
) {
    val palette = rpPalette
    TextButton(onClick = onClick) {
        if (icon != null) {
            Icon(icon, contentDescription = null, modifier = Modifier.size(16.dp))
            Spacer(Modifier.width(6.dp))
        }
        Text(label, color = if (allowed) palette.inFlight else palette.disabled)
    }
}

@Composable
private fun RingCard(
    ring: RingStatus,
    state: AppDetailState,
    viewModel: AppDetailViewModel,
    onAction: (ActionRequest) -> Unit,
    onOpenTestKit: (ring: String) -> Unit,
) {
    val palette = rpPalette
    val presentation = HealthPresentation.of(ring)

    val promote = viewModel.rules.promote(ring, state.rings)
    val seed = viewModel.rules.seed(ring)
    val rollback = viewModel.rules.rollback(ring)

    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = if (!ring.configured) {
                MaterialTheme.colorScheme.surfaceContainerLowest
            } else {
                MaterialTheme.colorScheme.surfaceContainerLow
            },
        ),
    ) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(
                Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Row(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    HealthDot(presentation)
                    Text(ring.ring.label, style = MaterialTheme.typography.titleMedium)
                }
                Text(
                    presentation.label,
                    style = MaterialTheme.typography.labelMedium,
                    color = presentation.tint(palette),
                )
            }

            if (!ring.configured) {
                Text(
                    "Not part of this application's pipeline.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                return@Column
            }

            Text(
                if (ring.isEmpty) "Nothing deployed" else ring.currentVersion,
                style = MaterialTheme.typography.bodyLarge,
                fontFamily = if (ring.isEmpty) FontFamily.Default else FontFamily.Monospace,
            )

            if (ring.previousVersion.isNotEmpty()) {
                Text(
                    "Previously ${ring.previousVersion}",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            // Drift is the thing an operator most needs told, and the thing a
            // plain "200 OK" health check hides.
            if (ring.versionIsDrifting) {
                Text(
                    "Serving ${ring.liveVersion}, but ${ring.currentVersion} was deployed.",
                    style = MaterialTheme.typography.bodySmall,
                    color = palette.gate,
                )
            }
            ring.liveHealthError?.let {
                Text(it, style = MaterialTheme.typography.bodySmall, color = palette.unhealthy)
            }
            ring.latencyMs?.let {
                Text(
                    "Health check ${it}ms",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            if (ring.gates.isGated) {
                FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    // A CR code and a sign-off are judged per action and per
                    // version, so here they only say "this gate exists". The
                    // maintenance window is a property of the ring itself, so
                    // it is the one that can honestly report its state.
                    if (ring.gates.changeRequest) {
                        GateBadge("Change request", GateTone.PRESENT)
                    }
                    if (ring.gates.qaSignoff) GateBadge("QA sign-off", GateTone.PRESENT)
                    if (ring.gates.maintenanceWindow) {
                        GateBadge(
                            if (ring.gates.maintenanceWindowOpen) "Window open" else "Window shut",
                            if (ring.gates.maintenanceWindowOpen) GateTone.SATISFIED
                            else GateTone.BLOCKING,
                        )
                    }
                }
            }

            HorizontalDivider()

            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                // Each button is enabled only when the rules say the server
                // would accept it. A disabled button still opens the sheet,
                // which explains why — silence would be worse.
                ActionButton(
                    label = "Promote",
                    icon = Icons.Filled.ArrowForward,
                    allowed = promote.isAllowed,
                    onClick = { onAction(ActionRequest.Promote(ring)) },
                )
                ActionButton(
                    label = "Seed",
                    icon = null,
                    allowed = seed.isAllowed,
                    onClick = { onAction(ActionRequest.Seed(ring)) },
                )
                ActionButton(
                    label = "Roll back",
                    icon = Icons.Filled.Undo,
                    allowed = rollback.isAllowed,
                    onClick = { onAction(ActionRequest.Rollback(ring)) },
                )
                if (!ring.isEmpty) {
                    ActionButton(
                        label = "Test",
                        icon = Icons.Filled.Science,
                        allowed = true,
                        onClick = { onOpenTestKit(ring.ring.name) },
                    )
                }
            }

            Row(
                Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(Modifier.weight(1f)) {
                    Text("Auto-promote", style = MaterialTheme.typography.bodyMedium)
                    if (ring.autoPromoteManaged) {
                        Text(
                            "Declared in the server's config.",
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
                Switch(
                    checked = ring.autoPromote,
                    enabled = !ring.autoPromoteManaged,
                    onCheckedChange = { enabled ->
                        onAction(ActionRequest.AutoPromote(ring, enabled))
                    },
                )
            }
        }
    }
}
