package org.fictionally.ringpromoter.ui.rings

import androidx.compose.animation.core.withInfiniteAnimationFrameMillis
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.wrapContentSize
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import org.fictionally.ringpromoter.ui.theme.RpPalette
import org.fictionally.ringpromoter.ui.theme.rpPalette
import kotlin.math.atan2
import kotlin.math.abs
import kotlin.math.cos
import kotlin.math.hypot
import kotlin.math.pow
import kotlin.math.sin

/**
 * The orbit: rings, spokes, trails, gates, group arcs, labels, Earth and the
 * comets.
 *
 * Everything static is one Canvas; only Earth and the comets redraw per frame,
 * and neither does when the device asks for reduced motion.
 */
@Composable
fun DescentStage(
    apps: List<DescentApp>,
    spans: List<DescentLayout.GroupSpan>,
    ringNames: List<String>,
    selectedId: String?,
    reduceMotion: Boolean,
    onSelect: (String) -> Unit,
    onDeselect: () -> Unit,
    onOpenGroup: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val palette = rpPalette
    val density = LocalDensity.current
    val ringCount = maxOf(1, ringNames.size)

    // One clock for the whole stage. `withInfiniteAnimationFrameMillis` stops
    // with the composition and is paused by the test framework, so nothing
    // here keeps a device awake.
    val elapsed by produceState(0.0, reduceMotion) {
        if (reduceMotion) return@produceState
        var start = 0L
        while (true) {
            withInfiniteAnimationFrameMillis { frameMillis ->
                if (start == 0L) start = frameMillis
                value = (frameMillis - start) / 1_000.0
            }
        }
    }

    BoxWithConstraints(
        modifier = modifier
            .fillMaxSize()
            .background(EarthPalette.space)
            .semantics {
                contentDescription = "Promotion rings ${ringNames.joinToString(", ")} " +
                    "around Earth, one spoke per application"
            },
    ) {
        // The fit is in dp; the Canvas gets the same frame scaled to pixels.
        val frame = DescentFrame.fit(
            width = maxWidth.value.toDouble(),
            height = maxHeight.value.toDouble(),
            topInset = 8.0,
            bottomInset = 8.0,
        )
        val framePx = frame.timesDensity(density.density)

        Canvas(
            Modifier
                .fillMaxSize()
                .pointerInput(apps, spans, framePx) {
                    detectTapGestures { at ->
                        handleTap(at, framePx, apps, spans, onSelect, onDeselect, onOpenGroup)
                    }
                },
        ) {
            drawStarField(size, 0.0)
            drawStatic(framePx, apps, spans, ringNames, ringCount, selectedId, palette)
            drawEarth(
                center = Offset(framePx.x(0.0), framePx.y(0.0)),
                radius = (DescentLayout.EARTH_R * framePx.scale).toFloat(),
                spin = EarthGeometry.spin(elapsed, reduceMotion),
            )
            if (!reduceMotion) {
                drawComets(framePx, apps, ringCount, selectedId, elapsed, palette)
            }
        }

        // Ring name tags in the 12 o'clock gap, and one name plate per spoke.
        // Text is a composable rather than canvas drawing so it scales with the
        // operator's font size setting.
        ringNames.forEachIndexed { index, name ->
            val r = DescentLayout.ringRadius(index, ringCount)
            StageText(x = frame.x(0.0).dp, y = frame.y(-r).dp, align = Alignment.Center) {
                Text(
                    name.uppercase(),
                    color = Color.White.copy(alpha = 0.55f),
                    fontSize = 9.sp,
                    fontFamily = FontFamily.Monospace,
                    fontWeight = FontWeight.Medium,
                    modifier = Modifier
                        .background(EarthPalette.space, RoundedCornerShape(3.dp))
                        .padding(horizontal = 4.dp),
                )
            }
        }

        apps.forEachIndexed { index, app ->
            val angle = DescentLayout.spokeAngle(index, apps.size)
            val anchor = DescentLayout.labelAnchor(angle)
            val point = DescentLayout.polar(angle, DescentLayout.LABEL_R)
            val dimmed = selectedId != null && selectedId != app.id
            StageText(
                x = frame.x(point.x).dp,
                y = frame.y(point.y).dp,
                align = when (anchor) {
                    DescentLayout.LabelAnchor.START -> Alignment.CenterStart
                    DescentLayout.LabelAnchor.END -> Alignment.CenterEnd
                    // Near 12 and 6 o'clock the label centres on its spoke, so
                    // it has to sit clear of it: below the point at the bottom
                    // of the stage, above it at the top.
                    DescentLayout.LabelAnchor.MIDDLE ->
                        if (sin(angle) > 0) Alignment.TopCenter else Alignment.BottomCenter
                },
            ) {
                SpokeLabel(
                    app = app,
                    selected = selectedId == app.id,
                    dimmed = dimmed,
                    onTap = { onSelect(app.id) },
                )
            }
        }
    }
}

