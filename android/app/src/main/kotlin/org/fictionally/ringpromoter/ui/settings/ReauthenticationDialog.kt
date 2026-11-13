package org.fictionally.ringpromoter.ui.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
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
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.store.Session
import org.fictionally.ringpromoter.ui.components.ErrorRow

/**
 * Shown when the server rejects the stored token mid-session.
 *
 * It takes the operator straight to the one thing that fixes it rather than
 * showing a dismissible error on whichever screen happened to make the call.
 * It cannot be dismissed by tapping outside: every screen behind it is now
 * unable to load, so sending the operator back to one would be a dead end.
 */
@Composable
fun ReauthenticationDialog(session: Session) {
    val scope = rememberCoroutineScope()
    var token by remember { mutableStateOf("") }
    var isSaving by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<ApiError?>(null) }
    val instance = session.activeInstance ?: return

    AlertDialog(
        onDismissRequest = { },
        title = { Text("Token expired") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text(
                    "${instance.name} rejected the saved API token. Paste a current one to " +
                        "carry on.",
                    style = MaterialTheme.typography.bodyMedium,
                )
                OutlinedTextField(
                    value = token,
                    onValueChange = { token = it },
                    label = { Text("API token") },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    keyboardOptions = KeyboardOptions(
                        capitalization = KeyboardCapitalization.None,
                        imeAction = ImeAction.Done,
                    ),
                    modifier = Modifier.fillMaxWidth(),
                )
                error?.let { ErrorRow(error = it) }
            }
        },
        confirmButton = {
            TextButton(
                enabled = !isSaving && token.isNotBlank(),
                onClick = {
                    scope.launch {
                        isSaving = true
                        error = null
                        val trimmed = token.trim()
                        try {
                            session.instances.save(instance, trimmed)
                            session.connect(instance, trimmed)
                            session.loadCapabilities()
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
            // Signing out is the only other way forward, and it is honest about
            // what it does: the cluster stays saved, the session does not.
            TextButton(
                enabled = !isSaving,
                onClick = {
                    session.clearReauthenticationFlag()
                    session.disconnect()
                },
            ) { Text("Sign out") }
        },
    )
}
