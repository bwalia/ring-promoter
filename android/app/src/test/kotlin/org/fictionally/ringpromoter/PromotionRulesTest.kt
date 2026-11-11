package org.fictionally.ringpromoter

import org.fictionally.ringpromoter.model.AppsResponse
import org.fictionally.ringpromoter.model.PromotionLegality
import org.fictionally.ringpromoter.model.PromotionRules
import org.fictionally.ringpromoter.model.Ring
import org.fictionally.ringpromoter.model.RingGates
import org.fictionally.ringpromoter.model.RingPipeline
import org.fictionally.ringpromoter.model.RingStatus
import org.fictionally.ringpromoter.model.RingsResponse
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/**
 * The rules that decide what the UI may offer.
 *
 * Ported from the iOS `PromotionRulesTests`. The whole point of
 * [PromotionRules] is that an operator is never invited to do something the
 * server would refuse, so these are the tests that matter most.
 */
class PromotionRulesTest {

    private val pipeline = Fixtures.decode<AppsResponse>("apps").pipeline

    private fun ring(
        name: String,
        configured: Boolean = true,
        current: String = "v1",
        previous: String = "",
        liveHealthy: Boolean = true,
        canPromoteFrom: Boolean = true,
        gates: RingGates = RingGates.NONE,
        autoPromoteManaged: Boolean = false,
    ) = RingStatus(
        ring = Ring(name, name.replaceFirstChar { it.uppercase() }),
        configured = configured,
        currentVersion = current,
        previousVersion = previous,
        liveVersion = current,
        healthy = liveHealthy,
        liveHealthy = liveHealthy,
        canPromoteFrom = canPromoteFrom,
        gates = gates,
        autoPromoteManaged = autoPromoteManaged,
        updatedAt = Instant.parse("2026-07-29T10:38:48Z"),
    )

    private fun rules(prodProtected: Boolean = true) =
        PromotionRules(pipeline, prodProtected = prodProtected)

    // MARK: promote

    @Test
    fun `promoting out of a healthy ring is allowed and targets the next ring`() {
        val source = ring("int")
        val legality = rules().promote(source, listOf(source, ring("test")))
        assertTrue(legality.isAllowed)
        assertEquals("test", legality.targetRing)
        assertTrue(legality.requirements.isEmpty)
    }

    @Test
    fun `promoting out of the last ring is blocked`() {
        val source = ring("prod")
        val legality = rules().promote(source, listOf(source))
        assertFalse(legality.isAllowed)
        assertEquals(PromotionLegality.Blocker.NoNextRing, legality.blocker)
    }

    @Test
    fun `promoting out of an empty ring is blocked but still names the target`() {
        val source = ring("int", current = "")
        val legality = rules().promote(source, listOf(source))
        assertFalse(legality.isAllowed)
        assertEquals(PromotionLegality.Blocker.SourceRingEmpty, legality.blocker)
        assertEquals("test", legality.targetRing)
    }

    @Test
    fun `an unhealthy source the server refuses is explained as unhealthy`() {
        val source = ring("int", liveHealthy = false, canPromoteFrom = false)
        val legality = rules().promote(source, listOf(source))
        assertFalse(legality.isAllowed)
        assertEquals(PromotionLegality.Blocker.SourceRingUnhealthy, legality.blocker)
    }

    @Test
    fun `an unconfigured ring is not part of the pipeline`() {
        val source = ring("int", configured = false)
        val legality = rules().promote(source, listOf(source))
        assertFalse(legality.isAllowed)
        assertEquals(PromotionLegality.Blocker.RingNotConfigured, legality.blocker)
    }

    @Test
    fun `promoting into production demands the production password`() {
        val source = ring("acc")
        val legality = rules().promote(source, listOf(source, ring("prod")))
        assertTrue(legality.isAllowed)
        assertEquals("prod", legality.targetRing)
        assertTrue(legality.requirements.productionPassword)
    }

    @Test
    fun `an unprotected server never demands a production password`() {
        val source = ring("acc")
        val legality = rules(prodProtected = false)
            .promote(source, listOf(source, ring("prod")))
        assertTrue(legality.isAllowed)
        assertFalse(legality.requirements.productionPassword)
    }

    @Test
    fun `the target ring's gates become the sheet's requirements`() {
        val source = ring("int")
        val target = ring(
            "test",
            gates = RingGates(
                maintenanceWindow = true,
                qaSignoff = true,
                changeRequest = true,
                changeRequestProvider = "jira",
                maintenanceWindowOpen = false,
            ),
        )
        val legality = rules().promote(source, listOf(source, target))
        assertTrue(legality.isAllowed)
        with(legality.requirements) {
            assertTrue(changeRequestCode)
            assertEquals("jira", changeRequestProvider)
            assertTrue(qaSignoff)
            assertTrue(maintenanceWindow)
            assertFalse(maintenanceWindowOpen)
            assertFalse("test is not the last ring", productionPassword)
        }
    }

