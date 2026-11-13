package org.fictionally.ringpromoter

import org.fictionally.ringpromoter.model.AppGroup
import org.fictionally.ringpromoter.model.GrafanaVerdict
import org.fictionally.ringpromoter.model.Ring
import org.fictionally.ringpromoter.model.RingGates
import org.fictionally.ringpromoter.model.RingStatus
import org.fictionally.ringpromoter.net.RpJson
import org.fictionally.ringpromoter.ui.overview.AppSummary
import org.fictionally.ringpromoter.ui.rings.DescentApp
import org.fictionally.ringpromoter.ui.rings.DescentFrame
import org.fictionally.ringpromoter.ui.rings.DescentLayout
import org.fictionally.ringpromoter.ui.rings.EarthGeometry
import org.fictionally.ringpromoter.ui.rings.FleetStatus
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant
import kotlin.math.PI
import kotlin.math.abs
import kotlin.math.acos

/**
 * The Rings of Applications screen ("Descent"): layout maths, spoke state and
 * the status roll-up.
 *
 * Ported from the iOS `RingsUniverseTests`, which in turn pin the web console's
 * `descent-layout.ts`. These numbers are the contract between three clients: an
 * operator moving between the web console and either phone must see the same
 * stage, so a constant changed in one place and not the others fails here.
 */
class RingsUniverseTest {

    private val order = listOf("int", "test", "acc", "prod")

    private fun ring(
        name: String,
        version: String = "",
        healthy: Boolean = true,
        configured: Boolean = true,
        gates: RingGates = RingGates.NONE,
        latency: Int? = null,
        ttfb: Int? = null,
    ) = RingStatus(
        ring = Ring(name, name.replaceFirstChar { it.uppercase() }),
        configured = configured,
        currentVersion = version,
        liveVersion = version,
        healthy = healthy,
        liveHealthy = healthy,
        gates = gates,
        latencyMs = latency,
        ttfbMs = ttfb,
        updatedAt = Instant.parse("2026-07-29T10:38:48Z"),
    )

    private val fullyGated = RingGates(
        maintenanceWindow = true,
        qaSignoff = true,
        changeRequest = true,
        maintenanceWindowOpen = false,
    )

    private val openWindow = RingGates(maintenanceWindow = true, maintenanceWindowOpen = true)

    private val grafanaNoGo =
        RingGates(grafana = true, grafanaStatus = GrafanaVerdict(verdict = "no_go"))

    // Constants

    @Test
    fun `constants match the web's descent-layout`() {
        assertEquals(1000.0, DescentLayout.DESIGN, 0.0)
        assertEquals(86.0, DescentLayout.EARTH_R, 0.0)
        assertEquals(132.0, DescentLayout.RING_INNER, 0.0)
        assertEquals(318.0, DescentLayout.RING_OUTER, 0.0)
        assertEquals(334.0, DescentLayout.GROUP_ARC_R, 0.0)
        assertEquals(350.0, DescentLayout.LABEL_R, 0.0)
        assertEquals(0.34, DescentLayout.TOP_GAP, 0.0)
        assertEquals(0.12, DescentLayout.LABEL_CENTER_BAND, 0.0)
        assertEquals(8.0, DescentLayout.NODE_R, 0.0)
        assertEquals(3.0, DescentLayout.EMPTY_R, 0.0)
        assertEquals(36, DescentLayout.MAX_SPOKES)
        assertEquals(320.0, DescentLayout.MIN_WIDTH, 0.0)
        assertEquals(11.0, DescentLayout.GATE_HALF, 0.0)
        assertEquals(1.6, DescentLayout.COMET_SECONDS, 0.0)
    }

    // Geometry

    @Test
    fun `the first ring is outermost and the last sits just above Earth`() {
        assertEquals(DescentLayout.RING_OUTER, DescentLayout.ringRadius(0, 4), 1e-9)
        assertEquals(DescentLayout.RING_INNER, DescentLayout.ringRadius(3, 4), 1e-9)
        assertEquals(256.0, DescentLayout.ringRadius(1, 4), 1e-9)
        assertEquals(194.0, DescentLayout.ringRadius(2, 4), 1e-9)
        // A single ring sits midway.
        assertEquals(225.0, DescentLayout.ringRadius(0, 1), 1e-9)
        assertEquals(225.0, DescentLayout.ringRadius(0, 0), 1e-9)
        assertTrue(DescentLayout.RING_INNER > DescentLayout.EARTH_R)
    }

