import SwiftUI
import WidgetKit

@main
struct RingPromoterWidgetBundle: WidgetBundle {
    var body: some Widget {
        PipelineWidget()
        HealthWidget()
        // PromotionLiveActivity omitted until ActivityKit is re-enabled for
        // TestFlight — see NSSupportsLiveActivities in RingPromoter-Info.plist.
    }
}
