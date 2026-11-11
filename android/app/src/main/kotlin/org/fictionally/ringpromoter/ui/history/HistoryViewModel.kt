package org.fictionally.ringpromoter.ui.history

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.sync.Mutex
import org.fictionally.ringpromoter.model.DiagnosisStatus
import org.fictionally.ringpromoter.model.HistoryEntry
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.store.Session
import java.time.LocalDate
import java.time.ZoneId

/**
 * One day's entries, newest first.
 *
 * The day is a [LocalDate] in the operator's own zone, and how it should be
 * named is left to the UI: "was that yesterday?" is asked in local time, and
 * the wording belongs with the formatting.
 */
data class DayGroup(
    val day: LocalDate,
    val relative: Relative,
    val entries: List<HistoryEntry>,
) {
    enum class Relative { TODAY, YESTERDAY, EARLIER }
}

data class HistoryState(
    val app: String,
    val entries: List<HistoryEntry> = emptyList(),
    val isLoading: Boolean = false,
    val error: ApiError? = null,
    /** Which rows are open. Collapsed by default: the list is for scanning. */
    val expanded: Set<Long> = emptySet(),
    /** Diagnoses fetched or generated in this session, by entry id. */
    val diagnoses: Map<Long, String> = emptyMap(),
    val diagnosing: Set<Long> = emptySet(),
    val diagnosisErrors: Map<Long, String> = emptyMap(),
) {
    val isEmpty: Boolean get() = entries.isEmpty() && !isLoading && error == null

    fun diagnosis(entry: HistoryEntry): String? =
        diagnoses[entry.id] ?: entry.diagnosis?.takeIf { it.isNotEmpty() }
}

/**
 * One application's history, grouped by day.
 */
class HistoryViewModel(
    app: String,
    private val session: Session,
    private val zone: ZoneId = ZoneId.systemDefault(),
) : ViewModel() {

    private val _state = MutableStateFlow(HistoryState(app = app))
    val state: StateFlow<HistoryState> = _state.asStateFlow()

    private val refreshing = Mutex()

    val aiEnabled: Boolean get() = session.aiEnabled

    /** Grouped for display. Called from the screen so it follows the state. */
    fun groups(entries: List<HistoryEntry>): List<DayGroup> = groupByDay(entries, zone)

    suspend fun refresh() {
        if (!refreshing.tryLock()) return
        try {
            _state.update { it.copy(isLoading = true) }
            try {
                val entries = session.api.value.history(_state.value.app)
                _state.update { it.copy(entries = entries, error = null) }
            } catch (e: ApiError) {
                session.note(e)
                _state.update { it.copy(error = e) }
            }
        } finally {
            _state.update { it.copy(isLoading = false) }
            refreshing.unlock()
        }
    }

    fun toggle(entry: HistoryEntry) {
        _state.update {
            it.copy(
                expanded = if (entry.id in it.expanded) {
                    it.expanded - entry.id
                } else {
                    it.expanded + entry.id
                },
            )
        }
    }

    /**
     * A diagnosis someone else already asked for is stored server-side, so look
     * before spending another model call. Silent on failure: this runs when a
     * row is opened, and an operator who did not ask for a diagnosis should not
     * be shown an error about one.
     */
    suspend fun loadExistingDiagnosis(entry: HistoryEntry) {
        if (entry.succeeded) return
        if (_state.value.diagnosis(entry) != null) return
        val response = runCatching {
            session.api.value.historyDiagnosis(_state.value.app, entry.id)
        }.getOrNull() ?: return
        if (response.diagnosisStatus != DiagnosisStatus.DONE) return
        val text = response.diagnosis?.takeIf { it.isNotEmpty() } ?: return
        _state.update { it.copy(diagnoses = it.diagnoses + (entry.id to text)) }
    }

    /**
     * Ask for a diagnosis, then poll until the server settles.
     *
     * The POST may answer with the text directly or only accept the work, so
     * both paths are handled rather than assuming one.
     */
    fun diagnoseAsync(entry: HistoryEntry) {
        viewModelScope.launch { diagnose(entry) }
    }

    suspend fun diagnose(entry: HistoryEntry) {
        val app = _state.value.app
        _state.update {
            it.copy(
                diagnosing = it.diagnosing + entry.id,
                diagnosisErrors = it.diagnosisErrors - entry.id,
            )
        }
        try {
            val started = session.api.value.diagnoseHistoryEntry(app, entry.id)
            started.diagnosis?.takeIf { it.isNotEmpty() }?.let { text ->
                _state.update { it.copy(diagnoses = it.diagnoses + (entry.id to text)) }
                return
            }
            repeat(POLL_ATTEMPTS) {
                delay(POLL_INTERVAL_MS)
                val latest = session.api.value.historyDiagnosis(app, entry.id)
                if (latest.diagnosisStatus == DiagnosisStatus.DONE) {
                    val text = latest.diagnosis.orEmpty()
                    if (text.isNotEmpty()) {
                        _state.update {
                            it.copy(diagnoses = it.diagnoses + (entry.id to text))
                        }
                        return
                    }
                }
                if (latest.diagnosisStatus == DiagnosisStatus.FAILED) {
                    // The endpoint reports the status but carries no reason, so
                    // there is nothing more honest to say than this.
                    fail(entry, "The diagnosis failed. Try again in a moment.")
                    return
                }
            }
            fail(entry, "The diagnosis did not finish. Try again in a moment.")
        } catch (e: ApiError) {
            session.note(e)
            fail(entry, e.userMessage)
        } finally {
            _state.update { it.copy(diagnosing = it.diagnosing - entry.id) }
        }
    }

    private fun fail(entry: HistoryEntry, message: String) {
        _state.update { it.copy(diagnosisErrors = it.diagnosisErrors + (entry.id to message)) }
    }

    companion object {
        private const val POLL_ATTEMPTS = 20
        private const val POLL_INTERVAL_MS = 2_000L

        /**
         * Bucket entries by calendar day in [zone], newest day first and newest
         * entry first within each day.
         *
         * Pure, so it is tested without a device: the grouping is the only part
         * of this screen that can be subtly wrong, and it is wrong in a way
         * nobody notices until an incident timeline is being read back.
         */
        fun groupByDay(
            entries: List<HistoryEntry>,
            zone: ZoneId = ZoneId.systemDefault(),
            today: LocalDate = LocalDate.now(zone),
        ): List<DayGroup> =
            entries
                .groupBy { it.createdAt.atZone(zone).toLocalDate() }
                .toSortedMap(reverseOrder())
                .map { (day, sameDay) ->
                    DayGroup(
                        day = day,
                        relative = when (day) {
                            today -> DayGroup.Relative.TODAY
                            today.minusDays(1) -> DayGroup.Relative.YESTERDAY
                            else -> DayGroup.Relative.EARLIER
                        },
                        entries = sameDay.sortedByDescending { it.createdAt },
                    )
                }
    }
}
