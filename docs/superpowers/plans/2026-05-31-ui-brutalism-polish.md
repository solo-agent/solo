# UI Brutalism Polish — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Polish Solo UI with dual-column layout, pixel agent avatars, colorful markdown replies, and task board color enhancements — all in neubrutalist style.

**Architecture:** Frontend-only changes. New NavBar component (56px icon strip) replaces bottom nav links in sidebar. Channel/DM list stays in a 200px panel. Right member panel removed — members accessed via popover from header. Pixel avatars rendered via CSS grid, agent replies get colored table headers/@mentions via custom react-markdown components.

**Tech Stack:** Next.js 16 / Tailwind CSS v4 / React Markdown / remark-gfm / Lucide icons

---

### Task 1: Pixel Avatar System (CSS + Component)

**Files:**
- Modify: `frontend/app/globals.brutal.css` (add pixel avatar CSS)
- Create: `frontend/components/ui/pixel-avatar.tsx`
- Modify: `frontend/components/agents/agent-card.tsx` (use PixelAvatar)
- Modify: `frontend/components/dashboard/agent-message.tsx` (use PixelAvatar)
- Modify: `frontend/components/dashboard/streaming-message.tsx` (use PixelAvatar)

- [ ] **Step 1: Add pixel avatar styles to globals.brutal.css**

Add after the existing `/* Agent message container */` section:

```css
/* ============================================================
   Pixel Avatar — 7×7 CSS grid pixel art (3px per cell)
   ============================================================ */

.pixel-avatar-grid {
  display: grid;
  grid-template-columns: repeat(7, 3px);
  grid-template-rows: repeat(7, 3px);
  gap: 0;
  width: 21px;
  height: 21px;
}

.pixel-avatar-cell {
  width: 3px;
  height: 3px;
}

.pixel-avatar-cell-filled { background: var(--pixel-fill, #000); }
.pixel-avatar-cell-empty { background: transparent; }

/* Pixel avatar wrapper sizes */
.pixel-avatar-sm { width: 24px; height: 24px; }
.pixel-avatar-md { width: 32px; height: 32px; }
```

- [ ] **Step 2: Create PixelAvatar component**

Create `frontend/components/ui/pixel-avatar.tsx`:

```tsx
'use client';

import { useMemo } from 'react';
import { cn } from '@/lib/utils';

// 8 preset pixel art patterns (7×7 grid, 0=empty 1=color1 2=color2)
// Each is a retro game character silhouette
const PATTERNS: number[][] = [
  // 0: Knight — helmet shape
  [0,0,1,1,1,0,0, 0,1,1,1,1,1,0, 1,1,0,1,0,1,1, 1,1,1,1,1,1,1, 1,1,0,1,0,1,1, 0,1,1,1,1,1,0, 0,0,1,1,1,0,0],
  // 1: Mage — pointed hat
  [0,0,0,2,0,0,0, 0,0,2,2,2,0,0, 0,1,1,1,1,1,0, 1,1,0,1,0,1,1, 1,1,1,1,1,1,1, 1,0,1,1,1,0,1, 0,1,0,1,0,1,0],
  // 2: Ranger — hood
  [0,0,1,1,1,0,0, 0,1,1,1,1,1,0, 1,1,1,1,1,1,1, 1,1,0,1,0,1,1, 1,1,1,1,1,1,1, 0,1,1,1,1,1,0, 0,1,0,1,0,1,0],
  // 3: Cleric — cross helmet
  [0,0,1,1,1,0,0, 0,1,1,2,1,1,0, 1,1,2,1,2,1,1, 1,1,1,1,1,1,1, 1,1,0,1,0,1,1, 0,1,1,1,1,1,0, 0,0,1,1,1,0,0],
  // 4: Rogue — mask
  [0,0,1,1,1,0,0, 0,1,1,1,1,1,0, 1,1,2,1,2,1,1, 1,1,1,2,1,1,1, 1,1,2,1,2,1,1, 0,1,1,1,1,1,0, 0,0,1,1,1,0,0],
  // 5: Monk — bald head
  [0,0,1,1,1,0,0, 0,1,1,1,1,1,0, 1,1,1,1,1,1,1, 1,1,0,1,0,1,1, 1,1,1,1,1,1,1, 0,1,1,1,1,1,0, 0,0,1,1,1,0,0],
  // 6: Mech — square head
  [0,1,1,1,1,1,0, 1,1,1,1,1,1,1, 1,1,2,1,2,1,1, 1,1,1,2,1,1,1, 1,1,2,1,2,1,1, 1,1,1,1,1,1,1, 0,1,1,1,1,1,0],
  // 7: Slime — round blob
  [0,0,1,1,1,0,0, 0,1,1,1,1,1,0, 1,1,1,1,1,1,1, 1,1,1,1,1,1,1, 1,1,1,1,1,1,1, 0,1,1,1,1,1,0, 0,0,1,1,1,0,0],
];

const COLOR_PAIRS: [string, string][] = [
  ['#4a6fa5', '#2d4a7a'], // Knight: steel blue
  ['#8b5cf6', '#ffd440'], // Mage: purple + gold
  ['#5d8a4c', '#8b6914'], // Ranger: green + brown
  ['#f8f8f8', '#fe7da8'], // Cleric: white + pink
  ['#1a1a1a', '#f97264'], // Rogue: black + red
  ['#f8a16f', '#ffd440'], // Monk: orange + yellow
  ['#808080', '#27ccf3'], // Mech: gray + cyan
  ['#a9d877', '#27ccf3'], // Slime: lime + cyan
];

export function getPixelAvatarIndex(agentId: string, existingUrl?: string | null): number {
  if (existingUrl?.startsWith('pixel:')) {
    const idx = parseInt(existingUrl.replace('pixel:', ''), 10);
    if (idx >= 0 && idx < 8) return idx;
  }
  // Hash the ID to 0-7
  let hash = 0;
  for (let i = 0; i < agentId.length; i++) {
    hash = ((hash << 5) - hash) + agentId.charCodeAt(i);
    hash |= 0;
  }
  return Math.abs(hash) % 8;
}

interface PixelAvatarProps {
  agentId: string;
  avatarUrl?: string | null;
  size?: 'sm' | 'md';
  className?: string;
}

export function PixelAvatar({ agentId, avatarUrl, size = 'sm', className }: PixelAvatarProps) {
  const index = useMemo(
    () => getPixelAvatarIndex(agentId, avatarUrl),
    [agentId, avatarUrl],
  );
  const pattern = PATTERNS[index];
  const [color1, color2] = COLOR_PAIRS[index];
  const sizeClass = size === 'sm' ? 'pixel-avatar-sm' : 'pixel-avatar-md';

  return (
    <div
      className={cn(
        'flex items-center justify-center border-2 border-black shadow-brutal-sm bg-white',
        sizeClass,
        className,
      )}
      aria-label={`Pixel avatar ${index}`}
    >
      <div className="pixel-avatar-grid">
        {pattern.map((cell, i) => (
          <div
            key={i}
            className="pixel-avatar-cell"
            style={{
              backgroundColor: cell === 1 ? color1 : cell === 2 ? color2 : 'transparent',
            }}
          />
        ))}
      </div>
    </div>
  );
}

export const PIXEL_AVATAR_COUNT = 8;
```

- [ ] **Step 3: Update agent-card.tsx to use PixelAvatar**

In `frontend/components/agents/agent-card.tsx:13`, change the import:

```tsx
// Replace Bot import with pixel avatar
import { PixelAvatar } from '@/components/ui/pixel-avatar';
```

Replace the Bot icon area (`agent-card.tsx` lines 81-84):

```tsx
// OLD (lines 81-84):
// <div className="flex h-12 w-12 flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-pink shadow-brutal-sm">
//   <Bot className="h-6 w-6 text-white" />
// </div>

// NEW:
<PixelAvatar agentId={agent.id} avatarUrl={agent.avatar_url} size="md" />
```