    @Test
    fun `spokes leave a symmetric gap at 12 o'clock`() {
        listOf(1, 2, 3, 7, 14, 36).forEach { count ->
            val first = DescentLayout.spokeAngle(0, count)
            val last = DescentLayout.spokeAngle(count - 1, count)
            val top = -PI / 2
            // Distance from 12 o'clock to the first spoke (clockwise) equals
            // the distance from the last spoke back to 12 o'clock.
            assertEquals(first - top, (top + 2 * PI) - last, 1e-9)
            assertTrue(first - top >= DescentLayout.TOP_GAP / 2)
            if (count > 1) {
                val step = DescentLayout.spokeAngle(1, count) - first
                assertEquals(2 * DescentLayout.spokeHalfWidth(count), step, 1e-9)
            }
        }
        // One spoke points straight down.
        assertEquals(PI / 2, DescentLayout.spokeAngle(0, 1), 1e-9)
    }

    @Test
    fun `spoke index inverts spoke angle and the gap hits nothing`() {
        listOf(1, 5, 36).forEach { count ->
            for (i in 0 until count) {
                val a = DescentLayout.spokeAngle(i, count)
                assertEquals(i, DescentLayout.spokeIndex(a, count))
                assertEquals(i, DescentLayout.spokeIndex(a + 2 * PI, count))
            }
            // Straight up is the ring tags' gap, and must select nothing.
            assertNull(DescentLayout.spokeIndex(-PI / 2, count))
        }
    }

    @Test
    fun `polar is clockwise from 3 o'clock with y pointing down`() {
        val p = DescentLayout.polar(PI / 2, 10.0)
        assertEquals(0.0, p.x, 1e-9)
        assertEquals(10.0, p.y, 1e-9)
    }

    @Test
    fun `labels read outward from the centre`() {
        assertEquals(DescentLayout.LabelAnchor.START, DescentLayout.labelAnchor(0.0))
        assertEquals(DescentLayout.LabelAnchor.END, DescentLayout.labelAnchor(PI))
        assertEquals(DescentLayout.LabelAnchor.MIDDLE, DescentLayout.labelAnchor(PI / 2))
        assertEquals(DescentLayout.LabelAnchor.MIDDLE, DescentLayout.labelAnchor(-PI / 2))
        val edge = acos(0.12)
        assertEquals(DescentLayout.LabelAnchor.MIDDLE, DescentLayout.labelAnchor(edge + 0.001))
        assertEquals(DescentLayout.LabelAnchor.START, DescentLayout.labelAnchor(edge - 0.001))
        assertEquals(DescentLayout.LabelAnchor.END, DescentLayout.labelAnchor(PI - edge + 0.001))
    }

    @Test
    fun `the orbit fits phone-width stages with at most 36 spokes`() {
        assertTrue(DescentLayout.descentFits(320.0, 36))
        assertTrue(DescentLayout.descentFits(390.0, 12))
        assertFalse(DescentLayout.descentFits(319.9, 5))
        assertFalse(DescentLayout.descentFits(1200.0, 37))
        assertTrue(DescentLayout.descentFits(874.0, 0))
    }

    // Spokes

    @Test
    fun `the frontier is the deepest ring holding the newest version`() {
        val spoke = DescentLayout.buildSpoke(
            listOf(
                ring("int", "2.3.1"), ring("test", "2.3.1"),
                ring("acc", "2.3.0"), ring("prod", "2.2.9"),
            ),
            order,
        )
        assertEquals("2.3.1", spoke.newest)
        assertEquals(1, spoke.frontier)
        assertEquals(-1, spoke.gateAt)
        assertEquals(listOf(true, true, false, false), spoke.nodes.map { it.fresh })
        assertTrue(spoke.nodes.all { it.state == DescentLayout.NodeState.HEALTHY })
        assertEquals(2, DescentLayout.nextRingIndex(spoke))
        assertEquals("2.3.1 has reached test, 2 rings to go", DescentLayout.spokeSentence(spoke))
    }

