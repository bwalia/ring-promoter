package org.fictionally.ringpromoter

import org.fictionally.ringpromoter.ui.components.Markdown
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * The small Markdown the AI diagnosis and test plans come back in.
 *
 * The rule the tests enforce is the same one the renderer is for: whatever the
 * model emits, the operator must be able to read every character of it. Text
 * may come out unstyled; it may never come out missing.
 */
class MarkdownTest {

    private fun rendered(text: String) = Markdown.parse(text).joinToString("") { it.text }

    @Test
    fun `bold runs are marked and their markers removed`() {
        val chunks = Markdown.parse("**What happened**\n\nIt broke.")
        assertEquals(Markdown.Chunk("What happened", bold = true), chunks.first())
        assertEquals("What happened\n\nIt broke.", rendered("**What happened**\n\nIt broke."))
    }

    @Test
    fun `inline code is marked and its backticks removed`() {
        val chunks = Markdown.parse("Run `kubectl get pods` next")
        assertEquals(listOf(false, true, false), chunks.map { it.code })
        assertEquals("Run kubectl get pods next", rendered("Run `kubectl get pods` next"))
    }

    @Test
    fun `bold and code can appear in one line`() {
        val chunks = Markdown.parse("**Check** `prod` now")
        assertEquals(
            listOf(
                Markdown.Chunk("Check", bold = true),
                Markdown.Chunk(" "),
                Markdown.Chunk("prod", code = true),
                Markdown.Chunk(" now"),
            ),
            chunks,
        )
    }

    @Test
    fun `an unclosed marker leaves the text readable`() {
        // Generated text is not guaranteed to be well formed, and swallowing
        // the rest of a diagnosis would be the worst possible failure here.
        assertEquals("A **partial thing", rendered("A **partial thing"))
        assertEquals("A `partial thing", rendered("A `partial thing"))
    }

    @Test
    fun `plain text is one chunk and is unchanged`() {
        val chunks = Markdown.parse("nothing special here")
        assertEquals(1, chunks.size)
        assertEquals(Markdown.Chunk("nothing special here"), chunks.single())
    }

    @Test
    fun `empty text produces nothing`() {
        assertEquals(emptyList<Markdown.Chunk>(), Markdown.parse(""))
    }

    @Test
    fun `every character of the demo diagnosis survives`() {
        val source = """
            **What happened**

            The deploy to `test` completed, but every health check came back `503`.

            1. `kubectl -n test get pods` — are the pods running?
        """.trimIndent()
        val expected = source.replace("**", "").replace("`", "")
        assertEquals(expected, rendered(source))
    }
}
