package org.fictionally.ringpromoter.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import org.fictionally.ringpromoter.model.RingStatus
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.ui.theme.HealthPresentation
import org.fictionally.ringpromoter.ui.theme.rpPalette

/**
 * One ring's health, as an icon and a colour together.
 *
 * The icon is the signal; the colour reinforces it. Never the colour alone.
 */
@Composable
fun HealthDot(
    presentation: HealthPresentation,
    modifier: Modifier = Modifier,
    size: Int = 18,
) {
    val palette = rpPalette
    Icon(
        imageVector = presentation.icon,
        contentDescription = presentation.label,
        tint = presentation.tint(palette),
        modifier = modifier.size(size.dp),
    )
}

/** A ring's name in a pill tinted by its health. */
@Composable
fun RingChip(
    ring: RingStatus,
    isBusy: Boolean = false,
    modifier: Modifier = Modifier,
) {
    val presentation = HealthPresentation.of(ring, isBusy)
    val palette = rpPalette
    Row(
        modifier = modifier
            .background(presentation.background(palette), RoundedCornerShape(8.dp))
            .padding(horizontal = 8.dp, vertical = 4.dp)
            .semantics {
                contentDescription = "${ring.ring.label}: ${presentation.label}"
            },
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(5.dp),
    ) {
        Icon(
            imageVector = presentation.icon,
            contentDescription = null,
            tint = presentation.tint(palette),
            modifier = Modifier.size(14.dp),
        )
        Text(
            text = ring.ring.label,
            style = MaterialTheme.typography.labelMedium,
            color = presentation.tint(palette),
        )
    }
}

/**
 * How a gate badge should read.
 *
 * Deliberately not a boolean. Most gates — a change-request code, a QA sign-off
 * — can only be judged against a specific action and version, so a list screen
 * cannot honestly call them satisfied. [PRESENT] says "there is a gate here",
 * which is all such a screen knows. Only a gate whose state is a property of
 * the ring itself, like an open maintenance window, earns [SATISFIED].
 */
enum class GateTone { PRESENT, SATISFIED, BLOCKING }

/** A gate, shown before an operator commits to anything. */
@Composable
fun GateBadge(label: String, tone: GateTone, modifier: Modifier = Modifier) {
    val palette = rpPalette
    val tint = when (tone) {
        GateTone.SATISFIED -> palette.healthy
        GateTone.PRESENT -> palette.gate
        GateTone.BLOCKING -> palette.unhealthy
    }
    Row(
        modifier = modifier
            .border(1.dp, tint.copy(alpha = 0.5f), RoundedCornerShape(6.dp))
            .padding(horizontal = 7.dp, vertical = 3.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Icon(
            imageVector = Icons.Filled.Lock,
            contentDescription = null,
            tint = tint,
            modifier = Modifier.size(11.dp),
        )
        Text(label, style = MaterialTheme.typography.labelSmall, color = tint)
    }
}

/**
 * The colour tag for the active control plane.
 *
 * Shown on every screen that can change something, because promoting on the
 * wrong cluster is the mistake the whole instance concept exists to prevent.
 */
@Composable
fun InstanceDot(color: Color, modifier: Modifier = Modifier) {
    Box(
        modifier
            .size(10.dp)
            .background(color, CircleShape)
            .clearAndSetSemantics { },
    )
}

/**
 * An error with the server's own words and, where it would help, a retry.
 *
 * The title and message come from [ApiError], which keeps each status distinct
 * — "Blocked" and "Sign in again" lead somewhere different, and collapsing them
 * into "something went wrong" would strand the operator.
 */
@Composable
fun ErrorRow(
    error: ApiError,
    modifier: Modifier = Modifier,
    onRetry: (() -> Unit)? = null,
) {
    val palette = rpPalette
    Column(
        modifier = modifier
            .fillMaxWidth()
            .background(palette.unhealthy.copy(alpha = 0.10f), RoundedCornerShape(12.dp))
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Text(error.title, style = MaterialTheme.typography.titleSmall, color = palette.unhealthy)
        Text(error.userMessage, style = MaterialTheme.typography.bodyMedium)
        if (onRetry != null && error.isRetryable) {
            Button(onClick = onRetry, modifier = Modifier.padding(top = 4.dp)) { Text("Try again") }
        }
    }
}

/** An empty state that says what to do next, not just that there is nothing. */
@Composable
fun EmptyState(title: String, detail: String, modifier: Modifier = Modifier) {
    Column(
        modifier = modifier.fillMaxWidth().padding(32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(title, style = MaterialTheme.typography.titleMedium)
        Text(
            detail,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}
