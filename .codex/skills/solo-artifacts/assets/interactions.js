/* ============================================================================
   solo-artifacts interaction modules — STATIC-FIRST: include only the modules the
   page actually needs (inline into a <script> tag). Every module is a no-op when
   its markup is absent, so pasting the whole file is harmless. No dependencies,
   no network. Each module is keyed to the HTML hooks documented in the references.
   ========================================================================== */

/* THEME TOGGLE + PRINT ------------------------------------------------------
   Needs: <button class="btn" data-theme-toggle>🌗</button>
          <button class="btn" data-print>⎙</button>  (optional)
   Restores saved theme before paint via the inline IIFE in starter.html. */
function initTheme() {
  const KEY = "solo-artifact-theme";
  const root = document.documentElement;
  const btn = document.querySelector("[data-theme-toggle]");
  if (btn) btn.addEventListener("click", () => {
    const next = root.getAttribute("data-theme") === "dark" ? "light" : "dark";
    root.setAttribute("data-theme", next);
    try { localStorage.setItem(KEY, next); } catch (e) {}
  });
  const p = document.querySelector("[data-print]");
  if (p) p.addEventListener("click", () => window.print());
}

/* TABS (hash-routed, deep-linkable, per-tab accent) -------------------------
   Needs: .tab-bar > .tab-btn[data-tab="x"]  and  .tab-panel#x[data-accent="#hex"] */
function initTabs() {
  const btns = [...document.querySelectorAll(".tab-btn[data-tab]")];
  if (!btns.length) return;
  const panels = [...document.querySelectorAll(".tab-panel")];
  function activate(id, push) {
    const btn = btns.find(b => b.dataset.tab === id) || btns[0];
    id = btn.dataset.tab;
    btns.forEach(b => b.classList.toggle("active", b === btn));
    panels.forEach(p => p.classList.toggle("active", p.id === id));
    const panel = document.getElementById(id);
    if (panel && panel.dataset.accent) document.documentElement.style.setProperty("--accent", panel.dataset.accent);
    btn.scrollIntoView({ inline: "center", block: "nearest" });
    if (push) history.replaceState(null, "", "#" + id);
  }
  btns.forEach(b => b.addEventListener("click", () => activate(b.dataset.tab, true)));
  // cross-tab anchor interception: links to an element inside another tab switch first
  document.addEventListener("click", e => {
    const a = e.target.closest('a[href^="#"]');
    if (!a) return;
    const el = document.getElementById(a.getAttribute("href").slice(1));
    const panel = el && el.closest(".tab-panel");
    if (panel && !panel.classList.contains("active")) {
      e.preventDefault();
      activate(panel.id, true);
      setTimeout(() => el.scrollIntoView({ behavior: "smooth" }), 80);
    }
  });
  window.addEventListener("hashchange", () => activate(location.hash.slice(1), false));
  activate(location.hash.slice(1), false);
}

/* SORTABLE + FILTERABLE TABLES ---------------------------------------------
   Sort: <th data-sort> (add data-sort="num" for numeric, data-goal="min|max").
   Filter: <div class="filter-bar" data-filter-for="tableId">
             <button class="chip active" data-filter="all">All</button>
             <button class="chip" data-filter="agent">Agent</button> ...
           rows tagged <tr data-type="agent">. */
function initTables() {
  document.querySelectorAll("table").forEach(table => {
    table.querySelectorAll("th[data-sort]").forEach((th, col) => {
      th.addEventListener("click", () => {
        const tb = table.tBodies[0];
        const rows = [...tb.rows];
        const numeric = th.dataset.sort === "num";
        const asc = !th.classList.contains("asc");
        table.querySelectorAll("th").forEach(h => h.classList.remove("asc", "desc"));
        th.classList.add(asc ? "asc" : "desc");
        rows.sort((a, b) => {
          let x = a.cells[col]?.innerText.trim() ?? "", y = b.cells[col]?.innerText.trim() ?? "";
          if (numeric) { x = parseFloat(x.replace(/[^0-9.\-]/g, "")); y = parseFloat(y.replace(/[^0-9.\-]/g, "")); x = isNaN(x) ? -Infinity : x; y = isNaN(y) ? -Infinity : y; return asc ? x - y : y - x; }
          return asc ? x.localeCompare(y) : y.localeCompare(x);
        });
        rows.forEach(r => tb.appendChild(r));
      });
    });
  });
  document.querySelectorAll(".filter-bar[data-filter-for]").forEach(bar => {
    const table = document.getElementById(bar.dataset.filterFor);
    if (!table) return;
    bar.addEventListener("click", e => {
      const chip = e.target.closest(".chip"); if (!chip) return;
      bar.querySelectorAll(".chip").forEach(c => c.classList.toggle("active", c === chip));
      const f = chip.dataset.filter;
      [...table.tBodies[0].rows].forEach(r => { r.style.display = (f === "all" || r.dataset.type === f) ? "" : "none"; });
    });
  });
}

