import Foundation

/// JSON coders configured for this API.
///
/// No key strategy is applied: the API is *mostly* snake_case, but the
/// config-defined recurring maintenance windows serialise with PascalCase keys
/// (the Go struct has no JSON tags). Every model therefore declares explicit
/// `CodingKeys`, which is also what makes the mapping auditable against the Go
/// source.
enum JSONCoding {
    static func makeDecoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder throws -> Date in
            try decodeTimestamp(decoder)
        }
        return decoder
    }

    static func makeEncoder() -> JSONEncoder {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .custom { date, encoder in
            var container = encoder.singleValueContainer()
            try container.encode(rfc3339String(from: date))
        }
        return encoder
    }

    /// RFC3339 without fractional seconds — the form the maintenance-window
    /// endpoint expects for `starts_at` / `ends_at`.
    static func rfc3339String(from date: Date) -> String {
        date.formatted(plainStyle)
    }

    /// Go's `time.Time` marshals to RFC3339, but with fractional seconds only
    /// when the value has them: `"…:52Z"` and `"…:47.905917Z"` both occur in a
    /// single response, so both forms are tried.
    ///
    /// A never-touched ring carries Go's zero time, `"0001-01-01T00:00:00Z"`,
    /// which is outside the formatter's supported range on some platforms and
    /// is therefore recognised explicitly. It maps to `.distantPast`, which is
    /// what `RingStatus.hasBeenDeployedTo` tests for.
    private static func decodeTimestamp(_ decoder: any Decoder) throws -> Date {
        let raw = try decoder.singleValueContainer().decode(String.self)
        if let date = try? Date(raw, strategy: fractionalStyle) { return date }
        if let date = try? Date(raw, strategy: plainStyle) { return date }
        if raw.hasPrefix("0001-01-01") { return .distantPast }
        throw DecodingError.dataCorrupted(
            .init(
                codingPath: decoder.codingPath,
                debugDescription: "Expected an RFC3339 timestamp, got \"\(raw)\""
            )
        )
    }

    // `Date.ISO8601FormatStyle` is a Sendable value type, so these are safe to
    // share across tasks — unlike `ISO8601DateFormatter`, which is a class.
    private static let fractionalStyle = Date.ISO8601FormatStyle(includingFractionalSeconds: true)
    private static let plainStyle = Date.ISO8601FormatStyle()
}
