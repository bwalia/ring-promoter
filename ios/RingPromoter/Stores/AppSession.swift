import Foundation
import SwiftUI

/// The app's root state: which control plane is active, which client talks to
/// it, and what that server said about itself.
///
/// Everything downstream reads its `api` from here, so switching instance or
/// entering demo mode swaps one object and the whole app follows.
@MainActor
@Observable
final class AppSession {
    /// How the app is currently talking to a control plane.
    enum Mode: Equatable {
        /// No instance chosen yet — onboarding.
        case disconnected
        /// Talking to a real server.
        case live(Instance)
        /// Driven entirely from bundled fixtures.
        case demo
    }

    private(set) var mode: Mode = .disconnected
    /// The client every screen uses. Swapped atomically with `mode`.
    ///
    /// Starts as `UnconfiguredClient`, which refuses everything: building a
    /// `DemoClient` here would decode the whole fixture corpus at every launch
    /// and would serve demo data to any screen that asked before connecting.
    private(set) var api: any RingPromoterAPI = UnconfiguredClient()
    /// What `GET /api/apps` last told us: the app list, the ring pipeline, and
    /// the two capability flags. Nil until the first successful load.
    private(set) var capabilities: AppsResponse?
    /// Set when the server rejected the token, so the UI can send the operator
    /// back to the token screen instead of showing a dismissible toast.
    private(set) var needsReauthentication = false

    var instances: InstanceStore
    var settings: AppSettings
    let biometrics: BiometricGate

    /// A shared URLSession: one connection pool for the whole app.
    private let session = RingPromoterClient.makeSession()

    init(
        instances: InstanceStore = InstanceStore(),
        settings: AppSettings = AppSettings(),
        biometrics: BiometricGate = BiometricGate()
    ) {
        self.instances = instances
        self.settings = settings
        self.biometrics = biometrics
    }

    // MARK: - Derived state

    var isDemo: Bool { mode == .demo }

    var activeInstance: Instance? {
        if case .live(let instance) = mode { return instance }
        return nil
    }

    /// The banner name and colour shown on every acting screen.
    var instanceName: String {
        switch mode {
        case .demo: "Demo"
        case .live(let instance): instance.name
        case .disconnected: "Not connected"
        }
    }

    var instanceTint: InstanceTint {
        switch mode {
        case .demo: .purple
        case .live(let instance): instance.tint
        case .disconnected: .graphite
        }
    }

    var pipeline: RingPipeline { capabilities?.pipeline ?? .empty }

    /// The server requires a production password for last-ring deploys.
    var prodProtected: Bool { capabilities?.prodProtected ?? false }

    var aiEnabled: Bool { capabilities?.aiEnabled ?? false }

    var rules: PromotionRules {
        PromotionRules(pipeline: pipeline, prodProtected: prodProtected)
    }

    /// True when this instance wants Face ID before a production action.
    var requiresBiometricsForProduction: Bool {
        activeInstance?.requireBiometricsForProduction ?? false
    }

    // MARK: - Connecting

    /// Restore the last-used instance at launch. Returns false when there is
    /// nothing to restore, or its token has gone.
    @discardableResult
    func restoreLastSession() -> Bool {
        if settings.startsInDemoMode {
            enterDemoMode()
            return true
        }
        guard let instance = instances.active,
              let token = (try? Keychain.token(for: instance.keychainAccount)) ?? nil,
              !token.isEmpty
        else { return false }
        connect(to: instance, token: token)
        return true
    }

    func connect(to instance: Instance, token: String) {
        api = RingPromoterClient(baseURL: instance.baseURL, token: token, session: session)
        mode = .live(instance)
        needsReauthentication = false
        capabilities = nil
        instances.setActive(instance)
        settings.startsInDemoMode = false
    }

    /// Switch to a saved instance whose token is already in the Keychain.
    /// Returns false when the token has been removed behind the app's back.
    @discardableResult
    func activate(_ instance: Instance) -> Bool {
        guard let token = (try? Keychain.token(for: instance.keychainAccount)) ?? nil,
              !token.isEmpty
        else {
            needsReauthentication = true
            return false
        }
        connect(to: instance, token: token)
        return true
    }

