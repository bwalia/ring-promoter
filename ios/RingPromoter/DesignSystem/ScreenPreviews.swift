import SwiftUI

// Every screen, in light, dark, and at the largest Dynamic Type size.
//
// The accessibility-size previews are the ones worth actually looking at: they
// are where a four-chip pipeline strip reflows to a vertical list, where badges
// wrap, and where a truncated label would go unnoticed at the default size.

#Preview("Overview — light") {
    PreviewHost { OverviewView() }
        .preferredColorScheme(.light)
}

#Preview("Overview — dark") {
    PreviewHost { OverviewView() }
        .preferredColorScheme(.dark)
}

#Preview("Overview — largest text") {
    PreviewHost { OverviewView() }
        .environment(\.dynamicTypeSize, .accessibility5)
}

#Preview("App detail — light") {
    PreviewHost {
        NavigationStack { AppDetailView(app: "payments-api") }
    }
    .preferredColorScheme(.light)
}

#Preview("App detail — dark") {
    PreviewHost {
        NavigationStack { AppDetailView(app: "payments-api") }
    }
    .preferredColorScheme(.dark)
}

#Preview("App detail — largest text") {
    PreviewHost {
        NavigationStack { AppDetailView(app: "payments-api") }
    }
    .environment(\.dynamicTypeSize, .accessibility5)
}

#Preview("Live job — running") {
    PreviewHost {
        NavigationStack { JobView(app: "payments-api", jobID: "job-1") }
    }
}

#Preview("Live job — dark") {
    PreviewHost {
        NavigationStack { JobView(app: "payments-api", jobID: "job-1") }
    }
    .preferredColorScheme(.dark)
}

#Preview("Job outcome states", traits: .sizeThatFitsLayout) {
    // The three terminal states side by side — they must never be mistaken for
    // one another.
    List {
        JobOutcomeBanner(job: PreviewData.runningJob)
        JobOutcomeBanner(job: PreviewData.rolledBackJob)
    }
}

#Preview("History — light") {
    PreviewHost {
        NavigationStack { HistoryView(app: "web-frontend") }
    }
}

#Preview("History — dark") {
    PreviewHost {
        NavigationStack { HistoryView(app: "web-frontend") }
    }
    .preferredColorScheme(.dark)
}

#Preview("Activity — light") {
    PreviewHost { ActivityFeedView() }
}

#Preview("Activity — dark") {
    PreviewHost { ActivityFeedView() }
        .preferredColorScheme(.dark)
}

#Preview("Maintenance") {
    PreviewHost {
        NavigationStack { MaintenanceView(app: "payments-api") }
    }
}

#Preview("Maintenance — dark") {
    PreviewHost {
        NavigationStack { MaintenanceView(app: "payments-api") }
    }
    .preferredColorScheme(.dark)
}

#Preview("Groups") {
    PreviewHost { GroupsView() }
}

#Preview("Settings — light") {
    PreviewHost { SettingsView() }
}

#Preview("Settings — dark") {
    PreviewHost { SettingsView() }
        .preferredColorScheme(.dark)
}

#Preview("Settings — largest text") {
    PreviewHost { SettingsView() }
        .environment(\.dynamicTypeSize, .accessibility5)
}

#Preview("Onboarding — light") {
    PreviewHost { OnboardingView() }
}

#Preview("Onboarding — dark") {
    PreviewHost { OnboardingView() }
        .preferredColorScheme(.dark)
}

#Preview("Add instance") {
    PreviewHost { AddInstanceView() }
}

#Preview("Lock screen") {
    PreviewHost { LockScreen() }
}

#Preview("Pipeline strip — largest text", traits: .sizeThatFitsLayout) {
    // At accessibility sizes the strip reflows to a vertical list rather than
    // squeezing four chips across the screen.
    VStack(alignment: .leading, spacing: 16) {
        PipelineStrip(rings: PreviewData.healthyRings, pipeline: PreviewData.pipeline)
        PipelineStrip(rings: PreviewData.troubledRings, pipeline: PreviewData.pipeline)
    }
    .padding()
    .environment(\.dynamicTypeSize, .accessibility3)
}

#Preview("Health legend", traits: .sizeThatFitsLayout) {
    // Every state, so it is obvious that each carries its own icon and is not
    // distinguished by colour alone.
    VStack(alignment: .leading, spacing: 10) {
        ForEach(
            [
                HealthPresentation.healthy, .unhealthy, .empty, .notConfigured, .inFlight,
            ],
            id: \.self
        ) { state in
            Label(state.label, systemImage: state.systemImage)
                .foregroundStyle(state.tint)
        }
    }
    .padding()
}
