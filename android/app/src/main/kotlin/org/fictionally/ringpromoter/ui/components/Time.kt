package org.fictionally.ringpromoter.ui.components

import android.text.format.DateUtils
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.TextStyle
import java.time.Duration
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * Times as the rest of the phone writes them.
 *
 * All of this defers to the platform — `DateUtils` for "5 minutes ago",
 * localised formatters for clock times — because an operator reads these
 * alongside their notification shade and their calendar, and a hand-rolled
 * format would be the only thing on the screen phrased differently. It also
 * means every locale and the 12/24-hour setting are already handled.
 */
object Times {
    private val clock: DateTimeFormatter =
        DateTimeFormatter.ofLocalizedTime(FormatStyle.SHORT).withZone(ZoneId.systemDefault())

    private val dayAndClock: DateTimeFormatter =
        DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT)
            .withZone(ZoneId.systemDefault())

    private val fullDay: DateTimeFormatter =
        DateTimeFormatter.ofLocalizedDate(FormatStyle.FULL).withZone(ZoneId.systemDefault())

    /** "14:32" — in the operator's own zone, which is how they read a log. */
    fun time(instant: Instant): String = clock.format(instant)

    /** "11 Nov 2026, 14:32" */
    fun dateAndTime(instant: Instant): String = dayAndClock.format(instant)

    /** "Wednesday, 11 November 2026" */
    fun fullDate(instant: Instant): String = fullDay.format(instant)

    /**
     * "5 minutes ago", "in 2 hours" — or "just now" under a minute, where
     * `DateUtils` would otherwise say "0 minutes ago".
     */
    fun relative(instant: Instant, now: Instant = Instant.now()): String {
        val gap = Duration.between(instant, now).abs()
        if (gap < Duration.ofMinutes(1)) return "just now"
        return DateUtils.getRelativeTimeSpanString(
            instant.toEpochMilli(),
            now.toEpochMilli(),
            DateUtils.MINUTE_IN_MILLIS,
        ).toString()
    }

    /** "14:00 – 15:30" when both ends fall on one day, else both dates. */
    fun range(from: Instant, to: Instant): String {
        val zone = ZoneId.systemDefault()
        val sameDay = from.atZone(zone).toLocalDate() == to.atZone(zone).toLocalDate()
        return if (sameDay) {
            "${dateAndTime(from)} – ${time(to)}"
        } else {
            "${dateAndTime(from)} – ${dateAndTime(to)}"
        }
    }
}

/** A quiet "Updated 2 minutes ago" line. */
@Composable
fun RelativeTimestamp(
    instant: Instant,
    modifier: Modifier = Modifier,
    prefix: String? = null,
    style: TextStyle = MaterialTheme.typography.labelSmall,
) {
    val text = Times.relative(instant)
    Text(
        text = if (prefix != null) "$prefix $text" else text,
        style = style,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = modifier,
    )
}
