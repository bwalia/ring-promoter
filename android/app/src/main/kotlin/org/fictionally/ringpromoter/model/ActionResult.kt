package org.fictionally.ringpromoter.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * What a seed / promote / rollback did.
 *
 * Mirrors `promoter.Result`. The server returns this with 200 when the
 * operation succeeded and **422 when it ran and failed** — both are real
 * outcomes carrying a [message], not transport errors.
 */
@Serializable
data class ActionResult(
    val app: String,
    /** "seed" | "promote" | "rollback" */
    val action: String,
    /** The affected (target) ring. */
    val ring: String,
    /** The promote source ring; absent for seed and rollback. */
    @SerialName("from_ring") val fromRing: String? = null,
    val version: String,
    /** Deployed AND passed its health check. */
    val success: Boolean,
    /**
     * The health check failed and the target was returned to its previous
     * version. The app shows this as its own terminal state.
     *
     * Omitted by the server when false (`omitempty`).
     */
    @SerialName("rolled_back") val rolledBack: Boolean = false,
    val message: String = "",
    val state: RingState = RingState(),
) {
    /** The three terminal outcomes the UI must distinguish. */
    enum class Outcome { SUCCEEDED, FAILED_AND_ROLLED_BACK, FAILED }

    val outcome: Outcome
        get() = when {
            success -> Outcome.SUCCEEDED
            rolledBack -> Outcome.FAILED_AND_ROLLED_BACK
            else -> Outcome.FAILED
        }
}

/** The action a request performs, used for labels and confirmation copy. */
enum class PromotionAction(val wire: String) {
    SEED("seed"),
    PROMOTE("promote"),
    ROLLBACK("rollback");

    val title: String
        get() = when (this) {
            SEED -> "Seed"
            PROMOTE -> "Promote"
            ROLLBACK -> "Roll back"
        }

    val pastTense: String
        get() = when (this) {
            SEED -> "Seeded"
            PROMOTE -> "Promoted"
            ROLLBACK -> "Rolled back"
        }

    companion object {
        fun from(raw: String): PromotionAction? = entries.firstOrNull { it.wire == raw }
    }
}
