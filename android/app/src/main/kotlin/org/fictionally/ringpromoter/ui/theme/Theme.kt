package org.fictionally.ringpromoter.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.platform.LocalContext

val LocalRpPalette = staticCompositionLocalOf { RpPalette.forScheme(dark = false) }

/** The user's appearance choice, mirroring iOS `AppSettings.Theme`. */
enum class ThemeChoice {
    SYSTEM, LIGHT, DARK;

    val label: String get() = name.lowercase().replaceFirstChar { it.uppercase() }
}

/**
 * Material 3 for the chrome, [RpPalette] for everything that means something
 * about a ring.
 *
 * Dynamic colour is used where the platform offers it, because the chrome
 * should look like the rest of the user's phone — but the health palette is
 * never dynamic. A ring's colour is load-bearing: an operator must be able to
 * learn "red means unhealthy" once, and have it stay true on every device and
 * under every wallpaper.
 */
@Composable
fun RingPromoterTheme(
    choice: ThemeChoice = ThemeChoice.SYSTEM,
    dynamicColor: Boolean = true,
    content: @Composable () -> Unit,
) {
    val dark = when (choice) {
        ThemeChoice.SYSTEM -> isSystemInDarkTheme()
        ThemeChoice.LIGHT -> false
        ThemeChoice.DARK -> true
    }
    val context = LocalContext.current
    val scheme = when {
        dynamicColor && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S ->
            if (dark) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)

        dark -> darkColorScheme()
        else -> lightColorScheme()
    }

    CompositionLocalProvider(LocalRpPalette provides RpPalette.forScheme(dark)) {
        MaterialTheme(colorScheme = scheme, content = content)
    }
}
