package org.fictionally.ringpromoter.ui.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
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
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.net.RingPromoterClient
import org.fictionally.ringpromoter.store.Instance
import org.fictionally.ringpromoter.store.Session
import org.fictionally.ringpromoter.ui.components.ErrorRow
import org.fictionally.ringpromoter.ui.components.InstanceDot
import org.fictionally.ringpromoter.ui.theme.InstanceTint
import org.fictionally.ringpromoter.ui.theme.rpPalette

/**
 * Add a control plane, or change an existing one's name, colour or token.
 *
 * The URL and token are always probed before anything is saved, so "wrong
 * address" and "wrong token" are told apart here rather than on the first
 * screen the operator opens. Editing an instance that already works may leave
 * the token blank, which keeps the stored one.
 */
@Composable
fun InstanceEditorDialog(
    session: Session,
    editing: Instance?,
    /** Connect to it once saved. False when adding several in a row. */
    switchesOnSave: Boolean,
    onDismiss: () -> Unit,
    onSaved: (Instance) -> Unit = {},
) {
    val palette = rpPalette
    val scope = rememberCoroutineScope()

    var urlText by remember { mutableStateOf(editing?.baseUrl.orEmpty()) }
    var token by remember { mutableStateOf("") }
    var name by remember { mutableStateOf(editing?.name.orEmpty()) }
    var tint by remember {
        mutableStateOf(editing?.tint ?: session.instances.nextUnusedTint())
    }
    var requireBiometrics by remember {
        mutableStateOf(editing?.requireBiometricsForProduction ?: true)
    }
    var isSaving by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<ApiError?>(null) }
    var urlProblem by remember { mutableStateOf<String?>(null) }

    val hasStoredToken = editing != null && !session.instances.token(editing).isNullOrEmpty()
    val normalised = Instance.normalise(urlText)
    val insecure = normalised?.startsWith("http://") == true

    AlertDialog(
        onDismissRequest = { if (!isSaving) onDismiss() },
        title = { Text(if (editing == null) "Add a cluster" else "Edit ${editing.name}") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                OutlinedTextField(
                    value = urlText,
                    onValueChange = { urlText = it; urlProblem = null },
                    label = { Text("Server URL") },
                    placeholder = { Text("rp.example.com") },
                    singleLine = true,
                    isError = urlProblem != null,
                    supportingText = { urlProblem?.let { Text(it) } },
                    keyboardOptions = KeyboardOptions(
                        keyboardType = KeyboardType.Uri,
                        capitalization = KeyboardCapitalization.None,
                        imeAction = ImeAction.Next,
                    ),
                    modifier = Modifier.fillMaxWidth(),
                )

                if (insecure) {
                    Row(
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        verticalAlignment = Alignment.Top,
                    ) {
                        Icon(
                            Icons.Filled.Warning,
                            contentDescription = null,
                            tint = palette.gate,
                            modifier = Modifier.size(16.dp),
                        )
                        Text(
                            "A plain HTTP address sends the token in clear. Fine for a " +
                                "local server; not for anything else.",
                            style = MaterialTheme.typography.bodySmall,
                            color = palette.gate,
                        )
                    }
                }

                OutlinedTextField(
                    value = token,
                    onValueChange = { token = it },
                    label = { Text(if (hasStoredToken) "New API token" else "API token") },
                    supportingText = {
                        if (hasStoredToken) Text("Leave blank to keep the stored one.")
                    },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    keyboardOptions = KeyboardOptions(
                        capitalization = KeyboardCapitalization.None,
                        imeAction = ImeAction.Next,
                    ),
                    modifier = Modifier.fillMaxWidth(),
                )

                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    label = { Text("Name") },
                    placeholder = {
                        Text(normalised?.let { Instance.defaultName(it) } ?: "Production")
                    },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                    modifier = Modifier.fillMaxWidth(),
                )

                Text("Colour", style = MaterialTheme.typography.labelMedium)
                FlowRow(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                    InstanceTint.entries.forEach { option ->
                        TextButton(onClick = { tint = option }) {
                            InstanceDot(option.color(palette))
                            Spacer(Modifier.width(5.dp))
                            Text(option.label, style = MaterialTheme.typography.labelSmall)
                            if (tint == option) {
                                Spacer(Modifier.width(3.dp))
                                Icon(
                                    Icons.Filled.Check,
                                    contentDescription = "Selected",
                                    modifier = Modifier.size(13.dp),
                                )
                            }
                        }
                    }
                }

                Row(
                    Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Column(Modifier.weight(1f)) {
                        Text(
                            "Confirm production actions",
                            style = MaterialTheme.typography.bodyMedium,
                        )
                        Text(
                            "Asks for the device unlock before a deploy into the last ring. " +
                                "Rollbacks are never blocked.",
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    Switch(
                        checked = requireBiometrics,
                        onCheckedChange = { requireBiometrics = it },
                    )
                }

                error?.let { ErrorRow(error = it) }
            }
        },
        confirmButton = {
            TextButton(
                enabled = !isSaving && urlText.isNotBlank() &&
                    (token.isNotBlank() || hasStoredToken),
                onClick = {
                    val url = Instance.normalise(urlText)
                    if (url == null) {
                        urlProblem = "That is not a usable http or https address."
                        return@TextButton
                    }
                    scope.launch {
                        isSaving = true
                        error = null
                        try {
                            val secret = token.trim().ifEmpty {
                                editing?.let { session.instances.token(it) }.orEmpty()
                            }
                            // Probe first: nothing is saved that cannot be
                            // authenticated to.
                            val probe = RingPromoterClient(url, secret)
                            probe.checkHealth()
                            probe.apps()

                            val record = (editing ?: session.instances.instance(url))?.copy(
                                name = name.ifBlank {
                                    editing?.name ?: Instance.defaultName(url)
                                },
                                baseUrl = url,
                                tint = tint,
                                requireBiometricsForProduction = requireBiometrics,
                            ) ?: Instance(
                                name = name.ifBlank { Instance.defaultName(url) },
                                baseUrl = url,
                                tint = tint,
                                requireBiometricsForProduction = requireBiometrics,
                            )
                            session.instances.save(record, secret)
                            // Editing the *active* instance updates it in place
                            // rather than reconnecting, which would blank the
                            // loaded capabilities and empty every screen.
                            if (switchesOnSave && session.activeInstance?.id != record.id) {
                                session.connect(record, secret)
                            } else if (session.activeInstance?.id == record.id) {
                                session.updateActiveInstance(record)
                            }
                            onSaved(record)
                            onDismiss()
                        } catch (e: ApiError) {
                            error = e
                        } finally {
                            isSaving = false
                        }
                    }
                },
            ) {
                if (isSaving) {
                    CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
                } else {
                    Text("Save")
                }
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !isSaving) { Text("Cancel") }
        },
    )
}
