package org.fictionally.ringpromoter.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.time.Instant

/**
 * One deployable version known to an application's source repository.
 *
 * Mirrors `deployer.Version`.
 */
@Serializable
data class AppVersion(
    val name: String,
    /** "branch" | "tag" */
    val type: String,
) {
    val id: String get() = "$type/$name"
    val isTag: Boolean get() = type == "tag"
}

/**
 * `GET /api/apps/{app}/versions`
 *
 * `supported == false` means the app's deployer cannot enumerate versions, so
 * the Seed sheet must fall back to free-form text input.
 */
@Serializable
data class VersionsResponse(
    val supported: Boolean = false,
    val versions: List<AppVersion> = emptyList(),
) {
    /**
     * Offer a picker only when the server can actually enumerate versions AND
     * found some.
     */
    val offersPicker: Boolean get() = supported && versions.isNotEmpty()

    val branches: List<AppVersion> get() = versions.filter { !it.isTag }
    val tags: List<AppVersion> get() = versions.filter { it.isTag }

    /**
     * The versions whose name contains [query], ignoring case, in server order
     * (branches first). A blank query matches everything.
     */
    fun matching(query: String): List<AppVersion> {
        val needle = query.trim()
        if (needle.isEmpty()) return versions
        return versions.filter { it.name.contains(needle, ignoreCase = true) }
    }

    /**
     * Whether [name] is exactly one of the listed branches or tags. Anything
     * else typed into the picker is offered as a commit SHA, which the server
     * resolves against the repository before deploying.
     */
    fun contains(name: String): Boolean {
        val trimmed = name.trim()
        return versions.any { it.name == trimmed }
    }

    companion object {
        val UNSUPPORTED = VersionsResponse(supported = false, versions = emptyList())
    }
}

/** `GET /version` — build metadata, shown in Settings. Unauthenticated. */
@Serializable
data class ServerVersion(
    val version: String = "unknown",
    val commit: String = "unknown",
    @SerialName("built_at") val builtAt: String = "unknown",
    /**
     * When this instance started which, with an immutable image, is when it was
     * last deployed.
     */
    @SerialName("started_at")
    @Serializable(with = InstantSerializer::class)
    val startedAt: Instant? = null,
) {
    /** Short commit for display. */
    val shortCommit: String get() = if (commit.length > 8) commit.take(8) else commit
}
