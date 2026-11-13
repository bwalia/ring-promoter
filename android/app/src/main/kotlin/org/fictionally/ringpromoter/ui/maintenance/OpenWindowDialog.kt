package org.fictionally.ringpromoter.ui.maintenance

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.ui.components.ErrorRow
import org.fictionally.ringpromoter.ui.components.Times
import java.time.Instant

/**
 * Open an ad-hoc window so a gated promotion can proceed.
 *
 * A reason is required, and the window closes itself. Both are deliberate: a
 * window with no reason is unauditable, and one that has to be closed by hand
 * is one that stays open until somebody notices.
 */
@Composable
fun OpenWindowDialog(
    rings: List<String>,
    initialRing: String,
    defaultAuthor: String,
    viewModel: MaintenanceViewModel,
    onDismiss: () -> Unit,
) {
    val scope = rememberCoroutineScope()
    var ring by remember { mutableStateOf(initialRing) }
    var duration by remember { mutableStateOf(MaintenanceViewModel.DURATIONS[1]) }
    var reason by remember { mutableStateOf("") }
    var author by remember { mutableStateOf(defaultAuthor) }
    var isSaving by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<ApiError?>(null) }

    AlertDialog(
        onDismissRequest = { if (!isSaving) onDismiss() },
        title = { Text("Open a window") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                if (rings.size > 1) {
                    Text("Ring", style = MaterialTheme.typography.labelMedium)
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        rings.forEach { candidate ->
                            FilterChip(
                                selected = ring == candidate,
                                onClick = { ring = candidate },
                                label = { Text(candidate) },
                            )
                        }
                    }
                } else {
                    Text(
                        "Ring: ${ring.ifEmpty { "all guarded rings" }}",
                        style = MaterialTheme.typography.bodyMedium,
                    )
                }

                Text("Length", style = MaterialTheme.typography.labelMedium)
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    MaintenanceViewModel.DURATIONS.forEach { option ->
                        FilterChip(
                            selected = duration == option,
                            onClick = { duration = option },
                            label = { Text(MaintenanceViewModel.label(option)) },
                        )
                    }
                }
                Text(
                    "Closes at ${Times.time(Instant.now().plus(duration))}.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )

                OutlinedTextField(
                    value = reason,
                    onValueChange = { reason = it },
                    label = { Text("Reason") },
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Next),
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = author,
                    onValueChange = { author = it },
                    label = { Text("Opened by") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                    modifier = Modifier.fillMaxWidth(),
                )

                error?.let { ErrorRow(error = it) }
            }
        },
        confirmButton = {
            TextButton(
                enabled = !isSaving && reason.isNotBlank(),
                onClick = {
                    scope.launch {
                        isSaving = true
                        error = null
                        try {
                            viewModel.openWindow(
                                ring = ring,
                                duration = duration,
                                reason = reason.trim(),
                                author = author.trim(),
                            )
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
                    Text("Open")
                }
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !isSaving) { Text("Cancel") }
        },
    )
}
