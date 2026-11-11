package org.fictionally.ringpromoter.ui.job

import androidx.lifecycle.ViewModel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import org.fictionally.ringpromoter.model.Job
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.store.Session
import kotlin.coroutines.cancellation.CancellationException

data class JobState(
    val job: Job? = null,
    val error: ApiError? = null,
    val isPolling: Boolean = false,
    /**
     * Set once the job first reaches a terminal state, so the UI can react
     * exactly once.
     */
    val didFinish: Boolean = false,
)

/**
 * Polls one job until it reaches a terminal state.
 *
 * The cadence is deliberate: a running deploy is polled about once a second so
 * it feels live, but the interval backs off as the job drags on, and any
 * transient failure backs off further rather than hammering a struggling
 * control plane from a phone. Polling stops the moment the job is finished, and
 * the coroutine is cancelled when the screen goes away.
 */
class JobViewModel(
    val app: String,
    val jobId: String,
    private val session: Session,
) : ViewModel() {

    private val _state = MutableStateFlow(JobState())
    val state: StateFlow<JobState> = _state.asStateFlow()

    /** Consecutive transport failures, backed off separately from the cadence. */
    private var consecutiveFailures = 0

    /** Run until the job finishes or the calling coroutine is cancelled. */
    suspend fun run() {
        _state.update { it.copy(isPolling = true) }
        try {
            var tick = 0
            while (true) {
                try {
                    val latest = session.api.value.job(app, jobId)
                    _state.update { it.copy(job = latest, error = null) }
                    consecutiveFailures = 0
                    if (latest.isFinished) {
                        _state.update { it.copy(didFinish = true) }
                        return
                    }
                } catch (e: CancellationException) {
                    throw e
                } catch (e: ApiError) {
                    if (e.isCancellation) return
                    consecutiveFailures += 1
                    // Keep showing the last good snapshot; only surface the
                    // error once retrying has clearly stopped helping.
                    if (consecutiveFailures >= 3 || _state.value.job == null) {
                        _state.update { it.copy(error = e) }
                    }
                }
                tick += 1
                delay(intervalMillis(tick))
            }
        } finally {
            _state.update { it.copy(isPolling = false) }
        }
    }

    /**
     * Ask the server for an AI diagnosis.
     *
     * The answer arrives on a later poll, which is why this does not return
     * one.
     */
    suspend fun requestDiagnosis() {
        try {
            session.api.value.diagnoseJob(app, jobId)
            // Re-read straight away so a server that answered synchronously
            // shows its result without waiting for the next tick.
            val latest = session.api.value.job(app, jobId)
            _state.update { it.copy(job = latest) }
        } catch (e: CancellationException) {
            throw e
        } catch (e: ApiError) {
            _state.update { it.copy(error = e) }
        }
    }

    /**
     * `base` while a job is young; it stretches towards `max` as the job runs
     * on, since a deploy still going after five minutes is not going to change
     * in the next 900ms. A run of failures backs off harder still.
     */
    private fun intervalMillis(tick: Int): Long {
        if (consecutiveFailures > 0) {
            return (BASE_MILLIS * (1L shl minOf(consecutiveFailures, 4)))
                .coerceAtMost(MAX_MILLIS)
        }
        val grown = BASE_MILLIS + (tick / 10L) * BASE_MILLIS
        return grown.coerceAtMost(MAX_MILLIS)
    }

    private companion object {
        const val BASE_MILLIS = 1_000L
        const val MAX_MILLIS = 8_000L
    }
}
