# Review / decision memo

Solo Warm Archive review-decision variant: use the warm paper palette from `assets/base.css`; keep surfaces softly bordered, restrained, and decision-first.

**Use when** you need to present analysis, a status write-up, a multi-perspective review, or a
plan/architecture proposal so the user can **read it, mark it up, and adjudicate decisions**
before discussing — i.e. the HTML is a decision-forcing function, not a passive dump.

## Section order (recommended)
1. **Header** + `.meta-bar` (date · branch/commit · scope).
2. **`needs-input`** — the real open questions only (see guardrail below).
3. **Executive summary** — the conclusion first, in 2–4 sentences.
4. **Body** — the analysis, organized as either a long single page (with sidebar) or tabs (see "Layout").
5. **Verdict** — `.verdict go|pursue|stop` with a `.verdict-badge` and the tradeoffs.
6. **Decisions** — a numbered list; **each decision carries an editable comment box** for the user.
7. **Evidence** — quote model/source output in `<blockquote>`; attribute it.
8. **Paste-ready output** — any code/config/text the user should apply themselves, each with a copy button (non-destructive: show, don't apply).
9. **Provenance footer** (mandatory).

## Decision + comment (the signature block)
```html
<div class="decision-item">
  <div class="decision-q">D1 · Should we phase the cache rollout?</div>
  <p>Context / recommendation in one or two lines.</p>
  <label><input type="checkbox" data-persist data-solo-comment data-question="D1 · Should we phase the cache rollout?" id="d1"> Request this follow-up on reject</label>
  <div class="commentbox" contenteditable data-persist data-solo-comment data-question="D1 · Should we phase the cache rollout?" id="d1-comment"></div>
</div>
```
`data-persist` + `id` make the comment survive a reload (via `initPersist`). `data-solo-comment`
marks text that is appended to the reject reason. Add `data-question` so reject comments include
both the question and the user's answer. Verdict card:
```html
<div class="verdict pursue"><span class="verdict-badge">Pursue — with pivot</span>
  <p>One-paragraph recommendation and the main tradeoff.</p></div>
```
Review action buttons, when the artifact is embedded inside Solo:
```html
<div class="review-actions">
  <textarea id="rejectReason" data-persist data-solo-comment placeholder="Rejection reason (required)"></textarea>
  <button class="btn success" data-solo-action="accept" data-task-id="TASK_ID">Accept</button>
  <button class="btn warning" data-solo-action="reject" data-task-id="TASK_ID" data-reason="#rejectReason">Reject & submit comment</button>
</div>
```
Status badges for items: `badge pass|fail|pend|new|exists`. Paste-ready block:
```html
<pre id="snip1"><code>…</code></pre>
<button class="copy-btn" data-copy="#snip1">Copy</button>
```

## Layout — pick one
- **Long single page + sidebar** (default for one coherent document): wrap in
  `<div class="layout"><nav class="sidebar">…anchor links…</nav><main>…sections…</main></div>`,
  add `<button class="btn back-to-top" data-top>↑</button>`. Drives `initScrollspy`.
- **Tabs** (when the content is long enough that one scroll causes fatigue, or splits into clear
  categories): `.tab-bar > .tab-btn[data-tab="x"]` + `.tab-panel#x[data-accent="#hex"]`, with an
  Overview tab first. Drives `initTabs` (hash-routed, deep-linkable, per-tab accent). Prefer this
  only when a single page is genuinely too long — don't tab a short memo.

## Interactivity
Include: `initCopy`, `initPersist`, `initSoloReviewActions`, plus `initScrollspy` **or** `initTabs` per layout. Add `initPrint` only when a print/PDF control is useful.

## Guardrails (enforced)
- **Surface only REAL open questions.** Do not pad `needs-input` with decisions already resolved
  in the plan, and do not propose unnecessary phasing/scope. If nothing genuinely needs the user,
  say so. Ask *true* questions.
- **Non-destructive**: never modify the user's files from the page; provide paste-ready text + a
  copy button, and state plainly what was and wasn't touched.
- **Redact PII** (name/DOB/passport/PNR/keys) for any life-admin or external-facing content, and
  add a "review before you send" note when output is meant to be sent.
- Add a **legend** wherever a badge color or letter encodes status.
