# SOLO Editor-Inspired Light Skins

**Status:** Approved for implementation
**Date:** 2026-07-15

## Goal

Keep the existing Classic Sunlight skin unchanged. Replace the other eight light palettes with recognizable editor-inspired colors while preserving SOLO's neo-brutalist black borders, hard shadows, white cards, and current browser-only persistence.

## Approved Direction

Use “SOLO editor edition”: borrow each editor theme's canvas and signature colors, then lift saturated colors enough to remain legible as large navigation, button, badge, and status surfaces with black foreground text. This is adaptation, not a pixel-for-pixel copy of an editor UI.

## Palette Lineup

Existing internal IDs remain stable so saved browser preferences continue to work. Only displayed names and CSS tokens change.

| Existing ID | Display name | Canvas | Primary | Accent | Success | Danger |
| --- | --- | --- | --- | --- | --- | --- |
| `classic` | Classic Sunlight | `#FFFAEF` | `#FFD23F` | `#FF6B6B` | `#88D498` | `#F97264` |
| `blueprint` | Light Modern | `#FFFFFF` | `#74B4EE` | `#C88BDD` | `#79C69A` | `#F0A6A0` |
| `ultraviolet` | GitHub Light | `#F6F8FA` | `#54AEFF` | `#D2A8FF` | `#56D364` | `#FF7B72` |
| `seasalt` | Quiet Light | `#F5F5F5` | `#C4B7D7` | `#91B3E0` | `#B8CF9E` | `#E9AAA3` |
| `tomato` | Solarized Light | `#FDF6E3` | `#73B8E6` | `#E2C66F` | `#A9B858` | `#E27D61` |
| `matcha` | Ayu Light | `#FAFAFA` | `#74BAF0` | `#C6A1E4` | `#B0CD61` | `#F39A9A` |
| `bubblegum` | Catppuccin Latte | `#EFF1F5` | `#89B4FA` | `#CBA6F7` | `#A6E3A1` | `#F38BA8` |
| `lavender` | Rosé Pine Dawn | `#FAF4ED` | `#9CCFD8` | `#C4A7E7` | `#A3BEA3` | `#EBBCBA` |
| `sky` | Gruvbox Light | `#FBF1C7` | `#83A598` | `#D3869B` | `#B8BB26` | `#FB4934` |

Supporting info, warning, violet, muted, and light-surface tokens will use the same source palette and remain visually subordinate to the five core colors above.

## Implementation

- Keep `themeOptions`, `data-skin`, pre-hydration restore, and `localStorage` behavior unchanged.
- Replace only the eight non-classic CSS variable blocks in `globals.brutal.css`.
- Replace the eight non-classic English and Chinese display names in `i18n.ts`.
- Keep the settings grid, selection behavior, responsive layout, black structure colors, and default `classic` ID unchanged.
- Add no dependency, backend setting, migration, custom color picker, or dark mode.

## Accessibility and Verification

- Black foreground text on every saturated semantic color must meet WCAG AA contrast of at least 4.5:1 for normal text.
- The existing theme source check must assert the approved names and core hex values, creating a regression check for this refresh.
- Production build and i18n checks must pass.
- Chromium end-to-end verification must cover all nine previews, instant switching, reload persistence, invalid-value fallback, responsive settings layout, and no new console errors.
- Final self-review must confirm that only palette/name/test files changed and that `AGENTS.md` remains untouched.

## References

- VS Code Light Modern and Quiet Light
- GitHub Light Default
- Solarized Light
- Ayu Light
- Catppuccin Latte
- Rosé Pine Dawn
- Gruvbox Light

These names describe visual inspiration; SOLO retains its own component styling and semantic color roles.
