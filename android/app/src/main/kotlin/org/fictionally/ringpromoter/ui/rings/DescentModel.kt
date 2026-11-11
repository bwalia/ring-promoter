package org.fictionally.ringpromoter.ui.rings

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Autorenew
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Error
import androidx.compose.material.icons.filled.MoreHoriz
import androidx.compose.material.icons.filled.RadioButtonUnchecked
import androidx.compose.material.icons.filled.Warning
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import org.fictionally.ringpromoter.model.AppGroup
import org.fictionally.ringpromoter.model.RingStatus
import org.fictionally.ringpromoter.ui.overview.AppSummary
import org.fictionally.ringpromoter.ui.theme.RpPalette

/**
 * The health of one application on the Rings of Applications screen.
 *
 * Mirrors the web console's fleet view exactly, so an operator moving between
 * the two sees the same verdicts: [DEPLOYING] overrides everything because a
 * running job makes the health snapshot momentarily untrustworthy, and the
 * aggregate is worst-first because one failing service is what the screen
 * exists to surface.
 */
enum class FleetStatus {
    HEALTHY,
    DEPLOYING,
    DEGRADED,
    FAILED,
    EMPTY,
    LOADING,
    ;

    val label: String
        get() = when (this) {
            HEALTHY -> "Healthy"
            DEPLOYING -> "Deploying"
            DEGRADED -> "Degraded"
            FAILED -> "Failing"
            EMPTY -> "No version"
            LOADING -> "Checking…"
        }

    /** Colour is never the only signal — every status also carries an icon. */
    val icon: ImageVector
        get() = when (this) {
            HEALTHY -> Icons.Filled.CheckCircle
            DEPLOYING -> Icons.Filled.Autorenew
            DEGRADED -> Icons.Filled.Error
            FAILED -> Icons.Filled.Warning
            EMPTY -> Icons.Filled.RadioButtonUnchecked
            LOADING -> Icons.Filled.MoreHoriz
        }

    /** Design-system colours, so the stage agrees with every other screen. */
    fun tint(palette: RpPalette): Color = when (this) {
        HEALTHY -> palette.healthy
        DEPLOYING -> palette.inFlight
        DEGRADED -> palette.gate
        FAILED -> palette.unhealthy
        EMPTY -> palette.neutral
        LOADING -> palette.disabled
    }

    companion object {
        /** Worst-first, matching the web console's aggregation order. */
        private val AGGREGATE_PRIORITY =
            listOf(FAILED, DEGRADED, DEPLOYING, LOADING, HEALTHY, EMPTY)

        /** The overall verdict for a set of applications. */
        fun aggregate(statuses: List<FleetStatus>): FleetStatus =
            AGGREGATE_PRIORITY.firstOrNull { it in statuses } ?: EMPTY

        /**
         * Health of one application from its rings and job state.
         *
         * @param isBusy a job is running right now — shown as deploying
         *   whatever the (soon to be stale) health snapshot says.
         * @param ringsUnknown the rings request failed and nothing is cached,
         *   so no verdict can honestly be given.
         */
        fun of(
            rings: List<RingStatus>,
            isBusy: Boolean,
            ringsUnknown: Boolean = false,
        ): FleetStatus {
            if (isBusy) return DEPLOYING
            if (ringsUnknown) return LOADING
            val active = rings.filter { it.configured && !it.isEmpty }
            if (active.isEmpty()) return EMPTY
            val healthy = active.count { it.isHealthy }
            return when {
                healthy == active.size -> HEALTHY
                healthy == 0 -> FAILED
                else -> DEGRADED
            }
        }
    }
}

/** Group arcs and lane headers cycle through existing design tokens — never new colours. */
internal fun groupColour(index: Int, palette: RpPalette): Color =
    listOf(palette.inFlight, palette.production, palette.neutral)[index % 3]

/**
 * One application as the stage and the lanes draw it: its spoke (per-ring state
 * along the promotion order) plus the facts the detail card needs.
 *
 * Built once per refresh so the views stay pure functions of data.
 */
