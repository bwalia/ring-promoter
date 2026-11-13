package org.fictionally.ringpromoter.ui.rings

import org.fictionally.ringpromoter.model.AppGroup
import org.fictionally.ringpromoter.model.RingStatus
import kotlin.math.PI
import kotlin.math.cos
import kotlin.math.floor
import kotlin.math.sin

/**
 * Descent — the Rings of Applications stage.
 *
 * The promotion rings **are** the orbits: the first ring in promotion order
 * (int) is the outermost circle and the last (prod) sits just above Earth,
 * which stands for live users. Every app owns one fixed spoke; the lit trail
 * on it shows how far the app's newest version has travelled toward Earth.
 *
 * Pure geometry on a 1000×1000 design canvas centred on (0, 0), +y down.
 *
 * THIS FILE IS A NUMBER-FOR-NUMBER PORT of `ios/.../DescentLayout.swift`, which
 * is itself a port of `web/src/lib/descent-layout.ts`. All three must stay in
 * lockstep: change a constant or a rule in one, change it in the others in the
 * same PR. An operator moving between the web console and either phone must see
 * the same stage.
 */
object DescentLayout {
    /** Design canvas edge; the stage scales it to fit. Origin is the centre. */
    const val DESIGN = 1000.0

    /** Earth's disc radius. */
    const val EARTH_R = 86.0

    /** Radius of the last (prod-most) ring. */
    const val RING_INNER = 132.0

    /** Radius of the first (int-most) ring. */
    const val RING_OUTER = 318.0

    /** Group arcs run just outside the outer ring. */
    const val GROUP_ARC_R = RING_OUTER + 16

    /** Where an app's name plate starts, measured from the centre. */
    const val LABEL_R = RING_OUTER + 32

    /** Radians left free at 12 o'clock for the ring name tags. */
    const val TOP_GAP = 0.34

    /** |cos(angle)| below this centres the label under or over its spoke. */
    const val LABEL_CENTER_BAND = 0.12

    /** Node radii: a deployed ring, and the dot for "nothing deployed". */
    const val NODE_R = 8.0
    const val EMPTY_R = 3.0

    /** Past this many spokes the labels crowd — fall back to lanes. */
    const val MAX_SPOKES = 36

    /**
     * Below this stage width (dp) the orbit is unreadable — lanes. 320 covers
     * the smallest phone still supported; the stage scales labels to fit.
     */
    const val MIN_WIDTH = 320.0

    /** Half-width of the amber gate bar drawn across a spoke. */
    const val GATE_HALF = 11.0

    /** One comet pass between two rings, seconds. */
    const val COMET_SECONDS = 1.6

    // Geometry

    /** Radius of the ring at [index] in promotion order (0 = first = outermost). */
    fun ringRadius(index: Int, count: Int): Double {
        if (count <= 1) return (RING_INNER + RING_OUTER) / 2
        val t = index.toDouble() / (count - 1).toDouble()
        return RING_OUTER + (RING_INNER - RING_OUTER) * t
    }

    /**
     * Angle of spoke [index] of [count], radians, clockwise from 3 o'clock
     * (+y points down). Spokes share the circle evenly except for the gap at
     * 12 o'clock that holds the ring tags.
     */
    fun spokeAngle(index: Int, count: Int): Double {
        val span = PI * 2 - TOP_GAP
        return -PI / 2 + TOP_GAP / 2 + (span * (index + 0.5)) / maxOf(1, count)
    }

    /** Half the angular width of one spoke's slice. */
    fun spokeHalfWidth(count: Int): Double = (PI * 2 - TOP_GAP) / maxOf(1, count) / 2

    /** A point on the design canvas, +y down. */
    data class Point(val x: Double, val y: Double)

    fun polar(angle: Double, r: Double): Point = Point(cos(angle) * r, sin(angle) * r)

    enum class LabelAnchor { START, MIDDLE, END }

    /** Labels read outward: right half left-aligned, left half right-aligned. */
    fun labelAnchor(angle: Double): LabelAnchor {
        val c = cos(angle)
        return when {
            c > LABEL_CENTER_BAND -> LabelAnchor.START
            c < -LABEL_CENTER_BAND -> LabelAnchor.END
            else -> LabelAnchor.MIDDLE
        }
    }

    /** True when the orbit layout is readable; otherwise render lanes. */
    fun descentFits(stageWidth: Double, spokes: Int): Boolean =
        stageWidth >= MIN_WIDTH && spokes <= MAX_SPOKES

    // Data → spoke state

    /**
     * One ring on one app's spoke.
     *
     * - [OFF]: this app does not use the ring (not configured) — nothing drawn.
     * - [EMPTY]: configured, nothing deployed — a small dot.
     * - [HEALTHY] / [FAILED]: deployed; the live health check passes or not.
     */
    enum class NodeState { OFF, EMPTY, HEALTHY, FAILED }

    data class SpokeNode(
        val ring: String,
        val label: String,
        val version: String?,
        val state: NodeState,
        /** Holds the app's newest version (solid) rather than an older one. */
        val fresh: Boolean,
        val ttfbMs: Int?,
        /** Why entry into this ring is blocked right now, if it is. */
        val gateClosed: String?,
    )

    data class Spoke(
        val nodes: List<SpokeNode>,
        /** Newest version anywhere on the spoke — the one travelling inward. */
        val newest: String?,
        /** Deepest ring index holding [newest]; -1 when nothing is deployed. */
        val frontier: Int,
        /** Ring index right after the frontier whose gate is closed; -1 if none. */
        val gateAt: Int,
    )