/**
 * Places [content] at a point on the stage, aligned the way a label at that
 * angle should read: outward from the centre.
 *
 * A zero-sized box at the point, with the content allowed to overflow it in
 * the chosen direction. Offsetting the content itself would anchor every label
 * by its top-left corner, which puts the left half of the stage's names on top
 * of the rings they belong to.
 */
@Composable
private fun androidx.compose.foundation.layout.BoxScope.StageText(
    x: androidx.compose.ui.unit.Dp,
    y: androidx.compose.ui.unit.Dp,
    align: Alignment,
    content: @Composable () -> Unit,
) {
    Box(
        Modifier
            .align(Alignment.TopStart)
            .offset(x = x, y = y)
            .size(0.dp),
    ) {
        Box(Modifier.wrapContentSize(align = align, unbounded = true)) { content() }
    }
}

/** The name plate at the end of a spoke: app name, then "version · ring". */
@Composable
private fun SpokeLabel(
    app: DescentApp,
    selected: Boolean,
    dimmed: Boolean,
    onTap: () -> Unit,
) {
    val alpha = if (dimmed) 0.22f else 1f
    Column(
        Modifier
            .pointerInput(app.id) { detectTapGestures { onTap() } }
            .padding(horizontal = 3.dp, vertical = 2.dp)
            .semantics {
                contentDescription = "${app.title}: ${app.status.label}. ${app.sentence}"
            },
    ) {
        Text(
            clip(app.title),
            color = Color.White.copy(alpha = (if (selected) 1f else 0.88f) * alpha),
            fontSize = 11.sp,
            fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Medium,
            maxLines = 1,
        )
        Text(
            app.subtitle,
            color = Color.White.copy(alpha = 0.5f * alpha),
            fontSize = 9.sp,
            fontFamily = FontFamily.Monospace,
            maxLines = 1,
        )
    }
}

/** Keep labels from running off the stage; the card has the full name. */
private fun clip(text: String): String =
    if (text.length > 24) text.take(23) + "…" else text

// Drawing

