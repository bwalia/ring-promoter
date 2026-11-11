package org.fictionally.ringpromoter.store

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import okhttp3.OkHttpClient
import org.fictionally.ringpromoter.demo.DemoClient
import org.fictionally.ringpromoter.demo.DemoWorld
import org.fictionally.ringpromoter.demo.FixtureSource
import org.fictionally.ringpromoter.model.AppsResponse
import org.fictionally.ringpromoter.model.PromotionRules
import org.fictionally.ringpromoter.model.RingPipeline
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.net.RingPromoterApi
import org.fictionally.ringpromoter.net.RingPromoterClient
import org.fictionally.ringpromoter.net.UnconfiguredClient
import org.fictionally.ringpromoter.ui.theme.InstanceTint

/**
 * The app's root state: which control plane is active, which client talks to
 * it, and what that server said about itself.
 *
 * Everything downstream reads its [api] from here, so switching instance or
 * entering demo mode swaps one object and the whole app follows.
 */
class Session(
    val instances: InstanceStore,
    val settings: Settings,
    private val fixtures: FixtureSource,
    private val http: OkHttpClient = RingPromoterClient.defaultHttpClient(),
    /**
     * Where the home-screen widget's snapshot goes. Null in tests and
     * previews, which have no widget host.
     */
    private val widget: WidgetPublisher? = null,
) {
    /** How the app is currently talking to a control plane. */
    sealed interface Mode {
        /** No instance chosen yet — onboarding. */
        data object Disconnected : Mode

        /** Talking to a real server. */
        data class Live(val instance: Instance) : Mode

        /** Driven entirely from bundled fixtures. */
        data object Demo : Mode
    }

    private val _mode = MutableStateFlow<Mode>(Mode.Disconnected)
    val mode: StateFlow<Mode> = _mode.asStateFlow()

    /**
     * The client every screen uses. Swapped atomically with [mode].
     *
     * Starts as [UnconfiguredClient], which refuses everything: building a demo
     * client here would load the whole fixture corpus at every launch and would
     * serve demo data to any screen that asked before connecting.
     */
    private val _api = MutableStateFlow<RingPromoterApi>(UnconfiguredClient)
    val api: StateFlow<RingPromoterApi> = _api.asStateFlow()

    /**
     * What `GET /api/apps` last told us: the app list, the ring pipeline, and
     * the capability flags. Null until the first successful load.
     */
    private val _capabilities = MutableStateFlow<AppsResponse?>(null)
    val capabilities: StateFlow<AppsResponse?> = _capabilities.asStateFlow()

    /**
     * Set when the server rejected the token, so the UI can send the operator
     * back to the token screen instead of showing a dismissible toast.
     */
    private val _needsReauthentication = MutableStateFlow(false)
    val needsReauthentication: StateFlow<Boolean> = _needsReauthentication.asStateFlow()

    /**
     * Bumped every time [api] is replaced, so a response that was in flight for
     * the previous control plane is dropped instead of being applied to the new
     * one.
     */
    private var generation = 0

    // Derived state

    val isDemo: Boolean get() = _mode.value is Mode.Demo

    /**
     * Identifies the control plane the app is talking to; null when
     * disconnected.
     *
     * The navigation tree is keyed on it, so switching instance rebuilds every
     * screen and view model instead of leaving the previous server's apps,
     * rings and history on screen.
     */
    val connectionKey: String?
        get() = when (val m = _mode.value) {
            is Mode.Disconnected -> null
            is Mode.Demo -> "demo"
            is Mode.Live -> m.instance.id
        }

    val activeInstance: Instance? get() = (_mode.value as? Mode.Live)?.instance

    /** The banner name shown on every acting screen. */
    val instanceName: String
        get() = when (val m = _mode.value) {
            is Mode.Demo -> "Demo"
            is Mode.Live -> m.instance.name
            is Mode.Disconnected -> "Not connected"
        }

    val instanceTint: InstanceTint
        get() = when (val m = _mode.value) {
            is Mode.Demo -> InstanceTint.PURPLE
            is Mode.Live -> m.instance.tint
            is Mode.Disconnected -> InstanceTint.GRAPHITE
        }

    val pipeline: RingPipeline get() = _capabilities.value?.pipeline ?: RingPipeline.EMPTY

    /** The server requires a production password for last-ring deploys. */
    val prodProtected: Boolean get() = _capabilities.value?.prodProtected ?: false

    val aiEnabled: Boolean get() = _capabilities.value?.aiEnabled ?: false

    val rules: PromotionRules get() = PromotionRules(pipeline, prodProtected)

    /** True when this instance wants biometrics before a production action. */
    val requiresBiometricsForProduction: Boolean
        get() = activeInstance?.requireBiometricsForProduction ?: false

    // Connecting

    /**
     * Restore the last-used instance at launch. Returns false when there is
     * nothing to restore, or its token has gone.
     */
    fun restoreLastSession(): Boolean {
        if (settings.startsInDemoMode.value) {
            enterDemoMode()
            return true
        }
        val instance = instances.active ?: return false
        val token = instances.token(instance)?.takeIf { it.isNotEmpty() } ?: return false
        connect(instance, token)
        return true
    }

    fun connect(instance: Instance, token: String) {
        _api.value = RingPromoterClient(instance.baseUrl, token, http)
        generation += 1
        _mode.value = Mode.Live(instance)
        _needsReauthentication.value = false
        _capabilities.value = null
        instances.setActive(instance)
        settings.setStartsInDemoMode(false)
    }

    /**
     * Switch to a saved instance whose token is already stored.
     *
     * Returns false when the token has gone behind the app's back; the caller
     * then asks for that instance's token. Nothing is flagged here:
     * [needsReauthentication] belongs to the *active* instance, and a missing
     * token on another one says nothing about it.
     */
    fun activate(instance: Instance): Boolean {
        val token = instances.token(instance)?.takeIf { it.isNotEmpty() } ?: return false
        connect(instance, token)
        return true
    }

    /**
     * Replace the active instance's metadata (name, colour, biometric
     * preference) in place.
     *
     * Deliberately does **not** go through [connect]: rebuilding the client
     * would clear the loaded capabilities, so flipping a toggle in Settings
     * would blank the ring pipeline until the next refresh.
     */
    fun updateActiveInstance(instance: Instance) {
        val current = (_mode.value as? Mode.Live)?.instance ?: return
        if (current.id != instance.id) return
        instances.update(instance)
        _mode.value = Mode.Live(instance)
    }

    fun enterDemoMode() {
        _api.value = DemoClient(DemoWorld.load(fixtures))
        generation += 1
        _mode.value = Mode.Demo
        _capabilities.value = null
        _needsReauthentication.value = false
        settings.setStartsInDemoMode(true)
    }

    /** Leave the current instance without deleting it, returning to onboarding. */
    fun disconnect() {
        widget?.clear()
        _mode.value = Mode.Disconnected
        _api.value = UnconfiguredClient
        generation += 1
        _capabilities.value = null
        settings.setStartsInDemoMode(false)
    }

    // Capabilities

    /**
     * Load `GET /api/apps`, which every screen depends on for the ring pipeline
     * and the capability flags.
     */
    suspend fun loadCapabilities(): AppsResponse {
        val started = generation
        try {
            val response = _api.value.apps()
            // The operator switched control plane while this was in flight:
            // these capabilities describe the old server, so drop them.
            if (started != generation) {
                throw ApiError.Transport(ApiError.TransportFailure.Cancelled)
            }
            _capabilities.value = response
            _needsReauthentication.value = false
            return response
        } catch (e: ApiError) {
            if (started == generation && e.requiresReauthentication) {
                _needsReauthentication.value = true
            }
            throw e
        }
    }

    /**
     * Report an error raised anywhere in the app, so a 401 from any screen
     * routes the operator back to the token screen exactly once.
     */
    fun note(error: ApiError) {
        if (error.requiresReauthentication) _needsReauthentication.value = true
    }

    /**
     * Hand the widget what the Overview just loaded.
     *
     * Only ever the app list, versions and health — never a token or a server
     * address. See [org.fictionally.ringpromoter.widget.WidgetSnapshot].
     */
    fun publishWidgetSnapshot(apps: List<org.fictionally.ringpromoter.ui.overview.AppSummary>) {
        widget?.publish(instanceName, instanceTint.name.lowercase(), isDemo, apps)
    }

    fun clearReauthenticationFlag() {
        _needsReauthentication.value = false
    }
}
