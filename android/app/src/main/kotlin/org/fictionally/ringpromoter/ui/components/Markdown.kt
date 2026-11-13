package org.fictionally.ringpromoter.ui.components

import androidx.compose.material3.LocalTextStyle
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withStyle

/**
 * The little Markdown the control plane's AI actually emits.
 *
 * The diagnosis and test-plan text come back as Markdown — `**bold**` headings
 * and `` `code` `` for ring names, versions and commands. Shown raw it reads as
 * a wall of asterisks and backticks, which is exactly the text an operator is
 * trying to read quickly during a failure. SwiftUI renders this for free
 * through `LocalizedStringKey`; Compose has no equivalent, so this is the
 * smallest thing that does the job.
 *
 * Bold and inline code only. Anything else is left exactly as written: a
 * half-finished Markdown renderer that mangles the text would be worse than
 * showing it plainly.
 */
object Markdown {

    /** One run of text and how it should look. */
    data class Chunk(val text: String, val bold: Boolean = false, val code: Boolean = false)

    private val PATTERN = Regex("""\*\*(.+?)\*\*|`([^`]+)`""", RegexOption.DOT_MATCHES_ALL)

    /**
     * Split [text] into styled runs.
     *
     * Pure, so it is tested on the JVM: an unbalanced marker in generated text
     * must leave the text readable rather than swallowing the rest of it.
     */
    fun parse(text: String): List<Chunk> {
        val chunks = mutableListOf<Chunk>()
        var cursor = 0
        for (match in PATTERN.findAll(text)) {
            if (match.range.first > cursor) {
                chunks += Chunk(text.substring(cursor, match.range.first))
            }
            val bold = match.groupValues[1]
            val code = match.groupValues[2]
            chunks += if (bold.isNotEmpty()) {
                Chunk(bold, bold = true)
            } else {
                Chunk(code, code = true)
            }
            cursor = match.range.last + 1
        }
        if (cursor < text.length) chunks += Chunk(text.substring(cursor))
        return chunks
    }
}

/** [text] with its bold and code runs rendered. */
@Composable
fun MarkdownText(
    text: String,
    modifier: Modifier = Modifier,
    style: TextStyle = LocalTextStyle.current,
) {
    val codeColour = MaterialTheme.colorScheme.onSurfaceVariant
    val annotated: AnnotatedString = buildAnnotatedString {
        Markdown.parse(text).forEach { chunk ->
            when {
                chunk.bold -> withStyle(SpanStyle(fontWeight = FontWeight.SemiBold)) {
                    append(chunk.text)
                }

                chunk.code -> withStyle(
                    SpanStyle(fontFamily = FontFamily.Monospace, color = codeColour),
                ) {
                    append(chunk.text)
                }

                else -> append(chunk.text)
            }
        }
    }
    Text(annotated, style = style, modifier = modifier)
}
