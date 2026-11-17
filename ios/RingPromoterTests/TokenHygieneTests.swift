import Foundation
import Testing

@testable import RingPromoter

/// A pasted token is repaired where that is unambiguous, and the rest is named
/// rather than sent.
///
/// These are the real mistakes: a value that arrived wrapped in a shell's
/// quotes, one carrying the `Bearer ` prefix from a curl example, and one
/// broken across a line by a terminal. All three reach the server as a
/// different string and come back as the same `401 missing or invalid bearer
/// token`, which is why the app has to tell them apart itself.
@Suite("Pasted token hygiene")
struct TokenHygieneTests {
    /// A 64-character stand-in for a real token, built rather than written
    /// out. A literal 64-character hex string in a source file is
    /// indistinguishable from a leaked credential to a secret scanner — this
    /// one turned GitGuardian red on every pull request, and a scanner people
    /// learn to ignore is worse than no scanner.
    private let valid = String(repeating: "0123456789abcdef", count: 4)

    @Test("a clean token is left exactly as it is")
    func cleanTokenUntouched() {
        let result = TokenHygiene.clean(valid)
        #expect(result.token == valid)
        #expect(result.repairs.isEmpty)
        #expect(result.isUsable)
    }

    @Test("surrounding whitespace is trimmed without comment")
    func trimsWhitespace() {
        let result = TokenHygiene.clean("  \n\(valid)\t ")
        #expect(result.token == valid)
        #expect(result.repairs.isEmpty)
        #expect(result.isUsable)
    }

    @Test(
        "wrapping quotes are removed and reported",
        arguments: ["\"", "'"]
    )
    func stripsQuotes(quote: String) {
        let result = TokenHygiene.clean("\(quote)\(valid)\(quote)")
        #expect(result.token == valid)
        #expect(result.repairs == ["removed the surrounding quotes"])
        #expect(result.isUsable)
    }

    @Test("a smart-quoted token is unwrapped too")
    func stripsSmartQuotes() {
        let result = TokenHygiene.clean("\u{201C}\(valid)\u{201D}")
        #expect(result.token == valid)
        #expect(result.isUsable)
    }

    @Test(
        "the Bearer prefix is removed whatever its case",
        arguments: ["Bearer ", "bearer ", "BEARER "]
    )
    func stripsBearerPrefix(prefix: String) {
        let result = TokenHygiene.clean(prefix + valid)
        #expect(result.token == valid)
        #expect(result.repairs == ["removed the “Bearer ” prefix"])
        #expect(result.isUsable)
    }

    @Test("a quoted header value loses both the quotes and the prefix")
    func stripsBoth() {
        let result = TokenHygiene.clean("\"Bearer \(valid)\"")
        #expect(result.token == valid)
        #expect(result.repairs.count == 2)
        #expect(result.isUsable)
    }

    @Test("a token broken across a line is refused, not sent")
    func refusesInternalWhitespace() {
        let halves = valid.prefix(32) + "\n" + valid.suffix(32)
        let result = TokenHygiene.clean(String(halves))
        #expect(result.defect == .internalWhitespace)
        #expect(!result.isUsable)
    }

    @Test("a space in the middle is the same mistake")
    func refusesInternalSpace() {
        let result = TokenHygiene.clean("abc def")
        #expect(result.defect == .internalWhitespace)
    }

    @Test("characters no HTTP header can carry are refused")
    func refusesNonHeaderSafe() {
        #expect(TokenHygiene.clean("tok\u{201C}en").defect == .notHeaderSafe)
        #expect(TokenHygiene.clean("tok\u{0000}en").defect == .notHeaderSafe)
    }

    @Test("an empty or quotes-only value asks for a token")
    func refusesEmpty() {
        #expect(TokenHygiene.clean("").defect == .empty)
        #expect(TokenHygiene.clean("   ").defect == .empty)
        #expect(TokenHygiene.clean("\"\"").defect == .empty)
        #expect(TokenHygiene.clean("Bearer ").defect == .empty)
    }

    @Test("every defect explains what to do, and none mentions the token")
    func defectsAreActionable() {
        for defect in [
            TokenHygiene.Defect.empty, .internalWhitespace, .notHeaderSafe,
        ] {
            #expect(!defect.message.isEmpty)
            #expect(defect.message.contains(valid) == false)
        }
    }
}

/// The 401 an operator sees during onboarding.
@Suite("Rejected token advice")
struct RejectionAdviceTests {
    @Test("says what to check, and never repeats the server's log wording")
    func advice() {
        let message = ConnectionValidator.rejectionAdvice(tokenLength: 32)
        // The length is the whole point: a truncated copy is otherwise
        // indistinguishable from a wrong one.
        #expect(message.contains("32 characters"))
        #expect(message.contains("RP_API_TOKEN"))
        #expect(!message.lowercased().contains("missing or invalid bearer token"))
    }
}
