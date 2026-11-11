package org.fictionally.ringpromoter

import kotlinx.serialization.json.Json
import org.fictionally.ringpromoter.net.RpJson
import java.io.File

/**
 * Loads the JSON fixtures the app ships in `assets/fixtures/`.
 *
 * These are **real captured responses** from a locally-run Ring Promoter
 * backend, copied byte for byte from the iOS app, not hand-written
 * approximations. That is the point: these tests run against them, so a change
 * to the Go models breaks the build rather than the app.
 *
 * Read straight off disk rather than through an `AssetManager`, which keeps
 * these as plain JVM tests — no Robolectric, no emulator, no device needed.
 * Gradle runs unit tests with the module directory as the working directory.
 */
object Fixtures {
    private val dir = File("src/main/assets/fixtures")

    fun text(name: String): String {
        val file = File(dir, "$name.json")
        require(file.exists()) {
            "Fixture ${file.path} is missing. Was it copied from ios/RingPromoter/Resources/Fixtures?"
        }
        return file.readText()
    }

    inline fun <reified T> decode(name: String, json: Json = RpJson): T =
        json.decodeFromString<T>(text(name))

    /** Every fixture in the directory, so a test can sweep the whole corpus. */
    fun names(): List<String> =
        dir.listFiles { f -> f.extension == "json" }
            .orEmpty()
            .map { it.nameWithoutExtension }
            .sorted()
}
