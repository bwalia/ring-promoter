package org.fictionally.ringpromoter.ui.overview

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.sync.Mutex
import org.fictionally.ringpromoter.model.AppGroup
import org.fictionally.ringpromoter.model.AppLocation
import org.fictionally.ringpromoter.model.Job
import org.fictionally.ringpromoter.model.RingStatus
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.store.Session
import java.time.Instant

/** One application as the Overview shows it. */
data class AppSummary(
    val name: String,
    val title: String,
    val rings: List<RingStatus>,
    /** The newest job for this app, from the shared `GET /api/jobs` feed. */
    val latestJob: Job? = null,
    /** Rings could not be loaded; the strip shows what was cached, if anything. */
    val loadError: String? = null,
    val location: AppLocation? = null,
) {
    val unhealthyRings: List<RingStatus> get() = rings.filter { it.needsAttention }

    val isInTrouble: Boolean
        get() = unhealthyRings.isNotEmpty() ||
            latestJob?.outcome == Job.Outcome.FAILED ||
            latestJob?.outcome == Job.Outcome.FAILED_AND_ROLLED_BACK ||
            loadError != null

    /**
     * An action is in flight for this application right now.
     *
     * Which *ring* it is deploying into cannot be shown: a running job carries
     * no result, and therefore no target ring, until it finishes. The row says
     * "promoting" for the whole application instead of guessing at a ring and
     * animating the wrong one.
     */
    val isBusy: Boolean get() = latestJob?.isFinished == false

    /** The action currently running, for the badge on the row. */
    val busyAction: String?
        get() = if (!isBusy) null
        else latestJob?.promotionAction?.title
            ?: latestJob?.action?.replaceFirstChar { it.uppercase() }

    /** The single line that explains why this app is pinned to the top. */
    val troubleSummary: String?
        get() {
            loadError?.let { return it }
            latestJob?.let { job ->
                if (job.outcome == Job.Outcome.FAILED_AND_ROLLED_BACK) {
                    return job.summaryMessage ?: "Last deploy failed and was rolled back."
                }
                if (job.outcome == Job.Outcome.FAILED) {
                    return job.summaryMessage ?: "Last deploy failed."
                }
            }
            val unhealthy = unhealthyRings
            if (unhealthy.isEmpty()) return null
            if (unhealthy.size == 1) {
                val ring = unhealthy.single()
                return ring.liveHealthError?.let { "${ring.ring.label}: $it" }
                    ?: "${ring.ring.label} is unhealthy."
            }
            return "${unhealthy.size} rings are unhealthy."
        }
}

/** Everything the Overview draws. */
data class OverviewState(
    val summaries: List<AppSummary> = emptyList(),
    val groups: List<AppGroup> = emptyList(),
    val isLoading: Boolean = false,
    /** When the data on screen was fetched. Null before the first load. */
    val lastUpdated: Instant? = null,
    /** The cached snapshot is being shown because the last refresh failed. */
    val isStale: Boolean = false,
    val error: ApiError? = null,
    val searchText: String = "",
    val selectedGroupId: String? = null,
) {
    val selectedGroup: AppGroup? get() = groups.firstOrNull { it.id == selectedGroupId }

    private val filtered: List<AppSummary>
        get() {
            var result = summaries
            selectedGroup?.let { group -> result = result.filter { it.name in group } }
            val query = searchText.trim()
            if (query.isNotEmpty()) {
                result = result.filter {
                    it.title.contains(query, ignoreCase = true) ||
                        it.name.contains(query, ignoreCase = true)
                }
            }
            return result
        }

    /**
     * Apps in trouble first, then the rest alphabetically by display title.
     *
     * Sorted here rather than in the composable, so "surface trouble first"
     * stays testable.
     */
    val orderedSummaries: List<AppSummary>
        get() = filtered.sortedWith(
            compareByDescending<AppSummary> { it.isInTrouble }
                .thenBy(String.CASE_INSENSITIVE_ORDER) { it.title },
        )

    val troubled: List<AppSummary> get() = orderedSummaries.filter { it.isInTrouble }
    val calm: List<AppSummary> get() = orderedSummaries.filterNot { it.isInTrouble }

    /** Everything loaded fine and nothing is unhealthy. */
    val isAllClear: Boolean get() = summaries.isNotEmpty() && summaries.none { it.isInTrouble }
}

