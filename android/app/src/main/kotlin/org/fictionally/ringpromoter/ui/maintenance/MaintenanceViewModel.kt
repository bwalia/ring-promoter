package org.fictionally.ringpromoter.ui.maintenance

import androidx.lifecycle.ViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import org.fictionally.ringpromoter.model.MaintenanceStatus
import org.fictionally.ringpromoter.model.MaintenanceWindow
import org.fictionally.ringpromoter.model.NewMaintenanceWindow
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.store.Session
import java.time.Duration
import java.time.Instant

data class MaintenanceState(
    val app: String,
    val status: MaintenanceStatus = MaintenanceStatus.UNGATED,
    val isLoading: Boolean = false,
    val error: ApiError? = null,
)

/**
 * One application's maintenance windows: what is scheduled, what is open now,
 * and what an operator has opened by hand.
 */
class MaintenanceViewModel(
    app: String,
    private val session: Session,
) : ViewModel() {

    private val _state = MutableStateFlow(MaintenanceState(app = app))
    val state: StateFlow<MaintenanceState> = _state.asStateFlow()

    /** Remembered from the last sign-off, so a name is not retyped. */
    val defaultAuthor: String get() = session.settings.signoffEngineer.value

    suspend fun refresh() {
        _state.update { it.copy(isLoading = true) }
        try {
            val status = session.api.value.maintenance(_state.value.app)
            _state.update { it.copy(status = status, error = null) }
        } catch (e: ApiError) {
            session.note(e)
            _state.update { it.copy(error = e) }
        } finally {
            _state.update { it.copy(isLoading = false) }
        }
    }

    /**
     * Open a window that starts now and closes itself.
     *
     * The start is backdated a minute. Without that slack, a clock a second or
     * two ahead of the server's leaves the operator looking at a window the
     * server does not yet consider open — and promoting anyway, which fails.
     */
    suspend fun openWindow(ring: String, duration: Duration, reason: String, author: String) {
        val start = Instant.now().minusSeconds(SKEW_SLACK_SECONDS)
        session.api.value.openMaintenanceWindow(
            _state.value.app,
            NewMaintenanceWindow(
                ring = ring,
                startsAt = start,
                endsAt = start.plus(duration).plusSeconds(SKEW_SLACK_SECONDS),
                reason = reason,
                createdBy = author,
            ),
        )
        if (author.isNotBlank()) session.settings.setSignoffEngineer(author)
        refresh()
    }

    suspend fun closeWindow(window: MaintenanceWindow) {
        try {
            session.api.value.closeMaintenanceWindow(_state.value.app, window.id)
            refresh()
        } catch (e: ApiError) {
            session.note(e)
            _state.update { it.copy(error = e) }
        }
    }

    companion object {
        private const val SKEW_SLACK_SECONDS = 60L

        /** The lengths offered. Long enough for a deploy, short enough to forget. */
        val DURATIONS: List<Duration> = listOf(
            Duration.ofMinutes(30),
            Duration.ofHours(1),
            Duration.ofHours(2),
            Duration.ofHours(4),
        )

        fun label(duration: Duration): String = when {
            duration.toMinutes() < 60 -> "${duration.toMinutes()} minutes"
            duration.toHours() == 1L -> "1 hour"
            else -> "${duration.toHours()} hours"
        }
    }
}