Also remove `Bot` from the lucide import since it's no longer used (line 12: `import { Bot, Edit, Trash2, Circle } from 'lucide-react';` → `import { Edit, Trash2, Circle } from 'lucide-react';`).

- [ ] **Step 4: Update agent-message.tsx to use PixelAvatar**

In `frontend/components/dashboard/agent-message.tsx:11`, add import:

```tsx
import { PixelAvatar } from '@/components/ui/pixel-avatar';
```

Replace the Bot icon avatar block (line 61-63):

```tsx
{/* Replace Bot icon div */}
<PixelAvatar agentId={message.user_id} size="sm" className="mt-0.5 flex-shrink-0" />
```

- [ ] **Step 5: Update streaming-message.tsx to use PixelAvatar**

In `frontend/components/dashboard/streaming-message.tsx`:

Remove `Bot` from import (line 14: `import { Bot } from 'lucide-react';` → delete line).
Add import: `import { PixelAvatar } from '@/components/ui/pixel-avatar';`

Replace avatar area (lines 118-121):
```tsx
// OLD:
// <div className="mt-0.5 flex h-8 w-8 flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-pink-light">
//   <Bot className="h-4 w-4 text-brutal-pink" />
// </div>

// NEW:
<PixelAvatar agentId={message.user_id} size="sm" className="mt-0.5 flex-shrink-0" />
```

- [ ] **Step 6: Verify pixel avatars render**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`
Expected: No new type errors.

- [ ] **Step 7: Commit**

```bash
git add frontend/components/ui/pixel-avatar.tsx \
        frontend/app/globals.brutal.css \
        frontend/components/agents/agent-card.tsx \
        frontend/components/dashboard/agent-message.tsx \
        frontend/components/dashboard/streaming-message.tsx
git commit -m "feat: add CSS pixel avatar system with 8 preset characters"
```

---

### Task 2: Narrow Icon NavBar Component

**Files:**
- Create: `frontend/components/ui/navbar.tsx`
- Modify: `frontend/app/globals.brutal.css` (add navbar styles)

- [ ] **Step 1: Add NavBar CSS styles to globals.brutal.css**

Add after the existing `/* Sidebar Overrides */` section:

```css
/* ============================================================
   NavBar — 56px icon-only navigation bar
   ============================================================ */

.navbar-brutal {
  background: #e5e0d8;
}

.navbar-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border: 2px solid #000;
  background: #fff;
  box-shadow: 3px 3px 0 0 #000;
  cursor: pointer;
  transition: transform 0.1s ease, box-shadow 0.1s ease;
  color: #000;
}

.navbar-icon:hover {
  transform: translate(-1px, -1px);
  box-shadow: 5px 5px 0 0 #000;
}

.navbar-icon:active {
  transform: translate(2px, 2px);
  box-shadow: none;
}

.navbar-icon-active {
  background: #fe7da8;
}
```

- [ ] **Step 2: Create NavBar component**

Create `frontend/components/ui/navbar.tsx`:

```tsx
'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import {
  Hash,
  ClipboardList,
  Users,
  Bot,
  Monitor,
  Settings,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { useAuth } from '@/lib/auth-context';
import { PixelAvatar } from '@/components/ui/pixel-avatar';

const NAV_ITEMS = [
  { href: '/dashboard', icon: Hash, label: '频道' },
  { href: '/tasks', icon: ClipboardList, label: '任务看板' },
  { href: '/teams', icon: Users, label: '团队' },
  { href: '/agents', icon: Bot, label: 'Agent 管理' },
  { href: '/computers', icon: Monitor, label: '电脑管理' },
] as const;

