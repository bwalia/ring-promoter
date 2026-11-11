package org.fictionally.ringpromoter.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.time.Instant

/**
 * One recorded seed / promote / rollback, success or failure.
 *
 * Mirrors `store.HistoryEntry` from `GET /api/apps/{app}/history` (newest
 * first).
 */
@Serializable
data class HistoryEntry(
    val id: Long,
    val app: String,
    val ring: String,
    /** "seed" | "promote" | "rollback" */
    val action: String,
    @SerialName("from_version") val fromVersion: String = "",
    @SerialName("to_version") val toVersion: String = "",
    /** "success" | "failure" */
    val result: String,
    val message: String = "",
    /** The stored AI explanation, present once someone has asked for one. */
    val diagnosis: String? = null,
    @SerialName("created_at")
    @Serializable(with = InstantSerializer::class)
    val createdAt: Instant,
) {
    val succeeded: Boolean get() = result == "success"

    val promotionAction: PromotionAction? get() = PromotionAction.from(action)

    /** Only failed entries can be diagnosed (the server returns 409 otherwise). */
    val canRequestDiagnosis: Boolean get() = !succeeded && diagnosis.isNullOrEmpty()

    /** "2.7.1 → 2.7.2", or just the target when there was nothing before it. */
    val versionTransition: String
        get() = if (fromVersion.isEmpty()) toVersion else "$fromVersion → $toVersion"
}

/** `GET /api/apps/{app}/history` */
@Serializable
data class HistoryResponse(val history: List<HistoryEntry> = emptyList())