    @Test
    fun `newest is the outermost deployed version even when int is empty`() {
        val spoke = DescentLayout.buildSpoke(
            listOf(ring("int"), ring("test", "1.1"), ring("acc", "1.1"), ring("prod", "1.0")),
            order,
        )
        assertEquals("1.1", spoke.newest)
        assertEquals(2, spoke.frontier)
        assertEquals(DescentLayout.NodeState.EMPTY, spoke.nodes[0].state)
        assertFalse(spoke.nodes[0].fresh)
    }

    @Test
    fun `unconfigured and unreported rings are off, and failing health is failed`() {
        val spoke = DescentLayout.buildSpoke(
            listOf(
                ring("int", "3.0", healthy = false),
                ring("test", "2.9", configured = false),
            ),
            order,
        )
        assertEquals(
            listOf(
                DescentLayout.NodeState.FAILED,
                DescentLayout.NodeState.OFF,
                DescentLayout.NodeState.OFF,
                DescentLayout.NodeState.OFF,
            ),
            spoke.nodes.map { it.state },
        )
        // An unconfigured ring never shows a version, even when one is reported.
        assertNull(spoke.nodes[1].version)
        // Unreported rings fall back to their name.
        assertEquals("acc", spoke.nodes[2].label)
        assertEquals(0, spoke.frontier)
        assertEquals(-1, DescentLayout.nextRingIndex(spoke))
        assertEquals("3.0 is live in int", DescentLayout.spokeSentence(spoke))
    }

    @Test
    fun `nothing deployed gives frontier -1 and a plain sentence`() {
        val spoke = DescentLayout.buildSpoke(order.map { ring(it) }, order)
        assertNull(spoke.newest)
        assertEquals(-1, spoke.frontier)
        assertEquals(-1, spoke.gateAt)
        assertTrue(
            spoke.nodes.all { it.state == DescentLayout.NodeState.EMPTY && !it.fresh },
        )
        assertEquals(-1, DescentLayout.nextRingIndex(spoke))
        assertEquals("Nothing deployed yet", DescentLayout.spokeSentence(spoke))

        val none = DescentLayout.buildSpoke(null, emptyList())
        assertTrue(none.nodes.isEmpty())
        assertEquals(-1, none.frontier)
    }

    @Test
    fun `an empty order falls back to the reported rings`() {
        val spoke = DescentLayout.buildSpoke(listOf(ring("a", "1"), ring("b")), emptyList())
        assertEquals(listOf("a", "b"), spoke.nodes.map { it.ring })
        assertEquals(0, spoke.frontier)
    }

    @Test
    fun `a closed gate on the next used ring is reported, skipping off rings`() {
        val spoke = DescentLayout.buildSpoke(
            listOf(
                ring("int", "2.3.1"), ring("test", "2.3.1"),
                ring("acc", configured = false),
                ring("prod", "2.2.0", gates = fullyGated),
            ),
            order,
        )
        assertEquals(1, spoke.frontier)
        assertEquals(3, spoke.gateAt)
        assertEquals("Maintenance window closed", spoke.nodes[3].gateClosed)
        assertEquals(
            "2.3.1 has reached test, 1 ring to go · prod gate closed: Maintenance window closed",
            DescentLayout.spokeSentence(spoke),
        )
    }

    @Test
    fun `an open window does not block, a Grafana no-go does`() {
        val open = DescentLayout.buildSpoke(
            listOf(ring("int", "1"), ring("test", gates = openWindow)),
            emptyList(),
        )
        assertEquals(-1, open.gateAt)

        val grafana = DescentLayout.buildSpoke(
            listOf(ring("int", "1"), ring("test", gates = grafanaNoGo)),
            emptyList(),
        )
        assertEquals(1, grafana.gateAt)
        assertEquals("Grafana no-go", grafana.nodes[1].gateClosed)
    }

    @Test
    fun `only the ring right after the frontier counts as gated`() {
        val later = DescentLayout.buildSpoke(
            listOf(ring("int", "1"), ring("test"), ring("acc", gates = fullyGated)),
            emptyList(),
        )
        assertEquals(-1, later.gateAt)
        // A gate on a ring that was never reported is never claimed.
        assertNull(DescentLayout.closedGate(null))
    }

    @Test
    fun `TTFB prefers ttfb_ms and falls back to latency_ms`() {
        assertEquals(
            40,
            DescentLayout.buildSpoke(listOf(ring("int", "1", latency = 40)), emptyList())
                .nodes[0].ttfbMs,
        )
        assertEquals(
            12,
            DescentLayout.buildSpoke(
                listOf(ring("int", "1", latency = 40, ttfb = 12)),
                emptyList(),
            ).nodes[0].ttfbMs,
        )
    }