export function NavBar() {
  const pathname = usePathname();
  const { user } = useAuth();

  return (
    <nav className="navbar-brutal flex w-14 flex-shrink-0 flex-col items-center gap-1 border-r-2 border-black py-3">
      {/* Workspace logo */}
      <Link
        href="/dashboard"
        className="mb-2 flex h-9 w-9 items-center justify-center border-2 border-black bg-brutal-pink shadow-brutal-sm"
        aria-label="Solo 工作区"
      >
        <span className="font-heading text-sm font-black text-black">S</span>
      </Link>

      {/* Divider */}
      <div className="mb-1 h-px w-8 bg-black/20" />

      {/* Nav items */}
      {NAV_ITEMS.map((item) => {
        const isActive = pathname.startsWith(item.href);
        return (
          <Link
            key={item.href}
            href={item.href}
            className={cn(
              'navbar-icon',
              isActive && 'navbar-icon-active',
            )}
            aria-label={item.label}
            title={item.label}
          >
            <item.icon className="h-4 w-4" />
          </Link>
        );
      })}

      {/* Spacer */}
      <div className="mt-auto flex flex-col items-center gap-1">
        {/* Settings */}
        <Link
          href="/settings"
          className={cn(
            'navbar-icon',
            pathname.startsWith('/settings') && 'navbar-icon-active',
          )}
          aria-label="个人设置"
          title="个人设置"
        >
          <Settings className="h-4 w-4" />
        </Link>

        {/* User avatar (pixel style for consistency) */}
        {user && (
          <Link
            href="/settings"
            className="navbar-icon mt-1"
            aria-label={user.display_name || user.email || '用户'}
            title={user.display_name || user.email || '用户'}
          >
            <PixelAvatar
              agentId={user.id || 'user'}
              size="sm"
            />
          </Link>
        )}
      </div>
    </nav>
  );
}
```

- [ ] **Step 3: Type-check**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`
Expected: No new type errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/components/ui/navbar.tsx frontend/app/globals.brutal.css
git commit -m "feat: add narrow icon-only NavBar component"
```

---

### Task 3: Dashboard Layout Restructure

**Files:**
- Modify: `frontend/app/dashboard/page.tsx` (dual-column layout)
- Modify: `frontend/components/dashboard/sidebar.tsx` (remove nav links, keep channels/DMs)

- [ ] **Step 1: Strip nav links from Sidebar, keep only channels + DMs + user**

In `frontend/components/dashboard/sidebar.tsx`, remove the navigation section and replace the bottom user area. Remove lines 80-125 (the "导航" section). Keep only:

1. Workspace header (lines 48-55)
2. Scrollable channel + DM area (lines 58-77)
3. User area at bottom (lines 128-143)

But also remove the imports no longer needed:

```tsx
// Remove from imports (line 8):
// Settings, Bot, ClipboardList, Users, Monitor
// Keep only what's used by channel/DM lists

// Remove Link from imports — no longer needed in sidebar
```

Wait — looking more carefully, the sidebar component also receives `onSelectChannel`, `onCreateChannel`, etc. All of that stays. I just need to remove the nav section and the Link import.

Let me write exact edits:

**Edit 1**: Remove unused imports (line 8)
```
// old:
import { Settings, Bot, ClipboardList, Users, Monitor } from 'lucide-react';
// new:
// (remove this line entirely)
```

Actually, looking at the Sidebar more carefully, the `Link` import from 'next/link' is used for the nav links. Let me just remove those lines:

```diff
- import Link from 'next/link';
- import { Settings, Bot, ClipboardList, Users, Monitor } from 'lucide-react';
```

**Edit 2**: Remove the nav section (lines 80-125):
These lines (from `{/* Navigation section */}` to the closing `</div>` after 电脑管理 link) should be removed.

**Edit 3**: Keep the user area at bottom but remove Settings link — actually leave it but simplify.

Wait, let me re-think. The user area at bottom has Avatar + name + Settings gear. Since the NavBar now has user avatar and settings, I can simplify this to just the user info or remove it entirely. Let me keep the user area minimal — just name for context.

Actually, the channel list sidebar should be clean. Let me remove the user area entirely since it's now in the NavBar. Keep only: workspace header + channel/DM lists.

- [ ] **Step 1: Modify sidebar.tsx — remove nav links and user area**

Change imports:

```tsx
// Remove Link import and nav icons
// OLD:
import Link from 'next/link';
import { Settings, Bot, ClipboardList, Users, Monitor } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { ChannelList } from './channel-list';
import { DMList } from './dm-list';
import { Avatar } from '@/components/ui/avatar';