data class DescentApp(
    val id: String,
    val title: String,
    val status: FleetStatus,
    val spoke: DescentLayout.Spoke,
    /** A job is running for this app — the comet flies. */
    val isBusy: Boolean,
    val busyAction: String?,
    val loadError: String?,
) {
    val sentence: String get() = DescentLayout.spokeSentence(spoke)

    /** "2.3.1 · acc", or "no version" — the second line under the name. */
    val subtitle: String
        get() {
            val newest = spoke.newest
            if (newest == null || spoke.frontier < 0) return "no version"
            return "$newest · ${spoke.nodes[spoke.frontier].ring}"
        }

    companion object {
        fun of(summary: AppSummary, order: List<String>): DescentApp = DescentApp(
            id = summary.name,
            title = summary.title,
            status = FleetStatus.of(
                rings = summary.rings,
                isBusy = summary.isBusy,
                ringsUnknown = summary.rings.isEmpty() && summary.loadError != null,
            ),
            spoke = DescentLayout.buildSpoke(summary.rings, order),
            isBusy = summary.isBusy,
            busyAction = summary.busyAction,
            loadError = summary.loadError,
        )

        /** Colour of one node, by its ring's live health. */
        fun tint(state: DescentLayout.NodeState, palette: RpPalette): Color = when (state) {
            DescentLayout.NodeState.HEALTHY -> palette.healthy
            DescentLayout.NodeState.FAILED -> palette.unhealthy
            DescentLayout.NodeState.EMPTY, DescentLayout.NodeState.OFF -> palette.disabled
        }

        /** Ordered for the stage: grouped apps contiguous, ungrouped last. */
        fun ordered(
            summaries: List<AppSummary>,
            groups: List<AppGroup>,
            order: List<String>,
        ): Pair<List<DescentApp>, List<DescentLayout.GroupSpan>> {
            val byName = summaries.associateBy { it.name }
            val (names, spans) = DescentLayout.orderByGroup(summaries.map { it.name }, groups)
            return names.mapNotNull { byName[it]?.let { s -> of(s, order) } } to spans
        }
    }
}

/**
 * Where the design canvas lands inside the view.
 *
 * Kept as plain numbers rather than a Compose type so the fit maths is tested
 * on the JVM: a stage that puts its labels off screen is the failure that costs
 * the whole screen its point.
 *
 * The maths is in **dp**, as the iOS port is in points, because the label
 * reserves below are sizes a person perceives rather than pixel counts. The
 * stage scales a copy by the display density for drawing, which is the only
 * place pixels are wanted.
 */
data class DescentFrame(val centerX: Double, val centerY: Double, val scale: Double) {

    /** The same frame in pixels, for a Canvas. */
    fun timesDensity(density: Float): DescentFrame = DescentFrame(
        centerX = centerX * density,
        centerY = centerY * density,
        scale = scale * density,
    )

    fun x(designX: Double): Float = (centerX + designX * scale).toFloat()

    fun y(designY: Double): Float = (centerY + designY * scale).toFloat()

    companion object {
        /**
         * Fit the rings plus label room into the view, leaving the chrome
         * insets clear. On wide stages labels reserve ~150dp beside the circle;
         * on phones the side reserve shrinks so the rings stay readable.
         */
        fun fit(
            width: Double,
            height: Double,
            topInset: Double,
            bottomInset: Double,
        ): DescentFrame {
            val labelSide = minOf(150.0, maxOf(48.0, width * 0.22))
            val labelBelow = 36.0
            val labelAbove = 12.0
            val r = DescentLayout.LABEL_R
            val availableHeight = maxOf(1.0, height - topInset - bottomInset)
            val scale = maxOf(
                0.12,
                minOf(
                    width / DescentLayout.DESIGN,
                    (width - 2 * labelSide) / (2 * r),
                    (availableHeight - labelBelow - labelAbove) / (2 * r),
                ),
            )
            val used = 2 * r * scale + labelAbove + labelBelow
            val centerY =
                topInset + maxOf(0.0, (availableHeight - used) / 2) + labelAbove + r * scale
            return DescentFrame(centerX = width / 2, centerY = centerY, scale = scale)
        }
    }
}
