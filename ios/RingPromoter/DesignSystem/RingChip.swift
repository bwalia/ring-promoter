import SwiftUI

/// One ring in a pipeline strip: the smallest unit of situational awareness.
///
/// Sized so four fit across a phone at the default text size, and so it still
/// works at the largest Dynamic Type setting — where the strip reflows to a
/// vertical list rather than shrinking the text.
struct RingChip: View {
    let ring: RingStatus
    var isBusy: Bool = false
    var isProduction: Bool = false
    /// Compact drops the version line — used inside dense lists and the widget.
    var isCompact: Bool = false

    @ScaledMetric(relativeTo: .caption) private var iconSize: CGFloat = 13

    private var presentation: HealthPresentation {
        HealthPresentation(ring, isBusy: isBusy)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            HStack(spacing: 4) {
                // The in-flight ring gets a turning glyph; everything else gets
                // its static state icon.
                Group {
                    if isBusy {
                        SpinningGlyph()
                    } else {
                        Image(systemName: presentation.systemImage)
                    }
                }
                .font(.system(size: iconSize, weight: .semibold))
                .foregroundStyle(presentation.tint)
                // The ring name is four characters at most and is the chip's
                // whole identity, so it never truncates — the version below is
                // what gives way when space is tight.
                Text(ring.ring.name.uppercased())
                    .font(.caption2.weight(.bold))
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .fixedSize()
                if isProduction {
                    Image(systemName: "lock.fill")
                        .font(.system(size: iconSize * 0.72))
                        .foregroundStyle(Color.rpProduction)
                        .accessibilityHidden(true)
                }
            }
            if !isCompact {
                Text(versionText)
                    .font(.caption.weight(.medium))
                    .monospacedDigit()
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .foregroundStyle(ring.isEmpty ? .secondary : .primary)
            }
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 6)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(presentation.background, in: .rect(cornerRadius: 8))
        .overlay {
            if !isBusy {
                RoundedRectangle(cornerRadius: 8)
                    .strokeBorder(presentation.tint.opacity(ring.needsAttention ? 0.55 : 0.18))
            }
        }
        .inFlight(isBusy, cornerRadius: 8)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(accessibilityLabel)
    }

    /// An em dash rather than "not deployed": at chip width that phrase
    /// truncates to "not…yed", which reads as noise. The icon and the
    /// VoiceOver label carry the meaning instead.
    private var versionText: String {
        ring.configured && !ring.isEmpty ? ring.currentVersion : "—"
    }

    private var accessibilityLabel: String {
        var parts = [ring.ring.label]
        if !ring.configured {
            parts.append("not part of this pipeline")
            return parts.joined(separator: ", ")
        }
        parts.append(ring.isEmpty ? "nothing deployed" : "version \(ring.currentVersion)")
        parts.append(presentation.label)
        if isProduction { parts.append("production ring") }
        if ring.autoPromote { parts.append("auto-promote on") }
        if ring.gates.isGated { parts.append("gated") }
        return parts.joined(separator: ", ")
    }
}

/// The four-ring strip shown per app on the Overview.
///
/// Reflows to a vertical stack at accessibility text sizes instead of
/// compressing four chips into an unreadable row.
struct PipelineStrip: View {
    let rings: [RingStatus]
    var pipeline: RingPipeline = .empty
    var busyRing: String?
    var isCompact: Bool = false

    @Environment(\.dynamicTypeSize) private var typeSize

    var body: some View {
        let layout = typeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 4))
            : AnyLayout(HStackLayout(alignment: .top, spacing: 5))

        layout {
            ForEach(rings) { ring in
                RingChip(
                    ring: ring,
                    isBusy: busyRing == ring.ring.name,
                    isProduction: pipeline.isProduction(ring.ring.name),
                    isCompact: isCompact
                )
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel("Ring pipeline")
    }
}

#Preview("Pipeline strip", traits: .sizeThatFitsLayout) {
    VStack(spacing: 16) {
        PipelineStrip(rings: PreviewData.healthyRings, pipeline: PreviewData.pipeline)
        PipelineStrip(rings: PreviewData.troubledRings, pipeline: PreviewData.pipeline)
        PipelineStrip(
            rings: PreviewData.healthyRings, pipeline: PreviewData.pipeline, busyRing: "test"
        )
    }
    .padding()
}