    @Test
    fun `gates come from the target ring, not the source`() {
        val source = ring("int", gates = RingGates(changeRequest = true))
        val target = ring("test")
        val legality = rules().promote(source, listOf(source, target))
        assertFalse(
            "the source's own gates guard entry into the source, not out of it",
            legality.requirements.changeRequestCode,
        )
    }

    // MARK: seed

    @Test
    fun `seeding any configured ring is allowed`() {
        val legality = rules().seed(ring("test"))
        assertTrue(legality.isAllowed)
        assertEquals("test", legality.targetRing)
    }

    @Test
    fun `seeding production demands the production password`() {
        val legality = rules().seed(ring("prod"))
        assertTrue(legality.isAllowed)
        assertTrue(legality.requirements.productionPassword)
    }

    @Test
    fun `seeding an unconfigured ring is blocked`() {
        val legality = rules().seed(ring("test", configured = false))
        assertFalse(legality.isAllowed)
        assertEquals(PromotionLegality.Blocker.RingNotConfigured, legality.blocker)
    }

    // MARK: rollback

    @Test
    fun `rollback is the least gated action in the app`() {
        val legality = rules().rollback(ring("prod", previous = "v0"))
        assertTrue(legality.isAllowed)
        assertTrue(
            "incident response is never gated, not even into production",
            legality.requirements.isEmpty,
        )
        assertFalse(legality.requirements.productionPassword)
    }

    @Test
    fun `rollback with nothing to return to is blocked`() {
        val legality = rules().rollback(ring("prod", previous = ""))
        assertFalse(legality.isAllowed)
        assertEquals(PromotionLegality.Blocker.NoPreviousVersion, legality.blocker)
    }

    @Test
    fun `a gated ring can still be rolled back`() {
        val gated = ring(
            "prod",
            previous = "v0",
            gates = RingGates(maintenanceWindow = true, maintenanceWindowOpen = false),
        )
        val legality = rules().rollback(gated)
        assertTrue("a shut window must not block a rollback", legality.isAllowed)
        assertTrue(legality.requirements.isEmpty)
    }

    // MARK: auto-promote

    @Test
    fun `enabling auto-promote into production needs the password`() {
        val legality = rules().toggleAutoPromote(ring("acc"), enabled = true)
        assertTrue(legality.isAllowed)
        assertTrue(
            "otherwise auto-promote is a way around the production password",
            legality.requirements.productionPassword,
        )
    }

    @Test
    fun `disabling auto-promote is always allowed without a password`() {
        val legality = rules().toggleAutoPromote(ring("acc"), enabled = false)
        assertTrue(legality.isAllowed)
        assertFalse(legality.requirements.productionPassword)
    }

    @Test
    fun `auto-promote owned by config cannot be toggled`() {
        val legality = rules()
            .toggleAutoPromote(ring("int", autoPromoteManaged = true), enabled = true)
        assertFalse(legality.isAllowed)
        assertEquals(PromotionLegality.Blocker.AutoPromoteManagedByConfig, legality.blocker)
    }

    @Test
    fun `enabling auto-promote on a ring that does not reach production needs no password`() {
        val legality = rules().toggleAutoPromote(ring("int"), enabled = true)
        assertTrue(legality.isAllowed)
        assertFalse(legality.requirements.productionPassword)
    }

    // MARK: against the real fixtures

    @Test
    fun `the gated fixture produces the gates the server declared`() {
        val rings = Fixtures.decode<RingsResponse>("rings-gated").rings
        val r = rules()
        rings.forEach { source ->
            val legality = r.promote(source, rings)
            val target = pipeline.next(source.ring.name)
            if (legality.isAllowed) {
                val gates = rings.first { it.ring.name == target!!.name }.gates
                assertEquals(gates.changeRequest, legality.requirements.changeRequestCode)
                assertEquals(gates.qaSignoff, legality.requirements.qaSignoff)
                assertEquals(gates.maintenanceWindow, legality.requirements.maintenanceWindow)
            }
        }
    }

    @Test
    fun `an empty pipeline never offers a promotion`() {
        val r = PromotionRules(RingPipeline.EMPTY, prodProtected = true)
        val legality = r.promote(ring("int"), emptyList())
        assertFalse(legality.isAllowed)
        assertEquals(PromotionLegality.Blocker.NoNextRing, legality.blocker)
    }
}