// NEW:
import { ChannelList } from './channel-list';
import { DMList } from './dm-list';
```

Remove `useAuth()` call and the `user` variable from the component body.

Remove lines 80-143 (the nav section and user area). The component should return just:

```tsx
<aside className="flex w-50 flex-col bg-sidebar text-sidebar-foreground border-r-2 border-sidebar-border flex-shrink-0">
  {/* Workspace header */}
  <div className="flex h-14 items-center border-b-2 border-sidebar-border px-4">
    <div className="flex items-center gap-2">
      <div className="flex h-8 w-8 items-center justify-center bg-brutal-pink border-2 border-black shadow-brutal-sm">
        <span className="text-sm font-bold text-black">S</span>
      </div>
      <span className="font-heading font-bold text-sidebar-foreground">Solo</span>
    </div>
  </div>

  {/* Scrollable channel + DM area */}
  <div className="flex-1 overflow-y-auto px-2 py-3">
    <ChannelList
      channels={channels}
      isLoading={isLoading}
      selectedChannelId={selectedChannelId}
      onSelectChannel={onSelectChannel}
      onCreateChannel={onCreateChannel}
      onDeleteChannel={onDeleteChannel}
    />
    <div className="mt-6">
      <DMList
        dms={dms}
        isLoading={dmsLoading}
        selectedDmId={selectedDmId}
        onSelectDM={onSelectDM}
        onCreateDM={onCreateDM}
      />
    </div>
  </div>
</aside>
```

Also update the width from `w-60` to `w-50` (200px).

- [ ] **Step 2: Update dashboard page.tsx layout**

In `frontend/app/dashboard/page.tsx`:

Add NavBar import:
```tsx
import { NavBar } from '@/components/ui/navbar';
```

Change the return JSX (lines 389-433) from:

```tsx
<div className="flex h-screen min-w-[1024px] overflow-hidden bg-brutal-cream">
  <Sidebar ... />
  <main className="flex flex-1 flex-col overflow-hidden">
    {renderMainContent()}
  </main>
  ...
</div>
```

To:

```tsx
<div className="flex h-screen min-w-[1024px] overflow-hidden bg-brutal-cream">
  <NavBar />
  <Sidebar
    channels={channels}
    ...
  />
  <main className="flex flex-1 flex-col overflow-hidden">
    {renderMainContent()}
  </main>
  ...modals stay the same...
</div>
```

- [ ] **Step 3: Type-check**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -30`
Expected: No new type errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/app/dashboard/page.tsx frontend/components/dashboard/sidebar.tsx
git commit -m "feat: restructure dashboard to dual-column NavBar + ChannelList layout"
```

---

### Task 4: Remove Right Member Panel, Add Member Popover

**Files:**
- Modify: `frontend/components/dashboard/channel-view.tsx`
- Modify: `frontend/components/dashboard/member-list.tsx` (make reusable in popover)

- [ ] **Step 1: Replace right MemberList panel with popover in channel-view.tsx**

In `frontend/components/dashboard/channel-view.tsx`:

Add state for member popover:
```tsx
const [isMemberPopoverOpen, setIsMemberPopoverOpen] = useState(false);
```

Replace the right member panel (lines 563-571):
```tsx
{/* Right: member list panel */}
<div className="hidden w-56 flex-shrink-0 border-l-2 border-black bg-brutal-cream lg:block">
  <MemberList ... />
</div>
```

Replace the header member count display (lines 447-463 area) — make the `Users` icon + count a clickable button:

```tsx
<div className="flex items-center gap-2 text-xs text-muted-foreground flex-shrink-0">
  {channelViewTab === 'messages' && (
    <ChannelSearch ... />
  )}
  <button
    type="button"
    onClick={() => setIsMemberPopoverOpen(true)}
    className="btn-brutal btn-brutal-sm flex items-center gap-1"
  >
    <Users className="h-3.5 w-3.5" />
    <span>{users.length + agents.length}</span>
  </button>
