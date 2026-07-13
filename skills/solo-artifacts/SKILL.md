---
name: solo-artifacts
description: >-
  Use when a Solo task or thread should become an interactive, reviewable,
  self-contained HTML artifact for progress/status, review/decision, or
  comparison/leaderboard work inside Solo.
---

# solo-artifacts

Solo-brutal fork of `work-canvas`: produce or publish **one self-contained HTML file** that renders a Solo task/thread for a human to understand and act on. Prefer the real deliverable over a summary page.

## Workflow

1. **Publish Existing Deliverable** if the current task/run names one.

   Trust only explicit current-task evidence, such as:

   ```text
   Deliverable: ./path/to/result.html
   ```

   You may also use a file path named in the current task/thread, or a file you personally created for this task now. Do not scan the workspace by newest file, mtime, or vague filename match; old workspace files can be stale. If several files are named, prefer the product/final deliverable over review/report/panel files unless the user explicitly asks for the review page. Do not switch to review-decision just because the task is in_review.

   - Existing `.html`: publish it directly.

     ```bash
     solo artifact publish --task <task-id> --mode <latest|final> --file <path-to-html>
     ```

   - Existing `.md`, `.txt`, `.json`, `.csv`, or other text/data: create a small self-contained HTML viewer that renders the exact source content, labels the source file near the top, then publish that wrapper. Do not summarize or rewrite the content.
   - Multiple deliverables or a directory: create a small index artifact with explicit links/file names and a short note about what each file is.

   If a real deliverable exists, stop here.

2. **Pick the artifact type** and read its reference:

   | The task needs to... | Type | Read |
   |---|---|---|
   | Show where long/messy work stands, what broke, what's next | **Progress / status report** | `references/progress-report.md` |
   | Review analysis or implementation and decide accept/reject/next step | **Review / decision memo** | `references/review-decision.md` |
   | Weigh options head-to-head and pick a winner | **Comparison / leaderboard** | `references/comparison.md` |

   If blended, lead with the primary type and borrow blocks from the other.

3. **Assemble from the template.** Copy `assets/starter.html`, then:
   - Replace `[[PASTE base.css]]` with the full contents of `assets/base.css`.
   - Replace `[[PASTE interactions.js]]` with only the needed modules from `assets/interactions.js`; pasting the whole file is fine because modules self-guard.
   - Fill the header, **What needs your input**, and body sections using the component recipes in the chosen reference.

4. **Embed everything**: inline CSS/JS, embed media as `data:` URIs, no CDN or external requests.

5. **Publish back to Solo**:

   ```bash
   solo artifact publish --task <task-id> --mode <latest|final> --file <path-to-html>
   ```

## Non-Negotiables

- Keep work-canvas structure and interactions intact; only the visual skin is Solo-brutal.
- Real deliverables beat summaries. Never replace an explicit deliverable file with a conversation recap.
- Surface only real human decisions. If nothing needs the user, say so.
- Add a legend wherever color, letters, or symbols encode meaning.
- Never modify Solo/source data from the page. Show paste-ready output with copy buttons.
- Include a provenance footer with task id, thread/channel scope, agent, model, and generation date when available.

## Files

- `assets/starter.html` — the skeleton to copy.
- `assets/base.css` — class-compatible Solo-brutal design system.
- `assets/interactions.js` — optional vanilla-JS modules: print, tabs, tables, lightbox, copy, scrollspy, persist.
- `references/*.md` — work-canvas section order, component recipes, and guardrails.
