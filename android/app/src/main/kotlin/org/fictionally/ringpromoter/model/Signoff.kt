package org.fictionally.ringpromoter.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.time.Instant

/**
 * A QA / release-engineer Go-No-Go decision for one **exact** version.
 *
 * Mirrors `store.Signoff`. Sign-offs are version-specific: a GO for `v1` does
 * not authorise `v2`, and the app must never imply otherwise.
 */
@Serializable
data class Signoff(
    val app: String,
    val ring: String,
    val version: String,
    val decision: Decision,
    /** The release engineer who recorded the decision. Required by the server. */
    val engineer: String = "",
    /** The QA outcome this attests to (free text, e.g. "passed"). */
    @SerialName("qa_status") val qaStatus: String = "",
    val note: String? = null,
    @SerialName("updated_at")
    @Serializable(with = InstantSerializer::class)
    val updatedAt: Instant,
) {
    /** The decision values the server accepts. */
    @Serializable(with = DecisionSerializer::class)
    enum class Decision(val wire: String) {
        GO("go"),
        NO_GO("no_go");

        val title: String get() = if (this == GO) "Go" else "No-Go"

        companion object {
            fun from(raw: String): Decision? = entries.firstOrNull { it.wire == raw }
        }
    }

    /** Unique per (app, ring, version) — the server stores at most one. */
    val id: String get() = "$app/$ring/$version"

    val isGo: Boolean get() = decision == Decision.GO
}

/**
 * An unrecognised decision is treated as NOT a go: failing closed is the only
 * safe reading for a gate.
 */
internal object DecisionSerializer : FallbackEnumSerializer<Signoff.Decision>(
    "Signoff.Decision",
    { it.wire },
    { Signoff.Decision.from(it) },
    Signoff.Decision.NO_GO,
)

/** `GET /api/apps/{app}/signoffs` */
@Serializable
data class SignoffsResponse(val signoffs: List<Signoff> = emptyList())

/** Body of `POST /api/apps/{app}/signoffs`. */
@Serializable
data class NewSignoff(
    val ring: String,
    val version: String,
    val decision: Signoff.Decision,
    val engineer: String,
    @SerialName("qa_status") val qaStatus: String,
    val note: String = "",
)

/**
 * The sign-off guarding one exact (ring, version), if any.
 *
 * This is the only lookup the QA gate permits — matching on ring alone would
 * let a GO for an older version appear to authorise a new one.
 */
fun Iterable<Signoff>.signoff(ring: String, version: String): Signoff? =
    firstOrNull { it.ring == ring && it.version == version }
