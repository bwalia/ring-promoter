/**
 * Links to pages that only the public site publishes (/privacy, /terms,
 * /support).
 *
 * The GitHub Pages build contains those pages, so links stay relative there.
 * The console image leaves them out (see the Dockerfile), so it sets
 * `PUBLIC_SITE_PAGES_URL` and the links point at the public site instead of a
 * 404. Read at build time by Server Components only.
 */
export function publicSitePage(path: `/${string}`): string {
  const base = process.env.PUBLIC_SITE_PAGES_URL?.trim().replace(/\/+$/, "");
  return base ? `${base}${path}/` : path;
}
