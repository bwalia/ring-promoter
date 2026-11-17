import Foundation

/// Keyword / wildcard filter for application lists, ported from the web
/// console's `web/src/lib/app-filter.ts` so the same query typed into either
/// client narrows the same way.
///
/// Query syntax, case-insensitive:
///
/// - Terms are separated by spaces or commas. An app shows when it matches
///   **any** include term and **no** exclude term.
/// - A plain term matches anywhere: `nebula` → nebuladb, nebuladb-edge…
/// - `*` (any run) and `?` (one character) make the term a whole-value glob:
///   `nebula*` starts with, `*api` ends with, `*-ui*` contains.
/// - A leading `-` or `!` excludes: `workstation* -*website*`.
/// - A query of only exclude terms starts from every app.
///
/// Each term is tested against every field given for the app — its id, its
/// display title, the groups it belongs to — so `Workstation*` matches the
/// title "Workstation OpsAPI" even though the id is lower-case.
struct AppQuery: Sendable {
    private let include: [NSRegularExpression]
    private let exclude: [NSRegularExpression]

    /// True when the query has no usable terms, which matches everything.
    let isEmpty: Bool

    init(_ raw: String) {
        var include: [NSRegularExpression] = []
        var exclude: [NSRegularExpression] = []
        for token in raw.split(whereSeparator: { $0 == " " || $0 == "," || $0 == "\n" }) {
            let negated = token.hasPrefix("-") || token.hasPrefix("!")
            let term = negated ? String(token.dropFirst()) : String(token)
            // A bare "-", "!" or "*" while still typing should filter nothing
            // rather than everything.
            if term.isEmpty || term.allSatisfy({ $0 == "*" }) { continue }
            guard let expression = Self.expression(for: term) else { continue }
            if negated { exclude.append(expression) } else { include.append(expression) }
        }
        self.include = include
        self.exclude = exclude
        isEmpty = include.isEmpty && exclude.isEmpty
    }

    /// A term with a wildcard anchors to the whole value; one without matches
    /// anywhere inside it.
    private static func expression(for term: String) -> NSRegularExpression? {
        let hasWildcard = term.contains("*") || term.contains("?")
        let pattern: String
        if hasWildcard {
            // Escape everything, then put the two wildcards back as regex.
            var body = NSRegularExpression.escapedPattern(for: term)
            body = body.replacingOccurrences(of: "\\*", with: ".*")
            body = body.replacingOccurrences(of: "\\?", with: ".")
            pattern = "^\(body)$"
        } else {
            pattern = NSRegularExpression.escapedPattern(for: term)
        }
        return try? NSRegularExpression(pattern: pattern, options: [.caseInsensitive])
    }

    /// Does an app, described by its searchable fields, pass?
    func matches(_ fields: [String]) -> Bool {
        guard !isEmpty else { return true }
        let hit: (NSRegularExpression) -> Bool = { expression in
            fields.contains { field in
                guard !field.isEmpty else { return false }
                let range = NSRange(field.startIndex..., in: field)
                return expression.firstMatch(in: field, range: range) != nil
            }
        }
        if exclude.contains(where: hit) { return false }
        return include.isEmpty || include.contains(where: hit)
    }
}
