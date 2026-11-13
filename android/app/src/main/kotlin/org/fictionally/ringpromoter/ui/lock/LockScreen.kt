package org.fictionally.ringpromoter.ui.lock

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.fragment.app.FragmentActivity
import kotlinx.coroutines.launch
import org.fictionally.ringpromoter.store.BiometricGate
import org.fictionally.ringpromoter.ui.theme.rpPalette

/**
 * Covers the whole app while it is locked.
 *
 * Opaque and on top of everything: the point is that a borrowed or
 * shoulder-surfed phone shows nothing about which production clusters this
 * operator can deploy to.
 */
@Composable
fun LockScreen(
    biometrics: BiometricGate,
    activity: FragmentActivity?,
    modifier: Modifier = Modifier,
) {
    val palette = rpPalette
    val scope = rememberCoroutineScope()
    var isAuthenticating by remember { mutableStateOf(false) }
    var failed by remember { mutableStateOf(false) }

    suspend fun unlock() {
        val host = activity ?: return
        if (isAuthenticating) return
        isAuthenticating = true
        try {
            failed = !biometrics.authenticate(
                activity = host,
                title = "Unlock Ring Promoter",
                subtitle = "Confirm it is you before showing your clusters",
            )
        } finally {
            isAuthenticating = false
        }
    }

    // Prompt straight away: the operator opened the app to do something.
    LaunchedEffect(Unit) { unlock() }

    Box(
        modifier = modifier
            .fillMaxSize()
            .background(MaterialTheme.colorScheme.background),
        contentAlignment = Alignment.Center,
    ) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(18.dp),
            modifier = Modifier.padding(32.dp),
        ) {
            Icon(
                Icons.Filled.Lock,
                contentDescription = null,
                modifier = Modifier.size(46.dp),
                tint = MaterialTheme.colorScheme.primary,
            )
            Text("Ring Promoter is locked", style = MaterialTheme.typography.titleMedium)
            if (failed) {
                Text(
                    "Authentication failed. Try again.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = palette.unhealthy,
                )
            }
            Button(
                onClick = { scope.launch { unlock() } },
                enabled = !isAuthenticating,
                modifier = Modifier.width(220.dp),
            ) {
                Text("Unlock")
            }
        }
    }
}
