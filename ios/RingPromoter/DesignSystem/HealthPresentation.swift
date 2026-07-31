import SwiftUI

/// How a ring's state is rendered.
///
/// **Colour is never the only signal.** Each state carries a distinct SF Symbol
/// *and* a distinct shape, so the pipeline is readable with any form of colour
/// blindness, in bright sun, or on a greyscale screenshot. This is also what
/// the VoiceOver label is built from, so what is spoken matches what is drawn.
enum HealthPresentation: Hashable, Sendable {
    /// Deployed and passing its health check.
    case healthy
    /// Deployed and failing its health check — the state that needs an
    /// operator.
    case unhealthy
    /// Configured, but nothing has ever been deployed here.
    case empty
    /// The app does not declare this ring at all.
    case notConfigured
    /// A deploy is in flight right now.
    case inFlight

    init(_ ring: RingStatus, isBusy: Bool = false) {
        if isBusy {
            self = .inFlight
        } else if !ring.configured {
            self = .notConfigured
        } else if ring.isEmpty {
            self = .empty
        } else {
            self = ring.isHealthy ? .healthy : .unhealthy
        }
    }

    var systemImage: String {
        switch self {
        case .healthy: "checkmark.circle.fill"
        case .unhealthy: "exclamationmark.triangle.fill"
        case .empty: "circle.dotted"
        case .notConfigured: "minus.circle"
        case .inFlight: "arrow.triangle.2.circlepath"
        }
    }

    var tint: Color {
        switch self {
        case .healthy: .rpHealthy
        case .unhealthy: .rpUnhealthy
        case .empty: .rpNeutral
        case .notConfigured: .rpDisabled
        case .inFlight: .rpInFlight
        }
    }

    /// A fill that stays legible in both appearances without shouting.
    var background: Color {
        switch self {
        case .notConfigured: Color.rpDisabled.opacity(0.08)
        default: tint.opacity(0.14)
        }
    }

    /// Spoken by VoiceOver and shown in the legend.
    var label: String {
        switch self {
        case .healthy: "Healthy"
        case .unhealthy: "Unhealthy"
        case .empty: "Never deployed"
        case .notConfigured: "Not in this pipeline"
        case .inFlight: "Deploying"
        }
    }

    /// Needs an operator's attention now.
    var isTrouble: Bool { self == .unhealthy }
}

extension Color {
    /// Semantic colours. Defined in code rather than the asset catalogue so
    /// each one can carry the reason it exists, and so the widget can use the
    /// identical values without a shared catalogue.
    ///
    /// Both appearances are specified explicitly: the dark variants are lifted
    /// in luminance so they hold contrast against a dark background.
    static let rpHealthy = Color(
        light: Color(red: 0.11, green: 0.47, blue: 0.28),
        dark: Color(red: 0.36, green: 0.80, blue: 0.53)
    )
    static let rpUnhealthy = Color(
        light: Color(red: 0.70, green: 0.13, blue: 0.13),
        dark: Color(red: 1.00, green: 0.45, blue: 0.42)
    )
    static let rpNeutral = Color(
        light: Color(red: 0.42, green: 0.45, blue: 0.50),
        dark: Color(red: 0.62, green: 0.65, blue: 0.70)
    )
    static let rpDisabled = Color(
        light: Color(red: 0.62, green: 0.64, blue: 0.67),
        dark: Color(red: 0.45, green: 0.47, blue: 0.51)
    )
    static let rpInFlight = Color(
        light: Color(red: 0.13, green: 0.40, blue: 0.75),
        dark: Color(red: 0.44, green: 0.68, blue: 1.00)
    )
    /// Used for gates and anything that says "you must do something first".
    static let rpGate = Color(
        light: Color(red: 0.55, green: 0.36, blue: 0.02),
        dark: Color(red: 0.95, green: 0.71, blue: 0.29)
    )
    /// Production. Deliberately distinct from `rpUnhealthy` so "this is
    /// production" never reads as "this is broken".
    static let rpProduction = Color(
        light: Color(red: 0.45, green: 0.16, blue: 0.55),
        dark: Color(red: 0.78, green: 0.55, blue: 0.95)
    )

    init(light: Color, dark: Color) {
        self = Color(
            uiColor: UIColor { traits in
                traits.userInterfaceStyle == .dark ? UIColor(dark) : UIColor(light)
            }
        )
    }
}

/// The colours an operator can tag a saved instance with, so "staging" and
/// "production" are never confused at a glance.
enum InstanceTint: String, CaseIterable, Codable, Hashable, Sendable, Identifiable {
    case red, orange, green, blue, purple, graphite

    var id: String { rawValue }

    var color: Color {
        switch self {
        case .red: .rpUnhealthy
        case .orange: .rpGate
        case .green: .rpHealthy
        case .blue: .rpInFlight
        case .purple: .rpProduction
        case .graphite: .rpNeutral
        }
    }

    var label: String { rawValue.capitalized }
}
