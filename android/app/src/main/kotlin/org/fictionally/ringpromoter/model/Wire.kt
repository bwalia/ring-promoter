package org.fictionally.ringpromoter.model

import kotlinx.serialization.KSerializer
import kotlinx.serialization.descriptors.PrimitiveKind
import kotlinx.serialization.descriptors.PrimitiveSerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder
import java.time.Instant
import java.time.OffsetDateTime
import java.time.format.DateTimeParseException

/**
 * Timestamps as Go's `time.Time` marshals them.
 *
 * Go emits RFC3339 with fractional seconds only when the value has them, so
 * `"…:52Z"` and `"…:47.905917Z"` both occur in a single response.
 * [Instant.parse] accepts both, and an explicit offset (`+01:00`) is handled by
 * falling back to [OffsetDateTime].
 *
 * A ring nothing has ever touched carries Go's zero time,
 * `"0001-01-01T00:00:00Z"`. That parses cleanly here, and lands before the
 * epoch — which is exactly what [RingStatus.hasBeenDeployedTo] tests for, so
 * the UI shows "never" rather than "1 Jan 0001".
 */
object InstantSerializer : KSerializer<Instant> {
    override val descriptor =
        PrimitiveSerialDescriptor("java.time.Instant", PrimitiveKind.STRING)

    override fun serialize(encoder: Encoder, value: Instant) {
        // No fractional seconds: the form the maintenance-window endpoint wants
        // for starts_at / ends_at.
        encoder.encodeString(value.truncatedTo(java.time.temporal.ChronoUnit.SECONDS).toString())
    }

    override fun deserialize(decoder: Decoder): Instant {
        val raw = decoder.decodeString()
        return try {
            Instant.parse(raw)
        } catch (_: DateTimeParseException) {
            try {
                OffsetDateTime.parse(raw).toInstant()
            } catch (e: DateTimeParseException) {
                throw IllegalArgumentException("Expected an RFC3339 timestamp, got \"$raw\"", e)
            }
        }
    }
}

/** A timestamp that is unset on the server side. */
val DISTANT_PAST: Instant = Instant.parse("0001-01-01T00:00:00Z")

/**
 * Decodes a string-valued enum, falling back instead of throwing when the
 * server sends a value this build has never heard of.
 *
 * The server is deployed independently of the app, so a newer control plane
 * will eventually name a status this binary does not know. Crashing on it would
 * take out the whole screen; falling back renders it neutrally and lets the
 * operator carry on.
 */
abstract class FallbackEnumSerializer<T>(
    name: String,
    private val wire: (T) -> String,
    private val parse: (String) -> T?,
    private val fallback: T,
) : KSerializer<T> {
    override val descriptor = PrimitiveSerialDescriptor(name, PrimitiveKind.STRING)
    override fun serialize(encoder: Encoder, value: T) = encoder.encodeString(wire(value))
    override fun deserialize(decoder: Decoder): T = parse(decoder.decodeString()) ?: fallback
}