class OverviewViewModel(private val session: Session) : ViewModel() {

    private val _state = MutableStateFlow(OverviewState())
    val state: StateFlow<OverviewState> = _state.asStateFlow()

    /** Guards against two refreshes racing (pull-to-refresh during a timer tick). */
    private val refreshing = Mutex()

    fun setSearchText(text: String) = _state.update { it.copy(searchText = text) }

    fun selectGroup(id: String?) = _state.update { it.copy(selectedGroupId = id) }

    /**
     * Refresh everything. Safe to call from pull-to-refresh, a timer and first
     * composition at once: a second call while one is in flight is dropped.
     */
    suspend fun refresh() {
        if (!refreshing.tryLock()) return
        try {
            performRefresh()
        } finally {
            refreshing.unlock()
        }
    }

    private suspend fun performRefresh() {
        _state.update { it.copy(isLoading = true) }
        try {
            val capabilities = try {
                session.loadCapabilities()
            } catch (e: ApiError) {
                // Keep whatever is on screen and mark it stale, so a flaky
                // network never blanks the operator's only view of the system.
                session.note(e)
                _state.update {
                    it.copy(error = e, isStale = it.summaries.isNotEmpty())
                }
                return
            }

            val api = session.api.value
            // These two are decoration: a control plane that cannot list jobs
            // or groups should not blank the pipeline, which is the part that
            // matters.
            val jobs = tolerate(emptyList()) { api.recentJobs() }
            val groups = tolerate(_state.value.groups) { api.groups() }

            // Fetched in parallel. The iOS port does this sequentially only
            // because nested Swift TaskGroups crashed inside the Observation
            // runtime; coroutines have no such constraint.
            val rings: Map<String, RingsOutcome> = coroutineScope {
                capabilities.apps
                    .map { app ->
                        async {
                            app to try {
                                RingsOutcome(api.rings(app), null)
                            } catch (e: ApiError) {
                                RingsOutcome(null, e)
                            }
                        }
                    }
                    .awaitAll()
                    .toMap()
            }

            val jobsByApp = jobs.groupBy { it.app }.mapValues { it.value.first() }
            val cached = _state.value.summaries.associateBy { it.name }

            // Preserve the server's app ordering; the state re-sorts by trouble.
            val summaries = capabilities.apps.map { app ->
                val outcome = rings[app]
                AppSummary(
                    name = app,
                    title = capabilities.title(app),
                    rings = outcome?.rings ?: cached[app]?.rings.orEmpty(),
                    latestJob = jobsByApp[app],
                    loadError = outcome?.error?.userMessage,
                    location = capabilities.locations[app],
                )
            }

            _state.update { current ->
                current.copy(
                    summaries = summaries,
                    groups = groups,
                    // A group deleted server-side must not keep filtering.
                    selectedGroupId = current.selectedGroupId
                        ?.takeIf { id -> groups.any { it.id == id } },
                    error = null,
                    isStale = false,
                    lastUpdated = Instant.now(),
                )
            }

            // The home-screen widget draws from this, and only this: a
            // refresh that failed leaves the previous snapshot alone rather
            // than replacing it with half a picture.
            session.publishWidgetSnapshot(summaries)
        } finally {
            _state.update { it.copy(isLoading = false) }
        }
    }

    private class RingsOutcome(val rings: List<RingStatus>?, val error: ApiError?)

    /**
     * Run a call whose failure is not worth surfacing, returning [fallback].
     *
     * Catches [ApiError] only — never a bare `Throwable` — so cancelling the
     * refresh (the operator switched control plane, or left the screen) still
     * propagates and tears the whole refresh down.
     */
    private inline fun <T> tolerate(fallback: T, call: () -> T): T =
        try {
            call()
        } catch (_: ApiError) {
            fallback
        }
}
