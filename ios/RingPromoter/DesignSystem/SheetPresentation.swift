import SwiftUI

// How a Form-based sheet presents itself.
//
// Both of these are no-ops on iOS, where the defaults are already right: a
// Form is grouped, and a sheet fills the screen. They exist because macOS
// gets neither for free and a sheet does not inherit either setting from
// whatever presented it — `AddInstanceView` alone is opened from four
// different places, so the screen has to say it rather than the caller.
extension View {
    /// macOS defaults a `Form` to `.columns`, which draws no section
    /// grouping and no insets — the Edit-cluster sheet ran its fields flush
    /// to both edges with the section titles loose among them. `.grouped` is
    /// what the rest of the app assumes.
    func rpGroupedForm() -> some View {
        #if os(macOS)
        formStyle(.grouped)
        #else
        self
        #endif
    }

    /// A sheet is the whole display on an iPhone and a free-floating panel on
    /// a Mac, where SwiftUI sizes it to its content and gives a Form barely
    /// more room than its longest line.
    func rpSheetFrame(width: CGFloat = 560, height: CGFloat = 560) -> some View {
        #if os(macOS)
        frame(
            minWidth: width, idealWidth: width,
            minHeight: height, idealHeight: height
        )
        #else
        self
        #endif
    }

    /// Both of the above, which is what every Form-in-a-sheet wants.
    func rpSheetForm(width: CGFloat = 560, height: CGFloat = 560) -> some View {
        rpGroupedForm().rpSheetFrame(width: width, height: height)
    }
}