private fun DrawScope.drawStatic(
    frame: DescentFrame,
    apps: List<DescentApp>,
    spans: List<DescentLayout.GroupSpan>,
    ringNames: List<String>,
    ringCount: Int,
    selectedId: String?,
    palette: RpPalette,
) {
    val s = frame.scale
    val center = Offset(frame.x(0.0), frame.y(0.0))

    // The orbits. Prod (last) a touch brighter: it is the one that matters.
    for (i in 0 until ringCount) {
        val r = (DescentLayout.ringRadius(i, ringCount) * s).toFloat()
        drawCircle(
            color = Color.White.copy(alpha = if (i == ringCount - 1) 0.22f else 0.13f),
            radius = r,
            center = center,
            style = Stroke(width = 1f),
        )
    }

    // Group arcs just outside the outer ring.
    val arcR = (DescentLayout.GROUP_ARC_R * s).toFloat()
    spans.forEachIndexed { i, span ->
        val (a0, a1) = DescentLayout.groupArcAngles(span, apps.size)
        drawArc(
            color = groupColour(i, palette).copy(alpha = 0.55f),
            startAngle = Math.toDegrees(a0).toFloat(),
            sweepAngle = Math.toDegrees(a1 - a0).toFloat(),
            useCenter = false,
            topLeft = Offset(center.x - arcR, center.y - arcR),
            size = androidx.compose.ui.geometry.Size(arcR * 2, arcR * 2),
            style = Stroke(width = maxOf(2f * s.toFloat(), 1.6f), cap = StrokeCap.Round),
        )
    }

    apps.forEachIndexed { index, app ->
        val dim = if (selectedId != null && selectedId != app.id) 0.22f else 1f
        drawSpoke(app, index, apps.size, ringCount, frame, selectedId == app.id, dim, palette)
    }
}

private fun DrawScope.drawSpoke(
    app: DescentApp,
    index: Int,
    count: Int,
    ringCount: Int,
    frame: DescentFrame,
    selected: Boolean,
    dim: Float,
    palette: RpPalette,
) {
    val s = frame.scale.toFloat()
    val angle = DescentLayout.spokeAngle(index, count)
    val spoke = app.spoke
    val outer = DescentLayout.ringRadius(0, ringCount)

    fun at(r: Double): Offset {
        val p = DescentLayout.polar(angle, r)
        return Offset(frame.x(p.x), frame.y(p.y))
    }

    // The rail every version travels down.
    drawLine(
        color = Color.White.copy(alpha = (if (selected) 0.35f else 0.12f) * dim),
        start = at(outer),
        end = at(DescentLayout.EARTH_R + 8),
        strokeWidth = maxOf(1.2f * s, 0.8f),
    )

    // The lit trail: outer ring → frontier. How far this app has travelled.
    if (spoke.frontier >= 0) {
        val tint = if (app.status == FleetStatus.EMPTY) {
            palette.healthy
        } else {
            app.status.tint(palette)
        }
        drawLine(
            color = tint.copy(alpha = 0.85f * dim),
            start = at(outer),
            end = at(DescentLayout.ringRadius(spoke.frontier, ringCount)),
            strokeWidth = maxOf(3f * s, 2.5f),
            cap = StrokeCap.Round,
        )
    }

    // A closed gate: an amber bar across the spoke, midway to that ring.
    if (spoke.gateAt >= 0 && spoke.frontier >= 0) {
        val mid = (
            DescentLayout.ringRadius(spoke.frontier, ringCount) +
                DescentLayout.ringRadius(spoke.gateAt, ringCount)
            ) / 2
        val m = DescentLayout.polar(angle, mid)
        val half = maxOf(DescentLayout.GATE_HALF, 6.0 / frame.scale)
        val px = -sin(angle) * half
        val py = cos(angle) * half
        drawLine(
            color = palette.gate.copy(alpha = dim),
            start = Offset(frame.x(m.x - px), frame.y(m.y - py)),
            end = Offset(frame.x(m.x + px), frame.y(m.y + py)),
            strokeWidth = maxOf(3f * s, 2.5f),
            cap = StrokeCap.Round,
        )
    }

    // Nodes where the spoke crosses each ring.
    spoke.nodes.forEachIndexed { i, node ->
        if (node.state == DescentLayout.NodeState.OFF) return@forEachIndexed
        val c = at(DescentLayout.ringRadius(i, ringCount))
        if (node.state == DescentLayout.NodeState.EMPTY) {
            drawCircle(
                color = palette.disabled.copy(alpha = dim),
                radius = maxOf(DescentLayout.EMPTY_R.toFloat() * s, 1.8f),
                center = c,
            )
            return@forEachIndexed
        }
        val tint = DescentApp.tint(node.state, palette).copy(alpha = dim)
        val r = maxOf(DescentLayout.NODE_R.toFloat() * s, 4.5f)
        if (node.fresh) {
            // Solid: this ring holds the newest version.
            drawCircle(color = tint, radius = r, center = c)
        } else {
            drawCircle(color = EarthPalette.space, radius = r, center = c)
            val w = maxOf(2.5f * s, 1.5f)
            drawCircle(color = tint, radius = r - w / 2, center = c, style = Stroke(width = w))
        }
    }
}

