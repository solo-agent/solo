# Comparison / leaderboard

**Use when** the user wants to weigh options head-to-head — models, pipelines, approaches,
configs — and **draw a conclusion / pick a winner**. Includes ranked leaderboards and A/B galleries.

## Section order (recommended)
1. **Header** + `.meta-bar`.
2. **`needs-input`** — usually one question: "which do we ship?" plus anything blocking the call.
3. **Legend** — map each option to its color/symbol (mandatory: color encodes identity here).
4. **Verdict** — `.verdict go` naming the winner *and the tradeoffs* up front.
5. **Summary / leaderboard table** — one row per option; sortable + filterable; highlight the best cell per column.
6. **Per-item detail** — side-by-side columns, or an image grid / filmstrip with lightbox, so the conclusion is backed by the actual samples.
7. **Provenance footer** (mandatory).

## Leaderboard table (sortable + filterable)
```html
<div class="filter-bar" data-filter-for="lb">
  <button class="chip active" data-filter="all">All</button>
  <button class="chip" data-filter="agent">Agent</button>
  <button class="chip" data-filter="rag">RAG</button>
</div>
<div class="table-wrap"><table id="lb">
  <thead><tr>
    <th data-sort>System</th>
    <th data-sort="num" data-goal="max" class="num">Score</th>
    <th data-sort="num" data-goal="min" class="num">Tokens</th>
    <th data-sort="num" data-goal="min" class="num">Cost $</th>
    <th data-sort="num" data-goal="min" class="num">Wall s</th>
  </tr></thead>
  <tbody>
    <tr data-type="agent"><td>Option A</td><td class="num best">305</td><td class="num">623M</td><td class="num">…</td><td class="num">…</td></tr>
    …
  </tbody>
</table></div>
```
- **Always include token / cost / time columns** — the user compares options on economics, not just
  correctness. Use `class="num"` (tabular-nums) and mark the winning cell per column `class="best"`.
- Comparison tables grow over time: keep the data **easy to extend** (add a `<tr>` / `<th>`); if there
  are many rows, consider a `const ROWS=[…]` array rendered by a small loop so adding a row is one line.
- Missing/partial data: show `–` (and footnote `†` for partial runs) rather than blanks.

## Side-by-side / media compare
```html
<div class="compare-grid">
  <div class="compare-col winner"><div class="col-head">Arm A ✓</div>
    <div class="img-grid"><img loading="lazy" src="data:…" alt="A1" data-caption="prompt…"></div></div>
  <div class="compare-col"><div class="col-head">Arm B</div>…</div>
</div>
```
Images in `.img-grid`/`.filmstrip` are click-to-zoom via `initLightbox`. Embed as `data:` URIs and
`loading="lazy"`. Agreement/consensus across options → `badge pass|fail|warn` per cell.

## Interactivity
Include: `initTables` (sort + filter), `initLightbox` (if media), `initTheme`. Otherwise static.

## Guardrails (enforced)
- **Reference-backed**: the user can't judge a metric without seeing the artifact — embed the actual
  samples next to the scores.
- **Legend + plain labels** for every color/symbol; never show a raw score the reader can't interpret.
- State the verdict's **tradeoffs**, not just the winner.