    /// Replace the active instance's metadata (name, colour, biometric
    /// preference) in place.
    ///
    /// Deliberately does **not** go through `connect`: rebuilding the client
    /// would clear the loaded capabilities, so flipping a toggle in Settings
    /// would blank the ring pipeline until the next refresh.
    func updateActiveInstance(_ instance: Instance) {
        guard case .live(let current) = mode, current.id == instance.id else { return }
        instances.update(instance)
        mode = .live(instance)
    }

    func enterDemoMode() {
        api = DemoClient()
        mode = .demo
        capabilities = nil
        needsReauthentication = false
        settings.startsInDemoMode = true
    }

    /// Leave the current instance without deleting it, returning to onboarding.
    func disconnect() {
        mode = .disconnected
        api = UnconfiguredClient()
        capabilities = nil
        settings.startsInDemoMode = false
        WidgetSnapshotStore.clear()
    }

    // MARK: - Capabilities

    /// Load `GET /api/apps`, which every screen depends on for the ring
    /// pipeline and the capability flags.
    @discardableResult
    func loadCapabilities() async throws(APIError) -> AppsResponse {
        do {
            let response = try await api.apps()
            capabilities = response
            needsReauthentication = false
            return response
        } catch {
            if error.requiresReauthentication { needsReauthentication = true }
            throw error
        }
    }

    /// Report an error raised anywhere in the app, so a 401 from any screen
    /// routes the operator back to the token screen exactly once.
    func note(_ error: APIError) {
        if error.requiresReauthentication { needsReauthentication = true }
    }

    func clearReauthenticationFlag() { needsReauthentication = false }

    /// Seed capabilities without a round trip, so a SwiftUI preview renders a
    /// populated screen immediately. Not used at runtime.
    func setCapabilitiesForPreview(_ response: AppsResponse) {
        capabilities = response
    }
}

/// User preferences that are not secrets.
///
/// Backed by `UserDefaults` — deliberately, so it is obvious that nothing
/// sensitive is kept here. Tokens live in the Keychain; see `Keychain`.
@MainActor
@Observable
final class AppSettings {
    private enum Key {
        static let refreshInterval = "rp.refreshInterval"
        static let lockOnOpen = "rp.lockOnOpen"
        static let theme = "rp.theme"
        static let demoMode = "rp.demoMode"
        static let signoffEngineer = "rp.signoffEngineer"
    }

    enum Theme: String, CaseIterable, Identifiable, Sendable {
        case system, light, dark

        var id: String { rawValue }
        var label: String { rawValue.capitalized }

        var colorScheme: ColorScheme? {
            switch self {
            case .system: nil
            case .light: .light
            case .dark: .dark
            }
        }
    }

    /// How often the Overview refreshes itself while on screen. Zero means
    /// manual only, which is the right choice on a metered connection.
    var refreshInterval: TimeInterval {
        didSet { defaults.set(refreshInterval, forKey: Key.refreshInterval) }
    }

    /// Require Face ID / Touch ID whenever the app comes to the foreground.
    var lockOnOpen: Bool {
        didSet { defaults.set(lockOnOpen, forKey: Key.lockOnOpen) }
    }

    var theme: Theme {
        didSet { defaults.set(theme.rawValue, forKey: Key.theme) }
    }

    var startsInDemoMode: Bool {
        didSet { defaults.set(startsInDemoMode, forKey: Key.demoMode) }
    }

    /// Remembered so a release engineer does not retype their name on every
    /// sign-off. A name, not a credential.
    var signoffEngineer: String {
        didSet { defaults.set(signoffEngineer, forKey: Key.signoffEngineer) }
    }

    static let refreshOptions: [TimeInterval] = [0, 15, 30, 60, 300]

    private let defaults: UserDefaults

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        let stored = defaults.object(forKey: Key.refreshInterval) as? TimeInterval
        refreshInterval = stored ?? 30
        lockOnOpen = defaults.bool(forKey: Key.lockOnOpen)
        theme = Theme(rawValue: defaults.string(forKey: Key.theme) ?? "") ?? .system
        startsInDemoMode = defaults.bool(forKey: Key.demoMode)
        signoffEngineer = defaults.string(forKey: Key.signoffEngineer) ?? ""
    }

    func label(forRefresh interval: TimeInterval) -> String {
        switch interval {
        case 0: "Manual only"
        case ..<60: "\(Int(interval)) seconds"
        case 60: "1 minute"
        default: "\(Int(interval / 60)) minutes"
        }
    }
}
