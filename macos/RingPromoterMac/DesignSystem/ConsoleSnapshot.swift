#if DEBUG
import AppKit
import SwiftUI

/// Renders the console chrome to PNGs and exits:
///
///     RingPromoterMac.app/Contents/MacOS/RingPromoterMac --snapshot <dir>
///
/// Debug-only, and not reachable without the argument.
///
/// It exists because a screenshot of the running window needs macOS Screen
/// Recording permission, which a terminal doing the build does not have — so
/// a UI change could be built, launched and still never looked at. This draws
/// the same views offscreen with `ImageRenderer`, in both appearances, using
/// the demo fixtures that ship in the bundle.
@MainActor
enum ConsoleSnapshot {
    /// The directory to write into, or nil when the app was launched normally.
    static var requestedDirectory: String? {
        let arguments = ProcessInfo.processInfo.arguments
        guard let flag = arguments.firstIndex(of: "--snapshot"),
              arguments.index(after: flag) < arguments.endIndex
        else { return nil }
        return arguments[arguments.index(after: flag)]
    }

    static func run(writingTo directory: String) async -> Never {
        let session = AppSession()
        session.enterDemoMode()
        let router = Router()
        let focus = ConsoleFocus()
        let store = OverviewStore(session: session)
        await store.refresh()

        // Neither a ScrollView nor a NavigationStack draws its contents
        // under ImageRenderer, so the shell is assembled from the same views
        // the window uses, minus those two wrappers.
        let shell = HStack(alignment: .top, spacing: 0) {
            VStack(spacing: 0) {
                MacSidebarSections(store: store)
                Spacer(minLength: 0)
                ConnectionFooter(
                    isAddingInstance: .constant(false),
                    isManagingInstances: .constant(false)
                )
            }
            .frame(width: 268, height: 900)
            .background(Color.rpSidebar)
            Rectangle()
                .fill(Color.rpBorder)
                .frame(width: 1)
            OverviewCardStack(store: store)
        }
        .frame(width: 1180, height: 900, alignment: .top)
        .background(Color.rpBackground)
        .environment(session)
        .environment(router)
        .environment(focus)

        // The detail pane, at the width the window actually gives it.
        let detail = AppDetailStore(app: store.summaries.first?.name ?? "web-frontend", session: session)
        await detail.refresh()
        let detailPane = AppDetailPipeline(store: detail) { _ in }
            .frame(width: 1011, alignment: .top)
            .background(Color.rpBackground)
            .environment(session)
            .environment(router)

        // And the same pane in a narrow window, to prove the side-by-side
        // arrangement falls back to a stack rather than squeezing.
        let detailNarrow = AppDetailPipeline(store: detail) { _ in }
            .frame(width: 600, alignment: .top)
            .background(Color.rpBackground)
            .environment(session)
            .environment(router)

        for (name, appearance) in [
            ("console-dark", NSAppearance(named: .darkAqua)),
            ("console-light", NSAppearance(named: .aqua)),
        ] {
            guard let appearance else { continue }
            let scheme: ColorScheme = name.hasSuffix("dark") ? .dark : .light
            appearance.performAsCurrentDrawingAppearance {
                write(shell.environment(\.colorScheme, scheme), to: "\(directory)/\(name).png")
                write(
                    detailPane.environment(\.colorScheme, scheme),
                    to: "\(directory)/\(name)-detail.png"
                )
                write(
                    detailNarrow.environment(\.colorScheme, scheme),
                    to: "\(directory)/\(name)-detail-narrow.png"
                )
            }
        }
        exit(0)
    }

    private static func write(_ view: some View, to path: String) {
        let renderer = ImageRenderer(content: view)
        renderer.scale = 2
        guard let image = renderer.nsImage,
              let tiff = image.tiffRepresentation,
              let bitmap = NSBitmapImageRep(data: tiff),
              let png = bitmap.representation(using: .png, properties: [:])
        else {
            FileHandle.standardError.write(Data("snapshot: render failed for \(path)\n".utf8))
            return
        }
        do {
            try png.write(to: URL(fileURLWithPath: path))
            FileHandle.standardError.write(Data("snapshot: wrote \(path)\n".utf8))
        } catch {
            FileHandle.standardError.write(
                Data("snapshot: \(path): \(error.localizedDescription)\n".utf8)
            )
        }
    }
}
#endif
