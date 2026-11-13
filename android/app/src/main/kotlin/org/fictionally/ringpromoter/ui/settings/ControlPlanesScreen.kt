package org.fictionally.ringpromoter.ui.settings

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
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Key
import androidx.compose.material.icons.filled.SignalWifiStatusbarConnectedNoInternet4
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import org.fictionally.ringpromoter.store.Instance
import org.fictionally.ringpromoter.store.ProbeStatus
import org.fictionally.ringpromoter.store.Session
import org.fictionally.ringpromoter.ui.components.EmptyState
import org.fictionally.ringpromoter.ui.components.InstanceDot
import org.fictionally.ringpromoter.ui.theme.rpPalette

/**
 * Every saved control plane, with how each one is doing right now.
 *
 * Tapping one switches to it and lands on its Overview. Each is probed with its
 * own token on a short timeout, one after another, so one dead host does not
 * hold up the list.
 */
@Composable
fun ControlPlanesScreen(
    session: Session,
    onOpened: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val palette = rpPalette
    val instances by session.instances.instances.collectAsStateWithLifecycle()
    val activeId by session.instances.activeId.collectAsStateWithLifecycle()

    val statuses = remember { mutableStateMapOf<String, ProbeStatus>() }
    var adding by remember { mutableStateOf(false) }
    var editing by remember { mutableStateOf<Instance?>(null) }
    var pendingRemoval by remember { mutableStateOf<Instance?>(null) }

    // Re-probe whenever the set of instances changes, not only on first
    // appearance: one just added should not sit on "Checking…".
    LaunchedEffect(instances) {
        instances.forEach { instance ->
            statuses[instance.id] = ProbeStatus.Checking
            statuses[instance.id] = ProbeStatus.probe(instance, session.instances.token(instance))
        }
    }

    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        item {
            OutlinedButton(
                onClick = { adding = true },
                modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp),
            ) {
                Icon(Icons.Filled.Add, contentDescription = null, modifier = Modifier.size(16.dp))
                Spacer(Modifier.width(6.dp))
                Text("Add a cluster")
            }
        }

        if (instances.isEmpty()) {
            item {
                EmptyState(
                    title = "No clusters yet",
                    detail = "Add the URL and API token of each Ring Promoter you look after.",
                )
            }
        }

        items(instances, key = { it.id }) { instance ->
            Row(
                Modifier
                    .fillMaxWidth()
                    .clickable {
                        if (instance.id == activeId) {
                            onOpened()
                        } else if (session.activate(instance)) {
                            onOpened()
                        } else {
                            // The token has gone from the key store; the only
                            // way onto this cluster is to paste a new one.
                            editing = instance
                        }
                    }
                    .padding(vertical = 8.dp)
                    .semantics(mergeDescendants = true) { },
                verticalAlignment = Alignment.CenterVertically,
            ) {
                InstanceDot(instance.tint.color(palette))
                Spacer(Modifier.width(10.dp))
                Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Row(
                        horizontalArrangement = Arrangement.spacedBy(6.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text(instance.name, style = MaterialTheme.typography.titleSmall)
                        if (instance.id == activeId) {
                            Text(
                                "active",
                                style = MaterialTheme.typography.labelSmall,
                                color = palette.healthy,
                            )
                        }
                    }
                    Text(
                        instance.displayHost,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    StatusLine(statuses[instance.id] ?: ProbeStatus.Checking)
                }
                IconButton(onClick = { editing = instance }) {
                    Icon(Icons.Filled.Edit, contentDescription = "Edit ${instance.name}")
                }
                TextButton(onClick = { pendingRemoval = instance }) { Text("Remove") }
            }
            HorizontalDivider()
        }

        item {
            Text(
                "Tap a cluster to open its applications and rings. Each one keeps its own " +
                    "token in the device key store.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 12.dp),
            )
        }
    }

    if (adding) {
        InstanceEditorDialog(
            session = session,
            editing = null,
            // An operator adding several clusters should not be switched to
            // each one as it is saved.
            switchesOnSave = instances.isEmpty(),
            onDismiss = { adding = false },
        )
    }

    editing?.let { instance ->
        InstanceEditorDialog(
            session = session,
            editing = instance,
            switchesOnSave = false,
            onDismiss = { editing = null },
        )
    }

    pendingRemoval?.let { instance ->
        AlertDialog(
            onDismissRequest = { pendingRemoval = null },
            title = { Text("Remove ${instance.name}?") },
            text = { Text("Its API token is deleted from the device key store too.") },
            confirmButton = {
                TextButton(onClick = {
                    val wasActive = session.activeInstance?.id == instance.id
                    session.instances.remove(instance)
                    if (wasActive) session.disconnect()
                    pendingRemoval = null
                }) { Text("Remove") }
            },
            dismissButton = {
                TextButton(onClick = { pendingRemoval = null }) { Text("Keep it") }
            },
        )
    }
}

/** What a quick look at one control plane found. */
@Composable
private fun StatusLine(status: ProbeStatus) {
    val palette = rpPalette
    when (status) {
        ProbeStatus.Checking -> Row(
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            CircularProgressIndicator(Modifier.size(11.dp), strokeWidth = 1.5.dp)
            Text(
                "Checking…",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        ProbeStatus.NoToken -> Labelled(
            Icons.Filled.Key,
            "No token saved — tap edit to add one",
            palette.gate,
        )

        ProbeStatus.TokenRejected -> Labelled(
            Icons.Filled.Key,
            "Token rejected — tap edit to replace it",
            palette.unhealthy,
        )

        is ProbeStatus.Unreachable -> Labelled(
            Icons.Filled.SignalWifiStatusbarConnectedNoInternet4,
            status.message,
            palette.unhealthy,
        )

        is ProbeStatus.Reachable -> Row(
            horizontalArrangement = Arrangement.spacedBy(10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Labelled(
                Icons.Filled.CheckCircle,
                "${status.apps} app${if (status.apps == 1) "" else "s"} · " +
                    "${status.rings} rings",
                palette.healthy,
            )
            if (status.failing > 0) {
                Text(
                    "${status.failing} failed",
                    style = MaterialTheme.typography.labelSmall,
                    color = palette.unhealthy,
                )
            }
            if (status.running > 0) {
                Text(
                    "${status.running} deploying",
                    style = MaterialTheme.typography.labelSmall,
                    color = palette.inFlight,
                )
            }
        }
    }
}

@Composable
private fun Labelled(
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    text: String,
    tint: androidx.compose.ui.graphics.Color,
) {
    Row(
        horizontalArrangement = Arrangement.spacedBy(5.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, contentDescription = null, tint = tint, modifier = Modifier.size(12.dp))
        Text(
            text,
            style = MaterialTheme.typography.labelSmall,
            color = tint,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
    }
}
