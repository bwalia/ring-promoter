import SwiftUI
import WidgetKit

@main
struct RingPromoterWidgetBundle: WidgetBundle {
    var body: some Widget {
        PipelineWidget()
        HealthWidget()
        PromotionLiveActivity()
    }
}
