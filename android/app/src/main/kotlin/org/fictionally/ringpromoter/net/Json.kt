package org.fictionally.ringpromoter.net

import kotlinx.serialization.json.Json

/**
 * The JSON configuration for this API.
 *
 * No naming strategy is applied. The API is *mostly* snake_case, but the
 * config-defined recurring maintenance windows serialise with PascalCase keys
 * (the Go struct has no JSON tags), so every model declares its keys with
 * explicit `@SerialName` — which is also what makes the mapping auditable
 * against the Go source.
 *
 * - [Json.ignoreUnknownKeys]: a newer control plane may add fields. Those must
 *   not take a screen down.
 * - [Json.coerceInputValues]: Go marshals an empty slice or map as `null`
 *   (`gated_rings`, `apps`, `logs`), and the app wants the empty collection the
 *   field already defaults to rather than a crash.
 * - [Json.explicitNulls] off: an absent optional is sent as absent, never as an
 *   explicit `null`. The server decodes with `DisallowUnknownFields`, and a
 *   null `password` is a different thing from not sending one.
 * - [Json.encodeDefaults] off: same reason, for fields left at their default.
 */
val RpJson = Json {
    ignoreUnknownKeys = true
    coerceInputValues = true
    explicitNulls = false
    encodeDefaults = false
    isLenient = false
}