    // Groups

    @Test
    fun `groups sit contiguously in config order with ungrouped apps last`() {
        val groups = listOf(
            AppGroup("g1", "Core", listOf("c", "a", "ghost"), Instant.EPOCH),
            AppGroup("g2", "Edge", listOf("a", "e"), Instant.EPOCH),
            AppGroup("g3", "Empty", listOf("ghost"), Instant.EPOCH),
        )
        val (ordered, spans) =
            DescentLayout.orderByGroup(listOf("a", "b", "c", "d", "e"), groups)
        assertEquals(listOf("c", "a", "e", "b", "d"), ordered)
        assertEquals(
            listOf(
                DescentLayout.GroupSpan("g1", "Core", 0, 1),
                // "a" already went with Core, its first group.
                DescentLayout.GroupSpan("g2", "Edge", 2, 2),
            ),
            spans,
        )

        val (plain, none) = DescentLayout.orderByGroup(listOf("x", "y"), emptyList())
        assertEquals(listOf("x", "y"), plain)
        assertTrue(none.isEmpty())
    }

    @Test
    fun `a group arc spans its members' spokes with a small gap each end`() {
        val span = DescentLayout.GroupSpan("g", "G", 1, 3)
        val (a0, a1) = DescentLayout.groupArcAngles(span, 8)
        val half = DescentLayout.spokeHalfWidth(8) - 0.035
        assertEquals(DescentLayout.spokeAngle(1, 8) - half, a0, 1e-9)
        assertEquals(DescentLayout.spokeAngle(3, 8) + half, a1, 1e-9)
        assertTrue(a1 > a0)
    }

    @Test
    fun `the stage orders apps by group and reads each spoke`() {
        val rings = listOf(ring("int", "2.7.2"), ring("test"), ring("acc"), ring("prod"))
        val summaries = listOf("a", "b", "c").map {
            AppSummary(name = it, title = it.uppercase(), rings = rings)
        }
        val group = AppGroup("g1", "Core", listOf("c"), Instant.EPOCH)
        val (apps, spans) = DescentApp.ordered(summaries, listOf(group), order)
        assertEquals(listOf("c", "a", "b"), apps.map { it.id })
        assertEquals(1, spans.size)
        assertEquals("2.7.2 · int", apps[0].subtitle)
        assertEquals(FleetStatus.HEALTHY, apps[0].status)
    }

    // Status

    @Test
    fun `a running job shows as deploying whatever the health snapshot says`() {
        val troubled = listOf(ring("int", "1.0"), ring("test", "1.0", healthy = false))
        assertEquals(FleetStatus.DEPLOYING, FleetStatus.of(troubled, isBusy = true))
    }

    @Test
    fun `ring health rolls up to healthy, degraded, failed or empty`() {
        val healthy = listOf(ring("int", "1.0"), ring("test", "1.0"))
        val troubled = listOf(ring("int", "1.0"), ring("test", "1.0", healthy = false))
        assertEquals(FleetStatus.HEALTHY, FleetStatus.of(healthy, isBusy = false))
        assertEquals(FleetStatus.DEGRADED, FleetStatus.of(troubled, isBusy = false))
        assertEquals(
            FleetStatus.FAILED,
            FleetStatus.of(listOf(ring("int", "1.0", healthy = false)), isBusy = false),
        )
        assertEquals(FleetStatus.EMPTY, FleetStatus.of(listOf(ring("int")), isBusy = false))
        assertEquals(
            FleetStatus.LOADING,
            FleetStatus.of(emptyList(), isBusy = false, ringsUnknown = true),
        )
    }

    @Test
    fun `the aggregate is worst-first`() {
        assertEquals(
            FleetStatus.FAILED,
            FleetStatus.aggregate(
                listOf(FleetStatus.HEALTHY, FleetStatus.FAILED, FleetStatus.DEPLOYING),
            ),
        )
        assertEquals(
            FleetStatus.DEGRADED,
            FleetStatus.aggregate(listOf(FleetStatus.HEALTHY, FleetStatus.DEGRADED)),
        )
        assertEquals(
            FleetStatus.HEALTHY,
            FleetStatus.aggregate(listOf(FleetStatus.HEALTHY, FleetStatus.EMPTY)),
        )
        assertEquals(FleetStatus.EMPTY, FleetStatus.aggregate(emptyList()))
    }

