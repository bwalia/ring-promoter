package org.fictionally.ringpromoter.ui.activity

import androidx.lifecycle.ViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.sync.Mutex
import org.fictionally.ringpromoter.model.Job
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.store.Session
import java.time.Instant

data class ActivityState(
    val jobs: List<Job> = emptyList(),
    val isLoading: Boolean = false,
    val lastUpdated: Instant? = null,
    val error: ApiError? = null,
) {
    /**
     * In flight first, newest first within each group: what is happening now
     * is the reason to open this screen, and what already finished is context.
     */
    val running: List<Job> =
        jobs.filterNot { it.isFinished }.sortedByDescending { it.startedAt }

    val finished: List<Job> = jobs.filter { it.isFinished }.sortedByDescending { it.startedAt }

    val isEmpty: Boolean get() = jobs.isEmpty() && !isLoading && error == null
}

/**
 * Everything running across the whole control plane, not just what this phone
 * started.
 *
 * `GET /api/jobs` returns the newest job per application, which is what makes
 * the feed genuinely shared: a promotion a colleague kicked off from the web UI
 * appears here too.
 */
class ActivityViewModel(private val session: Session) : ViewModel() {

    private val _state = MutableStateFlow(ActivityState())
    val state: StateFlow<ActivityState> = _state.asStateFlow()

    private val refreshing = Mutex()

    fun title(app: String): String = session.capabilities.value?.title(app) ?: app

    suspend fun refresh() {
        if (!refreshing.tryLock()) return
        try {
            _state.update { it.copy(isLoading = true) }
            try {
                val jobs = session.api.value.recentJobs()
                _state.update {
                    it.copy(jobs = jobs, error = null, lastUpdated = Instant.now())
                }
            } catch (e: ApiError) {
                session.note(e)
                _state.update { it.copy(error = e) }
            }
        } finally {
            _state.update { it.copy(isLoading = false) }
            refreshing.unlock()
        }
    }
}
