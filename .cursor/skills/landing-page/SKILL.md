---
name: landing-page
description: >-
  State-of-the-art marketing and product landing pages — hero composition,
  brand presence, section rhythm, CTAs, and motion. Use when editing
  web/src/app/landing/, landing components, GitHub Pages marketing surface,
  DIY Ring Promoter promo pages, Watch/YouTube sections, or any web landing /
  marketing UI for Ring Promoter or similar DevOps control-plane products.
---

# Landing Page

Build landings that sell one idea in one scroll story. Pair with
[frontend-design](../frontend-design/SKILL.md) for visual direction.

## Ring Promoter landing brief

- **Subject:** release promotion control plane (int → test → acc → prod).
- **Audience:** platform / DevOps engineers deploying K8s and VM apps.
- **Job:** explain the protocol, prove it with live-feeling UI, drive
  "Open the console" / GitHub.
- **Look:** dark control room, emerald signal lights, monospace data crumbs.
  Theme locked — see frontend-design skill.

## First viewport

```
┌─────────────────────────────────────────┐
│  brand (hero-level)                     │
│  one headline                           │
│  one supporting sentence                │
│  primary + secondary CTA                │
│  one dominant visual (sim / stage)      │
└─────────────────────────────────────────┘
```

- Brand name must survive the "remove the nav" test.
- Do not pack Watch, FactStrip, Protocol, or FAQ into the hero.
- Prefer full-bleed or edge-dominant visual — not a card collage.

## Section rhythm

| Order | Section | One job |
|-------|---------|---------|
| 1 | Hero | Thesis + CTA |
| 2 | Watch | Product video (YouTube) |
| 3 | Facts | Sparse proof points |
| 4 | Protocol | How promotion works |
| 5+ | Features | Auto-promote, live ops, gates, deployers, config |
| Last | FAQ → Closing CTA | Objections then convert |

Each section: one eyebrow (only if useful), one H2, one lede, one visual or
list — not all three competing.

## Typography & chrome

- Display: Space Grotesk; body: Geist; data: JetBrains Mono.
- Nav: sticky, quiet blur; brand mark + name left; CTAs right.
- Primary CTA: light fill on dark (`neutral-100` on void).
- Secondary: hairline border, no purple buttons.

## Motion

Landing uses `ls-*` CSS entrances (mask lines, rise). Keep to hero + a few
reveals. Respect `prefers-reduced-motion`. Do not GSAP every block.

## Copy

- Active voice, sentence case, plain verbs.
- Name user outcomes ("earns production"), not internal schema.
- CTAs: "Open the console", "Read the deployment model" — not "Submit" / "Learn more".

## DIY / console crossover

When the landing links into the Earth/rings console, match void colors and
emerald so the handoff feels continuous. Do not restyle the console theme.

## Done when

- [ ] Hero brand + headline + lede + CTAs + one visual only
- [ ] Watch section intact with embedded video
- [ ] Mobile: readable type, stacked CTAs, no horizontal clip
- [ ] Dark/emerald preserved
- [ ] `npm run build:embed` if shipping with the Go binary
