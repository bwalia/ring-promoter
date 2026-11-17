#if DEBUG
import SwiftUI

/// The app icon: `ConsoleBrandMark` at full size, which is what guarantees the
/// Dock and the sidebar show the same thing.
///
/// `--snapshot` renders this at all ten sizes macOS asks for, so every size is
/// the same artwork and none can drift from another. Only the backplate's
/// corner radius and inset are stated here, because Apple's icon grid is
/// specific about them and nothing else in the app has a backplate.
struct ConsoleAppIcon: View {
    var body: some View {
        ConsoleBrandMark(size: 1024)
            .frame(width: 1024, height: 1024)
    }
}
#endif
