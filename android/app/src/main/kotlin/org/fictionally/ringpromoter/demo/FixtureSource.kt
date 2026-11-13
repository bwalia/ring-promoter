package org.fictionally.ringpromoter.demo

import android.content.res.AssetManager

/**
 * Where the demo fixtures come from.
 *
 * An interface, not a direct [AssetManager] call, so the whole demo simulation
 * can be exercised by plain JVM unit tests — no emulator, no Robolectric. The
 * app passes [fromAssets]; tests read the same files off disk.
 */
fun interface FixtureSource {
    /** The JSON text of `fixtures/<name>.json`. Throws if it is not there. */
    fun read(name: String): String

    companion object {
        fun fromAssets(assets: AssetManager) = FixtureSource { name ->
            assets.open("fixtures/$name.json").bufferedReader().use { it.readText() }
        }
    }
}

/**
 * The fixture names demo mode and the tests refer to.
 *
 * Named constants rather than bare strings so a renamed or missing file is a
 * compile-time edit in one place.
 */
object FixtureName {
    const val APPS = "apps"
    const val RINGS = "rings"
    const val RINGS_GATED = "rings-gated"
    const val RINGS_MANAGED = "rings-managed"
    const val RINGS_UNHEALTHY = "rings-unhealthy"
    const val JOBS = "jobs"
    const val HISTORY = "history"
    const val HISTORY_WITH_FAILURES = "history-with-failures"
    const val GROUPS = "groups"
    const val SIGNOFFS = "signoffs"
    const val MAINTENANCE_GATED = "maintenance-gated"
}
