package org.fictionally.ringpromoter

import org.fictionally.ringpromoter.store.Instance
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * URL normalisation and naming for a saved control plane.
 *
 * Ported from the iOS `MultiInstanceTests`. These are the inputs an operator
 * actually types at 2am.
 */
class InstanceTest {

    @Test
    fun `a bare host gets https`() {
        assertEquals("https://rp.example.com", Instance.normalise("rp.example.com"))
    }

    @Test
    fun `trailing slashes and whitespace are dropped`() {
        assertEquals("https://rp.example.com", Instance.normalise("  https://rp.example.com/  "))
        assertEquals("https://rp.example.com", Instance.normalise("https://rp.example.com///"))
    }

    @Test
    fun `an explicit http target is kept, because localhost is legitimate`() {
        assertEquals("http://10.0.2.2:8080", Instance.normalise("http://10.0.2.2:8080"))
    }

    @Test
    fun `a path prefix survives normalisation`() {
        assertEquals("https://example.com/rp", Instance.normalise("https://example.com/rp/"))
    }

    @Test
    fun `nonsense is rejected rather than failing later at the first request`() {
        assertNull(Instance.normalise(""))
        assertNull(Instance.normalise("   "))
        assertNull(Instance.normalise("ftp://example.com"))
        assertNull(Instance.normalise("https://"))
    }

    @Test
    fun `an insecure instance is flagged but not blocked`() {
        val local = Instance(name = "Local", baseUrl = "http://10.0.2.2:8080")
        assertTrue(local.isInsecure)
        val prod = Instance(name = "Prod", baseUrl = "https://rp.example.com")
        assertFalse(prod.isInsecure)
    }

    @Test
    fun `the default name is the first meaningful host label`() {
        assertEquals("Workstation", Instance.defaultName("https://rp.workstation.co.uk"))
        assertEquals("Example", Instance.defaultName("https://api.example.com"))
    }

    @Test
    fun `an IP address or localhost keeps its own name`() {
        assertEquals("10.0.2.2", Instance.defaultName("http://10.0.2.2:8080"))
        assertEquals("localhost", Instance.defaultName("http://localhost:8080"))
    }

    @Test
    fun `the display host includes a non-default port`() {
        assertEquals("10.0.2.2:8080", Instance(name = "L", baseUrl = "http://10.0.2.2:8080").displayHost)
        assertEquals(
            "rp.example.com",
            Instance(name = "P", baseUrl = "https://rp.example.com").displayHost,
        )
    }

    @Test
    fun `each instance gets its own token account`() {
        val a = Instance(name = "A", baseUrl = "https://a.example.com")
        val b = Instance(name = "B", baseUrl = "https://b.example.com")
        assertTrue(
            "two control planes must never share a credential slot",
            a.tokenAccount != b.tokenAccount,
        )
    }
}
