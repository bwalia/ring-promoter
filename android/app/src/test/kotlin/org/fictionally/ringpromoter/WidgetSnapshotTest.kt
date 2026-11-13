package org.fictionally.ringpromoter

import org.fictionally.ringpromoter.model.Ring
import org.fictionally.ringpromoter.model.RingStatus
import org.fictionally.ringpromoter.net.RpJson
import org.fictionally.ringpromoter.ui.overview.AppSummary
import org.fictionally.ringpromoter.widget.WidgetSnapshot
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/**
 * What may reach the home-screen widget.
 *
 * The rule, ported from the iOS suite: nothing resembling a credential or a
 * server address. A widget cannot prompt for a password or handle a 401, so
 * giving it either would add risk with nothing to show for it.
 */
class WidgetSnapshotTest {

    private fun ring(name: String, version: String, healthy: Boolean = true, configured: Boolean = true) =
        RingStatus(
            ring = Ring(name, name.replaceFirstChar { it.uppercase() }),
            configured = configured,
            currentVersion = version,
            liveVersion = version,
            healthy = healthy,
            liveHealthy = healthy,
            updatedAt = Instant.parse("2026-07-29T10:38:48Z"),
        )

    private fun snapshot() = WidgetSnapshot.of(
        instanceName = "Production",
        instanceTint = "red",
        isDemo = false,
        summaries = listOf(
            AppSummary(
                name = "web-frontend",
                title = "Web Frontend",
                rings = listOf(ring("int", "2.7.2"), ring("prod", "2.6.9")),
            ),
            AppSummary(
                name = "payments-api",
                title = "Payments API",
                rings = listOf(ring("int", "4.2.0"), ring("test", "9.0.0", healthy = false)),
            ),
        ),
        capturedAt = Instant.parse("2026-11-11T18:00:00Z"),
    )

    @Test
    fun `no token, URL or host reaches the snapshot`() {
        val encoded = RpJson.encodeToString(snapshot())
        listOf("token", "bearer", "http", "://", "password", "secret", "authorization")
            .forEach { needle ->
                assertFalse(
                    "the widget snapshot must not carry \"$needle\"",
                    encoded.lowercase().contains(needle),
                )
            }
    }

    @Test
    fun `the snapshot round-trips`() {
        val original = snapshot()
        val decoded = RpJson.decodeFromString<WidgetSnapshot>(RpJson.encodeToString(original))
        assertEquals(original, decoded)
        assertEquals(Instant.parse("2026-11-11T18:00:00Z"), decoded.capturedAt)
    }

    @Test
    fun `the app in trouble is the one featured`() {
        // The widget has room for one application, so it must be the one an
        // operator would want to see.
        assertEquals("payments-api", snapshot().featured?.name)
        assertEquals(1, snapshot().unhealthyCount)
    }

    @Test
    fun `a healthy fleet features the first app`() {
        val healthy = WidgetSnapshot.of(
            instanceName = "Staging",
            instanceTint = "blue",
            isDemo = true,
            summaries = listOf(
                AppSummary("a", "A", listOf(ring("int", "1.0"))),
                AppSummary("b", "B", listOf(ring("int", "1.0"))),
            ),
        )
        assertEquals("a", healthy.featured?.name)
        assertEquals(0, healthy.unhealthyCount)
        assertTrue(healthy.isDemo)
    }

    @Test
    fun `an empty fleet features nothing rather than guessing`() {
        val empty = WidgetSnapshot.of("Production", "red", false, emptyList())
        assertNull(empty.featured)
        assertEquals(0, empty.unhealthyCount)
    }

    @Test
    fun `an unconfigured or empty ring is never counted as unhealthy`() {
        val snapshot = WidgetSnapshot.of(
            instanceName = "Production",
            instanceTint = "red",
            isDemo = false,
            summaries = listOf(
                AppSummary(
                    "a",
                    "A",
                    listOf(
                        ring("int", "", healthy = false),
                        ring("prod", "1.0", healthy = false, configured = false),
                    ),
                ),
            ),
        )
        assertEquals(0, snapshot.unhealthyCount)
    }
}