</div>
```

Add the member popover dialog:

```tsx
{/* Member popover */}
<Dialog open={isMemberPopoverOpen} onOpenChange={setIsMemberPopoverOpen}>
  <DialogHeader>
    <DialogTitle>
      <div className="flex items-center gap-2">
        <Users className="h-4 w-4" />
        频道成员
        <span className="font-mono text-sm font-normal text-muted-foreground">
          ({users.length + agents.length})
        </span>
      </div>
    </DialogTitle>
    <DialogCloseButton onClick={() => setIsMemberPopoverOpen(false)} />
  </DialogHeader>
  <div className="max-h-[60vh] overflow-y-auto">
    <MemberList
      users={users}
      agents={agents}
      isLoading={membersLoading}
      onAddAgent={() => {
        setIsMemberPopoverOpen(false);
        setIsAddAgentModalOpen(true);
      }}
    />
  </div>
</Dialog>
```

Also clean up the mobile member button (lines 655-666) — keep it but it opens the popover now.

```tsx
{/* Mobile: member button */}
<div className="lg:hidden">
  <button
    onClick={() => setIsMemberPopoverOpen(true)}
    className="btn-brutal fixed bottom-4 right-4 z-40 flex h-10 w-10 items-center justify-center shadow-brutal"
    aria-label="成员"
  >
    <Users className="h-5 w-5" />
  </button>
</div>
```

Make sure `DialogCloseButton` is imported (it already is — check imports on line 26).

- [ ] **Step 2: Type-check**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -30`
Expected: No new type errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/components/dashboard/channel-view.tsx
git commit -m "feat: replace right member panel with popover from header button"
```

---

### Task 5: Colorful Agent Markdown Replies

**Files:**
- Modify: `frontend/components/dashboard/agent-message.tsx`

- [ ] **Step 1: Install rehype-raw for @mention HTML rendering**

```bash
cd frontend && npm install rehype-raw
```

- [ ] **Step 2: Add @mention pre-processing helper**

Add before the `AgentMessage` component function in `frontend/components/dashboard/agent-message.tsx`:

```tsx
/** Wrap @mentions in HTML spans, protecting code fences */
function highlightMentions(text: string): string {
  const parts = text.split(/(```[\s\S]*?```)/g);
  return parts
    .map((part, i) => {
      if (i % 2 === 1) return part; // code block — leave untouched
      return part.replace(/@(\w[\w.-]*)/g, '<span class="mention-highlight">@$1</span>');
    })
    .join('');
}
```

- [ ] **Step 3: Customize markdown components — table, strong, link, code label**

In the existing `ReactMarkdown` usage (around line 78), add the import:

```tsx
import rehypeRaw from 'rehype-raw';
```

Update `ReactMarkdown` to use `rehypePlugins` and process content:

```tsx
<ReactMarkdown
  remarkPlugins={[remarkGfm]}
  rehypePlugins={[rehypeRaw]}
  components={{...}}
>
  {highlightMentions(message.content)}
</ReactMarkdown>
```

In the `components` object, make these changes:

**Table header** (`th`) — change `bg-brutal-cream` to `bg-brutal-yellow`:
```tsx
th({ children }) {
  return (
    <th className="border-b-2 border-black bg-brutal-yellow px-3 py-2 text-left font-heading font-bold text-black">
      {children}
    </th>
  );
},
```

**Code block language label** — change `bg-brutal-yellow/30` to `bg-brutal-yellow` in the `CodeBlock` function (line 33):
```tsx
<div className="border-b-2 border-black bg-brutal-yellow px-3 py-1 font-mono text-[10px] font-bold uppercase tracking-wider text-black">
```

**Strong (bold)** — change to `font-black`:
```tsx
strong({ children }) {
  return <strong className="font-heading font-black">{children}</strong>;
},
```

**Link** — add brutalist cyan:
```tsx
a({ href, children }) {
  return (
    <a href={href} target="_blank" rel="noopener noreferrer"
       className="text-brutal-cyan font-bold underline decoration-2 underline-offset-2 hover:text-brutal-pink transition-colors">
      {children}
    </a>
  );
},
```

- [ ] **Step 2: Add mention highlight CSS to globals.brutal.css**

```css
/* ============================================================
   @mention highlight in agent messages
   ============================================================ */