/**
 * A dot with a short tail flying from the frontier ring toward the next,
 * looping, for every app with a job in flight.
 */
private fun DrawScope.drawComets(
    frame: DescentFrame,
    apps: List<DescentApp>,
    ringCount: Int,
    selectedId: String?,
    elapsed: Double,
    palette: RpPalette,
) {
    val s = frame.scale.toFloat()
    val p = (elapsed / DescentLayout.COMET_SECONDS) % 1.0
    // Ease in and out, so it reads as a thing with mass rather than a timer.
    val e = if (p < 0.5) 2 * p * p else 1 - (-2 * p + 2).pow(2) / 2

    apps.forEachIndexed { index, app ->
        if (!app.isBusy) return@forEachIndexed
        val next = DescentLayout.nextRingIndex(app.spoke)
        if (next < 0) return@forEachIndexed
        val dim = if (selectedId != null && selectedId != app.id) 0.22f else 1f
        val angle = DescentLayout.spokeAngle(index, apps.size)
        val r0 = DescentLayout.ringRadius(app.spoke.frontier, ringCount)
        val r1 = DescentLayout.ringRadius(next, ringCount)
        val rr = r0 + (r1 - r0) * e
        val rt = minOf(r0, rr + 34)

        fun at(r: Double): Offset {
            val point = DescentLayout.polar(angle, r)
            return Offset(frame.x(point.x), frame.y(point.y))
        }

        val head = at(rr)
        drawLine(
            color = palette.inFlight.copy(alpha = 0.6f * dim),
            start = at(rt),
            end = head,
            strokeWidth = maxOf(3f * s, 2f),
            cap = StrokeCap.Round,
        )
        val hr = maxOf(6f * s, 3.5f)
        drawCircle(
            color = palette.inFlight.copy(alpha = (0.35 * (1 - p)).toFloat() * dim),
            radius = hr * (1 + p * 1.6f).toFloat(),
            center = head,
        )
        drawCircle(color = Color.White.copy(alpha = 0.9f * dim), radius = hr, center = head)
    }
}

/**
 * Turn a tap into a selection.
 *
 * The group arc is checked first and with a tight tolerance, because it sits
 * right next to the labels; everything between Earth and the label ring selects
 * the spoke it falls in, and anything else clears the selection.
 */
private fun handleTap(
    at: Offset,
    frame: DescentFrame,
    apps: List<DescentApp>,
    spans: List<DescentLayout.GroupSpan>,
    onSelect: (String) -> Unit,
    onDeselect: () -> Unit,
    onOpenGroup: (String) -> Unit,
) {
    val dx = (at.x - frame.centerX) / frame.scale
    val dy = (at.y - frame.centerY) / frame.scale
    val r = hypot(dx, dy)
    val index = DescentLayout.spokeIndex(atan2(dy, dx), apps.size)
    val slop = maxOf(10.0, 12.0 / frame.scale)

    if (abs(r - DescentLayout.GROUP_ARC_R) <= slop / 2 + 4 && index != null) {
        val span = spans.firstOrNull { index >= it.from && index <= it.to }
        if (span != null) {
            onOpenGroup(span.id)
            return
        }
    }
    if (r >= DescentLayout.EARTH_R && r <= DescentLayout.LABEL_R && index != null) {
        onSelect(apps[index].id)
        return
    }
    onDeselect()
}
