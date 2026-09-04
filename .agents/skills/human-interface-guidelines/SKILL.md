---
name: human-interface-guidelines
description: Apply Apple's Human Interface Guidelines (HIG) to design, review, and specify UI/UX for iOS, iPadOS, macOS, watchOS, tvOS, and visionOS. Use when choosing platform-native patterns, reviewing a design against HIG, converting PRD to HIG-aligned UI specs, or finding the right HIG page quickly.
---

# Human Interface Guidelines (HIG) skill

Use this skill as an operational wrapper around Apple's HIG: navigate guideline content quickly, extract the relevant recommendations, and convert them into actionable UI decisions.

## Golden rules

- Prefer platform conventions over custom UI.
- Optimize for clarity, deference, and depth.
- Always state platform + context + user goal before recommending UI.
- When uncertain, cite the specific HIG URL.

## Workflow

### 1) Identify design context

Capture:
- Platform(s): iOS / iPadOS / macOS / watchOS / tvOS / visionOS
- Form factor: phone / tablet / desktop / TV / spatial
- Input: touch / keyboard / pointer / crown / remote / gaze and hands
- Environment constraints: motion, lighting, distance, accessibility needs
- Primary user goal and top tasks

### 2) Find entry points

Start with:
- `references/hig_section_map.md` for hierarchy
- `references/hig_catalog.md` for title + abstract browsing

### 3) Search candidate pages

- Use `scripts/search_hig.py` to search titles/abstracts by keyword.
- Open candidate URLs for the authoritative content.

### 4) Fetch canonical page content

When abstract is not enough:
- Run `python scripts/fetch_hig_page.py --path "/design/human-interface-guidelines/<slug>"`
- The script outputs best-effort Markdown from Apple's DocC JSON.

### 5) Convert guidance into decisions

Format output as:
- **User goal**
- **Recommended pattern/component** (plus what to avoid)
- **Platform-specific notes**
- **Accessibility considerations**
- **Edge cases** (error/empty/loading/offline/permissions)
- **HIG references** (URLs)

## Common tasks

### HIG review checklist
1. Navigation: clear hierarchy, predictable back/close behavior
2. Layout: spacing, alignment, typography, Dynamic Type readiness
3. Inputs: touch target, keyboard/pointer behavior, focus model
4. Feedback: loading/progress/success/error/confirmation
5. Accessibility: VoiceOver, contrast, motion sensitivity, localization
6. System integrations: share sheet, Sign in with Apple, Apple Pay, widgets

Deliver feedback with:
- Findings
- Severity (must-fix / should-fix / nice-to-have)
- Suggested fix
- HIG citations

### HIG-aligned UI spec outline
- Overview and user stories
- Information architecture
- Screens and states
- Components (with platform variants)
- Motion and transitions
- Copywriting and labels
- Accessibility requirements
- Open questions

## Bundled references

- `references/hig_section_map.md` - hierarchy map
- `references/hig_catalog.md` - pages with title and abstract

## Bundled scripts

- `scripts/search_hig.py` - keyword search on local catalog
- `scripts/fetch_hig_page.py` - fetch and render a HIG page to Markdown
