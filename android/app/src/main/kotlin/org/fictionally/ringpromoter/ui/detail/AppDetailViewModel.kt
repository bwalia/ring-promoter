package org.fictionally.ringpromoter.ui.detail

import androidx.lifecycle.ViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.sync.Mutex
import org.fictionally.ringpromoter.model.MaintenanceStatus
import org.fictionally.ringpromoter.model.PromotionRules
import org.fictionally.ringpromoter.model.RingPipeline
import org.fictionally.ringpromoter.model.RingStatus
import org.fictionally.ringpromoter.model.Signoff
import org.fictionally.ringpromoter.model.VersionsResponse
import org.fictionally.ringpromoter.model.signoff
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.store.Session
import java.time.Instant

data class AppDetailState(
    val app: String,
    val title: String,
    val rings: List<RingStatus> = emptyList(),
    val versions: VersionsResponse = VersionsResponse.UNSUPPORTED,
    val maintenance: MaintenanceStatus = MaintenanceStatus.UNGATED,
    val signoffs: List<Signoff> = emptyList(),
    val isLoading: Boolean = false,
    val lastUpdated: Instant? = null,
    val error: ApiError? = null,
) {
    /**
     * Rings the app actually declares. An unconfigured ring is still shown, but
     * greyed out, so the pipeline's shape is never misleading.
     */
    val configuredRings: List<RingStatus> get() = rings.filter { it.configured }

    val unhealthyRings: List<RingStatus> get() = rings.filter { it.needsAttention }

    fun ring(named: String): RingStatus? = rings.firstOrNull { it.ring.name == named }

    /**
     * The sign-off recorded for one exact (ring, version), which is the only
     * match the QA gate accepts.
     */
    fun signoffFor(ring: String, version: String): Signoff? = signoffs.signoff(ring, version)
}

/**
 * Everything one application's detail screen needs, and the actions it can
 * start.
 *
 * The rules live in [PromotionRules]; this view model only asks. That is what
 * keeps "what may I offer?" in one tested place instead of scattered through
 * composables.
 */
class AppDetailViewModel(
    app: String,
    private val session: Session,
) : ViewModel() {

    private val _state = MutableStateFlow(
        AppDetailState(app = app, title = session.capabilities.value?.title(app) ?: app),
    )
    val state: StateFlow<AppDetailState> = _state.asStateFlow()

    private val refreshing = Mutex()

    val rules: PromotionRules get() = session.rules
    val pipeline: RingPipeline get() = session.pipeline
    val prodProtected: Boolean get() = session.prodProtected
    val aiEnabled: Boolean get() = session.aiEnabled

    /** This instance asks for the device unlock before a production deploy. */
    val requiresBiometricsForProduction: Boolean
        get() = session.requiresBiometricsForProduction

    suspend fun refresh() {
        if (!refreshing.tryLock()) return
        try {
            performRefresh()
        } finally {
            refreshing.unlock()
        }
    }

    private suspend fun performRefresh() {
        val app = _state.value.app
        _state.update { it.copy(isLoading = true) }
        try {
            val api = session.api.value
            try {
                val rings = api.rings(app)
                _state.update { it.copy(rings = rings, error = null, lastUpdated = Instant.now()) }
            } catch (e: ApiError) {
                session.note(e)
                _state.update { it.copy(error = e) }
            }

            // The rest is supporting detail: a failure here must not blank the
            // pipeline the operator came to look at.
            val versions = tolerate(_state.value.versions) { api.versions(app) }
            val maintenance = tolerate(_state.value.maintenance) { api.maintenance(app) }
            val signoffs = tolerate(_state.value.signoffs) { api.signoffs(app) }

            _state.update {
                it.copy(
                    versions = versions,
                    maintenance = maintenance,
                    signoffs = signoffs,
                    title = session.capabilities.value?.title(app) ?: it.title,
                )
            }
        } finally {
            _state.update { it.copy(isLoading = false) }
        }
    }

    /**
     * Reload only the parts an action sheet depends on, without the flicker of
     * a full refresh.
     */
    suspend fun refreshGates() {
        val app = _state.value.app
        val api = session.api.value
        val maintenance = tolerate(_state.value.maintenance) { api.maintenance(app) }
        val signoffs = tolerate(_state.value.signoffs) { api.signoffs(app) }
        // Ring gates carry `maintenance_window_open`, so they must be refreshed
        // too, or the badge and the sheet would disagree.
        val rings = tolerate(_state.value.rings) { api.rings(app) }
        _state.update {
            it.copy(maintenance = maintenance, signoffs = signoffs, rings = rings)
        }
    }

    // Actions. Each returns the id of the job the server started, which the
    // caller opens as a live view.

    suspend fun promote(fromRing: String, crCode: String?, password: String?): String =
        session.api.value.promote(_state.value.app, fromRing, crCode, password)

    suspend fun seed(
        ring: String,
        version: String,
        crCode: String?,
        password: String?,
    ): String = session.api.value.seed(_state.value.app, ring, version, crCode, password)

    suspend fun rollback(ring: String): String =
        session.api.value.rollback(_state.value.app, ring)

    suspend fun setAutoPromote(ring: String, enabled: Boolean, password: String?) {
        session.api.value.setAutoPromote(_state.value.app, ring, enabled, password)
        refresh()
    }

    suspend fun recordSignoff(
        ring: String,
        version: String,
        decision: Signoff.Decision,
        engineer: String,
        qaStatus: String,
        note: String,
    ) {
        session.api.value.recordSignoff(
            _state.value.app,
            org.fictionally.ringpromoter.model.NewSignoff(
                ring = ring,
                version = version,
                decision = decision,
                engineer = engineer,
                qaStatus = qaStatus,
                note = note,
            ),
        )
        session.settings.setSignoffEngineer(engineer)
        refreshGates()
    }

    /** See [org.fictionally.ringpromoter.ui.overview.OverviewViewModel] — ApiError only. */
    private inline fun <T> tolerate(fallback: T, call: () -> T): T =
        try {
            call()
        } catch (_: ApiError) {
            fallback
        }
}
