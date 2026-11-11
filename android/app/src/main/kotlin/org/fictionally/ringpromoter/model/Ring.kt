package org.fictionally.ringpromoter.model

import kotlinx.serialization.Serializable

/**
 * One stage in the shared promotion pipeline.
 *
 * Mirrors `ring.Ring` in the backend. The ordered list of rings is served by
 * `GET /api/apps` — the app never hard-codes it, so adding a ring server-side
 * flows through without an app release.
 */
@Serializable
data class Ring(
    /** Stable identifier used in every API path and body. */
    val name: String,
    /** Human-friendly label shown in the UI ("Integration", "Production"). */
    val label: String,
)

/**
 * The ordered ring pipeline for this control plane.
 *
 * Wraps the server's ring list so promotion legality ("what is the next ring?",
 * "is this the last ring?") is answered in one place instead of being
 * re-derived at each call site.
 */
@JvmInline
value class RingPipeline(val rings: List<Ring>) {

    /** The last ring — the one a production password protects. */
    val productionRing: Ring? get() = rings.lastOrNull()

    val names: List<String> get() = rings.map { it.name }

    fun indexOf(name: String): Int? = rings.indexOfFirst { it.name == name }.takeIf { it >= 0 }

    fun ring(named: String): Ring? = rings.firstOrNull { it.name == named }

    /**
     * The ring immediately after [name], or null when [name] is the last ring
     * or is unknown. Promotion always targets exactly this ring — the app must
     * never offer a target the server would refuse.
     */
    fun next(after: String): Ring? {
        val i = indexOf(after) ?: return null
        return rings.getOrNull(i + 1)
    }

    /**
     * Whether promoting out of [from] would deploy into the production ring.
     * This is the condition that requires the production password.
     */
    fun promotingTargetsProduction(from: String): Boolean {
        val next = next(from) ?: return false
        val prod = productionRing ?: return false
        return next.name == prod.name
    }

    /** Whether seeding [name] directly deploys into the production ring. */
    fun isProduction(name: String): Boolean = productionRing?.name == name

    companion object {
        val EMPTY = RingPipeline(emptyList())
    }
}
