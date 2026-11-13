package org.fictionally.ringpromoter.ui.groups

import androidx.lifecycle.ViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import org.fictionally.ringpromoter.model.AppGroup
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.store.Session

data class GroupsState(
    val groups: List<AppGroup> = emptyList(),
    val isLoading: Boolean = false,
    val error: ApiError? = null,
) {
    val isEmpty: Boolean get() = groups.isEmpty() && !isLoading && error == null
}

/**
 * Server-side application groups.
 *
 * These are **shared**: they live on the control plane, not on this phone, so
 * renaming or deleting one does it for everyone on the team. Every destructive
 * action here says so before it happens.
 */
class GroupsViewModel(private val session: Session) : ViewModel() {

    private val _state = MutableStateFlow(GroupsState())
    val state: StateFlow<GroupsState> = _state.asStateFlow()

    /** Every app on this control plane, in the server's own order. */
    val allApps: List<String> get() = session.capabilities.value?.apps ?: emptyList()

    fun title(app: String): String = session.capabilities.value?.title(app) ?: app

    suspend fun refresh() {
        _state.update { it.copy(isLoading = true) }
        try {
            val groups = session.api.value.groups()
            _state.update { it.copy(groups = groups, error = null) }
        } catch (e: ApiError) {
            session.note(e)
            _state.update { it.copy(error = e) }
        } finally {
            _state.update { it.copy(isLoading = false) }
        }
    }

    /**
     * Create or rename a group. Members are ordered as the server lists its
     * apps, not as they were tapped, so the stored order stays stable.
     */
    suspend fun save(existing: AppGroup?, name: String, members: Set<String>) {
        val ordered = allApps.filter { it in members }
        if (existing == null) {
            session.api.value.createGroup(name, ordered)
        } else {
            session.api.value.updateGroup(existing.id, name, ordered)
        }
        refresh()
    }

    suspend fun delete(group: AppGroup) {
        try {
            session.api.value.deleteGroup(group.id)
            refresh()
        } catch (e: ApiError) {
            session.note(e)
            _state.update { it.copy(error = e) }
        }
    }

    fun memberSummary(group: AppGroup): String =
        if (group.apps.isEmpty()) {
            "No applications"
        } else {
            group.apps.joinToString(", ") { title(it) }
        }
}
