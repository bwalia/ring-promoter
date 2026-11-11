package org.fictionally.ringpromoter

import android.content.Context
import androidx.compose.runtime.staticCompositionLocalOf
import org.fictionally.ringpromoter.demo.FixtureSource
import org.fictionally.ringpromoter.store.InstanceStore
import org.fictionally.ringpromoter.store.Session
import org.fictionally.ringpromoter.store.Settings
import org.fictionally.ringpromoter.store.TokenStore

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

    val session = Session(
        instances = instances,
        settings = settings,
        fixtures = FixtureSource.fromAssets(app.assets),
    )
}

val LocalContainer = staticCompositionLocalOf<AppContainer> {
    error("No AppContainer provided. Wrap the tree in a CompositionLocalProvider.")
}
