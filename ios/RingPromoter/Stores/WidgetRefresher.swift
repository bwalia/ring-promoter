import Foundation
import WidgetKit

/// Nudges WidgetKit after the app writes a new snapshot.
///
/// Kept behind a tiny wrapper so the call sites do not import WidgetKit, and so
/// reloads can be rate-limited: the system budgets widget refreshes, and firing
/// one on every 15-second poll would spend that budget for no visible gain.
enum WidgetRefresher {
    /// Minimum gap between reloads.
    private static let minimumInterval: TimeInterval = 60

    nonisolated(unsafe) private static var lastReload: Date?
    private static let lock = NSLock()

    static func reloadAll() {
        lock.lock()
        let now = Date()
        if let last = lastReload, now.timeIntervalSince(last) < minimumInterval {
            lock.unlock()
            return
        }
        lastReload = now
        lock.unlock()
        WidgetCenter.shared.reloadAllTimelines()
    }

    /// Reload regardless of the rate limit — used after an action finishes,
    /// where the state really has changed.
    static func reloadNow() {
        lock.lock()
        lastReload = Date()
        lock.unlock()
        WidgetCenter.shared.reloadAllTimelines()
    }
}
