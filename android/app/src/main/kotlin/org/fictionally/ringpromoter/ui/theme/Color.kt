package org.fictionally.ringpromoter.ui.theme

import androidx.compose.runtime.Composable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.ui.graphics.Color

/**
 * Semantic colours, ported value for value from the iOS app's
 * `DesignSystem/HealthPresentation.swift`.
 *
 * Defined in code rather than in `res/values/colors.xml` for the same reason
 * iOS kept them out of the asset catalogue: each one can carry the reason it
 * exists, and the Glance widget can use the identical values without a shared
 * resource file.
 *
 * Both appearances are specified explicitly. The dark variants are lifted in
 * luminance so they hold contrast against a dark background — they are not
 * computed from the light ones.
 */
object RpColors {
    /** Deployed and passing its health check. */
    val healthyLight = Color(0.11f, 0.47f, 0.28f)
    val healthyDark = Color(0.36f, 0.80f, 0.53f)

    /** Deployed and failing — the state that needs an operator. */
    val unhealthyLight = Color(0.70f, 0.13f, 0.13f)
    val unhealthyDark = Color(1.00f, 0.45f, 0.42f)

    /** Configured, but nothing has ever been deployed here. */
    val neutralLight = Color(0.42f, 0.45f, 0.50f)
    val neutralDark = Color(0.62f, 0.65f, 0.70f)

    /** Not part of this application's pipeline. */
    val disabledLight = Color(0.62f, 0.64f, 0.67f)
    val disabledDark = Color(0.45f, 0.47f, 0.51f)

    /** A deploy is in flight right now. */
    val inFlightLight = Color(0.13f, 0.40f, 0.75f)
    val inFlightDark = Color(0.44f, 0.68f, 1.00f)

    /** Gates, and anything that says "you must do something first". */
    val gateLight = Color(0.55f, 0.36f, 0.02f)
    val gateDark = Color(0.95f, 0.71f, 0.29f)

    /**
     * Production. Deliberately distinct from [unhealthyLight] so "this is
     * production" never reads as "this is broken".
     */
    val productionLight = Color(0.45f, 0.16f, 0.55f)
    val productionDark = Color(0.78f, 0.55f, 0.95f)

    /** The near-black of the Rings of Applications stage, in both appearances. */
    val space = Color(0.027f, 0.027f, 0.039f)
}

/**
 * The semantic palette resolved for the current appearance.
 *
 * Held as a value class rather than reached through `MaterialTheme` because
 * these colours are not Material roles: they mean something specific about a
 * ring, and mapping them onto `primary`/`error` would lose that.
 */
data class RpPalette(
    val healthy: Color,
    val unhealthy: Color,
    val neutral: Color,
    val disabled: Color,
    val inFlight: Color,
    val gate: Color,
    val production: Color,
    val space: Color,
) {
    companion object {
        fun forScheme(dark: Boolean) = if (dark) {
            RpPalette(
                healthy = RpColors.healthyDark,
                unhealthy = RpColors.unhealthyDark,
                neutral = RpColors.neutralDark,
                disabled = RpColors.disabledDark,
                inFlight = RpColors.inFlightDark,
                gate = RpColors.gateDark,
                production = RpColors.productionDark,
                space = RpColors.space,
            )
        } else {
            RpPalette(
                healthy = RpColors.healthyLight,
                unhealthy = RpColors.unhealthyLight,
                neutral = RpColors.neutralLight,
                disabled = RpColors.disabledLight,
                inFlight = RpColors.inFlightLight,
                gate = RpColors.gateLight,
                production = RpColors.productionLight,
                space = RpColors.space,
            )
        }
    }
}

/**
 * The colours an operator can tag a saved control plane with, so "staging" and
 * "production" are never confused at a glance.
 *
 * Stored by [name], so the order and the names are part of the persisted
 * format — renaming a case orphans every record that used it.
 */
enum class InstanceTint {
    RED, ORANGE, GREEN, BLUE, PURPLE, GRAPHITE;

    fun color(palette: RpPalette): Color = when (this) {
        RED -> palette.unhealthy
        ORANGE -> palette.gate
        GREEN -> palette.healthy
        BLUE -> palette.inFlight
        PURPLE -> palette.production
        GRAPHITE -> palette.neutral
    }

    val label: String get() = name.lowercase().replaceFirstChar { it.uppercase() }
}

/** The resolved palette for the current appearance. */
val rpPalette: RpPalette
    @Composable @ReadOnlyComposable get() = LocalRpPalette.current
