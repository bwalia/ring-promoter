package org.fictionally.ringpromoter

import android.content.Context
import androidx.compose.runtime.staticCompositionLocalOf
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.fictionally.ringpromoter.demo.FixtureSource
import org.fictionally.ringpromoter.store.BiometricGate
import org.fictionally.ringpromoter.store.InstanceStore
import org.fictionally.ringpromoter.store.Session
import org.fictionally.ringpromoter.store.Settings
import org.fictionally.ringpromoter.store.TokenStore
import org.fictionally.ringpromoter.ui.DeepLink

/**
 * Everything the app depends on, built once.
 *
 * No DI framework. The iOS app injects through SwiftUI environment objects, and
 * the Android equivalent is one container reached through [LocalContainer] —
 * one fewer thing to understand at 3am, and the whole graph is visible here.
 */
class AppContainer(context: Context) {
    private val app = context.applicationContext

    val tokens = TokenStore(app)

    val instances = InstanceStore(
        prefs = app.getSharedPreferences("rp.prefs", Context.MODE_PRIVATE),
        tokens = tokens,
    )

    val settings = Settings(app.getSharedPreferences("rp.prefs", Context.MODE_PRIVATE))

    val biometrics = BiometricGate(app)

    val session = Session(
        instances = instances,
        settings = settings,
        fixtures = FixtureSource.fromAssets(app.assets),
    )

    /**
     * A `ringpromoter://` link waiting to be navigated to.
     *
     * Held here rather than read from the Activity's intent inside the
     * navigation tree, because that tree is rebuilt whenever the control plane
     * changes and would otherwise re-navigate to a stale link.
     */
    private val _pendingLink = MutableStateFlow<DeepLink?>(null)
    val pendingLink: StateFlow<DeepLink?> = _pendingLink.asStateFlow()

    /** Ignores anything that is not a link this app recognises. */
    fun open(url: String?) {
        _pendingLink.value = url?.let { DeepLink.parse(it) }
    }

    fun clearPendingLink() {
        _pendingLink.value = null
    }
}

val LocalContainer = staticCompositionLocalOf<AppContainer> {
    error("No AppContainer provided. Wrap the tree in a CompositionLocalProvider.")
}