    @Test
    fun `the grafana gate decodes and is read by the stage`() {
        val json = """
            {
              "ring": {"name": "prod", "label": "Production"},
              "configured": true, "current_version": "1.0", "previous_version": "",
              "live_version": "1.0", "healthy": true, "live_healthy": true,
              "latency_ms": 42, "auto_promote": false, "auto_promote_managed": false,
              "updated_at": "2026-07-29T10:38:47Z", "can_promote_from": true,
              "gates": {
                "maintenance_window": false, "qa_signoff": false,
                "change_request": false, "maintenance_window_open": false,
                "grafana": true,
                "grafana_status": {"verdict": "no_go", "checked_at": "2026-07-29T10:38:47Z"}
              }
            }
        """.trimIndent()
        val decoded = RpJson.decodeFromString<RingStatus>(json)
        assertEquals(42, decoded.latencyMs)
        assertEquals(true, decoded.gates.grafana)
        assertEquals("no_go", decoded.gates.grafanaStatus?.verdict)
        assertEquals("Grafana no-go", DescentLayout.closedGate(decoded))
    }

    // Earth

    @Test
    fun `a point at lat 0 lng 0 faces the camera at the disc centre`() {
        val p = EarthGeometry.projectOrtho(0.0, 0.0, 58.0, 0.0)
        assertEquals(0.0, p.x, 1e-3)
        assertEquals(0.0, p.y, 1e-3)
        assertTrue(p.front)
        assertEquals(58.0, p.z, 1e-3)
    }

    @Test
    fun `the north pole projects to the top of the disc`() {
        val p = EarthGeometry.projectOrtho(90.0, 0.0, 58.0, 0.0, centerX = 200.0, centerY = 200.0)
        assertEquals(200.0, p.x, 1e-3)
        assertEquals(142.0, p.y, 1e-3)
    }

    @Test
    fun `spin hides a point that started on the front`() {
        assertTrue(EarthGeometry.projectOrtho(0.0, 0.0, 58.0, 0.0).front)
        assertFalse(EarthGeometry.projectOrtho(0.0, 0.0, 58.0, PI).front)
    }

    @Test
    fun `Earth completes one revolution every 96 seconds`() {
        assertEquals(96.0, EarthGeometry.SPIN_PERIOD, 0.0)
        assertEquals(20.0 * 60, EarthGeometry.SPIN_PERIOD_REDUCED, 0.0)
        assertEquals(PI, EarthGeometry.spin(48.0, reduceMotion = false), 1e-3)
        assertEquals(0.0, EarthGeometry.spin(96.0, reduceMotion = false), 1e-3)
        // Reduced motion slows the globe rather than freezing it: a disc that
        // never moves stops reading as a sphere.
        assertEquals(0.0, EarthGeometry.spin(0.0, reduceMotion = true), 0.0)
        assertTrue(EarthGeometry.spin(96.0, reduceMotion = true) < 0.6)
    }

    // Stage fit

    @Test
    fun `the stage keeps the label ring inside the view`() {
        listOf(874.0 to 300.0, 1024.0 to 1200.0, 360.0 to 740.0).forEach { (w, h) ->
            val frame = DescentFrame.fit(w, h, topInset = 52.0, bottomInset = 44.0)
            val r = DescentLayout.LABEL_R * frame.scale
            assertTrue(frame.centerY - r >= 52.0 - 1e-9)
            assertTrue(frame.centerY + r <= h - 44.0 + 1e-9)
            assertTrue(frame.centerX - r >= -1e-9)
            assertTrue(frame.centerX + r <= w + 1e-9)
        }
    }

    @Test
    fun `the frame maps design coordinates into the view`() {
        val frame = DescentFrame.fit(1000.0, 1000.0, topInset = 0.0, bottomInset = 0.0)
        assertEquals(frame.centerX.toFloat(), frame.x(0.0), 1e-4f)
        assertEquals(frame.centerY.toFloat(), frame.y(0.0), 1e-4f)
        assertTrue(abs(frame.x(100.0) - frame.x(-100.0)) > 0f)
    }
}
