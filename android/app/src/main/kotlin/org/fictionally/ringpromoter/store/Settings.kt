package org.fictionally.ringpromoter.store

import android.content.SharedPreferences
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.fictionally.ringpromoter.ui.theme.ThemeChoice

/**
 * User preferences that are not secrets.
 *
 * Backed by `SharedPreferences` — deliberately, so it is obvious that nothing
 * sensitive is kept here. Tokens live in the [TokenStore].
 */
class Settings(private val prefs: SharedPreferences) {

    /**
     * How often the Overview refreshes itself while on screen, in seconds.
     * Zero means manual only, which is the right choice on a metered
     * connection.
     */
    private val _refreshInterval = MutableStateFlow(prefs.getInt(KEY_REFRESH, 30))
    val refreshInterval: StateFlow<Int> = _refreshInterval.asStateFlow()

    /** Require biometric unlock whenever the app comes to the foreground. */
    private val _lockOnOpen = MutableStateFlow(prefs.getBoolean(KEY_LOCK, false))
    val lockOnOpen: StateFlow<Boolean> = _lockOnOpen.asStateFlow()

    private val _theme = MutableStateFlow(
        runCatching { ThemeChoice.valueOf(prefs.getString(KEY_THEME, null) ?: "") }
            .getOrDefault(ThemeChoice.SYSTEM),
    )
    val theme: StateFlow<ThemeChoice> = _theme.asStateFlow()

    private val _startsInDemoMode = MutableStateFlow(prefs.getBoolean(KEY_DEMO, false))
    val startsInDemoMode: StateFlow<Boolean> = _startsInDemoMode.asStateFlow()

    /**
     * Remembered so a release engineer does not retype their name on every
     * sign-off. A name, not a credential.
     */
    private val _signoffEngineer = MutableStateFlow(prefs.getString(KEY_ENGINEER, "").orEmpty())
    val signoffEngineer: StateFlow<String> = _signoffEngineer.asStateFlow()

    fun setRefreshInterval(seconds: Int) {
        _refreshInterval.value = seconds
        prefs.edit().putInt(KEY_REFRESH, seconds).apply()
    }

    fun setLockOnOpen(enabled: Boolean) {
        _lockOnOpen.value = enabled
        prefs.edit().putBoolean(KEY_LOCK, enabled).apply()
    }

    fun setTheme(choice: ThemeChoice) {
        _theme.value = choice
        prefs.edit().putString(KEY_THEME, choice.name).apply()
    }

    fun setStartsInDemoMode(enabled: Boolean) {
        _startsInDemoMode.value = enabled
        prefs.edit().putBoolean(KEY_DEMO, enabled).apply()
    }

    fun setSignoffEngineer(name: String) {
        _signoffEngineer.value = name
        prefs.edit().putString(KEY_ENGINEER, name).apply()
    }

    companion object {
        val REFRESH_OPTIONS = listOf(0, 15, 30, 60, 300)

        fun refreshLabel(seconds: Int): String = when {
            seconds == 0 -> "Manual only"
            seconds < 60 -> "$seconds seconds"
            seconds == 60 -> "1 minute"
            else -> "${seconds / 60} minutes"
        }

        private const val KEY_REFRESH = "rp.refreshInterval"
        private const val KEY_LOCK = "rp.lockOnOpen"
        private const val KEY_THEME = "rp.theme"
        private const val KEY_DEMO = "rp.demoMode"
        private const val KEY_ENGINEER = "rp.signoffEngineer"
    }
}
