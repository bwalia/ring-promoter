/**
 * The few facts the privacy policy, terms and support page all need, in one
 * place so they can never disagree with each other.
 *
 *  ⚠️ The values marked FILL IN below are placeholders. Apple opens the
 * support URL and emails the support address during review, so shipping these
 * unchanged is a Guideline 1.5 / 2.1 rejection. `ios/scripts/
 * verify-app-store-readiness.sh` fails while any FILL_IN_ sentinel survives,
 * so a release cannot go out with them still in.
 */

/** Legal or trading name of the publisher, as it should appear on the pages. */
export const PUBLISHER = "FILL_IN_PUBLISHER_NAME";

/** A mailbox that is actually monitored — Apple does write to it. */
export const CONTACT_EMAIL = "FILL_IN_SUPPORT_EMAIL";

/** Governing law for the terms, e.g. "England and Wales". */
export const JURISDICTION = "FILL_IN_JURISDICTION";

/**
 * Postal address for questions, complaints and claims.
 *
 * Required twice over: by clause 8 of Apple's Minimum Terms for a
 * developer-supplied EULA, and by the trader details the App Store collects
 * for EU consumers under the Digital Services Act. One line per array entry.
 */
export const POSTAL_ADDRESS = ["FILL_IN_ADDRESS_LINE_1", "FILL_IN_ADDRESS_LINE_2"];

/** Shown as "last updated" on every legal page. Bump when the text changes. */
export const LAST_UPDATED = "6 November 2026";

export const APP_NAME = "Ring Promoter";
export const BUNDLE_ID = "org.fictionally.ringpromoter";
export const GITHUB_URL = "https://github.com/bwalia/ring-promoter";
export const SITE_URL = "https://ring-promoter-pages.fictionally.org";
