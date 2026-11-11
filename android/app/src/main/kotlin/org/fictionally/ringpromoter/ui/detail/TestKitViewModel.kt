package org.fictionally.ringpromoter.ui.detail

import androidx.lifecycle.ViewModel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import org.fictionally.ringpromoter.model.DiagnosisStatus
import org.fictionally.ringpromoter.model.TestKit
import org.fictionally.ringpromoter.model.TestLink
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.store.Session

data class TestKitState(
    val app: String,
    val ring: String,
    val kit: TestKit? = null,
    val isRequesting: Boolean = false,
    val error: String? = null,
    /** Checklist items the tester has ticked. Local to this sitting. */
    val done: Set<Int> = emptySet(),
) {
    /**
     * Links that can actually be opened. [TestLink.destination] is null for
     * anything that is not http(s) — these URLs arrive over the network, and
     * handing an arbitrary scheme to an Intent is how an app ends up launching
     * something it never meant to.
     */
    private val usable: List<TestLink> get() = kit?.links.orEmpty().filter { it.destination != null }

    val tryable: List<TestLink> get() = usable.filter { !it.isFromLogs && it.isTryable }
    val outputs: List<TestLink> get() = usable.filter { !it.isFromLogs && !it.isTryable }
    val fromLogs: List<TestLink> get() = usable.filter { it.isFromLogs }
    val hasLinks: Boolean get() = usable.isNotEmpty()

    val planIsRunning: Boolean
        get() = isRequesting || kit?.planStatus == DiagnosisStatus.RUNNING
}

/**
 * Where to test a ring's current version, and the AI's suggestion of what to
 * check.
 */
class TestKitViewModel(
    app: String,
    ring: String,
    private val session: Session,
) : ViewModel() {

    private val _state = MutableStateFlow(TestKitState(app = app, ring = ring))
    val state: StateFlow<TestKitState> = _state.asStateFlow()

    suspend fun load() {
        try {
            val kit = session.api.value.testKit(_state.value.app, _state.value.ring)
            _state.update { it.copy(kit = kit, error = null) }
        } catch (e: ApiError) {
            session.note(e)
            _state.update { it.copy(error = e.userMessage) }
        }
    }

    fun toggle(index: Int) {
        _state.update {
            it.copy(done = if (index in it.done) it.done - index else it.done + index)
        }
    }

    /**
     * Ask the server to plan, then poll until it settles.
     *
     * The plan is generated server-side and shared, so a colleague's plan is
     * the one that loads; [refresh] is the only way to replace it.
     */
    suspend fun requestPlan(refresh: Boolean) {
        _state.update { it.copy(isRequesting = true, error = null, done = emptySet()) }
        try {
            var current = session.api.value.planTestKit(
                _state.value.app,
                _state.value.ring,
                refresh,
            )
            _state.update { it.copy(kit = current) }
            var attempts = 0
            while (current.planStatus == DiagnosisStatus.RUNNING && attempts < POLL_ATTEMPTS) {
                delay(POLL_INTERVAL_MS)
                current = session.api.value.testKit(_state.value.app, _state.value.ring)
                _state.update { it.copy(kit = current) }
                attempts += 1
            }
        } catch (e: ApiError) {
            session.note(e)
            _state.update { it.copy(error = e.userMessage) }
        } finally {
            _state.update { it.copy(isRequesting = false) }
        }
    }

    private companion object {
        const val POLL_ATTEMPTS = 120
        const val POLL_INTERVAL_MS = 2_000L
    }
}
