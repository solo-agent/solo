---
name: solo-artifacts
description: Use when a Solo task or thread needs to become a reviewable artifact, decision memo, progress board, comparison board, or self-contained HTML summary for the user to inspect inside Solo or open offline.
---

# solo-artifacts

Produce **one self-contained HTML artifact** for a Solo task/thread. The artifact is a human review surface, not a transcript dump and not a product UI.

## Default Path

For task artifacts, default to **review-decision** unless the user explicitly asks for a progress report or comparison.

| User needs | Type | Read |
|---|---|---|
| Decide whether work is acceptable, review a plan, mark up conclusions | `review-decision` | `references/review-decision.md` |
| Understand long-running progress, failures, blockers, next commands | `progress-report` | `references/progress-report.md` |
| Compare options, models, pipelines, vendors, or outputs | `comparison` | `references/comparison.md` |

## Workflow

1. Pick the artifact type.
2. Copy `assets/starter.html`.
3. Inline the full `assets/base.css`.
4. Inline only needed modules from `assets/interactions.js`; using the whole file is acceptable because modules self-guard.
5. Fill the header, **What needs your input**, and body sections from the chosen reference.
6. Output a single HTML file.

## Solo Rules

- Use the Solo-brutal visual system from `assets/base.css`; do not invent a new theme.
- Put conclusions before evidence.
- List only real human decisions in **What needs your input**. If none exist, say none.
- Do not modify source files from the page. Show paste-ready output with copy buttons.
- Keep the artifact self-contained: inline CSS/JS, embed media as `data:` URIs, no CDN, no external requests.
- Include a provenance footer with task id, thread/channel scope, agent, model, and generation date when available.

## Files

- `assets/starter.html` — Solo-brutal skeleton.
- `assets/base.css` — Solo-brutal design system for artifacts.
- `assets/interactions.js` — optional vanilla JS modules: theme, tabs, tables, lightbox, copy, scrollspy, persist.
- `references/*.md` — section order and component recipes.
