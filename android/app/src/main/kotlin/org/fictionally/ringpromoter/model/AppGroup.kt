package org.fictionally.ringpromoter.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.time.Instant

/**
 * A user-defined collection of applications, stored server-side and shared by
 * every operator of this control plane.
 *
 * Mirrors `store.Group`.
 */
@Serializable
data class AppGroup(
    val id: String,
    val name: String,
    /** The server omits an empty member list as null. */
    val apps: List<String> = emptyList(),
    @SerialName("updated_at")
    @Serializable(with = InstantSerializer::class)
    val updatedAt: Instant,
) {
    operator fun contains(app: String): Boolean = apps.contains(app)
}

/** `GET /api/groups` */
@Serializable
data class GroupsResponse(val groups: List<AppGroup> = emptyList())

/** Body of `POST /api/groups` and `PUT /api/groups/{id}`. */
@Serializable
data class GroupBody(val name: String, val apps: List<String>)
