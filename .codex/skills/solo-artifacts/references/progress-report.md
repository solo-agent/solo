# Progress / status report

**Use when** a task is long-running, spans multiple subsystems, or involves trial-and-error,
and the user wants to grasp the whole picture, the progress, what went wrong, and what needs
their input — without reading terminal logs. (The canonical example: a benchmark reproduction
that took days of debugging.)

## Section order (recommended)
1. **Header** — `h1` + `.subtitle` (one-line thesis) + `.meta-bar` (date · scope · model/host · status).
2. **`needs-input`** — only the genuine decisions/inputs you need, each with the concrete next command.
3. **KPI cards** — `.grid` of `.card` (one headline number each: resolution rate, count done, etc.), with a `.progress-fill`.
4. **Diagnostic callout** — the "Why is the number X and not Y?" box; root causes as a short list. This is the single highest-value block — it turns a metric into an explanation.
5. **Results / per-unit tables** — breakdown by category or sub-unit, with inline bars.
6. **Timeline** — the run + the *debugging journey* (include the pitfalls: what broke, when, the fix). Color the dots by outcome.
7. **Action items / TODO** — each with a one-line concrete next step or command.
8. **Quick-reference commands** — a table of commands the user can run themselves to re-check.
9. **Provenance footer** (mandatory).

## Components → classes
- Status vocabulary: `<span class="badge done|running|blocked|waiting"><span class="dot live"></span>…</span>` (`.dot.live` pulses — use for the in-progress item only).
- KPI card:
  ```html
  <div class="card"><div class="card-title">Tasks resolved</div>
    <div class="card-value" style="color:var(--green)">305/500</div>
    <div class="card-detail">61.0%</div>
    <div class="progress-bar"><div class="progress-fill green" style="width:61%"></div></div></div>
  ```
- Diagnostic callout:
  ```html
  <div class="callout warn"><div class="callout-title">Why 61% and not the ~80% target?</div>
    <ul><li><b>Tooling misconfig (main cause):</b> 474/500 runs hit avoidable tool failures.</li> …</ul></div>
  ```
- Timeline item:
  ```html
  <div class="timeline-item"><div class="timeline-dot" style="background:var(--red)"></div>
    <div class="timeline-date">Jun 13</div><div class="timeline-title">266 eval errors</div>
    <div class="timeline-desc">Docker images missing for sphinx-doc, sympy.</div></div>
  ```
- Tables go in `.table-wrap`; numeric cells use `class="num"`; commands in `<code>`.

## Interactivity (static-first)
Default to **zero JS** — this type is usually read-only. The only common module is the theme
toggle/print toolbar (`initTheme`). Include it only if useful.

## Workflow notes
- **Update in place**: regenerate the *same* file as the run progresses so the link stays stable.
  **Finalize only once all runs are complete** — don't ship a half-filled comparison.
- For long jobs, this can be built by a cheap sub-agent to save main-model tokens.
- Theme: dashboards read well in **dark**; set `data-theme="dark"` on `<html>` if the user prefers the dashboard look, otherwise keep the light default.
