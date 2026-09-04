# Human Interface Guidelines Skill

[![ci](https://github.com/zanwei/human-interface-guidelines-skill/actions/workflows/ci.yml/badge.svg)](https://github.com/zanwei/human-interface-guidelines-skill/actions/workflows/ci.yml)

Apply Apple's Human Interface Guidelines (HIG) to design, review, and specify UI/UX for iOS, iPadOS, macOS, watchOS, tvOS, and visionOS.

## Prerequisites

- Python 3.10+
- `pip install -r requirements.txt`

## Install (Remote)

```bash
npx skills add zanwei/human-interface-guidelines-skill --skill human-interface-guidelines
```

## Install (Local, no owner/repo)

```bash
cd ~/.cursor/skills
npx skills add ./human-interface-guidelines --skill human-interface-guidelines -a cursor -g -y
```

## Verify

```bash
npx skills list -g -a cursor
python3 scripts/search_hig.py --top 3 "apple pay"
python3 scripts/fetch_hig_page.py --help
```

## Update

```bash
npx skills check
npx skills update
```

## Includes

- `SKILL.md`: operational workflow and output templates
- `references/hig_section_map.md`: HIG hierarchy map
- `references/hig_catalog.md`: page catalog with title + abstract
- `scripts/search_hig.py`: keyword search on local catalog
- `scripts/fetch_hig_page.py`: fetch and render HIG DocC pages to Markdown

## Usage examples

- "Review this iOS settings page against HIG and list must-fix issues."
- "Convert this PRD into a HIG-aligned iOS UI spec."
- "Pick the right component pattern for a visionOS picker flow."