    /**
     * The live reason a ring cannot be entered, or null.
     *
     * Only the gates whose state can be observed without choosing a version. A
     * CR code and a sign-off are judged per action and per version, so they are
     * deliberately absent: this screen cannot honestly call them closed.
     */
    fun closedGate(ring: RingStatus?): String? {
        val gates = ring?.gates ?: return null
        if (gates.maintenanceWindow && !gates.maintenanceWindowOpen) {
            return "Maintenance window closed"
        }
        if (gates.grafana == true && gates.grafanaStatus?.verdict == "no_go") {
            return "Grafana no-go"
        }
        return null
    }

    /**
     * Build one app's spoke. [order] is the canonical promotion order
     * (int → … → prod) from `/api/apps`; rings the server did not report are
     * [NodeState.OFF].
     */
    fun buildSpoke(rings: List<RingStatus>?, order: List<String>): Spoke {
        val all = rings.orEmpty()
        val byName = all.associateBy { it.ring.name }
        val names = order.ifEmpty { all.map { it.ring.name } }

        val raw = names.map { name ->
            val view = byName[name]
            val configured = view?.configured ?: false
            val version = view?.currentVersion
                ?.takeIf { configured && it.isNotEmpty() }
            SpokeNode(
                ring = name,
                label = view?.ring?.label ?: name,
                version = version,
                state = when {
                    !configured -> NodeState.OFF
                    version == null -> NodeState.EMPTY
                    view?.liveHealthy == true -> NodeState.HEALTHY
                    else -> NodeState.FAILED
                },
                fresh = false,
                ttfbMs = view?.ttfbMs ?: view?.latencyMs,
                gateClosed = if (configured) closedGate(view) else null,
            )
        }

        // Versions enter at the first ring, so the outermost deployed version
        // is the newest one in flight.
        val newest = raw.firstOrNull { it.version != null }?.version
        var frontier = -1
        if (newest != null) {
            raw.forEachIndexed { index, node -> if (node.version == newest) frontier = index }
        }

        var gateAt = -1
        if (frontier >= 0) {
            var i = frontier + 1
            while (i < raw.size) {
                if (raw[i].state == NodeState.OFF) {
                    i += 1
                    continue
                }
                if (raw[i].gateClosed != null) gateAt = i
                break
            }
        }

        return Spoke(
            nodes = raw.map { it.copy(fresh = newest != null && it.version == newest) },
            newest = newest,
            frontier = frontier,
            gateAt = gateAt,
        )
    }

    /** Next ring the newest version would enter, skipping unused rings. */
    fun nextRingIndex(spoke: Spoke): Int {
        if (spoke.frontier < 0) return -1
        var i = spoke.frontier + 1
        while (i < spoke.nodes.size) {
            if (spoke.nodes[i].state != NodeState.OFF) return i
            i += 1
        }
        return -1
    }

    /** One-line reading of a spoke, for the detail card and screen readers. */
    fun spokeSentence(spoke: Spoke): String {
        if (spoke.frontier < 0) return "Nothing deployed yet"
        val at = spoke.nodes[spoke.frontier]
        val newest = spoke.newest.orEmpty()
        if (nextRingIndex(spoke) < 0) return "$newest is live in ${at.ring}"

        val hops = (spoke.frontier + 1 until spoke.nodes.size)
            .count { spoke.nodes[it].state != NodeState.OFF }
        var sentence = "$newest has reached ${at.ring}, $hops ring${if (hops == 1) "" else "s"} to go"
        if (spoke.gateAt >= 0) {
            val gated = spoke.nodes[spoke.gateAt]
            sentence += " · ${gated.ring} gate closed: ${gated.gateClosed.orEmpty()}"
        }
        return sentence
    }

    // Groups

    data class GroupSpan(val id: String, val name: String, val from: Int, val to: Int)

    /**
     * Order apps so each group's spokes sit together, groups in config order,
     * ungrouped apps last. An app in several groups goes with the first one.
     */
    fun orderByGroup(
        apps: List<String>,
        groups: List<AppGroup>,
    ): Pair<List<String>, List<GroupSpan>> {
        val known = apps.toSet()
        val placed = mutableSetOf<String>()
        val ordered = mutableListOf<String>()
        val spans = mutableListOf<GroupSpan>()

        for (group in groups) {
            val members = mutableListOf<String>()
            for (app in group.apps) {
                if (app in known && app !in placed && app !in members) members += app
            }
            if (members.isEmpty()) continue
            spans += GroupSpan(
                id = group.id,
                name = group.name,
                from = ordered.size,
                to = ordered.size + members.size - 1,
            )
            members.forEach {
                placed += it
                ordered += it
            }
        }
        apps.filterNot { it in placed }.forEach { ordered += it }
        return ordered to spans
    }

    /**
     * Start and end angles of a group span's arc, with a small gap at each end.
     */
    fun groupArcAngles(span: GroupSpan, count: Int): Pair<Double, Double> {
        val half = spokeHalfWidth(count) - 0.035
        return spokeAngle(span.from, count) - half to spokeAngle(span.to, count) + half
    }

    /**
     * Which spoke a direction from the centre falls in, or null inside the
     * 12 o'clock gap. The inverse of [spokeAngle].
     */
    fun spokeIndex(angle: Double, count: Int): Int? {
        if (count <= 0) return null
        val start = -PI / 2 + TOP_GAP / 2
        var rel = (angle - start) % (2 * PI)
        if (rel < 0) rel += 2 * PI
        val span = PI * 2 - TOP_GAP
        if (rel >= span) return null
        return minOf(count - 1, floor(rel / (span / count)).toInt())
    }
}
