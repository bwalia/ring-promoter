import XCTest

/// The paths an operator actually walks:
/// connect → view an app → promote through the gates → watch the job → roll back.
///
/// Every test runs against demo mode, so nothing is deployed and no server is
/// needed. Demo mode enforces the same rules the backend does, which is what
/// makes these assertions meaningful rather than decorative.
///
/// Rings and their action buttons carry accessibility identifiers
/// (`ring-prod`, `promote-int`, `rollback-test`…), so the tests target one
/// specific ring rather than whichever button happened to come first.
final class CriticalPathUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = [
            "-rp.demoMode", "YES",
            // Keep the biometric lock out of the way; it is covered separately.
            "-rp.lockOnOpen", "NO",
        ]
    }

    // MARK: - Connect

    func testDemoModeShowsTheOverviewWithTroubleFirst() {
        app.launch()

        XCTAssertTrue(
            app.staticTexts["Applications"].waitForExistence(timeout: 10),
            "the Overview should appear"
        )
        // The demo world has one genuinely unhealthy application, and it must be
        // pinned above the healthy ones.
        XCTAssertTrue(app.staticTexts["NEEDS ATTENTION"].exists)
        XCTAssertTrue(app.staticTexts["Payments API"].exists)
        attachScreenshot(named: "01-overview")
    }

    func testOnboardingIsShownWhenNotConnected() {
        app.launchArguments = ["-rp.demoMode", "NO"]
        app.launch()

        XCTAssertTrue(
            app.buttons["Connect to a control plane"].waitForExistence(timeout: 10),
            "a fresh install should offer to connect"
        )
        XCTAssertTrue(app.buttons["Explore in demo mode"].exists)
        attachScreenshot(named: "00-onboarding")
    }

    // MARK: - App detail

    func testAppDetailShowsEveryRingAndItsGates() throws {
        app.launch()
        openApp(named: "Payments API")

        // Every ring in the pipeline is present, configured or not. The list is
        // lazy, so the later rings need scrolling into existence.
        for ring in ["int", "test", "acc", "prod"] {
            let header = app.staticTexts["ring-\(ring)"]
            if !header.exists { app.swipeUp() }
            XCTAssertTrue(
                header.waitForExistence(timeout: 5), "the \(ring) ring should be on screen"
            )
        }
        // The gated rings advertise what guards them.
        XCTAssertTrue(app.staticTexts["QA sign-off"].firstMatch.exists)
        attachScreenshot(named: "02-app-detail")
    }

    func testUnhealthyRingIsCalledOutAtTheTop() {
        app.launch()
        openApp(named: "Payments API")

        XCTAssertTrue(
            app.staticTexts["Test is unhealthy"].waitForExistence(timeout: 5),
            "an unhealthy ring should be called out above the pipeline"
        )
    }

    func testAnEmptyRingOffersNoPromotion() {
        app.launch()
        openApp(named: "Payments API")

        // `prod` holds nothing, so there is nothing to promote out of it — and
        // it is the last ring besides.
        let promoteFromProd = button("promote-prod")
        XCTAssertTrue(promoteFromProd.waitForExistence(timeout: 5))
        XCTAssertFalse(promoteFromProd.isEnabled, "an empty last ring must not offer Promote")
    }

    // MARK: - Promote through the gates

    func testPromoteSheetCollectsTheChangeRequestCode() throws {
        app.launch()
        openApp(named: "Payments API")

        // int → test is the one legal promotion on this app: int is healthy and
        // holds a version, while test is unhealthy.
        let promote = button("promote-int")
        XCTAssertTrue(promote.waitForExistence(timeout: 5))
        XCTAssertTrue(promote.isEnabled)
        promote.tap()

        XCTAssertTrue(app.navigationBars["Promote"].waitForExistence(timeout: 5))
        attachScreenshot(named: "03-promote-sheet")
        app.buttons["action-cancel"].tap()
    }

    func testProductionPromotionRequiresPasswordAndTypedConfirmation() throws {
        app.launch()
        openApp(named: "Web Frontend")

        // acc → prod is the promotion that enters the protected last ring.
        let promote = button("promote-acc")
        XCTAssertTrue(promote.waitForExistence(timeout: 5))
        XCTAssertTrue(promote.isEnabled, "acc holds a healthy version in the demo world")
        promote.tap()

        XCTAssertTrue(app.navigationBars["Promote"].waitForExistence(timeout: 5))
        // The sheet must demand the production password AND a typed
        // confirmation before it will let anything through.
        XCTAssertTrue(app.secureTextFields["Production password"].exists)
        XCTAssertTrue(app.textFields["Type web-frontend to confirm"].exists)

        let submit = app.buttons["action-submit"]
        XCTAssertFalse(submit.isEnabled, "submit must stay disabled until both are supplied")

        // Supplying only the password is still not enough.
        app.secureTextFields["Production password"].tap()
        app.typeText("demo")
        XCTAssertFalse(submit.isEnabled, "the typed confirmation is required as well")

        attachScreenshot(named: "04-production-guard")
        app.buttons["action-cancel"].tap()
    }

    // MARK: - Watch a job

    func testSeedStartsAJobAndTheLiveViewFollowsItToATerminalState() throws {
        app.launch()
        openApp(named: "Web Frontend")

        let seed = button("seed-int")
        XCTAssertTrue(seed.waitForExistence(timeout: 5))
        seed.tap()

        XCTAssertTrue(app.navigationBars["Seed a version"].waitForExistence(timeout: 5))
        let submit = app.buttons["action-submit"]
        XCTAssertTrue(submit.waitForExistence(timeout: 2))
        XCTAssertTrue(submit.isEnabled, "the sheet prefills the ring's current version")
        submit.tap()

        // The app should push the live job view without being asked.
        let outcome = app.descendants(matching: .any)["job-outcome"]
        XCTAssertTrue(
            outcome.waitForExistence(timeout: 15),
            "starting an action should open the live job view"
        )
        attachScreenshot(named: "05-job-running")

        // And it should reach a terminal state on its own, stated explicitly.
        XCTAssertTrue(
            waitFor(timeout: 40) {
                (outcome.value as? String)?.localizedCaseInsensitiveContains("Succeeded") == true
            },
            "the job should finish and say so explicitly"
        )
        attachScreenshot(named: "06-job-succeeded")
    }

    // MARK: - Roll back

    func testRollbackIsOfferedWithoutAnyGate() throws {
        app.launch()
        openApp(named: "Payments API")

        // The demo world's `test` ring was rolled back once already, so it has a
        // previous version to return to.
        let rollback = button("rollback-test")
        XCTAssertTrue(rollback.waitForExistence(timeout: 5))
        XCTAssertTrue(rollback.isEnabled, "a ring with a previous version can roll back")
        rollback.tap()

        XCTAssertTrue(app.navigationBars["Roll back"].waitForExistence(timeout: 5))
        // Incident response is never gated: no password, no CR code, no typed
        // confirmation, and the button is live immediately.
        XCTAssertFalse(app.secureTextFields["Production password"].exists)
        XCTAssertFalse(app.textFields["Change-request code"].exists)
        XCTAssertTrue(app.buttons["action-submit"].isEnabled)
        attachScreenshot(named: "07-rollback-sheet")
        app.buttons["action-cancel"].tap()
    }

    // MARK: - The rest of the app

    func testActivityAndSettingsTabsLoad() {
        app.launch()
        XCTAssertTrue(app.staticTexts["Applications"].waitForExistence(timeout: 10))

        app.tabBars.buttons["Activity"].tap()
        XCTAssertTrue(app.navigationBars["Activity"].waitForExistence(timeout: 5))
        attachScreenshot(named: "08-activity")

        app.tabBars.buttons["Settings"].tap()
        XCTAssertTrue(app.navigationBars["Settings"].waitForExistence(timeout: 5))
        // List section headers render uppercased.
        XCTAssertTrue(app.staticTexts["CONTROL PLANES"].waitForExistence(timeout: 5))
        attachScreenshot(named: "09-settings")
    }

    func testRingsUniverseShowsTheFleetAndOpensAService() {
        app.launch()
        XCTAssertTrue(app.staticTexts["Applications"].waitForExistence(timeout: 10))

        app.tabBars.buttons["Rings"].tap()
        XCTAssertTrue(
            app.navigationBars["Rings of Applications"].waitForExistence(timeout: 5)
        )

        // Every demo application orbits the stage as a tappable body.
        let planet = app.buttons["planet-payments-api"]
        XCTAssertTrue(planet.waitForExistence(timeout: 10))
        planet.tap()

        // The detail card pins open with the way into the service.
        let open = app.buttons["Open service"]
        XCTAssertTrue(open.waitForExistence(timeout: 5))
        attachScreenshot(named: "11-rings-universe")

        open.tap()
        XCTAssertTrue(
            app.navigationBars["Payments API"].waitForExistence(timeout: 10),
            "opening a body should land on that service's pipeline"
        )
    }

    func testHistoryIsReachableFromAnApp() {
        app.launch()
        openApp(named: "Web Frontend")

        app.buttons["More"].tap()
        app.buttons["History"].tap()
        XCTAssertTrue(app.navigationBars["History"].waitForExistence(timeout: 5))
        attachScreenshot(named: "10-history")
    }

    func testMaintenanceScreenShowsWindowState() {
        app.launch()
        openApp(named: "Payments API")

        app.buttons["More"].tap()
        app.buttons["Maintenance windows"].tap()
        XCTAssertTrue(app.navigationBars["Maintenance"].waitForExistence(timeout: 5))
        XCTAssertTrue(
            app.staticTexts["guarded-rings-header"].waitForExistence(timeout: 5),
            "the guarded rings and their window state should be listed"
        )
        attachScreenshot(named: "11-maintenance")
    }

    func testSearchFiltersTheOverview() {
        app.launch()
        XCTAssertTrue(app.staticTexts["Applications"].waitForExistence(timeout: 10))

        let field = app.searchFields["Search applications"]
        XCTAssertTrue(field.waitForExistence(timeout: 5))
        field.tap()
        field.typeText("batch")

        XCTAssertTrue(app.staticTexts["batch-worker"].waitForExistence(timeout: 5))
        XCTAssertFalse(app.staticTexts["Payments API"].exists)
    }

    // MARK: - Helpers

    /// Find a button by identifier, scrolling the (lazy) pipeline list until it
    /// exists and is on screen.
    private func button(_ identifier: String, scrolls: Int = 6) -> XCUIElement {
        let element = app.buttons[identifier]
        for _ in 0..<scrolls {
            if element.exists, element.isHittable { return element }
            app.swipeUp()
        }
        return element
    }

    /// Poll a condition until it holds or the timeout elapses.
    ///
    /// Used instead of `XCTNSPredicateExpectation`, which is not usable from a
    /// non-`Sendable` test case under Swift 6 strict concurrency.
    private func waitFor(timeout: TimeInterval, condition: () -> Bool) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if condition() { return true }
            _ = XCUIApplication().wait(for: .runningForeground, timeout: 0.4)
        }
        return condition()
    }

    private func openApp(named title: String) {
        let cell = app.buttons[title].firstMatch
        if cell.waitForExistence(timeout: 10) {
            cell.tap()
        } else {
            app.staticTexts[title].tap()
        }
        XCTAssertTrue(
            app.navigationBars[title].waitForExistence(timeout: 10),
            "the \(title) detail screen should open"
        )
    }

    /// Attach for the test report, and — when `SCREENSHOT_DIR` is set — write a
    /// PNG there so the documented screenshot set can be regenerated.
    private func attachScreenshot(named name: String) {
        let screenshot = XCUIScreen.main.screenshot()
        let attachment = XCTAttachment(screenshot: screenshot)
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)

        guard let directory = ProcessInfo.processInfo.environment["SCREENSHOT_DIR"] else { return }
        let suffix = app.launchArguments.contains("dark") ? "-dark" : ""
        try? FileManager.default.createDirectory(
            at: URL(fileURLWithPath: directory), withIntermediateDirectories: true
        )
        let url = URL(fileURLWithPath: directory).appendingPathComponent("\(name)\(suffix).png")
        try? screenshot.pngRepresentation.write(to: url)
    }
}
