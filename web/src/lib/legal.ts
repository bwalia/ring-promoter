/**
 * The few facts the privacy policy, terms and support page all need, in one
 * place so they can never disagree with each other.
 *
 * Everything here comes from the environment so the same codebase can be
 * published under a different company without a code change. See
 * `web/.env.example` for the full list and `ios/docs/APP-STORE-RELEASE.md`
 * for where CI sets them.
 *
 * These pages are Server Components in a static export, so Next.js reads
 * `process.env` at **build time** and bakes the result into the exported HTML.
 * Nothing is read in the browser, which is why the variables need no
 * `NEXT_PUBLIC_` prefix. Importing this module from a Client Component would
 * break that — the values would arrive as `undefined` — so keep it on the
 * server side.
 */

/**
 * A production build refuses to render a legal page with a missing value.
 *
 * This is deliberately fatal rather than a fallback: a privacy policy that
 * names the wrong publisher, or a support address that bounces, is a
 * Guideline 1.5 rejection and worse than having no page at all. Failing
 * `next build` puts the error where someone will see it, and replaces the
 * hard-coded placeholder values this file used to carry.
 *
 * `next dev` keeps working with a visible marker so the rest of the site can
 * be developed without the branding configured.
 */
function required(name: string, value: string | undefined): string {
  const trimmed = value?.trim();
  if (trimmed) return trimmed;
  if (process.env.NODE_ENV === "production") {
    throw new Error(
      `${name} is not set. The legal pages are built from environment ` +
        `variables so the site can be rebranded without a code change — set ` +
        `it in the build environment (see web/.env.example). Refusing to ` +
        `publish a legal page with a missing ${name}.`,
    );
  }
  return `⟨${name} not set⟩`;
}

/** Legal or trading name of the publisher, as it should appear on the pages. */
export const PUBLISHER = required("PUBLISHER", process.env.PUBLISHER);

/** A mailbox that is actually monitored — Apple does write to it. */
export const CONTACT_EMAIL = required("CONTACT_EMAIL", process.env.CONTACT_EMAIL);

/** Governing law for the terms, e.g. "England and Wales". */
export const JURISDICTION = required("JURISDICTION", process.env.JURISDICTION);

/**
 * Postal address for questions, complaints and claims, as `|`-separated lines
 * (`POSTAL_ADDRESS="1 Example Street|London|EC1A 1AA"`).
 *
 * Required twice over: by clause 8 of Apple's Minimum Terms for a
 * developer-supplied EULA, and by the trader details the App Store collects
 * for EU consumers under the Digital Services Act.
 */
export const POSTAL_ADDRESS = required("POSTAL_ADDRESS", process.env.POSTAL_ADDRESS)
  .split("|")
  .map((line) => line.trim())
  .filter(Boolean);

/**
 * Product identity. Overridable for a white-label launch, but defaulted to
 * this project's own values so an ordinary build needs no configuration —
 * unlike the four above, getting these wrong is cosmetic rather than a legal
 * misstatement, so a default is safe here and is not above.
 */
export const APP_NAME = process.env.APP_NAME?.trim() || "Ring Promoter";
export const BUNDLE_ID = process.env.BUNDLE_ID?.trim() || "org.fictionally.ringpromoter";
export const GITHUB_URL =
  process.env.GITHUB_URL?.trim() || "https://github.com/bwalia/ring-promoter";
export const SITE_URL =
  process.env.SITE_URL?.trim() || "https://ring-promoter-pages.fictionally.org";

/**
 * Shown as "last updated" on every legal page.
 *
 * Defaults to the date the current wording was written. Override it only when
 * a rebrand also changes the text; bumping it for an unchanged policy tells
 * readers something changed when nothing did.
 */
export const LAST_UPDATED = process.env.LEGAL_LAST_UPDATED?.trim() || "6 November 2026";
