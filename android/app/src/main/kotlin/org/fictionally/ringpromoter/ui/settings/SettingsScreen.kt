package org.fictionally.ringpromoter.ui.settings

import androidx.compose.foundation.clickable
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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Layers
import androidx.compose.material.icons.filled.TheaterComedy
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import org.fictionally.ringpromoter.model.ServerVersion
import org.fictionally.ringpromoter.store.BiometricGate
import org.fictionally.ringpromoter.store.Session
import org.fictionally.ringpromoter.store.Settings
import org.fictionally.ringpromoter.ui.components.InstanceDot
import org.fictionally.ringpromoter.ui.components.RelativeTimestamp
import org.fictionally.ringpromoter.ui.theme.ThemeChoice
import org.fictionally.ringpromoter.ui.theme.rpPalette

/**
 * Clusters, security, refresh, appearance, and what the server says it is.
 */
@Composable
fun SettingsScreen(
    session: Session,
    biometrics: BiometricGate,
    onManageClusters: () -> Unit,
    onOpenGroups: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val palette = rpPalette
    val instances by session.instances.instances.collectAsStateWithLifecycle()
    val refreshInterval by session.settings.refreshInterval.collectAsStateWithLifecycle()
    val lockOnOpen by session.settings.lockOnOpen.collectAsStateWithLifecycle()
    val theme by session.settings.theme.collectAsStateWithLifecycle()

    var serverVersion by remember { mutableStateOf<ServerVersion?>(null) }
    var adding by remember { mutableStateOf(false) }
    var confirming by remember { mutableStateOf<Confirmation?>(null) }

    val active = session.activeInstance

    LaunchedEffect(session.connectionKey) {
        serverVersion = runCatching { session.api.value.serverVersion() }.getOrNull()
    }

    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        item { SectionTitle("Clusters") }

        item {
            Row(
                Modifier.fillMaxWidth().padding(vertical = 4.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                InstanceDot(session.instanceTint.color(palette))
                Spacer(Modifier.width(10.dp))
                Column(Modifier.weight(1f)) {
                    Text(session.instanceName, style = MaterialTheme.typography.titleSmall)
                    Text(
                        active?.displayHost ?: "Bundled fixtures — no server involved",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }

        item {
            NavRow(
                icon = Icons.Filled.Dns,
                title = "Manage clusters",
                detail = "${instances.size} saved",
                onClick = onManageClusters,
            )
        }

        item {
            NavRow(
                icon = Icons.Filled.Add,
                title = "Add a cluster",
                detail = null,
                onClick = { adding = true },
            )
        }

        if (!session.isDemo) {
            item {
                NavRow(
                    icon = Icons.Filled.TheaterComedy,
                    title = "Switch to demo mode",
                    detail = "Nothing you do there reaches a real server",
                    onClick = { session.enterDemoMode() },
                )
            }
        }

        item {
            Text(
                if (session.isDemo) {
                    "Demo mode is active. Nothing you do here reaches a real server."
                } else {
                    "Tokens are stored in this device's hardware key store, on this " +
                        "device only, and are left out of backups."
                },
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        // Reachable in demo mode too: the demo client implements groups in
        // full, and a screen nobody can open is a screen nobody reviews.
        if (active != null || session.isDemo) {
            item {
                NavRow(
                    icon = Icons.Filled.Layers,
                    title = "Application groups",
                    detail = "Shared by everyone on this server",
                    onClick = onOpenGroups,
                )
            }
        }

        item { SectionTitle("Security") }

        item {
            ToggleRow(
                title = "Lock the app when it closes",
                detail = when (val availability = biometrics.availability) {
                    is BiometricGate.Availability.Available ->
                        "Asks for ${availability.label} when the app returns."

                    is BiometricGate.Availability.Unavailable ->
                        "Unavailable: ${availability.reason}."
                },
                checked = lockOnOpen,
                enabled = biometrics.canAuthenticate,
                onCheckedChange = session.settings::setLockOnOpen,
            )
        }

        if (active != null) {
            item {
                ToggleRow(
                    title = "Confirm production actions",
                    detail = "Asks for the device unlock before a deploy into the last " +
                        "ring. Rollbacks are never blocked, so incident response is " +
                        "always available.",
                    checked = active.requireBiometricsForProduction,
                    enabled = biometrics.canAuthenticate,
                    onCheckedChange = {
                        session.updateActiveInstance(
                            active.copy(requireBiometricsForProduction = it),
                        )
                    },
                )
            }
        }

        item { SectionTitle("Display") }

        item {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text("Refresh", style = MaterialTheme.typography.bodyMedium)
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Settings.REFRESH_OPTIONS.forEach { seconds ->
                        FilterChip(
                            selected = refreshInterval == seconds,
                            onClick = { session.settings.setRefreshInterval(seconds) },
                            label = { Text(Settings.refreshLabel(seconds)) },
                        )
                    }
                }
                Text(
                    "The Overview refreshes itself at this interval while it is on screen. " +
                        "Choose “Manual only” on a metered connection.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }

        item {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text("Appearance", style = MaterialTheme.typography.bodyMedium)
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    ThemeChoice.entries.forEach { choice ->
                        FilterChip(
                            selected = theme == choice,
                            onClick = { session.settings.setTheme(choice) },
                            label = { Text(choice.label) },
                        )
                    }
                }
            }
        }

        item { SectionTitle("Server") }

        item {
            Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
                val version = serverVersion
                if (version == null) {
                    Text(
                        "Reading…",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                } else {
                    Fact("Version", version.version)
                    Fact("Commit", version.shortCommit, monospace = true)
                    Fact("Built", version.builtAt)
                    version.startedAt?.let {
                        Row(
                            Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                        ) {
                            Text("Started", style = MaterialTheme.typography.bodySmall)
                            RelativeTimestamp(it, style = MaterialTheme.typography.bodySmall)
                        }
                    }
                }
                active?.displayHost?.let { Fact("Host", it) }
                Fact(
                    "Production password",
                    if (session.prodProtected) "Required" else "Not configured",
                )
                Fact("AI diagnosis", if (session.aiEnabled) "Available" else "Not configured")
            }
        }

        item { HorizontalDivider(Modifier.padding(vertical = 10.dp)) }

        item {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                OutlinedButton(
                    onClick = { confirming = Confirmation.SIGN_OUT },
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Text("Sign out", color = palette.unhealthy)
                }
                OutlinedButton(
                    onClick = { confirming = Confirmation.FORGET_EVERYTHING },
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Text("Forget every cluster", color = palette.unhealthy)
                }
                Text(
                    "Forgetting removes every saved server and deletes its token from the " +
                        "key store. Nothing is left on this device.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }

    if (adding) {
        InstanceEditorDialog(
            session = session,
            editing = null,
            switchesOnSave = instances.isEmpty(),
            onDismiss = { adding = false },
        )
    }

    confirming?.let { which ->
        androidx.compose.material3.AlertDialog(
            onDismissRequest = { confirming = null },
            title = {
                Text(
                    when (which) {
                        Confirmation.SIGN_OUT -> "Sign out of ${session.instanceName}?"
                        Confirmation.FORGET_EVERYTHING -> "Forget every cluster?"
                    },
                )
            },
            text = {
                Text(
                    when (which) {
                        Confirmation.SIGN_OUT ->
                            "The cluster stays saved, along with its token."

                        Confirmation.FORGET_EVERYTHING ->
                            "Every saved server and every stored token is deleted. This " +
                                "cannot be undone."
                    },
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    when (which) {
                        Confirmation.SIGN_OUT -> session.disconnect()
                        Confirmation.FORGET_EVERYTHING -> {
                            session.instances.removeAll()
                            session.disconnect()
                        }
                    }
                    confirming = null
                }) {
                    Text(
                        when (which) {
                            Confirmation.SIGN_OUT -> "Sign out"
                            Confirmation.FORGET_EVERYTHING -> "Forget everything"
                        },
                        color = palette.unhealthy,
                    )
                }
            },
            dismissButton = {
                TextButton(onClick = { confirming = null }) { Text("Cancel") }
            },
        )
    }
}

private enum class Confirmation { SIGN_OUT, FORGET_EVERYTHING }

@Composable
private fun SectionTitle(text: String) {
    Text(
        text,
        style = MaterialTheme.typography.labelLarge,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(top = 14.dp),
    )
}

@Composable
private fun NavRow(
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    title: String,
    detail: String?,
    onClick: () -> Unit,
) {
    Row(
        Modifier.fillMaxWidth().clickable(onClick = onClick).padding(vertical = 10.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, contentDescription = null, modifier = Modifier.size(18.dp))
        Column(Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.bodyMedium)
            detail?.let {
                Text(
                    it,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun ToggleRow(
    title: String,
    detail: String,
    checked: Boolean,
    enabled: Boolean,
    onCheckedChange: (Boolean) -> Unit,
) {
    Row(
        Modifier.fillMaxWidth().padding(vertical = 6.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.bodyMedium)
            Text(
                detail,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Switch(checked = checked, enabled = enabled, onCheckedChange = onCheckedChange)
    }
}

@Composable
private fun Fact(label: String, value: String, monospace: Boolean = false) {
    Row(
        Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Text(label, style = MaterialTheme.typography.bodySmall)
        Text(
            value,
            style = MaterialTheme.typography.bodySmall,
            fontFamily = if (monospace) FontFamily.Monospace else null,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}
