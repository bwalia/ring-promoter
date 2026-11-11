package org.fictionally.ringpromoter.store

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import org.fictionally.ringpromoter.ui.theme.InstanceTint
import java.net.URI
import java.util.UUID

/**
 * A saved control plane.
 *
 * Operators routinely have more than one — a staging control plane and the
 * production one — and promoting on the wrong cluster is the mistake this whole
 * type exists to prevent. Hence the required name and colour tag, and hence the
 * fact that the active instance is shown on every screen that can change
 * something.
 *
 * The token is **not** stored here. Only its key-store account id lives in this
 * record, which is what makes this struct safe to keep in `SharedPreferences`.
 */
@Serializable
data class Instance(
    val id: String = UUID.randomUUID().toString(),
    /** Operator-supplied label ("Production", "Staging"). */
    val name: String,
    @SerialName("base_url") val baseUrl: String,
    val tint: InstanceTint = InstanceTint.BLUE,
    /**
     * Require biometric confirmation before any action that targets the last
     * ring on this instance.
     */
    @SerialName("require_biometrics_for_production")
    val requireBiometricsForProduction: Boolean = true,
    @SerialName("added_at") val addedAtMillis: Long = System.currentTimeMillis(),
) {
    /** The key-store account under which this instance's token is filed. */
    val tokenAccount: String get() = id

    /** Host and port, for the subtitle in the instance list. */
    val displayHost: String
        get() {
            val uri = runCatching { URI(baseUrl) }.getOrNull() ?: return baseUrl
            val host = uri.host ?: return baseUrl
            return if (uri.port > 0) "$host:${uri.port}" else host
        }

    /**
     * The scheme is not HTTPS, so the token would cross the network in clear.
     *
     * Surfaced as a warning rather than a hard block, because a local
     * `http://10.0.2.2:8080` is a legitimate development target.
     */
    val isInsecure: Boolean
        get() = runCatching { URI(baseUrl).scheme?.lowercase() }.getOrNull() != "https"

    companion object {
        /**
         * The name used when the operator leaves it blank: the first meaningful
         * label of the host, capitalised ("rp.workstation.co.uk" →
         * "Workstation"), or the host itself for `localhost` and IP addresses.
         */
        fun defaultName(url: String): String {
            val host = runCatching { URI(url).host }.getOrNull()
            if (host.isNullOrEmpty()) return url
            if (host.all { it.isDigit() || it == '.' || it == ':' }) return host
            val skip = setOf("www", "api", "rp", "ring-promoter", "ringpromoter")
            val labels = host.split(".")
            if (labels.size > 1) {
                val first = labels.firstOrNull { it.lowercase() !in skip }
                if (first != null && first != labels.last()) {
                    return first.replaceFirstChar { it.uppercase() }
                }
            }
            return host
        }

        /**
         * Normalises what an operator is likely to type: bare hosts get
         * `https://`, trailing slashes and stray whitespace are dropped.
         *
         * Returns null when the result is not a usable http(s) URL, so the
         * onboarding screen can say so rather than failing later at the first
         * request.
         */
        fun normalise(urlText: String): String? {
            var text = urlText.trim()
            if (text.isEmpty()) return null
            if (!text.contains("://")) text = "https://$text"
            text = text.trimEnd('/')
            val uri = runCatching { URI(text) }.getOrNull() ?: return null
            val scheme = uri.scheme?.lowercase() ?: return null
            if (scheme != "https" && scheme != "http") return null
            if (uri.host.isNullOrEmpty()) return null
            return text
        }
    }
}
