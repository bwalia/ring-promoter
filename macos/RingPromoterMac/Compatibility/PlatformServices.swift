import AppKit
import SwiftUI

// Stand-ins for the UIKit services the shared code reaches for, each keeping
// the iOS call signature so the shared file does not change.

// MARK: - Haptics

/// Replaces `DesignSystem/Haptics.swift`, which is excluded from this target.
///
/// Not a no-op: Force Touch trackpads do have a haptic engine, and
/// `NSHapticFeedbackManager` drives it. On a Mac without one — a desktop, or an
/// external keyboard — AppKit discards the request, which is the correct
/// outcome and needs no check here.
///
/// The three-way split of the iOS version collapses because AppKit offers no
/// success/failure distinction. `.levelChange` is the sharpest of the available
/// patterns, so it marks the two transitions an operator is waiting for, and
/// `.generic` carries the rest.
@MainActor
enum Haptics {
    private static func perform(_ pattern: NSHapticFeedbackManager.FeedbackPattern) {
        NSHapticFeedbackManager.defaultPerformer.perform(pattern, performanceTime: .now)
    }

    /// An action was accepted by the server and a job is now running.
    static func actionStarted() { perform(.generic) }

    /// A job finished successfully.
    static func success() { perform(.levelChange) }

    /// A job failed, or was rolled back.
    static func failure() { perform(.levelChange) }

    /// Something was refused before it was sent.
    static func warning() { perform(.generic) }
}

// MARK: - Pasteboard

/// Covers `UIPasteboard.general.url = url` in TestKitView.
///
/// A URL is written as a plain string rather than as `.URL`: AppKit's URL type
/// makes the receiving app resolve a file reference, and every URL copied here
/// is an http(s) endpoint an operator is about to paste into a browser or a
/// terminal.
@MainActor
final class UIPasteboard {
    static let general = UIPasteboard()

    private init() {}

    var url: URL? {
        get {
            NSPasteboard.general.string(forType: .string).flatMap(URL.init(string:))
        }
        set {
            guard let newValue else { return }
            NSPasteboard.general.clearContents()
            NSPasteboard.general.setString(newValue.absoluteString, forType: .string)
        }
    }

    var string: String? {
        get { NSPasteboard.general.string(forType: .string) }
        set {
            guard let newValue else { return }
            NSPasteboard.general.clearContents()
            NSPasteboard.general.setString(newValue, forType: .string)
        }
    }
}

// MARK: - Application

/// Covers `UIApplication.shared.registerForRemoteNotifications()` in
/// PushRegistration.
///
/// This is a real forward, not a stub: macOS supports APNs through
/// `NSApplication`. Whether a token actually arrives depends on the push
/// entitlement, which this target does not yet carry — see macos/README.md.
@MainActor
final class UIApplication {
    static let shared = UIApplication()

    private init() {}

    func registerForRemoteNotifications() {
        NSApplication.shared.registerForRemoteNotifications()
    }

    func unregisterForRemoteNotifications() {
        NSApplication.shared.unregisterForRemoteNotifications()
    }
}

// MARK: - Live Activities

/// Replaces `Stores/LiveActivityController.swift`, which is excluded.
///
/// ActivityKit is iPhone-only — there is no Lock Screen or Dynamic Island to
/// draw on. The surface is reproduced exactly so `JobView` compiles unchanged;
/// every method does nothing.
///
/// The Mac equivalent of "watch this job without keeping the window open" is a
/// user notification, which is a separate piece of work and is not pretended at
/// here.
@MainActor
final class LiveActivityController {
    static let shared = LiveActivityController()

    private init() {}

    func start(job: Job, appTitle: String, targetRing: String, version: String) {}

    func update(with job: Job) async {}

    func end(with job: Job) async {}

    func endOrphans() async {}
}