.mention-highlight {
  display: inline;
  background: #fe7da8;
  color: #000;
  font-weight: 700;
  padding: 0 0.25rem;
}
```

- [ ] **Step 3: Type-check**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`
Expected: No new type errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/components/dashboard/agent-message.tsx frontend/app/globals.brutal.css
git commit -m "feat: add colorful markdown — yellow table headers, pink @mentions, cyan links"
```

---

### Task 6: Task Column Full-Saturation Headers

**Files:**
- Modify: `frontend/components/tasks/task-column.tsx`

- [ ] **Step 1: Update column header to full-saturation color bar**

In `frontend/components/tasks/task-column.tsx`, modify the `STATUS_COLUMN_CONFIG` to include a lighter variant for badges, and add a header background:

Already defined as:
```tsx
export const STATUS_COLUMN_CONFIG: Record<TaskStatus, { label: string; bgClass: string; textClass: string }> = {
  todo: { label: 'TODO', bgClass: 'bg-brutal-stone', textClass: 'text-black' },
  ...
};
```

These are already full-saturation. The issue is the column header rendering (lines 371-381) uses plain text without background. Change it to a colored bar:

```tsx
return (
  <div className="flex w-[280px] flex-shrink-0 flex-col">
    {/* Column header — full-saturation color bar */}
    <div
      className={cn(
        'mb-3 flex items-center gap-2 border-2 border-black px-3 py-2 shadow-brutal-sm',
        STATUS_COLUMN_CONFIG[status].bgClass,
        STATUS_COLUMN_CONFIG[status].textClass,
      )}
    >
      <h3 className="font-heading text-sm font-black tracking-tight">
        {label}
      </h3>
      <span className="flex h-5 min-w-[20px] items-center justify-center border-2 border-black bg-white px-1.5 font-mono text-[11px] font-bold text-black">
        {count}
      </span>
    </div>
    {/* ...rest stays the same... */}
  </div>
);
```

- [ ] **Step 2: Type-check**

No new files needed — just check for type errors:
Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`

- [ ] **Step 3: Commit**

```bash
git add frontend/components/tasks/task-column.tsx
git commit -m "feat: full-saturation color bars for task column headers"
```

---

### Task 7: Reply Count Badge Redesign

**Files:**
- Modify: `frontend/components/dashboard/message-list.tsx`

- [ ] **Step 1: Replace reply count text link with badge style**

In `frontend/components/dashboard/message-list.tsx`, lines 414-426:

Change from:
```tsx
{/* Thread reply count */}
{(message.reply_count ?? 0) > 0 && onReply && (
  <div className="mt-2">
    <button
      type="button"
      onClick={(e) => { e.stopPropagation(); onReply(message); }}
      className="flex items-center gap-1 text-xs font-heading font-bold text-brutal-pink hover:text-brutal-pink/80 transition-colors"
    >
      <MessageSquare className="h-3 w-3" />
      <span>{message.reply_count} 条回复</span>
    </button>
  </div>
)}
```

To:
```tsx
{/* Thread reply count — brutalist badge */}
{(message.reply_count ?? 0) > 0 && onReply && (
  <div className="mt-2">
    <button
      type="button"
      onClick={(e) => { e.stopPropagation(); onReply(message); }}
      className={cn(
        'badge-brutal cursor-pointer transition-all',
        hasUnreadThread
          ? 'bg-brutal-pink text-black border-brutal-pink'
          : 'bg-white text-black hover:bg-brutal-pink hover:-translate-y-px hover:shadow-brutal',
      )}
    >
      <MessageSquare className="mr-1 h-3 w-3" />
      <span>{message.reply_count} REPLIES</span>
    </button>
  </div>
)}
```

