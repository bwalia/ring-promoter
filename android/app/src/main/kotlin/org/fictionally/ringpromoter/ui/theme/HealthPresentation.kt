package org.fictionally.ringpromoter.ui.theme

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.RemoveCircleOutline
import androidx.compose.material.icons.filled.Sync
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material.icons.outlined.Circle
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import org.fictionally.ringpromoter.model.RingStatus

/**
 * How a ring's state is rendered.
 *
 * **Colour is never the only signal.** Each state carries a distinct icon as
 * well, so the pipeline is readable with any form of colour blindness, in
 * bright sun, or on a greyscale screenshot. The content description is built
 * from the same source, so what is announced matches what is drawn.
 */
enum class HealthPresentation {
    /** Deployed and passing its health check. */
    HEALTHY,

    /** Deployed and failing its health check — the state that needs an operator. */
    UNHEALTHY,

    /** Configured, but nothing has ever been deployed here. */
    EMPTY,

    /** The app does not declare this ring at all. */
    NOT_CONFIGURED,

    /** A deploy is in flight right now. */
    IN_FLIGHT;

    val icon: ImageVector
        get() = when (this) {
            HEALTHY -> Icons.Filled.CheckCircle
            UNHEALTHY -> Icons.Filled.Warning
            EMPTY -> Icons.Outlined.Circle
            NOT_CONFIGURED -> Icons.Filled.RemoveCircleOutline
            IN_FLIGHT -> Icons.Filled.Sync
        }

    fun tint(palette: RpPalette): Color = when (this) {
        HEALTHY -> palette.healthy
        UNHEALTHY -> palette.unhealthy
        EMPTY -> palette.neutral
        NOT_CONFIGURED -> palette.disabled
        IN_FLIGHT -> palette.inFlight
    }

    /** A fill that stays legible in both appearances without shouting. */
    fun background(palette: RpPalette): Color = when (this) {
        NOT_CONFIGURED -> palette.disabled.copy(alpha = 0.08f)
        else -> tint(palette).copy(alpha = 0.14f)
    }

    /** Announced by TalkBack and shown in the legend. */
    val label: String
        get() = when (this) {
            HEALTHY -> "Healthy"
            UNHEALTHY -> "Unhealthy"
            EMPTY -> "Never deployed"
            NOT_CONFIGURED -> "Not in this pipeline"
            IN_FLIGHT -> "Deploying"
        }

    /** Needs an operator's attention now. */
    val isTrouble: Boolean get() = this == UNHEALTHY

    companion object {
        fun of(ring: RingStatus, isBusy: Boolean = false): HealthPresentation = when {
            isBusy -> IN_FLIGHT
            !ring.configured -> NOT_CONFIGURED
            ring.isEmpty -> EMPTY
            ring.isHealthy -> HEALTHY
            else -> UNHEALTHY
        }
    }
}
