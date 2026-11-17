import Foundation
import Testing

@testable import RingPromoter

/// The filter on the Rings of Applications stage.
///
/// These assertions are the documented behaviour of the web console's
/// `app-filter.ts`, which this is a port of — the point of the port is that
/// the same query typed into either client narrows the same way, and that is
/// only true if it is checked.
@Suite("Application filter query")
struct AppQueryTests {
    private let fields = ["nebuladb-edge", "NebulaDB edge (DNS + vhost)", "Workstation"]

    @Test("an empty query matches everything")
    func emptyMatchesAll() {
        #expect(AppQuery("").isEmpty)
        #expect(AppQuery("   ").matches(fields))
        #expect(AppQuery(",, ").matches([]))
    }

    @Test("a plain term matches anywhere, ignoring case")
    func plainTerm() {
        #expect(AppQuery("nebula").matches(fields))
        #expect(AppQuery("NEBULA").matches(fields))
        #expect(AppQuery("edge").matches(fields))
        #expect(!AppQuery("payments").matches(fields))
    }

    @Test("a term matches against any field, including the title and group")
    func anyField() {
        #expect(AppQuery("workstation").matches(fields))
        #expect(AppQuery("vhost").matches(fields))
    }

    @Test("a wildcard anchors the term to the whole value")
    func wildcardsAnchor() {
        #expect(AppQuery("nebula*").matches(["nebuladb-edge"]))
        #expect(!AppQuery("nebula*").matches(["my-nebuladb"]))
        #expect(AppQuery("*api").matches(["workstation-opsapi"]))
        #expect(!AppQuery("*api").matches(["opsapi-ui"]))
        #expect(AppQuery("*-ui*").matches(["opsapi-ui-thing"]))
    }

    @Test("? matches exactly one character")
    func singleCharacterWildcard() {
        #expect(AppQuery("ap?").matches(["api"]))
        #expect(!AppQuery("ap?").matches(["apiv2"]))
    }

    @Test("include terms are OR-ed together")
    func includesAreOred() {
        let query = AppQuery("nebula payments")
        #expect(query.matches(["nebuladb"]))
        #expect(query.matches(["payments-api"]))
        #expect(!query.matches(["web-frontend"]))
    }

    @Test("a leading - or ! excludes, and beats an include")
    func excludeWins() {
        #expect(!AppQuery("-test").matches(["portfolio-test"]))
        #expect(!AppQuery("!test").matches(["portfolio-test"]))
        #expect(AppQuery("-test").matches(["payments-api"]))
        #expect(!AppQuery("workstation* -*website*").matches(["workstation-website"]))
        #expect(AppQuery("workstation* -*website*").matches(["workstation-opsapi"]))
    }

    @Test("a query of only excludes starts from every app")
    func onlyExcludes() {
        let query = AppQuery("-nebula")
        #expect(query.matches(["payments-api"]))
        #expect(!query.matches(["nebuladb"]))
    }

    @Test("a half-typed term filters nothing rather than everything")
    func halfTypedIsHarmless() {
        // The moment after typing "-" or "*", every app would otherwise vanish.
        #expect(AppQuery("-").matches(["anything"]))
        #expect(AppQuery("!").matches(["anything"]))
        #expect(AppQuery("*").matches(["anything"]))
        #expect(AppQuery("***").matches(["anything"]))
        #expect(AppQuery("-").isEmpty)
    }

    @Test("regex metacharacters in a term are literal, not a pattern")
    func metacharactersAreLiteral() {
        #expect(AppQuery("a.b").matches(["a.b-service"]))
        #expect(!AppQuery("a.b").matches(["axb-service"]))
        #expect(AppQuery("(dns").matches(["edge (dns + vhost)"]))
    }

    @Test("terms separate on commas as well as spaces")
    func commaSeparated() {
        let query = AppQuery("nebula,payments")
        #expect(query.matches(["payments-api"]))
        #expect(query.matches(["nebuladb"]))
    }

    @Test("an empty field never matches")
    func emptyFieldsIgnored() {
        #expect(!AppQuery("nebula").matches([""]))
    }
}