Note: `hasUnreadThread` is already computed at line 136. Move the reply count render to use it. The `cn` import is already in the file.

- [ ] **Step 2: Type-check**

Run: `cd frontend && npx tsc --noEmit 2>&1 | head -20`

- [ ] **Step 3: Commit**

```bash
git add frontend/components/dashboard/message-list.tsx
git commit -m "feat: redesign reply count as brutalist badge with unread state"
```

---

### Task 8: Channel Background Color

**Files:**
- Modify: `frontend/components/dashboard/channel-view.tsx`
- Modify: `frontend/components/dashboard/dm-view.tsx`

- [ ] **Step 1: Add subtle background to channel main area**

In `frontend/components/dashboard/channel-view.tsx`, the main content area uses `flex flex-1 flex-col overflow-hidden`. Add background:

In the messages tab area (around line 467-504), wrap content with a background:

Actually, simpler: add `bg-brutal-stone/15` to the main content container div (line 410 area):

```tsx
{/* Left: message area */}
<div className="flex min-w-0 flex-1 flex-col overflow-hidden bg-brutal-stone/15">
```

And for the tasks tab area (line 508-536):
```tsx
<div className="flex flex-1 flex-col overflow-hidden bg-brutal-stone/15">
```

Actually the header should retain its white/cream background for contrast. Let me apply the background only to the scrollable content area:

For messages tab:
```tsx
<MessageList ... />
``` 
is inside the `channelViewTab === 'messages'` block. The `bg-brutal-stone/15` should go on the wrapper or on the scrollable container.

Simplest approach: Add it to the top-level container but make the header have white bg:

The header already has `bg-white` implicitly. Just add `bg-brutal-stone/15` to the scrollable area divs that wrap MessageList and TaskBoard.

For messages, there's no explicit wrapper — the `MessageList` takes up full space. Let me look at the structure again.

Actually, the simplest approach is to create a wrapper div with the background:

```tsx
{/* Messages tab */}
{channelViewTab === 'messages' && (
  <div className="flex flex-1 flex-col overflow-hidden bg-brutal-stone/15">
    <MessageList ... />
    <MessageInput ... />
  </div>
)}
```

For tasks tab:
```tsx
{channelViewTab === 'tasks' && (
  <div className="flex flex-1 flex-col overflow-hidden bg-brutal-stone/15">
    {/* existing task content */}
  </div>
)}
```

- [ ] **Step 2: Apply same to DM view**

In `frontend/components/dashboard/dm-view.tsx`, find the message area wrapper and add `bg-brutal-stone/15`.

Since DMView has a similar structure, let me check the exact structure. I know it has a header + MessageList + MessageInput. Add `bg-brutal-stone/15` to the scrollable content area.

- [ ] **Step 3: Type-check and commit**

```bash
cd frontend && npx tsc --noEmit 2>&1 | head -20
git add frontend/components/dashboard/channel-view.tsx frontend/components/dashboard/dm-view.tsx
git commit -m "feat: add subtle channel background color (bg-brutal-stone/15)"
```

---

### Task 9: Integration Verification

**Files:**
- No new changes. Verify everything compiles and the dev server starts.

- [ ] **Step 1: Full TypeScript check**

Run: `cd frontend && npx tsc --noEmit 2>&1`
Expected: No errors.

- [ ] **Step 2: Build check**

Run: `cd frontend && npm run build 2>&1 | tail -20`
Expected: Successful build.

- [ ] **Step 3: Start dev server and check UI**

Run: `cd frontend && npm run dev`
Then manually verify:
1. NavBar shows on left with icons
2. Channel list panel next to it
3. No right member panel — member button in header works
4. Agent messages show pixel avatars
5. Table headers are yellow in agent replies
6. @mentions are highlighted pink in agent replies
7. Task columns have full-saturation headers
8. Reply count is a badge-style element
9. Channel area has subtle bg-stone/15

- [ ] **Step 4: Final commit for any fixes**

```bash
git add -A
git commit -m "fix: integration fixes from visual verification"
```