/* LIGHTBOX (click any .img-grid/.filmstrip img, or [data-zoom]) -------------- */
function initLightbox() {
  const imgs = document.querySelectorAll(".img-grid img, .filmstrip img, [data-zoom]");
  if (!imgs.length) return;
  let dlg = document.querySelector("dialog.lightbox");
  if (!dlg) {
    dlg = document.createElement("dialog");
    dlg.className = "lightbox";
    dlg.innerHTML = '<img alt=""><div class="cap"></div>';
    document.body.appendChild(dlg);
    dlg.addEventListener("click", () => dlg.close());
  }
  const big = dlg.querySelector("img"), cap = dlg.querySelector(".cap");
  imgs.forEach(im => im.addEventListener("click", () => {
    big.src = im.currentSrc || im.src;
    cap.textContent = im.dataset.caption || im.alt || "";
    dlg.showModal();
  }));
}

/* COPY-TO-CLIPBOARD ---------------------------------------------------------
   Needs: <button class="copy-btn" data-copy="#sourceId or literal text">Copy</button> */
function initCopy() {
  document.querySelectorAll(".copy-btn[data-copy]").forEach(btn => {
    btn.addEventListener("click", async () => {
      const v = btn.dataset.copy;
      const src = v.startsWith("#") ? document.querySelector(v) : null;
      const text = src ? src.innerText : v;
      try { await navigator.clipboard.writeText(text); } catch (e) {}
      const old = btn.textContent; btn.textContent = "Copied ✓"; btn.classList.add("copied");
      setTimeout(() => { btn.textContent = old; btn.classList.remove("copied"); }, 1400);
    });
  });
}

/* SIDEBAR SCROLLSPY + BACK-TO-TOP (long-form review) ------------------------
   Needs: .sidebar a[href="#sectionId"], sections with matching id, and
          <button class="btn back-to-top" data-top>↑</button> (optional). */
function initScrollspy() {
  const links = [...document.querySelectorAll(".sidebar a[href^='#']")];
  if (links.length) {
    const ids = links.map(a => a.getAttribute("href").slice(1));
    const onScroll = () => {
      let cur = ids[0];
      for (const id of ids) { const el = document.getElementById(id); if (el && el.getBoundingClientRect().top <= 120) cur = id; }
      links.forEach(a => a.classList.toggle("active", a.getAttribute("href") === "#" + cur));
    };
    window.addEventListener("scroll", onScroll, { passive: true }); onScroll();
  }
  const top = document.querySelector("[data-top]");
  if (top) {
    top.addEventListener("click", () => window.scrollTo({ top: 0, behavior: "smooth" }));
    window.addEventListener("scroll", () => top.classList.toggle("visible", window.scrollY > 400), { passive: true });
  }
}

/* PERSIST MARKUP (review type) ---------------------------------------------
   Saves the text of every [contenteditable][data-persist], and the state of
   <input data-persist>, to localStorage keyed by id, so the reviewer's comments
   and checkboxes survive a reload. */
function initPersist() {
  const els = document.querySelectorAll("[data-persist]");
  els.forEach(el => {
    const key = "wc-" + (el.id || el.dataset.persist);
    try {
      const saved = localStorage.getItem(key);
      if (saved !== null) { if (el.type === "checkbox") el.checked = saved === "1"; else el.textContent = saved; }
    } catch (e) {}
    const save = () => { try { localStorage.setItem(key, el.type === "checkbox" ? (el.checked ? "1" : "0") : el.textContent); } catch (e) {} };
    el.addEventListener(el.type === "checkbox" ? "change" : "input", save);
  });
}

document.addEventListener("DOMContentLoaded", () => {
  initTheme(); initTabs(); initTables(); initLightbox(); initCopy(); initScrollspy(); initPersist();
});
