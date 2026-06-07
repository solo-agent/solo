# `components/ui/` — Shared Brutal Components

Single source of truth for cross-page UI primitives. All components are
designed against the v3.0 semantic tokens defined in `app/globals.brutal.css`.

> Migration policy: prefer these components over raw `<button>`, raw `<select>`,
> ad-hoc badge divs, etc. New pages should import from here. Old pages can be
> migrated incrementally — old color aliases (`brutal-pink`, etc.) still resolve
> in v3.0 but will be removed in v4.0.

---

## Spinner

```tsx
import { Spinner } from '@/components/ui/spinner';

<Spinner size="sm" />            // 14px, inline
<Spinner size="md" />            // 20px, for blocks
<Spinner label="上传中" />        // screen-reader label
<Spinner className="text-…" />   // custom text color
```

| Prop | Type | Default | Notes |
| --- | --- | --- | --- |
| `size` | `'sm' \| 'md'` | `'sm'` | `sm` for inline/buttons, `md` for blocks |
| `label` | `string` | `'加载中'` | `aria-label` |
| `className` | `string` | — | extra classes (e.g. text color) |

Respects `prefers-reduced-motion` via the global override.

---

## Button

```tsx
import { Button } from '@/components/ui/button';

<Button variant="primary">提交</Button>
<Button variant="danger" onClick={onDelete}>删除</Button>
<Button variant="outline">取消</Button>
<Button variant="ghost">次要操作</Button>
<Button size="sm">小按钮</Button>
<Button size="icon" aria-label="关闭"><X /></Button>
```

| Prop | Type | Default | Notes |
| --- | --- | --- | --- |
| `variant` | `'primary' \| 'danger' \| 'outline' \| 'ghost'` | `'primary'` | + `default`/`destructive`/`secondary`/`link` as deprecated aliases |
| `size` | `'default' \| 'sm' \| 'lg' \| 'icon'` | `'default'` | h-10 / h-8 / h-12 / h-10 w-10 |
| `…button props` | — | — | extends `ButtonHTMLAttributes` |

Variants map to v3.0 tokens: `primary` = `bg-brutal-primary`, `danger` = `bg-brutal-danger text-white`, `outline` = `bg-brutal-white border-2 border-black`, `ghost` = transparent with yellow hover.

---

## SelectableRow

```tsx
import { SelectableRow } from '@/components/ui/selectable-row';

<SelectableRow selected={id === current} onClick={() => setCurrent(id)}>
  {label}
</SelectableRow>
```

| Prop | Type | Default | Notes |
| --- | --- | --- | --- |
| `selected` | `boolean` | `false` | drives the yellow fill + border |
| `onClick` | `(e) => void` | — | |
| `leading` | `ReactNode` | — | icon / avatar slot |
| `trailing` | `ReactNode` | — | badge / count slot |

Renders `<button>` with `aria-current="true"` when selected. Keyboard accessible by default.

---

## SectionHeader

```tsx
import { SectionHeader } from '@/components/ui/section-header';

// Static header
<SectionHeader label="频道" count={12} />

// Collapsible
<SectionHeader
  label="私信"
  count={3}
  expanded={open}
  onToggle={() => setOpen(!open)}
/>
```

| Prop | Type | Default | Notes |
| --- | --- | --- | --- |
| `label` | `string` | — | required, uppercase tracking-widest |
| `count` | `number` | — | optional, rendered in mono |
| `expanded` | `boolean` | — | omit for static; pass for collapsible |
| `onToggle` | `() => void` | — | when provided, renders a `<button>` with `aria-expanded` |
| `as` | `'h1' … 'h6'` | `'h3'` | heading level for static variant |
| `trailing` | `ReactNode` | — | e.g. a "+" action button |

---

## EmptyState

```tsx
import { EmptyState } from '@/components/ui/empty-state';

<EmptyState
  icon={<Inbox />}
  title="还没有任务"
  description="点击下方按钮创建第一个任务"
  actionLabel="创建任务"
  onAction={onCreate}
  variant="plain"
/>
```

| Prop | Type | Default | Notes |
| --- | --- | --- | --- |
| `icon` | `ReactNode` | — | rendered in a bordered yellow-tinted box |
| `title` | `string` | — | required |
| `description` | `string` | — | |
| `actionLabel` + `onAction` | `string`, `() => void` | — | optional primary button |
| `variant` | `'plain' \| 'dashed'` | `'plain'` | `dashed` for drop targets / search panels |

---

## BrutalAlert

```tsx
import { BrutalAlert } from '@/components/ui/brutal-alert';

<BrutalAlert variant="error" title="保存失败">
  请检查网络连接后重试。
</BrutalAlert>
```

| Prop | Type | Default | Notes |
| --- | --- | --- | --- |
| `variant` | `'error' \| 'warning' \| 'success' \| 'info'` | `'info'` | drives background + text color |
| `title` | `string` | — | bold heading line |
| `children` | `ReactNode` | — | body content |
| `hideIcon` | `boolean` | `false` | skip the leading icon |
| `role` | `'alert' \| 'status'` | auto | default `'alert'` for error/warning, `'status'` for success/info |

---

## Tag

```tsx
import { Tag } from '@/components/ui/tag';

<Tag variant="status">TODO</Tag>
<Tag variant="type">#frontend</Tag>
<Tag variant="agent">@Claude</Tag>
<Tag variant="deleted">已删除</Tag>
```

| Prop | Type | Default | Notes |
| --- | --- | --- | --- |
| `variant` | `'status' \| 'type' \| 'agent' \| 'deleted'` | `'status'` | yellow / blue / violet / muted |
| `children` | `ReactNode` | — | content (icon + label) |

---

## Select

```tsx
import { Select, type SelectOption } from '@/components/ui/select';

const options: SelectOption[] = [
  { value: 'todo', label: 'TODO' },
  { value: 'done', label: 'DONE' },
];

<Select
  value={status}
  onChange={(v) => setStatus(v)}
  options={options}
  placeholder="选择状态"
  size="sm"
/>
```

| Prop | Type | Default | Notes |
| --- | --- | --- | --- |
| `options` | `SelectOption[]` | — | required |
| `placeholder` | `string` | — | shown in trigger when no value is selected |
| `size` | `'sm' \| 'md'` | `'sm'` | h-8 / h-10 |
| `name` | `string` | — | renders a hidden `<input>` for form submission / `Controller` |
| `onChange` | `(value: string) => void` | — | receives the selected value, not an event |

Custom dropdown panel — open state matches the brutalist design system (hard border + hard offset shadow). Keyboard: Esc closes, Enter/Space toggles or selects, ArrowUp/Down navigate. For react-hook-form, wrap in `<Controller>`.

---

## Color tokens reference

| New (v3.0) | Old alias (deprecated) | Light | Light-light (8% tint) |
| --- | --- | --- | --- |
| `primary` | `pink` | `#FFD23F` | `#FFF9E0` |
| `accent` | `yellow` | `#FF6B6B` | `#FFD4D4` |
| `info` | `cyan` | `#74B9FF` | `#E0F0FF` |
| `success` | `lime` | `#88D498` | `#E0F5E0` |
| `warning` | `orange` | `#f8a16f` | `#fff0e0` |
| `danger` | `red` | `#f97264` | `#ffe0dc` |
| `violet` | `lavender` | `#bbafe6` | `#f0ecff` |
| `muted` | `stone` | `#c0b9b1` | `#ece6df` |

Use the new names in new code. Old names still work in v3.0 via CSS `var()` aliases.
