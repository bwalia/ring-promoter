package org.fictionally.ringpromoter.ui.onboarding

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
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
import org.fictionally.ringpromoter.ui.theme.rpPalette

/**
 * The first screen, before any control plane exists.
 *
 * Demo mode is offered as a first-class choice, not buried: the app is useless
 * without a server you run yourself, so being able to see the whole thing
 * without one is the difference between an evaluation and a dead end.
 */
@Composable
fun OnboardingScreen(
    session: Session,
    onConnected: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val palette = rpPalette
    val scope = rememberCoroutineScope()

    var urlText by remember { mutableStateOf("") }
    var token by remember { mutableStateOf("") }
    var name by remember { mutableStateOf("") }
    var isConnecting by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<ApiError?>(null) }
    var urlProblem by remember { mutableStateOf<String?>(null) }

    val normalised = Instance.normalise(urlText)
    val insecure = normalised?.startsWith("http://") == true

    Column(
        modifier = modifier
            .fillMaxSize()
            // The app draws edge to edge, so this screen — which has no app bar
            // to do it for us — must keep its own content out from under the
            // status and navigation bars.
            .safeDrawingPadding()
            .verticalScroll(rememberScrollState())
            .padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Text("Ring Promoter", style = MaterialTheme.typography.headlineMedium)
        Text(
            "A client for a control plane you run yourself. There is no account to " +
                "create and no backend of ours involved.",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        OutlinedButton(
            onClick = {
                session.enterDemoMode()
                onConnected()
            },
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text("Explore in demo mode")
        }
        Text(
            "Runs the whole app — promotions, rollbacks, live jobs and every gate " +
                "that says no — from data bundled inside it. No server, no network.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        HorizontalDivider(Modifier.padding(vertical = 6.dp))

        Text("Connect to a control plane", style = MaterialTheme.typography.titleMedium)

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
                    "This is a plain HTTP address, so the token would cross the network " +
                        "in clear. Fine for a local server; not for anything else.",
                    style = MaterialTheme.typography.bodySmall,
                    color = palette.gate,
                )
            }
        }

        OutlinedTextField(
            value = token,
            onValueChange = { token = it },
            label = { Text("API token") },
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
            label = { Text("Name (optional)") },
            placeholder = { Text(normalised?.let { Instance.defaultName(it) } ?: "Production") },
            supportingText = {
                Text("Shown on every screen that can change something.")
            },
            singleLine = true,
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
            modifier = Modifier.fillMaxWidth(),
        )

        error?.let { ErrorRow(error = it) }

        Button(
            enabled = !isConnecting && urlText.isNotBlank() && token.isNotBlank(),
            onClick = {
                val url = Instance.normalise(urlText)
                if (url == null) {
                    urlProblem = "That is not a usable http or https address."
                    return@Button
                }
                scope.launch {
                    isConnecting = true
                    error = null
                    try {
                        // Probe before saving, so "wrong URL" and "wrong token"
                        // are told apart here rather than on the first screen
                        // the operator opens.
                        val probe = RingPromoterClient(url, token.trim())
                        probe.checkHealth()
                        probe.apps()

                        val existing = session.instances.instance(withBaseUrl = url)
                        val instance = existing?.copy(
                            name = name.ifBlank { existing.name },
                        ) ?: Instance(
                            name = name.ifBlank { Instance.defaultName(url) },
                            baseUrl = url,
                            tint = session.instances.nextUnusedTint(),
                        )
                        session.instances.save(instance, token.trim())
                        session.connect(instance, token.trim())
                        onConnected()
                    } catch (e: ApiError) {
                        error = e
                    } finally {
                        isConnecting = false
                    }
                }
            },
            modifier = Modifier.fillMaxWidth(),
        ) {
            if (isConnecting) {
                CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
            } else {
                Text("Connect")
            }
        }
    }
}
