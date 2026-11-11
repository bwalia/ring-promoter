package org.fictionally.ringpromoter.ui

import java.net.URI
import java.net.URLDecoder

/**
 * Where a `ringpromoter://` link should land.
 *
 * Parsed by hand, as pure Kotlin, for two reasons: it is unit-testable on the
 * JVM with no Android framework, and the path has to be split *before* it is
 * percent-decoded. An application called `team/app` arrives as `team%2Fapp`;
 * decoding first would split it in two and open the wrong application — or
 * none. Sending an operator to the wrong app is worse than ignoring the link.
 */
sealed interface DeepLink {
    val app: String

    data class App(override val app: String) : DeepLink
    data class Job(override val app: String, val jobId: String) : DeepLink
    data class History(override val app: String) : DeepLink

    companion object {
        const val SCHEME = "ringpromoter"

        /**
         * `ringpromoter://app/{name}`, `…/{name}/job/{id}` and
         * `…/{name}/history`. Anything else returns null and is ignored.
         */
        fun parse(url: String): DeepLink? {
            val uri = runCatching { URI(url) }.getOrNull() ?: return null
            if (uri.scheme?.lowercase() != SCHEME) return null

            // For ringpromoter://app/web-frontend the authority is "app" and
            // the rest is the path. `rawPath` keeps the escapes intact so the
            // split below cannot cut an application name in half.
            val parts = buildList {
                uri.authority?.takeIf { it.isNotEmpty() }?.let { add(decode(it)) }
                uri.rawPath.orEmpty()
                    .split('/')
                    .filter { it.isNotEmpty() }
                    .forEach { add(decode(it)) }
            }

            if (parts.firstOrNull() != "app" || parts.size < 2) return null
            val app = parts[1].takeIf { it.isNotEmpty() } ?: return null

            return when {
                parts.size >= 4 && parts[2] == "job" -> Job(app, parts[3])
                parts.size >= 3 && parts[2] == "history" -> History(app)
                parts.size == 2 -> App(app)
                // A shape we do not recognise: do nothing rather than guess.
                else -> null
            }
        }

        private fun decode(segment: String): String =
            runCatching { URLDecoder.decode(segment, "UTF-8") }.getOrDefault(segment)

        /** The link the widget and a share sheet hand out. */
        fun url(app: String): String = "$SCHEME://app/${encode(app)}"

        fun url(app: String, jobId: String): String =
            "$SCHEME://app/${encode(app)}/job/${encode(jobId)}"

        /**
         * Percent-encodes a path segment. `URLEncoder` is form encoding, which
         * turns a space into `+`; in a path that is a literal plus.
         */
        private fun encode(segment: String): String =
            java.net.URLEncoder.encode(segment, "UTF-8").replace("+", "%20")
    }
}
