---
name: frontend-design
description: >-
  Distinctive, intentional frontend visual design for web UI — typography,
  atmosphere, hero composition, motion hierarchy, and anti-AI-slop aesthetics.
  Use when building or reshaping landing pages, marketing surfaces, dashboards,
  control-room consoles, DIY Ring Promoter UI, solar/Earth rings stages,
  orbital visualizations, fleet-system views, or any web frontend that must
  feel product-specific rather than templated.
---

# Frontend Design

Act as design lead for a studio hired to make this product unmistakable. The
client rejected templated proposals. Make deliberate palette, type, and layout
choices grounded in the brief — and take one justified aesthetic risk.

**Ring Promoter theme lock:** keep the dark control-room look — near-black
(`#090909` / `#07070a`), emerald accents (`#22c55e`), neutral type. Do **not**
introduce purple/indigo gradients, Inter/Roboto/Arial defaults, or a blue-violet
restyle (reverted in PR #31). Layout, motion, typography, hierarchy, and
composition may change freely within that palette.

## When this skill applies

Landing pages, marketing heroes, DIY console (`ring-promoter.diytaxreturn.co.uk`),
Rings of Applications / Earth globe / solar-system stages, fleet overviews, and
any web UI polish pass.

## Ground it in the subject

Pin subject, audience, and the page's single job before designing. Distinctive
choices come from the product's world: rings, promotion hops, Earth as hub,
Sun = Ring Promoter, isolated app orbits, live TTFB, control-room ops.

## Hard composition rules (landing / promo)

1. **One composition** in the first viewport — not a dashboard.
2. **Brand first** — product name is hero-level, not only nav chrome.
3. **Hero budget** — brand, one headline, one supporting sentence, one CTA group,
   one dominant visual. No stats strips, schedules, or secondary promos in fold.
4. **Full-bleed visual plane** for Earth/orbital heroes — edge-to-edge stage,
   not inset cards or floating media tiles.
5. **No hero overlays** — no floating badges, promo stickers, or info chips on
   media.
6. **Cards only for interaction** — default: no cards. Hero never uses cards.
7. **One job per section** — one purpose, one headline, usually one lede.

## Anti-slop (avoid unless the brief demands it)

- Purple-on-white / purple→indigo gradients
- Inter / Roboto / Arial / generic system stacks for display
- Warm cream (#F4F1EA) + terracotta serif brochure look
- Broadsheet hairline newspaper columns
- Identical rounded SaaS cards with soft grey shadows
- ALL-CAPS tracked eyebrows on every heading
- Scattered fade-up-on-scroll on every section
- Glow spam, rounded-full pill clusters, emoji decoration

## Process

1. **Token plan** — 4–6 named hex values; display + body (+ mono for data);
   one-sentence layout concept; one signature element.
2. **Critique** — if it would look the same for any SaaS brief, revise.
3. **Build** from the plan; keep CSS specificity clean.
4. **Quality floor** — mobile + desktop, focus rings, `prefers-reduced-motion`.

## Motion (2–3 intentional)

Prefer one orchestrated entrance, one ambient loop (e.g. Earth spin, corona
breathe, star twinkle), and one interaction response (focus zoom, hover). Gate
infinite loops on reduced motion. Do not animate everything.

## Full-viewport data stages (Earth / rings)

- Stage fills usable viewport; chrome (roster, view toggle) is secondary.
- **Sun = Ring Promoter** — clear, readable hub label; not a tiny afterthought.
- Isolated rings must stay separable; labels collision-aware with leaders.
- Hierarchy: hub → Earth → active ring/body → roster list.
- Dim non-active orbits; brighten focused ring; roster mirrors focus.

## Ring Promoter tokens (locked)

| Role | Value |
|------|--------|
| Void | `#090909` / `#07070a` |
| Emerald | `#22c55e` |
| Type primary | neutral-50 / neutral-100 |
| Type mute | neutral-400 / neutral-500 |
| Display | Space Grotesk (`--font-display`) |
| Mono | JetBrains Mono |
| Body | Geist Sans |

## Checklist before done

- [ ] Brand readable without relying on nav alone
- [ ] Theme still dark/emerald (no palette drift)
- [ ] 2–3 motions max, reduced-motion safe
- [ ] Desktop + mobile readable
- [ ] Earth/rings: Sun labeled, rings isolatable, roster usable
- [ ] No AI-slop defaults from the anti-slop list

## Related

- For marketing-page section structure and CTA flow, also read
  [landing-page](../landing-page/SKILL.md).
