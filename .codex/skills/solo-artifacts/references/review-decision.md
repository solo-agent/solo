# Solo-brutal review-decision

Use `review-decision` when a Solo task needs a readable artifact for review, acceptance, rejection, or product/technical decisions. This is the default Solo artifact type.

## Section Order

1. **Header** — title, one-line conclusion, task/status metadata.
2. **What needs your input** — only real user decisions. If none, say "No user input needed right now."
3. **Executive summary** — conclusion first, 2-4 short sentences.
4. **Verdict** — `.verdict go|pursue|stop`, with tradeoffs.
5. **Decisions** — numbered decision cards with editable comment boxes.
6. **Evidence** — short attributed quotes or facts from the task/thread.
7. **Paste-ready output** — code/config/text the user may apply manually, with copy buttons.
8. **Provenance footer** — task id, thread/channel, agent, model, date.

## Components

Decision card:
```html
<div class="decision-item">
  <div class="decision-q">D1 - Should we accept this implementation?</div>
  <p>Recommendation: accept after the listed follow-up is tracked.</p>
  <div class="commentbox" contenteditable data-persist id="decision-d1"></div>
</div>
```

Verdict:
```html
<div class="verdict pursue">
  <span class="verdict-badge">Pursue - with one follow-up</span>
  <p>The implementation is directionally right; the remaining tradeoff is operational polish.</p>
</div>
```

Evidence:
```html
<blockquote>
  <p>Short quote or fact from the thread.</p>
  <cite>Task thread - message id or agent name</cite>
</blockquote>
```

Paste-ready output:
```html
<pre id="next-step"><code>solo task accept -n 12 -c channel-id</code></pre>
<button class="copy-btn" data-copy="#next-step">Copy</button>
```

## Layout

Use a single page by default. Use tabs only when there are three or more dense sections that would make one scroll hard to scan.

## Interactivity

Include `initTheme`, `initCopy`, `initPersist`. Add `initScrollspy` only for long pages; add `initTabs` only when using tabs.

## Guardrails

- Do not dump the full thread. Summarize the decision-relevant facts.
- Do not invent open questions. If the work is ready, say that.
- Keep labels plain. If a badge/color encodes meaning, include a legend.
- Do not let the page modify Solo data. It may show commands or copyable text only.
