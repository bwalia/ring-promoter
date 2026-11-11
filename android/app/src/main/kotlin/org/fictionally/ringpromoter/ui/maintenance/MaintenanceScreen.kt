package org.fictionally.ringpromoter.ui.maintenance

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
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.CalendarMonth
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.LockOpen
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import org.fictionally.ringpromoter.model.MaintenanceWindow
import org.fictionally.ringpromoter.ui.components.EmptyState
import org.fictionally.ringpromoter.ui.components.ErrorRow
import org.fictionally.ringpromoter.ui.components.GateBadge
import org.fictionally.ringpromoter.ui.components.GateTone
import org.fictionally.ringpromoter.ui.components.Times
import java.time.Instant

/**
 * The maintenance gate, stated in full: which rings it guards, whether each is
 * open right now, the permanent windows from the server's config, and the
 * ad-hoc ones an operator can open and close.
 */
@Composable
fun MaintenanceScreen(
    state: MaintenanceState,
    viewModel: MaintenanceViewModel,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val scope = rememberCoroutineScope()
    var pendingClose by remember { mutableStateOf<MaintenanceWindow?>(null) }
    var openingFor by remember { mutableStateOf<String?>(null) }
    val status = state.status

    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        state.error?.takeIf { !status.gated }?.let {
            item { ErrorRow(error = it, onRetry = onRetry) }
        }

        if (!status.gated) {
            if (!state.isLoading) {
                item {
                    EmptyState(
                        title = "No maintenance gate",
                        detail = "This application can be promoted at any time.",
                    )
                }
            }
            return@LazyColumn
        }

        item {
            Section("Guarded rings") {
                status.gatedRings.forEach { ring ->
                    val isOpen = status.isOpen(ring)
                    Row(
                        Modifier.fillMaxWidth().padding(vertical = 4.dp),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text(ring, style = MaterialTheme.typography.bodyMedium)
                        GateBadge(
                            if (isOpen) "Open now" else "Shut",
                            if (isOpen) GateTone.SATISFIED else GateTone.BLOCKING,
                        )
                    }
                }
                Text(
                    "Nothing may be promoted into these rings unless a window is open — " +
                        "either a scheduled one or one opened here.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }

        if (status.recurring.isNotEmpty()) {
            item {
                Section("Scheduled") {
                    status.recurring.forEach { window ->
                        Row(
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            modifier = Modifier.padding(vertical = 3.dp),
                        ) {
                            Icon(
                                Icons.Filled.CalendarMonth,
                                contentDescription = null,
                                modifier = Modifier.size(16.dp),
                                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                            Text(window.summary, style = MaterialTheme.typography.bodyMedium)
                        }
                    }
                    Text(
                        "Defined in the server's config file and read-only from here.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }

        item {
            Text(
                "Ad-hoc windows",
                style = MaterialTheme.typography.labelLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        if (status.windows.isEmpty()) {
            item {
                Text(
                    "None open.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }

        items(status.windows, key = { it.id }) { window ->
            WindowCard(window) { pendingClose = window }
        }

        item {
            OutlinedButton(
                onClick = { openingFor = status.gatedRings.firstOrNull() ?: "" },
                modifier = Modifier.fillMaxWidth(),
            ) {
                Icon(Icons.Filled.Add, contentDescription = null, modifier = Modifier.size(16.dp))
                Spacer(Modifier.width(6.dp))
                Text("Open a window")
            }
        }
    }

    openingFor?.let { ring ->
        OpenWindowDialog(
            rings = state.status.gatedRings,
            initialRing = ring,
            defaultAuthor = viewModel.defaultAuthor,
            viewModel = viewModel,
            onDismiss = { openingFor = null },
        )
    }

    pendingClose?.let { window ->
        AlertDialog(
            onDismissRequest = { pendingClose = null },
            title = { Text("Close this window?") },
            text = {
                Text(
                    "Promotions into ${window.ringLabel} will be blocked again until " +
                        "another window opens.",
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    val target = window
                    pendingClose = null
                    scope.launch { viewModel.closeWindow(target) }
                }) { Text("Close window") }
            },
            dismissButton = {
                TextButton(onClick = { pendingClose = null }) { Text("Keep it open") }
            },
        )
    }
}

@Composable
private fun Section(title: String, content: @Composable () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(
            title,
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Card(
            colors = CardDefaults.cardColors(
                containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
            ),
        ) {
            Column(
                Modifier.padding(14.dp),
                verticalArrangement = Arrangement.spacedBy(6.dp),
            ) { content() }
        }
    }
}

/** One ad-hoc window and its live state. */
@Composable
private fun WindowCard(window: MaintenanceWindow, onClose: () -> Unit) {
    val now = Instant.now()
    val isActive = window.isActive(now)
    val expired = window.hasExpired(now)

    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
        ),
    ) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(
                Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(window.ringLabel, style = MaterialTheme.typography.titleSmall)
                Row(
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Icon(
                        if (isActive) Icons.Filled.LockOpen else Icons.Filled.Lock,
                        contentDescription = null,
                        modifier = Modifier.size(14.dp),
                        tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Text(
                        if (isActive) "Open" else if (expired) "Expired" else "Scheduled",
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            Text(
                Times.range(window.startsAt, window.endsAt),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (window.reason.isNotEmpty()) {
                Text(window.reason, style = MaterialTheme.typography.bodySmall)
            }
            if (window.createdBy.isNotEmpty()) {
                Text(
                    "Opened by ${window.createdBy}",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (!expired) {
                TextButton(onClick = onClose, modifier = Modifier.align(Alignment.End)) {
                    Text("Close now")
                }
            }
        }
    }
}
