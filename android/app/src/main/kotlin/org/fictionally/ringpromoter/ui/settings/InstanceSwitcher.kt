package org.fictionally.ringpromoter.ui.settings

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.TheaterComedy
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import org.fictionally.ringpromoter.store.Instance
import org.fictionally.ringpromoter.store.Session
import org.fictionally.ringpromoter.ui.components.InstanceDot
import org.fictionally.ringpromoter.ui.theme.rpPalette

/**
 * The instance banner as a menu: tap it on any top-level screen to jump to
 * another saved control plane, without a detour through Settings.
 *
 * The banner itself is unchanged, so "which cluster am I on?" still reads at a
 * glance — promoting on the wrong cluster is the mistake the whole instance
 * concept exists to prevent. The menu only adds a way off it.
 */
@Composable
fun InstanceSwitcher(
    session: Session,
    onManageClusters: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val palette = rpPalette
    val instances by session.instances.instances.collectAsStateWithLifecycle()
    val activeId by session.instances.activeId.collectAsStateWithLifecycle()

    var expanded by remember { mutableStateOf(false) }
    var needsToken by remember { mutableStateOf<Instance?>(null) }

    Row(
        modifier = modifier
            .clickable { expanded = true }
            .padding(horizontal = 10.dp, vertical = 6.dp)
            .semantics { contentDescription = "Cluster: ${session.instanceName}. Switch." },
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        InstanceDot(session.instanceTint.color(palette))
        Text(session.instanceName, style = MaterialTheme.typography.labelMedium)
    }

    DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
        instances.forEach { instance ->
            DropdownMenuItem(
                onClick = {
                    expanded = false
                    if (instance.id != activeId && !session.activate(instance)) {
                        // The token has gone behind the app's back; the only
                        // way onto this cluster is to paste a new one.
                        needsToken = instance
                    }
                },
                leadingIcon = { InstanceDot(instance.tint.color(palette)) },
                trailingIcon = {
                    if (instance.id == activeId) {
                        Icon(
                            Icons.Filled.Check,
                            contentDescription = "Active",
                            modifier = Modifier.size(16.dp),
                        )
                    }
                },
                text = {
                    Column {
                        Text(instance.name, style = MaterialTheme.typography.bodyMedium)
                        Text(
                            instance.displayHost,
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                },
            )
        }

        if (instances.isNotEmpty()) HorizontalDivider()

        DropdownMenuItem(
            onClick = { expanded = false; onManageClusters() },
            leadingIcon = {
                Icon(Icons.Filled.Dns, contentDescription = null, Modifier.size(18.dp))
            },
            text = { Text("Manage clusters…") },
        )

        if (!session.isDemo) {
            DropdownMenuItem(
                onClick = { expanded = false; session.enterDemoMode() },
                leadingIcon = {
                    Icon(
                        Icons.Filled.TheaterComedy,
                        contentDescription = null,
                        Modifier.size(18.dp),
                    )
                },
                text = { Text("Switch to demo mode") },
            )
        }
    }

    needsToken?.let { instance ->
        InstanceEditorDialog(
            session = session,
            editing = instance,
            switchesOnSave = true,
            onDismiss = { needsToken = null },
        )
    }
}
