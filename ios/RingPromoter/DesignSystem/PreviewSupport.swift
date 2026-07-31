import SwiftUI

/// Builds a ready-to-render environment for previews.
///
/// Previews run against `DemoClient` — the same fixture-backed simulation demo
/// mode uses — so a preview that looks right is evidence the real screen will
/// too, rather than a hand-arranged mock that can drift from the API.
@MainActor
enum PreviewSupport {
    /// A session already in demo mode, with `GET /api/apps` preloaded so the
    /// ring pipeline and capability flags are available synchronously.
    static func session() -> AppSession {
        let session = AppSession(
            instances: InstanceStore(defaults: previewDefaults),
            settings: AppSettings(defaults: previewDefaults)
        )
        session.enterDemoMode()
        session.preloadForPreview(PreviewData.appsResponse)
        return session
    }

    static func router() -> Router { Router() }

    /// An isolated defaults suite, so opening a preview cannot overwrite the
    /// real app's saved instances on the same simulator.
    private static let previewDefaults: UserDefaults = {
        UserDefaults(suiteName: "RingPromoter.previews") ?? .standard
    }()
}

extension AppSession {
    /// Inject capabilities without a round trip. Previews only.
    func preloadForPreview(_ response: AppsResponse) {
        setCapabilitiesForPreview(response)
    }
}

/// A container that supplies the preview environment to any screen.
struct PreviewHost<Content: View>: View {
    @State private var session = PreviewSupport.session()
    @State private var router = PreviewSupport.router()
    private let content: Content

    init(@ViewBuilder content: () -> Content) {
        self.content = content()
    }

    var body: some View {
        content
            .environment(session)
            .environment(router)
            .environment(PushRegistration())
    }
}
