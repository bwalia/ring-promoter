/**
 * Keyword / wildcard filter for application lists (the Rings of Apps stage).
 *
 * Query syntax, case-insensitive:
 *   - Terms are separated by spaces or commas. An app is shown when it matches
 *     ANY include term (OR), and NO exclude term.
 *   - A plain term matches anywhere:        `nebula`   → nebuladb, nebuladb-edge…
 *   - `*` (any run) and `?` (one character) make the term a whole-value glob:
 *                                            `nebula*`  → starts with "nebula"
 *                                            `*api`     → ends with "api"
 *                                            `*-ui*`    → contains "-ui"
 *   - A leading `-` or `!` excludes:        `workstation* -*website*`
 *   - A query of only exclude terms starts from every app.
 *
 * Each term is tested against every field supplied for the app — its id, its
 * display title and the names of the groups it belongs to — so `Workstation*`
 * matches the title "Workstation OpsAPI" even though the id is lower-case.
 */

export type AppQuery = {
  include: RegExp[];
  exclude: RegExp[];
  /** True when the query has no usable terms (empty or only separators). */
  empty: boolean;
};

function escapeRegExp(s: string): string {
  return s.replace(/[.+^${}()|[\]\\]/g, "\\$&");
}

function termToRegExp(term: string): RegExp {
  if (/[*?]/.test(term)) {
    const body = escapeRegExp(term).replace(/\*/g, ".*").replace(/\?/g, ".");
    return new RegExp(`^${body}$`, "i");
  }
  return new RegExp(escapeRegExp(term), "i");
}

export function parseAppQuery(raw: string): AppQuery {
  const include: RegExp[] = [];
  const exclude: RegExp[] = [];
  for (const token of raw.split(/[\s,]+/)) {
    if (!token) continue;
    const negated = token.startsWith("-") || token.startsWith("!");
    const term = negated ? token.slice(1) : token;
    // A bare "-" / "!" / "*" while typing filters nothing rather than everything.
    if (!term || /^\*+$/.test(term)) continue;
    (negated ? exclude : include).push(termToRegExp(term));
  }
  return { include, exclude, empty: include.length === 0 && exclude.length === 0 };
}

/** Does an app, described by its searchable fields, pass the query? */
export function matchesAppQuery(query: AppQuery, fields: string[]): boolean {
  if (query.empty) return true;
  const hit = (re: RegExp) => fields.some((f) => f && re.test(f));
  if (query.exclude.some(hit)) return false;
  return query.include.length === 0 || query.include.some(hit);
}
