# Frontend Redesign v3.0 — Neubrutalist Design System for Solo

> Based on analysis of **neubrutalism.com** (the canonical reference for neubrutalist web design) and the observed production UI at **slock.ai**.
>
> Version 3.0 consolidates learnings from both sources into a repeatable, tokenized design system for Solo's Tailwind v4 + shadcn/ui frontend.
>
> Research date: 2026-05-11. Source: https://neubrutalism.com/ (full CSS + HTML analyzed).

---

## Table of Contents

1. [Design Philosophy](#1-design-philosophy)
2. [Neubrutalism.com Reference Analysis](#2-neubrutalismcom-reference-analysis)
3. [Gap Analysis: neubrutalism.com vs Solo](#3-gap-analysis-neubrutalismcom-vs-solo)
4. [Token System — Tailwind v4 Theme](#4-token-system--tailwind-v4-theme)
5. [Color System](#5-color-system)
6. [Typography](#6-typography)
7. [Border System](#7-border-system)
8. [Shadow System](#8-shadow-system)
9. [Spacing System](#9-spacing-system)
10. [Component Specifications](#10-component-specifications)
11. [Responsive Breakpoints](#11-responsive-breakpoints)
12. [Animation & Transition Rules](#12-animation--transition-rules)
13. [Accessibility](#13-accessibility)
14. [Edge Cases & States](#14-edge-cases--states)
15. [Migration Guide](#15-migration-guide)

---

## 1. Design Philosophy

Solo is a channel-based real-time multi-Agent collaboration platform. The UI implements **Neubrutalist Web Design** as defined by the canonical reference at neubrutalism.com.

### Core Tenets (from neubrutalism.com)

| Tenet | Definition | How Solo applies |
|-------|-----------|-----------------|
| **Graphic bluntness** | High contrast, bold shapes, conspicuous structure | 2px solid black borders, hard offset shadows |
| **Explicitness over subtlety** | The interface declares its presence | Zero border-radius, thick outlines |
| **Personality over invisibility** | Memorable structure over perfect polish | Pink accent, playful lift/press effects |
| **Categorical color** | Flat fills, no gradients, pop-art energy | Flat hex colors, structural palette |
| **Zero blur depth** | Anti-naturalistic shadows, hard offset | `box-shadow: x y 0 0 #000` (spread = 0) |
| **Single stroke width** | One canonical border width for most components | 2px solid #000, 3px for emphasis |

### Design Metaphors

- **Brutalist architecture**: Raw materials, exposed structure, honest design
- **Playful SaaS**: Pink accents, bold typography, lift-on-hover interactions
- **Swiss design precision**: Clean information hierarchy, grid-based layout

### What Changed From v2

| Aspect | v2.0 | v3.0 | Source |
|--------|------|------|--------|
| Default border | 2px solid #000 | 2px solid #000 (kept) | neubrutalism.com uses 3px; Solo stays at 2px for chat-density |
| Thick border | Not defined | 3px solid #000 for hero/prominent | neubrutalism.com `--border-thick: 4px` scaled down |
| Shadow default | 4px 4px 0px #141111 | 5px 5px 0 0 #000 | Canonical neubrutalism 5px 5px, pure black |
| Shadow hover | 6px 6px 0px #141111 | 7px 7px 0 0 #000 | Canonical hover pattern |
| Shadow active | 1px 1px 0px #141111 | translate(3px,3px) + no shadow | Canonical active pattern |
| Text color | #141111 | #000 (pure black) | Canonical uses pure black |
| Body font | Space Grotesk only | Space Grotesk (UI) + Inter (dense text) | neubrutalism.com uses Inter for body |
| Display font | Space Grotesk | Syne 800 (optional, hero/logo only) | neubrutalism.com uses Syne 800 |
| Focus outline | 2px solid black | 3px solid #74B9FF (blue) offset 2px | neubrutalism.com focus pattern |
| Form elements | Native/default | Custom brutalist (checkbox, radio, toggle, select) | neubrutalism.com form patterns |
| Cursor | Default | Custom cursor (optional, brand elements) | neubrutalism.com custom cursor pattern |

---

## 2. neubrutalism.com Reference Analysis

### 2.1 The Site's Own Design Tokens

```css
/* neubrutalism.com — DESIGN TOKENS (exact) */
:root {
  /* Border system */
  --border: 3px solid #000;
  --border-thin: 2px solid #000;
  --border-thick: 4px solid #000;

  /* Shadow system (hard offset, zero blur) */
  --shadow-sm: 3px 3px 0 0 #000;
  --shadow: 5px 5px 0 0 #000;
  --shadow-lg: 8px 8px 0 0 #000;
  --shadow-xl: 12px 12px 0 0 #000;

  /* Radius: zero. That's the point. */
  --radius: 0;

  /* Color palette */
  --black: #000;
  --white: #fff;
  --bg: #FFFDF5;              /* Page background */
  --bg-warm: #f5f0e8;         /* Warm card backgrounds */
  --yellow: #FFD23F;
  --yellow-light: #FFF3C4;
  --pink: #FF6B6B;
  --pink-light: #FFE0E0;
  --blue: #74B9FF;
  --blue-light: #E3F2FD;
  --green: #88D498;
  --green-light: #E8F5E9;
  --orange: #FFA552;
  --orange-light: #FFF0E0;
  --purple: #B8A9FA;
  --purple-light: #F0ECFF;
  --cyan: #7FDBDA;
  --red: #FF4444;

  /* Typography */
  --font-display: 'Syne', sans-serif;        /* Display heads — 800 weight */
  --font-heading: 'Space Grotesk', sans-serif; /* UI headings — 700 weight */
  --font-body: 'Inter', sans-serif;            /* Body text — 400 weight */
  --font-mono: 'Space Mono', monospace;

  /* Spacing */
  --space-xs: 0.5rem;
  --space-sm: 1rem;
  --space-md: 1.5rem;
  --space-lg: 2rem;
  --space-xl: 3rem;
  --space-2xl: 4rem;
  --space-3xl: 6rem;
  --space-4xl: 8rem;
}
```

### 2.2 Component Patterns From the Reference

**Buttons (canonical pattern):**
```css
.btn {
  border: 3px solid #000;        /* --border */
  box-shadow: 5px 5px 0 0 #000; /* --shadow */
  font-family: 'Space Grotesk', sans-serif;
  font-weight: 700;
  transition: transform 0.1s ease, box-shadow 0.1s ease;
}
.btn:hover {
  transform: translate(-2px, -2px);
  box-shadow: 7px 7px 0 0 #000; /* Larger offset on hover */
}
.btn:active {
  transform: translate(3px, 3px);
  box-shadow: none;              /* Presses flat */
}
```

**Cards (canonical pattern):**
```css
.card {
  border: 3px solid #000;
  box-shadow: 5px 5px 0 0 #000;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}
.card:hover {
  transform: translate(-2px, -2px);
  box-shadow: 8px 8px 0 0 #000;
}
```

**Inputs (canonical pattern):**
```css
.nb-input {
  border: 3px solid #000;
  border-radius: 0;
  box-shadow: 3px 3px 0 0 #000;  /* --shadow-sm */
  padding: 0.75rem 1rem;
  font-family: 'Inter', sans-serif;
  font-size: 1rem;
}
.nb-input:focus {
  outline: 3px solid #74B9FF;     /* Focus outline, not border change */
  outline-offset: 2px;
  box-shadow: 5px 5px 0 0 #000;   /* Shadow grows on focus */
  transform: translate(-1px, -1px);
}
```

**Badges (canonical pattern):**
```css
.badge {
  display: inline-block;
  padding: 0.25rem 0.75rem;
  font-family: 'Space Mono', monospace;
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  border: 2px solid #000;           /* --border-thin */
  box-shadow: 3px 3px 0 0 #000;     /* --shadow-sm */
}
```

### 2.3 Three-Tier Shadow System (neubrutalism.com canonical)

| Tier | Offset | Usage |
|------|--------|-------|
| Small | 3px 3px | Badges, chips, inline actions, input resting |
| Medium | 5px 5px | Cards, buttons, panels (default) |
| Large | 8px 8px | Overlays, hero elements, hover state |
| XL | 12px 12px | Modals, hero sections, emphasis |

### 2.4 Responsive Shadow Scaling

neubrutalism.com reduces shadows at 768px breakpoint:
```
--shadow: 5px 5px 0 0 #000  →  4px 4px 0 0 #000
--shadow-lg: 8px 8px  →  6px 6px
--shadow-xl: 12px 12px  →  8px 8px
```

At 320px: further reduction to 3px/5px/6px.

---

## 3. Gap Analysis: neubrutalism.com vs Solo

### 3.1 What Solo Already Has (matches neubrutalism canon)

| Feature | Solo v2 | neubrutalism.com | Match? |
|---------|---------|------------------|--------|
| Zero border-radius | Yes (0px) | Yes (0px) | Aligned |
| Solid black borders | Yes (2px) | Yes (3px default) | Solo lighter for chat density |
| Hard offset shadows | Yes (4px 4px) | Yes (5px 5px) | Close (Solo slightly smaller) |
| Flat colors, no gradients | Yes | Yes | Aligned |
| Thick black dividers | Yes (border-t-2 border-black) | Yes (2-4px) | Aligned |
| Bold typography | Yes (700 bold on UI) | Yes (700 heading, 800 display) | Aligned |
| Space Grotesk | Yes (all UI) | Yes (headings) | Aligned |
| Space Mono | Yes (code) | Yes (code) | Aligned |
| Pink accent | Yes (#fe7da8) | Yes (#FF6B6B) | Different shades, same role |
| lift/press animation | Yes (translate + shadow) | Yes (translate + shadow) | Aligned |
| Focus-visible style | Yes (2px black outline) | Yes (3px blue outline) | v2 is weaker |
| Scrollbar hidden | Yes | N/A | Solo correct for chat |

### 3.2 What Solo Is Missing (gaps from neubrutalism.com)

| Feature | neubrutalism.com | Solo v2 | Priority |
|---------|-----------------|---------|----------|
| **Syne display font** | Syne 800 for hero/logo | Space Grotesk only for all | Low (optional upgrade) |
| **Inter body font** | Inter 400 for body text | Space Grotesk for everything | Medium (legibility) |
| **Input focus: blue outline** | 3px solid #74B9FF offset 2px | 2px solid black outline | High (a11y) |
| **Custom checkbox/radio/toggle** | nb-checkbox, nb-radio, nb-toggle | Native browser defaults | Medium (consistency) |
| **Custom select** | nb-select with SVG arrow | Native select | Low |
| **Badge component** | mono, uppercase, 2px border, shadow-sm | None defined | Medium |
| **Code block style** | dark bg (#1a1a2e), syntax colors, border | None defined | Medium |
| **Toast component** | 4 variants (success/error/info/warning) | None | Medium |
| **Pull quote / callout** | 6px left border, big quote | None | Low |
| **Skip link** | Top of page, yellow bg, border | None | High (a11y) |
| **prefers-reduced-motion** | Disables all lift/scroll animations | None | High (a11y) |
| **Pure black text** | #000 | #141111 | Low (minor diff) |
| **3px default border** | 3px | 2px | Low (chat density > canon) |
| **Custom cursor** | SVG cursor on body and interactive | None | Low (brand flair) |
| **Highlight mark styling** | mark { bg: yellow, border-bottom: 2px black } | None | Low |
| **Selection styling** | ::selection { bg: yellow, color: black } | None | Low |
| **Dividers with color bands** | divider-band variants (yellow, pink, blue...) | None | Low |
| **WCAG contrast checker** | Built-in demo on site | None | Low (docs) |
| **Spacing scale with rem** | xs:0.5 to 4xl:8 | Tailwind default | Solo uses Tailwind scale |

### 3.3 Solo's Custom Variations (deviations from neubrutalism canon)

| Variation | Solo Value | neubrutalism.com Value | Rationale |
|-----------|-----------|----------------------|-----------|
| Pink shade | #fe7da8 (softer) | #FF6B6B (coral) | Brand identity, product's established color |
| Pink accent role | Primary buttons, links | Accent only | Solo uses pink as primary CTA color |
| Cream background | #fffaef | #FFFDF5 | Near-identical (2% difference) |
| Text color | #141111 (near-black) | #000 (pure black) | v2: slightly softer for readability |
| Border width | 2px (default) | 3px (default) | Chat app density: 2px is less visually heavy |
| Shadow default offset | 4px | 5px | Chat app: 4px is sufficient for cards |
| Shadow hover offset | 6px | 7-8px | Slightly less aggressive lift |
| Shadow active offset | 1px (with translate) | None (translate only) | Solo retains shadow on press for affordance |
| Lavender accent | #bbafe6 | #B8A9FA (purple) | Near-identical |
| Lime/success | #a9d877 | #88D498 (green) | Solo uses more yellow-green |
| Cyan/info | #27ccf3 | #74B9FF / #7FDBDA | Solo uses more vibrant cyan |
| Orange/warning | #f8a16f | #FFA552 | Solo uses more peachy orange |
| Error | #f97264 | #FF4444 | Solo uses softer red |
| Display font | Space Grotesk (all) | Syne (display) + Space Grotesk (headings) | Solo simplifies to one UI font |
| Focus outline color | Black | Blue (#74B9FF) | Blue provides better contrast on dark elements |

---

## 4. Token System — Tailwind v4 Theme

This is the definitive `@theme` block for Solo. It maps neubrutalism.com's canonical tokens to Solo's brand colors and SaaS-use-case scale.

### 4.1 Complete @theme Block

```css
/* ============================================================
   globals.css — Solo Neubrutalist Design System v3.0
   Based on neubrutalism.com canonical patterns
   ============================================================ */

@import "tailwindcss";

/* Google Fonts */
@import url('https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@400;500;600;700&family=Space+Mono:wght@400;700&family=Inter:wght@400;500&family=Syne:wght@700;800&display=swap');

@theme inline {
  /* ---- Typography ---- */
  --font-display: 'Syne', sans-serif;
  --font-heading: 'Space Grotesk', sans-serif;
  --font-body: 'Inter', sans-serif;
  --font-mono: 'Space Mono', monospace;

  /* ---- Color: Brand Palette ---- */
  --color-brutal-bg: #fffaef;
  --color-brutal-fg: #000000;
  --color-brutal-cream: #fffaef;
  --color-brutal-pink: #fe7da8;
  --color-brutal-pink-light: #ffe0ec;
  --color-brutal-yellow: #ffd440;
  --color-brutal-yellow-light: #fff8e0;
  --color-brutal-lavender: #bbafe6;
  --color-brutal-lavender-light: #f0ecff;
  --color-brutal-cyan: #27ccf3;
  --color-brutal-cyan-light: #e0f8ff;
  --color-brutal-lime: #a9d877;
  --color-brutal-lime-light: #eef8e0;
  --color-brutal-orange: #f8a16f;
  --color-brutal-orange-light: #fff0e0;
  --color-brutal-red: #f97264;
  --color-brutal-red-light: #ffe0dc;
  --color-brutal-stone: #c0b9b1;

  /* ---- Color: Functional (for shadcn/ui compatibility) ---- */
  --color-background: #fffaef;
  --color-foreground: #000000;
  --color-card: #ffffff;
  --color-card-foreground: #000000;
  --color-popover: #ffffff;
  --color-popover-foreground: #000000;
  --color-primary: #fe7da8;
  --color-primary-foreground: #000000;
  --color-secondary: #ffffff;
  --color-secondary-foreground: #000000;
  --color-muted: #f5f0e8;
  --color-muted-foreground: #666666;
  --color-accent: #ffd440;
  --color-accent-foreground: #000000;
  --color-destructive: #f97264;
  --color-destructive-foreground: #000000;
  --color-border: #000000;
  --color-input: #ffffff;
  --color-ring: #74b9ff;
  --color-sidebar: #fffaef;
  --color-sidebar-foreground: #000000;
  --color-sidebar-muted: #f5f0e8;
  --color-sidebar-muted-foreground: #666666;
  --color-sidebar-border: #000000;
  --color-sidebar-accent: #ffe0ec;
  --color-sidebar-accent-foreground: #000000;

  /* ---- Border Radius: zero (neubrutalism canon) ---- */
  --radius: 0px;

  /* ---- Custom Keyframes ---- */
  --animate-slide-in-from-right: slide-in-from-right 0.2s ease-out;
  --animate-slide-in-from-left: slide-in-from-left 0.2s ease-out;
}

@keyframes slide-in-from-right {
  from { opacity: 0; transform: translateX(100%); }
  to { opacity: 1; transform: translateX(0); }
}

@keyframes slide-in-from-left {
  from { opacity: 0; transform: translateX(-100%); }
  to { opacity: 1; transform: translateX(0); }
}
```

### 4.2 Brutalist Utility Classes

These go in `@layer components` or `globals.css` as utility classes:

```css
/* ============================================================
   Brutalist Component Utilities
   ============================================================ */

/* ---- Shadow System (hard offset, zero blur, pure black) ---- */
.shadow-brutal {
  --tw-shadow: 5px 5px 0 0 var(--tw-shadow-color, #000);
  box-shadow: var(--tw-inset-shadow), var(--tw-inset-ring-shadow),
              var(--tw-ring-offset-shadow), var(--tw-ring-shadow),
              var(--tw-shadow);
}

.shadow-brutal-sm {
  --tw-shadow: 3px 3px 0 0 var(--tw-shadow-color, #000);
  box-shadow: ...;
}

.shadow-brutal-lg {
  --tw-shadow: 7px 7px 0 0 var(--tw-shadow-color, #000);
  box-shadow: ...;
}

.shadow-brutal-xl {
  --tw-shadow: 10px 10px 0 0 var(--tw-shadow-color, #000);
  box-shadow: ...;
}

/* ---- Color-Filled Shadow Variants ---- */
.shadow-brutal-pink {
  --tw-shadow-color: #fe7da8;
}
.shadow-brutal-yellow {
  --tw-shadow-color: #ffd440;
}
.shadow-brutal-blue {
  --tw-shadow-color: #74b9ff;
}
.shadow-brutal-green {
  --tw-shadow-color: #a9d877;
}

/* ---- Border Utilities ---- */
.border-brutal {
  border: 2px solid #000;
}
.border-brutal-thick {
  border: 3px solid #000;
}

/* ---- Button: Primary Action ---- */
.btn-brutal {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0.5rem 1.25rem;
  font-family: var(--font-heading);
  font-weight: 700;
  font-size: 0.875rem;
  border: 2px solid #000;
  cursor: pointer;
  transition: transform 0.1s ease, box-shadow 0.1s ease;
  box-shadow: 5px 5px 0 0 #000;
}
.btn-brutal:hover {
  transform: translate(-1px, -1px);
  box-shadow: 7px 7px 0 0 #000;
}
.btn-brutal:active {
  transform: translate(3px, 3px);
  box-shadow: none;
}
.btn-brutal:disabled {
  opacity: 0.5;
  pointer-events: none;
}

/* ---- Button: Small ---- */
.btn-brutal-sm {
  padding: 0.375rem 0.875rem;
  font-size: 0.8125rem;
  box-shadow: 3px 3px 0 0 #000;
}
.btn-brutal-sm:hover {
  box-shadow: 5px 5px 0 0 #000;
}
.btn-brutal-sm:active {
  box-shadow: none;
  transform: translate(2px, 2px);
}

/* ---- Button: Flat (no border/shadow) ---- */
.btn-flat {
  background: none;
  border: none;
  font-family: var(--font-heading);
  font-weight: 500;
  color: #666;
  cursor: pointer;
  padding: 0.375rem 0.75rem;
  transition: background 0.1s ease, color 0.1s ease;
}
.btn-flat:hover {
  background: rgba(0,0,0,0.05);
  color: #000;
}
.btn-flat:active {
  background: rgba(0,0,0,0.1);
}

/* ---- Card ---- */
.card-brutal {
  border: 2px solid #000;
  background: #fff;
  box-shadow: 5px 5px 0 0 #000;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}
.card-brutal:hover {
  transform: translate(-1px, -1px);
  box-shadow: 7px 7px 0 0 #000;
}

/* ---- Input ---- */
.input-brutal {
  border: 2px solid #000;
  background: #fff;
  font-family: var(--font-body);
  padding: 0.5rem 0.75rem;
  font-size: 0.9375rem;
  box-shadow: 3px 3px 0 0 #000;
  outline: none;
  width: 100%;
  transition: box-shadow 0.15s ease;
}
.input-brutal:focus {
  box-shadow: 5px 5px 0 0 #000;
  outline: 3px solid #74b9ff;
  outline-offset: 2px;
}
.input-brutal::placeholder {
  color: #999;
}

/* ---- Badge ---- */
.badge-brutal {
  display: inline-flex;
  align-items: center;
  padding: 0.125rem 0.5rem;
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  border: 2px solid #000;
  box-shadow: 2px 2px 0 0 #000;
  background: #fff;
}

/* ---- Divider ---- */
.divider-brutal {
  border: none;
  border-top: 2px solid #000;
  margin: 1rem 0;
}
.divider-brutal-thick {
  border-top: 3px solid #000;
}

/* ---- Global focus-visible ---- */
*:focus-visible {
  outline: 3px solid #74b9ff;
  outline-offset: 2px;
}

/* ---- Reduced Motion ---- */
@media (prefers-reduced-motion: reduce) {
  .btn-brutal, .btn-brutal-sm, .card-brutal {
    transition: none;
  }
  .btn-brutal:hover, .btn-brutal-sm:hover {
    transform: none;
    box-shadow: 5px 5px 0 0 #000;
  }
  .btn-brutal:active, .btn-brutal-sm:active {
    transform: none;
    box-shadow: 5px 5px 0 0 #000;
  }
  .card-brutal:hover {
    transform: none;
    box-shadow: 5px 5px 0 0 #000;
  }
  html {
    scroll-behavior: auto;
  }
}
```

---

## 5. Color System

### 5.1 Brand Palette (Solo-specific hex values, v3.0)

```
Black:        #000000    — Borders, primary text, structural
White:        #ffffff    — Card backgrounds, input backgrounds
Cream:        #fffaef    — Page/shell background (was slock's brutal-cream)

Pink:         #fe7da8    — PRIMARY BRAND ACCENT (buttons, links, highlights)
Pink Light:   #ffe0ec    — Pink background tint

Yellow:       #ffd440    — Theme meta-color, warm accent
Yellow Light: #fff8e0    — Yellow background tint

Lavender:     #bbafe6    — Secondary accent (agent badges)
Lavender L:   #f0ecff    — Lavender background tint

Cyan:         #27ccf3    — Info accent
Cyan Light:   #e0f8ff    — Cyan background tint

Lime:         #a9d877    — Success/online accent
Lime Light:   #eef8e0    — Lime background tint

Orange:       #f8a16f    — Warning/thinking accent
Orange Light: #fff0e0    — Orange background tint

Red:          #f97264    — Error/destructive accent
Red Light:    #ffe0dc    — Red background tint

Stone:        #c0b9b1    — Neutral/muted accent
```

### 5.2 Comparison: Solo v3 vs neubrutalism.com vs Solo v2

| Color | neubrutalism.com | Solo v2 | Solo v3 | Notes |
|-------|-----------------|---------|---------|-------|
| Black | #000000 | (not defined) | #000000 | v3 adds pure black token |
| Off-white bg | #FFFDF5 | #fffaef | #fffaef | Kept v2 (nearly identical) |
| Pink | #FF6B6B (coral) | #fe7da8 | #fe7da8 | Kept v2 (brand color) |
| Yellow | #FFD23F | #ffd440 | #ffd440 | Kept v2 (near-identical) |
| Blue | #74B9FF | #27ccf3 (cyan) | #27ccf3 (keep cyan as info) | Solo is more cyan; #74B9FF used for focus ring |
| Green | #88D498 | #a9d877 (lime) | #a9d877 (keep lime) | Solo uses more yellow-green |
| Orange | #FFA552 | #f8a16f | #f8a16f | Kept v2 |
| Purple | #B8A9FA | #bbafe6 | #bbafe6 | Kept v2 (near-identical) |
| Red | #FF4444 | #f97264 | #f97264 | Kept v2 (softer) |
| Text | #000 (pure) | #141111 | #000000 | Updated to pure black per canon |
| Focus ring | #74B9FF | black outline | #74B9FF | Updated to blue per canon |

### 5.3 Semantic Color Map

| Token | CSS Variable | Value | Usage |
|-------|-------------|-------|-------|
| `bg-page` | `var(--color-brutal-bg)` | #fffaef | Page/shell background |
| `bg-card` | `var(--color-card)` | #ffffff | Card, input, popover backgrounds |
| `text-primary` | `var(--color-brutal-fg)` | #000000 | Primary body text, headings |
| `text-muted` | `--muted-foreground` | #666666 | Timestamps, secondary info |
| `text-placeholder` | — | #999999 | Input placeholders |
| `border-default` | `var(--color-border)` | #000000 | All borders, dividers |
| `accent-primary` | `var(--color-brutal-pink)` | #fe7da8 | Primary CTA buttons, links |
| `accent-yellow` | `var(--color-brutal-yellow)` | #ffd440 | Highlights, badges, accent fills |
| `accent-lavender` | `var(--color-brutal-lavender)` | #bbafe6 | Agent badges, secondary highlights |
| `accent-cyan` | `var(--color-brutal-cyan)` | #27ccf3 | Info state, tooltips, tips |
| `accent-lime` | `var(--color-brutal-lime)` | #a9d877 | Online status, success |
| `accent-orange` | `var(--color-brutal-orange)` | #f8a16f | Warning, thinking/loading states |
| `accent-red` | `var(--color-brutal-red)` | #f97264 | Errors, destructive actions |
| `accent-stone` | `var(--color-brutal-stone)` | #c0b9b1 | Neutral muted accents |
| `focus-ring` | `var(--color-ring)` | #74b9ff | Focus-visible outline (a11y) |

---

## 6. Typography

### 6.1 Font Stack (v3.0)

Following neubrutalism.com's three-tier system, adapted for Solo:

```css
/* Display — Syne 700/800 */
/* Usage: Logo, hero headings, brand moments */
--font-display: 'Syne', system-ui, sans-serif;

/* UI/Heading — Space Grotesk 400-700 */
/* Usage: ALL interface text (buttons, labels, sidebar, messages, headings) */
--font-heading: 'Space Grotesk', system-ui, sans-serif;

/* Body — Inter 400/500 */
/* Usage: Message content, long-form text, dense paragraphs */
--font-body: 'Inter', system-ui, sans-serif;

/* Monospace — Space Mono 400/700 */
/* Usage: Code blocks, inline code, monospace identifiers */
--font-mono: 'Space Mono', ui-monospace, monospace;
```

### 6.2 Usage Rules

| Context | Font | Weight | Notes |
|---------|------|--------|-------|
| **Logo / Brand mark** | `font-display` (Syne) | 800 | Optional upgrade from Space Grotesk |
| **Hero headings** | `font-display` (Syne) | 700-800 | Landing page, empty states |
| **UI labels, buttons, nav** | `font-heading` (Space Grotesk) | 700 | ALL interactive text (bold is default) |
| **Section headings** | `font-heading` (Space Grotesk) | 700 | Modal titles, section headers |
| **Channel names** | `font-heading` (Space Grotesk) | 700 | Sidebar channel names |
| **Message content** | `font-body` (Inter) | 400 | Primary reading text |
| **User messages in chat** | `font-body` (Inter) | 400 | Long-form conversation |
| **Timestamps, badges** | `font-mono` (Space Mono) | 400 | Small metadata |
| **Code blocks** | `font-mono` (Space Mono) | 400 | Pre/code elements |
| **Inline code** | `font-mono` (Space Mono) | 400 | Backtick code spans |
| **Agent names** | `font-heading` (Space Grotesk) | 700 | Bold agent display names |
| **Sender names in chat** | `font-heading` (Space Grotesk) | 700 | Bold sender attribution |

**Migration note from v2:** v2 used Space Grotesk for ALL text. v3 introduces Inter for body/message content for better long-form readability. Components using Space Grotesk remain unchanged where appropriate (UI chrome, labels, buttons).

### 6.3 Font Weights

| Weight | Token | Usage |
|--------|-------|-------|
| 400 | `font-normal` | Body text, Inter paragraph content |
| 500 | `font-medium` | Inter dense text |
| 600 | `font-semibold` | Available for hierarchy |
| 700 | `font-bold` | All UI text (buttons, labels, channel names) |
| 800 | `font-extrabold` | Display/Syne hero text |
| 400 | `font-mono-normal` | Code, timestamps |

### 6.4 Font Size Scale

```
xs:   0.75rem  (12px) — Timestamps, badges, metadata
sm:   0.8125rem (13px) — Badges, small labels
base: 0.875rem (14px) — Button text, labels, secondary text (DEFAULT)
md:   0.9375rem (15px) — Body text (Inter message content)
lg:   1rem     (16px) — Body emphasis, larger text
xl:   1.125rem (18px) — Card titles, message sender
2xl:  1.25rem  (20px) — Section headings ("Sign in")
3xl:  1.5rem   (24px) — Modal titles, page titles
4xl:  2rem     (32px) — Display headings
5xl:  2.5rem   (40px) — Hero headings
```

Note: v3 introduces `md` size (15px) as the default for Inter body text. This is between Tailwind's `sm` (14px) and `base` (16px), optimized for chat readability.

---

## 7. Border System

### 7.1 Border Width Scale

| Token | Width | Style | Color | Usage |
|-------|-------|-------|-------|-------|
| `border-brutal` | 2px | solid | #000 | Default: cards, buttons, inputs, sidebar |
| `border-brutal-thick` | 3px | solid | #000 | Emphasis: hero elements, modals, prominent cards |
| `border-brutal-thin` | 1px | solid | #000 | Subtle dividers (less common) |
| `border-brutal-divider` | 2px | solid | #000 | Section dividers, hr elements |

### 7.2 Border Variations by Element

| Element | Width | Notes |
|---------|-------|-------|
| Cards (default) | 2px | Card component |
| Buttons | 2px | All button variants |
| Inputs | 2px | Text inputs, textareas |
| Modals | 3px | Dialog windows (emphasis) |
| Sidebar items | 2px | Channel items, DM items |
| Dividers (hr) | 2px | Section separators |
| Hero/landing cards | 3px | Promotional elements |
| Badges | 2px | Status badges, counts |
| Focus outline | 3px | #74b9FF blue (a11y) |

### 7.3 Border Color Variants

For colored-border elements (less common, emphasis use only):

```css
.border-brutal-pink { border-color: #fe7da8; }
.border-brutal-yellow { border-color: #ffd440; }
.border-brutal-blue { border-color: #74b9ff; }
.border-brutal-green { border-color: #a9d877; }
```

---

## 8. Shadow System

### 8.1 Four-Tier Shadow Scale (v3.0)

| Tier | Token | Value | Offset | Usage |
|------|-------|-------|--------|-------|
| sm | `shadow-brutal-sm` | `3px 3px 0 0 #000` | 3px | Badges, chips, inline actions, input resting |
| md | `shadow-brutal` | `5px 5px 0 0 #000` | 5px | Cards, buttons, panels (DEFAULT) |
| lg | `shadow-brutal-lg` | `7px 7px 0 0 #000` | 7px | Hover state, elevated cards |
| xl | `shadow-brutal-xl` | `10px 10px 0 0 #000` | 10px | Modals, overlays, hero elements |

### 8.2 Shadow Behavior by State

| Element | Resting | Hover | Active |
|---------|---------|-------|--------|
| Button | 5px 5px, no translate | 7px 7px, translate(-1,-1) | none, translate(3,3) |
| Button (small) | 3px 3px, no translate | 5px 5px, translate(-1,-1) | none, translate(2,2) |
| Card | 5px 5px, no translate | 7px 7px, translate(-1,-1) | (not applicable) |
| Input | 3px 3px, no translate | (focus) 5px 5px + blue outline | (not applicable) |
| Badge | 2px 2px, no translate | No hover change | (not applicable) |
| Modal overlay | 10px 10px | No hover change | (not applicable) |

### 8.3 Shadow Color Variants

For branded/colored shadows (limited use):

```css
--tw-shadow-color: #fe7da8;  /* Pink shadow (e.g. dark button on pink) */
--tw-shadow-color: #ffd440;  /* Yellow shadow */
--tw-shadow-color: #74b9ff;  /* Blue shadow */
```

---

## 9. Spacing System

### 9.1 Spacing Scale (v3.0 — custom, not Tailwind default)

Solo's spacing uses neubrutalism.com's canonical spacing scale, which differs from Tailwind's default. These map to CSS custom properties for consistency.

```
--space-xs:  0.25rem  (4px)   — Tight spacing, gap between icon and text
--space-sm:  0.5rem   (8px)   — Badge padding, small element gaps
--space-md:  1rem     (16px)  — Card padding, form field gaps (DEFAULT)
--space-lg:  1.5rem   (24px)  — Section spacing within cards
--space-xl:  2rem     (32px)  — Card containers, modal padding
--space-2xl: 3rem     (48px)  — Page sections, auth card vertical
--space-3xl: 4rem     (64px)  — Large page sections
```

### 9.2 Usage Map

| Context | Spacing | Notes |
|---------|---------|-------|
| Button padding | py-2 px-3 (8px 12px) | Tailwind's space-2/3 |
| Card padding | p-6 (24px) | ~space-lg |
| Input padding | py-2 px-3 (8px 12px) | Tailwind's space-2/3 |
| Form gap (between fields) | space-y-4 (16px) | ~space-md |
| Label-to-input gap | space-y-2 (8px) | ~space-sm |
| Section margins | mt-6 mb-6 (24px) | ~space-lg |
| Container padding | px-6 (24px) | ~space-lg |
| Auth card vertical | py-16 (64px) | ~space-3xl |
| Modal padding | p-6 (24px) | ~space-lg |
| Message padding | px-3 py-2 (12px 8px) | Tailwind space |

---

## 10. Component Specifications

### 10.1 Button (`btn-brutal` variants)

Based on neubrutalism.com's button pattern, adapted for Solo's 2px border.

```
STRUCTURE:
  display: inline-flex, items-center, justify-center
  font: font-heading (Space Grotesk)
  font-weight: 700
  font-size: 0.875rem (text-sm)
  border: 2px solid black
  box-shadow: 5px 5px 0 0 #000
  transition: transform 0.1s ease, box-shadow 0.1s ease

STATES:
  Default: 5px 5px shadow, no transform
  Hover:   translate(-1px, -1px), 7px 7px shadow
  Active:  translate(3px, 3px), no shadow
  Disabled: opacity-50, pointer-events-none
  Focus:   3px solid #74b9ff outline, offset 2px

SIZES:
  Default: px-3 py-2 (12px 8px), h-10 (40px)
  Small:   px-2.5 py-1.5 (10px 6px), h-8 (32px), text-xs, 3px 3px shadow
  Large:   px-4 py-2.5 (16px 10px), h-12 (48px), text-base

VARIANT     | Background     | Text Color   | Shadow Color
------------|---------------|-------------|-------------
Primary     | bg-pink       | text-black   | #000
Default     | bg-white      | text-black   | #000
Dark        | bg-black      | text-white   | #000 or #fe7da8
Yellow      | bg-yellow     | text-black   | #000
Ghost       | transparent   | text-black/70 | none (btn-flat)
Danger      | bg-red        | text-black   | #000
```

### 10.2 Card (`card-brutal` variants)

```
STRUCTURE:
  border: 2px solid black
  background: white
  box-shadow: 5px 5px 0 0 #000
  transition: transform 0.15s ease, box-shadow 0.15s ease
  padding: p-6 (24px) [default]

STATES:
  Default: 5px 5px shadow
  Hover:   translate(-1px, -1px), 7px 7px shadow
  Flat (card-flat): no shadow, no hover lift

VARIANTS:
  card-default:  standard (white bg, black shadow)
  card-cream:    bg-cream background
  card-flat:     no shadow (for inner cards, list items)
  card-bordered: 3px thick border (for modals)
```

### 10.3 Input (`input-brutal` variants)

```
STRUCTURE:
  border: 2px solid black
  background: white
  font: font-body (Inter)
  font-size: 0.9375rem
  padding: 0.5rem 0.75rem (8px 12px)
  box-shadow: 3px 3px 0 0 #000
  outline: none
  width: 100%
  transition: box-shadow 0.15s ease

STATES:
  Default:    3px 3px shadow
  Focus:      5px 5px shadow + 3px #74b9FF outline (offset 2px)
  Placeholder: #999 (gray, not black/40)
  Disabled:   opacity-50, cursor-not-allowed
  Error:      border-color: #f97264, shadow-color: #f97264

SIZES:
  Default: height: 44px (h-11)
  Textarea: min-h-[80px], resize-vertical
```

### 10.4 Badge (`badge-brutal`)

```
STRUCTURE:
  display: inline-flex, items-center
  font: font-mono (Space Mono)
  font-size: 0.6875rem (11px)
  font-weight: 700
  text-transform: uppercase
  letter-spacing: 0.05em
  border: 2px solid black
  box-shadow: 2px 2px 0 0 #000
  padding: 0.125rem 0.5rem (2px 8px)
  background: white

VARIANTS (background):
  badge-white:    bg-white (default)
  badge-pink:     bg-pink
  badge-yellow:   bg-yellow
  badge-lavender: bg-lavender
  badge-lime:     bg-lime
  badge-cyan:     bg-cyan
  badge-red:      bg-red
  badge-black:    bg-black, text-white
```

### 10.5 Form Elements (from neubrutalism.com patterns)

**Checkbox:**
```css
.nb-checkbox input[type="checkbox"] {
  appearance: none;
  width: 24px; height: 24px;
  border: 2px solid #000;
  border-radius: 0;
  background: white;
  cursor: pointer;
  position: relative;
}
.nb-checkbox input:checked {
  background: var(--yellow);  /* #ffd440 */
}
.nb-checkbox input:checked::after {
  content: '\2713';  /* ✓ checkmark */
  position: absolute;
  top: 50%; left: 50%;
  transform: translate(-50%, -50%);
  font-weight: 800;
  font-size: 1rem;
}
```

**Toggle Switch:**
```css
/* Track */
.nb-toggle-track {
  width: 52px; height: 28px;
  border: 2px solid #000;
  background: #f5f0e8;
  position: relative;
  transition: background 0.2s;
}
/* Thumb */
.nb-toggle-thumb {
  width: 18px; height: 18px;
  background: white;
  border: 2px solid #000;
  position: absolute; top: 2px; left: 2px;
  transition: transform 0.2s cubic-bezier(0.2, 0, 0.2, 1);
}
/* Checked */
.nb-toggle-label input:checked + .nb-toggle-track {
  background: #a9d877;  /* lime/success */
}
.nb-toggle-label input:checked + .nb-toggle-track .nb-toggle-thumb {
  transform: translateX(24px);
}
```

**Select Dropdown:**
```css
.nb-select {
  appearance: none;
  padding: 0.75rem 1rem;
  border: 2px solid #000;
  font-family: var(--font-body);
  font-size: 1rem;
  background: white;
  box-shadow: 3px 3px 0 0 #000;
  background-image: url("data:image/svg+xml,...");
  background-repeat: no-repeat;
  background-position: right 1rem center;
  padding-right: 2.5rem;
}
```

### 10.6 Modal / Dialog

```
STRUCTURE:
  Overlay: position: fixed, inset: 0, bg-black/30, backdrop-blur-sm
  Container: center-fixed, z-50
  Dialog box:
    border: 3px solid black (thick for emphasis)
    background: white
    box-shadow: 10px 10px 0 0 #000 (shadow-xl)
    padding: p-6

TRANSITION:
  Enter: opacity 0 → 1 (0.15s), scale 0.95 → 1 (0.2s)
  Exit:  opacity 1 → 0 (0.1s)
```

### 10.7 Toast / Notification

```
STRUCTURE:
  border: 2px solid black
  box-shadow: 5px 5px 0 0 #000
  font-family: font-heading (Space Grotesk)
  font-weight: 700
  font-size: 0.875rem
  padding: 0.75rem 1rem
  display: flex, items-center, gap-3
  max-width: 340px

VARIANTS:
  Success: bg-lime (#a9d877)
  Error:   bg-red (#f97264)
  Info:    bg-cyan (#27ccf3)
  Warning: bg-yellow (#ffd440)

ANIMATION:
  Enter: translateY(20px) scale(0.95) → translateY(0) scale(1), 0.3s cubic-bezier
  Exit:  translateY(0) → translateY(-10px) scale(0.95), 0.25s cubic-bezier
```

### 10.8 Code Block

```
STRUCTURE:
  background: #1a1a2e (dark navy)
  border: 2px solid #000
  box-shadow: 5px 5px 0 0 #000
  font-family: Space Mono
  font-size: 0.85rem
  line-height: 1.7
  padding: 1rem
  overflow-x: auto

SYNTAX COLORS (from neubrutalism.com):
  comment:  #6b7280
  property: #7FDBDA (cyan)
  value:    #FFD23F (yellow)
  selector: #FF6B6B (pink)
  tag:      #74B9FF (blue)
  string:   #88D498 (green)
```

### 10.9 Divider Variants

```css
/* Standard 2px black divider */
.divider-brutal {
  border: none;
  border-top: 2px solid #000;
  margin: 1rem 0;
}

/* Thick 3px divider */
.divider-brutal-thick {
  border-top: 3px solid #000;
}

/* Color band divider (from neubrutalism.com) */
.divider-band {
  border: none;
  border-top: 3px solid #000;
  border-bottom: 3px solid #000;
  margin: 0;
  height: 16px;
}
.divider-band-pink { background: #fe7da8; }
.divider-band-yellow { background: #ffd440; }
.divider-band-blue { background: #74b9ff; }
.divider-band-lime { background: #a9d877; }
```

---

## 11. Responsive Breakpoints

### 11.1 Breakpoint Definitions

Solo uses Tailwind v4's default breakpoints with semantic mappings:

| Breakpoint | Min Width | Tailwind | Behavior Change |
|-----------|-----------|----------|-----------------|
| xs | 0 | default | Single column, overlay panels |
| sm | 640px | `sm:` | Adaptive layouts begin |
| md | 768px | `md:` | Sidebar fixed, right panel relative |
| lg | 1024px | `lg:` | Three-column layout, full panels |
| xl | 1280px | `xl:` | Wider containers |
| 2xl | 1536px | `2xl:` | Max-width content |

### 11.2 Responsive Shadow Scaling

Following neubrutalism.com's pattern, shadows scale down on smaller viewports:

```
@media (max-width: 768px) {
  --shadow-brutal: 4px 4px 0 0 #000;    /* 5px → 4px */
  --shadow-brutal-lg: 6px 6px 0 0 #000; /* 7px → 6px */
  --shadow-brutal-xl: 8px 8px 0 0 #000; /* 10px → 8px */
}

@media (max-width: 320px) {
  --shadow-brutal: 3px 3px 0 0 #000;    /* 4px → 3px */
  --shadow-brutal-lg: 5px 5px 0 0 #000; /* 6px → 5px */
  --shadow-brutal-xl: 6px 6px 0 0 #000; /* 8px → 6px */
}
```

### 11.3 Responsive Layout Behaviors

```
Mobile (default, < 640px):
  - Sidebar: overlay, position fixed, z-50
  - Right panel: overlay, full-screen, z-40
  - Message input: bottom-anchored
  - Bottom nav (if applicable)
  - Shadows reduced per scaling rules

Tablet (sm-md, 640-768px):
  - Sidebar: compact (w-64)
  - Right panel: overlay or slide-in (border left)
  - Main content: full remainder

Desktop (lg+, 1024px+):
  - Sidebar: fixed w-72 or w-80
  - Right panel: relative, max-w-[28rem]
  - Three-column layout
  - Full shadow scale
```

---

## 12. Animation & Transition Rules

### 12.1 Timing Chart

| Element | Transition | Property | Duration | Easing |
|---------|-----------|----------|----------|--------|
| Button hover | transform + shadow | translate, box-shadow | 0.1s | ease |
| Button active | transform + shadow | translate, box-shadow | 0.1s | ease |
| Card hover | transform + shadow | translate, box-shadow | 0.15s | ease |
| Input focus | shadow | box-shadow | 0.15s | ease |
| Panel slide-in | transform | translateX | 0.2s | ease-out |
| Modal enter | opacity + scale | opacity, scale | 0.15-0.2s | ease-out |
| Modal exit | opacity | opacity | 0.1s | ease-in |
| Toast enter | transform + opacity | translateY, scale, opacity | 0.3s | cubic-bezier(.2,0,.2,1) |
| Toggle | transform | translateX | 0.2s | cubic-bezier(.2,0,.2,1) |
| Skeleton pulse | opacity | opacity | 2s | ease-in-out |

### 12.2 Interaction Patterns

```
LIFT (hover):
  1. translate(-1px, -1px) — element floats up
  2. shadow grows 5px → 7px — more depth below
  Used on: buttons, cards, clickable items

PRESS (active/click):
  1. translate(3px, 3px) — element moves down-right
  2. shadow disappears (none) — squashed flat
  Used on: buttons, toggles

FOCUS (keyboard):
  1. 3px solid #74b9FF outline appears
  2. offset-2px from element edge
  Used on: all interactive elements

PANEL OPEN:
  1. Element slides from right: translateX(100%) → 0
  2. Overlay fades in: opacity 0 → 1
  Duration: 0.2s ease-out
```

### 12.3 Reduced Motion

```css
@media (prefers-reduced-motion: reduce) {
  html { scroll-behavior: auto; }
  
  /* Buttons: no lift/press */
  .btn-brutal, .btn-brutal-sm {
    transition: none;
  }
  .btn-brutal:hover, .btn-brutal-sm:hover {
    transform: none;
    box-shadow: 5px 5px 0 0 #000;
  }
  .btn-brutal:active, .btn-brutal-sm:active {
    transform: none;
    box-shadow: 5px 5px 0 0 #000;
  }
  
  /* Cards: no hover lift */
  .card-brutal:hover {
    transform: none;
    box-shadow: 5px 5px 0 0 #000;
  }
  
  /* Panels: no slide, instant show */
  .panel-slide {
    transform: none !important;
    opacity: 1 !important;
  }
  
  /* Marquee: no scroll */
  .marquee-track {
    animation: none;
  }
  
  /* Toggle: instant */
  .nb-toggle-thumb {
    transition: none;
  }
}
```

---

## 13. Accessibility

### 13.1 WCAG Compliance Targets

| Criterion | Target | Current Status |
|-----------|--------|---------------|
| Contrast ratio (normal text) | AA 4.5:1 | Pass: #000 on #fffaef = 18.5:1 |
| Contrast ratio (large text) | AA 3:1 | Pass |
| Focus indicators | 3px solid visible outline | Updated to #74b9FF |
| Non-text content (icons) | Text alternative | Alt tags on images |
| Keyboard navigation | Full tab order | Tabindex management |
| Touch targets | Min 44x44px | 40-44px targets |
| Reduced motion | Respect OS preference | Media query |
| Error identification | Describe errors in text | Role="alert" |
| Color not sole indicator | Don't rely on color only | Icon + text patterns |

### 13.2 Accessibility Features

```html
<!-- Skip Link (from neubrutalism.com pattern) -->
<a class="skip-link" href="#main">Skip to content</a>

<!-- Focus Ring (global) -->
<style>
  *:focus-visible {
    outline: 3px solid #74b9ff;
    outline-offset: 2px;
  }
  
  /* Custom focus only for keyboard users */
  *:focus:not(:focus-visible) {
    outline: none;
  }
</style>

<!-- Semantic HTML -->
<button>   → Always use <button>, not <div role="button">
<nav>      → Navigation landmarks
<main>     → Main content landmark
<section>  → Section with aria-label or heading
<dialog>   → Modal dialogs
```

**Why blue (#74b9FF) for focus?**
- neubrutalism.com uses #74B9FF as its focus-visible color
- Blue provides strong contrast against both white (#fff) and black (#000) backgrounds
- Unlike pink (#fe7da8), blue is not used as a brand accent, so it reads clearly as a "system" indicator
- Meets WCAG 2.2 focus appearance requirements

### 13.3 Screen Reader Considerations

- ARIA labels on icon-only buttons
- `aria-expanded`, `aria-controls` on expandable elements
- `role="alert"` on error messages and notifications
- Live regions for message updates (`aria-live="polite"`)
- `aria-current="page"` on active navigation items
- Announce loading state changes

---

## 14. Edge Cases & States

| State | Visual Treatment | Notes |
|-------|-----------------|-------|
| **Loading** | Skeleton with card-brutal borders, pulse animation, no content | - |
| **Empty** | Centered text, icon, description, optional CTA | "No messages yet" |
| **Error** | red accent border/text, retry button (btn-brutal), role="alert" | - |
| **Offline** | Top banner, yellow bg, black border, muted interactions | ConnectionBanner |
| **Slow connection** | Disabled states, optimistic UI, spinners on buttons | - |
| **Long text** | Overflow hidden, ellipsis, tooltip on hover | CSS text-overflow |
| **Long list** | Virtualized / infinite scroll | - |
| **Keyboard nav** | 3px #74b9FF visible focus ring on ALL interactive | - |
| **Touch device** | 44px min tap targets, no hover states, tap highlight transparent | - |
| **Mobile** | Overlay sidebar, bottom sheet nav, scaled shadows | - |
| **Reduced motion** | All animations disabled, instant state changes | prefers-reduced-motion |
| **High contrast mode** | Retains 2px black borders (already high contrast) | - |
| **Zoom 200%** | Layout remains functional, no horizontal scroll | - |

---

## 15. Migration Guide

### Phase 1: Core Theme (1h)

Update `app/globals.css` with the new `@theme` block and brutalist utility classes. This is the foundation.

**Files to modify:**
- `app/globals.css` — Replace entire file with v3.0 theme
- `app/layout.tsx` — Update font loading (add Inter)

### Phase 2: Component Update (3h)

Update shadcn/ui components to brutalist styles:

| Component | File | Change |
|-----------|------|--------|
| Button | `components/ui/button.tsx` | Replace variants with btn-brutal variants |
| Input | `components/ui/input.tsx` | input-brutal styles, blue focus ring |
| Card | `components/ui/card.tsx` | card-brutal, rounded-none, shadow-brutal |
| Dialog | `components/ui/dialog.tsx` | 3px border, shadow-xl, blue focus |

**Files to add (new components):**
- `components/ui/badge.tsx` — badge-brutal with color variants
- `components/ui/toggle.tsx` — nb-toggle from neubrutalism patterns
- `components/ui/checkbox.tsx` — nb-checkbox with yellow checked state
- `components/ui/select.tsx` — nb-select with custom arrow
- `components/ui/code-block.tsx` — Dark syntax-highlighted code block
- `components/ui/toast.tsx` — Color-coded toast with enter/exit animations

### Phase 3: Layout Update (1h)

| File | Change |
|------|--------|
| `app/auth/layout.tsx` | bg-cream, remove gradient, center card |
| `app/auth/login/page.tsx` | Use input-brutal, card-brutal, btn-brutal-pink |
| `app/auth/register/page.tsx` | Same as login |
| `app/dashboard/page.tsx` | bg-cream sidebar, 2px dividers |

### Phase 4: Accessibility (1h)

- Add `skip-link` to root layout
- Add `prefers-reduced-motion` media query to globals.css
- Verify all focus-visible indicators use 3px #74b9FF
- Add role="alert" to all error messages
- Ensure 44px touch targets on all mobile elements

### Phase 5: Polish (1h)

- Custom selection colors (`::selection { bg: yellow }`)
- Verify responsive shadow scaling
- Test all edge case states (loading, empty, error)
- Audit for hardcoded colors and replace with CSS variables

**Total migration effort: ~7 hours**

---

## Appendix A: Before/After Summary

| Element | Before (shadcn/ui default) | After (neubrutalist v3.0) |
|---------|---------------------------|--------------------------|
| Page background | White (#fff) | Cream (#fffaef) |
| Card style | Rounded (8px), 1px gray border, soft shadow | Square, 2px black border, 5px hard shadow |
| Button default | Blue bg, white text, rounded | Pink bg, black text, square, 5px shadow |
| Button hover | Darker blue | Lifts up, shadow grows to 7px |
| Button active | Darker shade | Presses down, shadow vanishes |
| Input style | Rounded, 1px gray border, blue focus ring | Square, 2px black border, 3px shadow, blue outline |
| Input focus | Blue ring, no shadow change | Shadow grows to 5px + 3px blue outline |
| Body font | Inter (default shadcn) | Inter (v2 was Space Grotesk for all) |
| UI font | Inter | Space Grotesk (bold) |
| Display font | Inter | Syne (hero) or Space Grotesk (all) |
| Borders | 1px subtle gray (#e2e2e2) | 2px solid black |
| Box shadow | Blurred, rgba, multi-layer | Hard offset, zero blur, pure black |
| Border radius | 8-12px (rounded-lg/xl) | 0px (square) |
| Sidebar | Blue-tinted or dark | Cream bg, 2px black dividers |
| Message text | Inter (gray-900) | Inter (black) |
| Focus style | Blue ring (2px) | Blue outline (3px #74b9FF) |
| Dividers | 1px gray | 2px solid black |
| Checkbox | Browser default | Custom square, yellow checked |
| Toggle | Browser default | Custom, border, lime checked |
| Loader | Spinning border | Spinning border (kept) |
| Error display | Red text | Red bg badge + text |
| Icons | lucide-react | lucide-react (kept) |
| Empty state | Minimal text | Icon + heading + description |

---

## Appendix B: Token Quick Reference

```css
/* === CSS CUSTOM PROPERTIES === */
--color-brutal-bg: #fffaef;
--color-brutal-fg: #000000;
--color-brutal-pink: #fe7da8;
--color-brutal-yellow: #ffd440;
--color-brutal-lavender: #bbafe6;
--color-brutal-cyan: #27ccf3;
--color-brutal-lime: #a9d877;
--color-brutal-orange: #f8a16f;
--color-brutal-red: #f97264;
--color-brutal-stone: #c0b9b1;

/* === BORDERS === */
border-standard: 2px solid #000;
border-thick:    3px solid #000;

/* === SHADOWS === */
shadow-sm:  3px 3px 0 0 #000;
shadow-md:  5px 5px 0 0 #000;  /* DEFAULT */
shadow-lg:  7px 7px 0 0 #000;
shadow-xl:  10px 10px 0 0 #000;

/* === RADIUS === */
radius: 0px;  /* ALL elements */

/* === TYPOGRAPHY === */
font-display: 'Syne', sans-serif;
font-heading: 'Space Grotesk', sans-serif;
font-body:    'Inter', sans-serif;
font-mono:    'Space Mono', monospace;
```

---

## Appendix C: Sources

1. **neubrutalism.com** (2026-05-11) — The definitive guide to Neubrutalist Web Design. Full CSS and HTML analyzed. Canonical token definitions, component patterns, accessibility guidance, and responsive rules extracted from live site.

2. **slock.ai production** (observed via app.slock.ai, compiled CSS from `/assets/index-CcraCW7A.css`) — Observed color tokens, component classes (btn-brutal, input-brutal, card-brutal), layout measurements, and responsive behaviors.

---

*Document version: v3.0*
*Last updated: 2026-05-11*
*Authored by: fe1 (frontend lead)*
