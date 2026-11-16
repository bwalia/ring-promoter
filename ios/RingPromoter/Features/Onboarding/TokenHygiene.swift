import Foundation

/// Repairs and inspects a pasted bearer token before it is sent anywhere.
///
/// This exists because the server can only ever answer "no". Every way a token
/// gets mangled on its way from a terminal or a secret store into a text field
/// — wrapped in the quotes a shell printed, carrying the `Bearer ` prefix
/// copied along with the header, broken across a line by a terminal that
/// wrapped it — arrives as a different string and comes back as the same flat
/// `401 missing or invalid bearer token`. That message is true and useless: it
/// is written for whoever is reading the server's logs, not for the operator
/// holding the token.
///
/// So the repairable mistakes are repaired and reported, and the ones that
/// cannot be repaired are named before a request is made at all.
enum TokenHygiene {
    /// Something wrong with the token that the app will not guess its way past.
    enum Defect: Equatable, Sendable {
        case empty
        /// A gap in the middle. A bearer token has none, so this is almost
        /// always a copy that broke across a line.
        case internalWhitespace
        /// Control characters or anything outside printable ASCII. These
        /// cannot go in an HTTP header and would fail before reaching the
        /// network.
        case notHeaderSafe

        var message: String {
            switch self {
            case .empty:
                "Paste the API token for this server."
            case .internalWhitespace:
                "This token has a space or a line break in the middle of it, which no bearer "
                    + "token has. It was most likely copied from a terminal that wrapped the "
                    + "line — copy it again as a single piece."
            case .notHeaderSafe:
                "This token contains characters that cannot be sent in a request header, such "
                    + "as a line break or a smart quote. Copy it again as plain text."
            }
        }
    }

    /// The outcome of cleaning a pasted value.
    struct Cleaned: Equatable, Sendable {
        /// What should actually be sent and stored.
        let token: String
        /// Repairs that were applied, phrased for the operator. Shown on
        /// success too, so a token that works in the app but not in `curl` is
        /// never a mystery.
        let repairs: [String]
        /// Set when the value cannot be used as it stands.
        let defect: Defect?

        var isUsable: Bool { defect == nil }
    }

    static func clean(_ raw: String) -> Cleaned {
        var token = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        var repairs: [String] = []

        // `kubectl ... -o jsonpath` and most shells hand back a quoted value
        // when the command is wrapped in echo, and the quotes come along.
        for quote in ["\"", "'", "\u{201C}", "\u{2018}"] {
            let closing = closingQuote(for: quote)
            if token.count >= 2, token.hasPrefix(quote), token.hasSuffix(closing) {
                token = String(token.dropFirst().dropLast())
                    .trimmingCharacters(in: .whitespacesAndNewlines)
                repairs.append("removed the surrounding quotes")
                break
            }
        }

        // Copied from a curl command or a header example rather than from the
        // value itself.
        //
        // Matched as the word followed by a gap OR the end of the string, not
        // as the literal "bearer ". Trimming has already taken the trailing
        // space off a value that was nothing but the prefix, and that has to
        // end up empty — asking for a token — rather than as a six-character
        // token called "Bearer". Requiring the gap is what keeps a real token
        // that happens to begin "bearer…" intact.
        let word = "bearer"
        if token.count >= word.count, token.prefix(word.count).lowercased() == word {
            let rest = token.dropFirst(word.count)
            if rest.isEmpty || rest.first?.isWhitespace == true {
                token = String(rest).trimmingCharacters(in: .whitespacesAndNewlines)
                repairs.append("removed the “Bearer ” prefix")
            }
        }

        return Cleaned(token: token, repairs: repairs, defect: defect(in: token))
    }

    private static func closingQuote(for opening: String) -> String {
        switch opening {
        case "\u{201C}": "\u{201D}"
        case "\u{2018}": "\u{2019}"
        default: opening
        }
    }

    private static func defect(in token: String) -> Defect? {
        if token.isEmpty { return .empty }
        if token.unicodeScalars.contains(where: { !(0x21...0x7E).contains($0.value) }) {
            // Checked before the whitespace case so a tab or a newline is
            // reported as the gap it is, rather than as an exotic character.
            return token.contains(where: \.isWhitespace) ? .internalWhitespace : .notHeaderSafe
        }
        return nil
    }
}
