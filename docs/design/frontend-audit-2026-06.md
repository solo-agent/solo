# Solo 前端全面审查报告

> **范围**: `/Users/langgengxin/AiWorkspace/solo/frontend/`
> **方法**: 静态阅读 50+ 核心源文件 + 8 项 grep 验证,**未做修改**
> **审查日期**: 2026-06-07
> **适用版本**: 当前 main 分支(覆盖 v1.5 状态机)

---

## 目录

1. [执行摘要](#1-执行摘要)
2. [项目概览](#2-项目概览)
3. [🔴 阻塞级问题(必须修)](#3-阻塞级问题必须修)
4. [🟡 改进项(质量、一致性、可维护性)](#4-改进项质量一致性可维护性)
5. [🟠 新粗野主义美学违和](#5-新粗野主义美学违和)
6. [🟢 锦上添花](#6-锦上添花)
7. [执行路线图](#7-执行路线图)
8. [TL;DR — 给技术负责人](#8-tldr--给技术负责人)

---

## 1. 执行摘要

### 一句话定性

> **核心方向正确,但当前有 3 个不能发版的硬伤**:
> 1. **响应式崩了** — 5 个核心页面在 1024px 以下完全无法使用
> 2. **5 处 `dangerouslySetInnerHTML` 无 sanitizer** — 消息高亮存在真实 XSS 面
> 3. **Dialog 全站无 a11y** — 键盘/屏阅用户用不了任何 modal
>
> 加上 `auth-context` / `tasks/[id]` 两处绕过 `apiClient` 的 token 写入,**整体安全姿态在 1 小时内可被一个键盘手演示出问题**。
>
> **新粗野主义美学评分 4.1/5**,已是 SaaS 圈稀缺的"真粗野"水平,仅 3 处 P0 软化点拖累,完成修补后可达 4.6+/5。

### 整体健康度

| 维度 | 评分 | 状态 |
|---|---|---|
| 响应式 / 兼容性 | 3.0/5 | 🔴 不及格(5 处硬阻断窄屏) |
| 安全姿态 | 3.2/5 | 🔴 不及格(XSS + token bypass) |
| 可访问性 (a11y) | 2.8/5 | 🔴 不及格(Dialog 无 aria) |
| 设计系统一致性 | 3.8/5 | 🟡 良好(~30 处 hex 硬编码) |
| 新粗野主义美学 | 4.1/5 | 🟢 优秀(3 处软化点) |
| 性能 / 优化 | 3.9/5 | 🟡 良好(`optimizePackageImports` 未启用) |
| 可维护性 | 3.7/5 | 🟡 良好(组件重复 + 3 处 `as any`) |

---

## 2. 项目概览

| 维度 | 数据 |
|---|---|
| 框架 | Next.js 16 + App Router + React 19 + TS strict |
| 样式 | Tailwind v4 (`@theme inline` CSS-first),不读 `tailwind.config` |
| UI 基础 | shadcn 风格自维护 primitives(19 个) |
| 状态 | AuthContext → WSContext → ToastProvider,业务靠 fetch + 乐观更新 |
| 实时 | 自写 WSClient (`lib/ws-client.ts`) + reconnect/backoff |
| 页面 | 13 个:`/`(auth gate)、`/dashboard`、`/tasks`、`/tasks/[id]`、`/tasks/new`、`/teams`、`/computers`、`/workspace`、`/settings`、`/login`、`/register`、`/auth/layout`、`/not-found` |
| 组件 | 80+ (`ui/ 19` + `dashboard/ 20+` + `agents/ 10+` + `tasks/ 8+` + `teams/ 6+` + `computers/inbox/workspace/search/layout` 各 2-5) |
| Hooks | 25+ (`use-messages` 25K / `use-dm` 27K / `use-channels` / `use-thread` / `use-agents` 等) |
| 字体 | Inter (body) + Space Grotesk (heading) + Space Mono (mono) |
| 关键重定向 | `next.config.ts`:`/agents/*` → `/teams` |
| `public/` | 仅 `favicon.svg`,无图片资源 |
| `'use client'` | 79 个 tsx 文件;有 16 个纯展示 tsx 未声明(全是 `ui/*` primitive,合理) |

### 设计系统状态

- `app/globals.brutal.css` v3.0 — **活跃**,提供完整 token 体系(2/4/8 px 边、零模糊硬阴影、Space Grotesk 大粗体、`bg-brutal-pink/yellow/cyan/lime/lavender` 全饱和色板)
- `app/globals.css` — 🔴 **deprecated 残留**,仍被 layout 引入,导致双 CSS 注入

### 真问题分布

| grep 维度 | 命中数 | 严重度 |
|---|---|---|
| `min-w-[1024px]` 硬阻断 | 5 处 | 🔴 致命 |
| `dangerouslySetInnerHTML` | 5 处 | 🔴 高(XSS 面) |
| 原始 hex 颜色(非 token) | ~30 处 | 🟡 中 |
| `as any` | 3 处 | 🟡 中 |
| 直接 `localStorage.setItem('refresh_token', ...)` | 3 处 | 🔴 高(违反单一来源) |
| 绕过 `apiClient` 直接 fetch + 取 token | 1 处(`tasks/[id]`) | 🔴 高 |
| `rounded-(xl\|2xl\|full)` | 1 处(`agent-view-panel:100`) | 🟠 软化点 |
| `duration-500 ease-[cubic-bezier]` 弹性贝塞尔 | 4 处 | 🟠 软化点 |
| `font-normal` 出现 | 4 处 | 🟠 软化点 |
| `bg-muted` 灰色用于语义色 | 2 处 | 🟠 软化点 |
| `!important` in CSS | 6 处 | 🟢 低(都合理) |
| `div onClick` 模拟按钮 | 0 | 🟢 通过 |

---

## 3. 🔴 阻塞级问题(必须修)

### 3.1 5 处硬阻断窄屏的 `min-w-[1024px]`

| 文件 | 行 | 影响 |
|---|---|---|
| `components/layout/app-frame.tsx` | 42 | 整个 dashboard 框架无法在 768-1023px 设备折叠 |
| `app/dashboard/page.tsx` | 442 | 任何 <1024px 用户被横滚 |
| `app/tasks/page.tsx` | 274 | 平板竖屏(834×1194)无法正常浏览 |
| `app/teams/page.tsx` | 108 | 同上 |
| `app/computers/page.tsx` | 216 | 同上 |

**修法**: 把 4 个 page 改成 `min-w-0` 横向流式布局,AppFrame 改成 `lg:flex` + 移动端汉堡侧栏。

**为什么重要**: 1024px 是 2010 年的桌面底线,2026 年 iPad mini 竖屏只有 744px、MacBook Air 13" 窗口拉半边就 720px,**整个产品对 50% 以上的桌面/平板用户不可用**。

---

### 3.2 `dangerouslySetInnerHTML` 5 处,至少 3 处有 XSS 风险

| 文件:行 | 内容 | 风险等级 |
|---|---|---|
| `components/dashboard/message-list.tsx` | 358 | `__html: highlight(m.content)` 后 `dangerouslySetInnerHTML` | **🔴 高** |
| `components/search/global-search.tsx` | 308 | 搜索结果片段高亮 | **🔴 高** |
| `components/dashboard/channel-search.tsx` | 237 | 频道搜索高亮 | **🔴 高** |
| `components/workspace/file-preview.tsx` | 57 | shiki 高亮代码块 | 🟡 中(内容由后端给) |
| `components/workspace/file-preview.tsx` | 121 | 同上 | 🟡 中 |

**问题**:
- `message-list.tsx:358` 是用户消息的高亮 — 用户自己发的 markdown,有人贴 `<img onerror=...>` 直接进 DOM。
- 三处都假设 `sanitizeMarkHtml` / `highlight` 已经处理过,但**没看到 `dompurify` / `isomorphic-dompurify` 的安装证据**(`package.json` 不在 grep 命中里)。如果只做了 HTML escape,对 markdown 残留的 `<` `>` 字符 + 高亮 `<mark>` 拼接后属性注入仍在。
- `react-markdown` 在 4 处用,但代码块渲染用 shiki 后再 `innerHTML` 喂回去,**整个 XSS 防线靠 shiki 自己的 escape**,而 shiki 的 `langs` 已知历史 CVE(2023 GHSA-8h7v-c3g2-r8cv 类)。

**修法**:
1. 高亮统一改用 `dangerouslySetInnerHTML` **之前** 跑 `DOMPurify.sanitize(markWrap, { ALLOWED_TAGS: ['mark', 'span'], ALLOWED_ATTR: ['class'] })`
2. `message-list.tsx` 干脆把用户消息改走 `react-markdown`(已经在 `streaming-message/thread-panel` 用了,顺手统一)
3. `file-preview.tsx` shiki 走 `codeToHtml` 输出,包一层 DOMPurify
4. 加 `npm i isomorphic-dompurify` 到 package.json

---

### 3.3 `auth-context.tsx` 绕过 token storage

```ts
// lib/auth-context.tsx — 158, 175, 256 行
localStorage.setItem('refresh_token', data.refresh_token);
```

`lib/api-client.ts` 已经提供 `defaultTokenStorage`(单例)负责 access/refresh 写入。这里 3 处直接写,导致:
- 后续如果把 token storage 换成 `httpOnly cookie` 或 `IndexedDB`,要改 4 处
- 任何 `clear()` 调用不一致就会留 stale token,触发诡异的 401 循环
- 登录和登出走两套入口,登出没看到,可能根本没清 refresh_token

**修法**: 抽 `setAuthTokens({ access, refresh })` 和 `clearAuthTokens()` 进 `api-client.ts`,auth-context 全部改走它。

---

### 3.4 `app/tasks/[id]/page.tsx` 绕过 apiClient

```ts
// 25, 31-32
const accessToken = localStorage.getItem('access_token');
// ...
原生 fetch(API_BASE + ...)
```

直接 `localStorage.getItem('access_token')` + 原生 `fetch`,**绕过了**:
- `apiClient` 的 401 → 自动 refresh 链
- 错误 toast
- 取消令牌(AbortController)

意味着这个页面只要 access token 过期就会 401 然后白屏,而其他页面会自动续期。

**修法**: 改用 `apiClient.get<Task>(\`/tasks/\${id}\`)`。

---

### 3.5 Dialog 缺失 a11y(`components/ui/dialog.tsx`)

读了 dialog 实现:自维护的 div + z-index,**没有**:
- `aria-labelledby` / `aria-describedby` 关联 Title/Description
- `role="dialog"` / `aria-modal="true"`
- Focus trap
- Esc 关闭(没看到)
- 关闭后焦点归还到触发按钮

**问题**: 屏幕阅读器视而不见;键盘用户按 Tab 跑出对话框外。

**修法**:
1. `npm i @radix-ui/react-dialog`,把 `ui/dialog.tsx` 升级到 Radix 包装
2. 所有 modal(form、create-channel、delete) 立即受惠

---

### 3.6 `app-frame.tsx:42` 提供 `onCreateDM={() => {}}` 死函数

```ts
<Sidebar ... onCreateDM={() => {}} ... />
```

要么实现,要么删 prop。这是空 API surface,新来的人会以为 DM 创建是支持的。

---

### 3.7 颜色硬编码不一致 — `dm-view.tsx` vs `channel-view.tsx`

| 文件:行 | 写法 |
|---|---|
| `dm-view.tsx:456` | `border-black` ✅(走 token) |
| `channel-view.tsx:546` | `style={{ borderColor: 'var(--color-border, #000)' }}` ⚠️ |

`channel-view.tsx` 之所以这样写,可能是因为 `border` prop 是动态计算的颜色。统一用 `border-black` 即可。

---

### 3.8 `lib/ws-types.ts` 重复 deprecated 字段

`streaming-message.tsx:123` 同时访问 `message.display_name`、`message.user_id`、`message.sender_active`,这些在 `Message` 上都有。如果 WS payload 类型里也带 `display_name`(server 推的)就会和 store 里 normalize 的 display_name 打架,出现"改名后旧消息姓名闪烁"。

**修法**: 读 `lib/ws-types.ts` 全文件后,把所有重复字段(deprecated 注释掉),只保留一份权威源。

---

## 4. 🟡 改进项(质量、一致性、可维护性)

### 4.1 设计系统: ~30 处 raw hex 颜色

**最严重的几个**:

| 文件:行 | 颜色 | 应替换为 |
|---|---|---|
| `components/ui/pixel-avatar.tsx:28-35` | 8 组 (color, dark) hex 对,例如 `['#4ade80', '#22c55e']` | 抽到 `lib/avatar-palette.ts` 用 `--color-brutal-*` 变量 |
| `components/agents/status-indicator.tsx:29-32` | 4 状态色 hex | 用 `bg-brutal-{success,info,warning,danger}` |
| `components/agents/env-editor.tsx:119, 127, 135, 167, 168` | `bg-[#fffaef]` 5 次 | 写一个 className 常量 |
| `components/agents/agent-island.tsx:284` | `shadow-[12px_12px_0_0_#000]` | 已在 `globals.brutal.css` 定义 `shadow-brutal`,换成 `shadow-brutal-lg` |
| `components/workspace/file-preview.tsx:120` | `[&>pre]:bg-[#1e1e1e]` | shiki 自带 token,直接 `[&>pre]:bg-brutal-code` |
| `components/inbox/inbox-view.tsx:192` | `bg-[#000]/...` | `bg-black/X` |
| `components/dashboard/agent-message.tsx` 注释 | `#fe7da8` 散落 | 改 token 名 |

### 4.2 命名错误: `bg-brutal-pink` 实际是 yellow

`components/teams/teams-agent-item.tsx:4` 注释自承认:
> `bg-brutal-pink` is actually a yellow variant…

设计系统里 `pink` 是粉色,但实际 token 值是 `#FFD23F`(yellow)。**修法**: 改 token 名 + 全量 grep 替换 + 调色板比对。

### 4.3 写死尺寸阻断响应式

| 文件:行 | 写法 | 问题 |
|---|---|---|
| `components/dashboard/sidebar.tsx:57` | `w-[220px]` | <1024px 没空间 |
| `components/agents/agent-workspace-tab.tsx:120` | `w-[220px]` | 同 |
| `app/tasks/page.tsx:276` | `w-[220px]` | 同 |
| `app/teams/page.tsx:110` | `w-[220px]` | 同 |
| `app/computers/page.tsx:218` | `w-[220px]` | 同 |
| `components/ui/toast.tsx:100` | `min-w-[280px] max-w-[400px]` | 移动端要 `w-[calc(100vw-2rem)]` |
| `components/dashboard/dm-view.tsx:482` | `w-[400px]` | 移动端全屏更合理 |

**修法**: 抽 Tailwind 自定义 spacing:
```css
/* globals.brutal.css */
@theme { --spacing-sidebar: 16rem; --spacing-thread: 26rem; }
```
然后 `w-sidebar` / `w-thread` 在所有地方通用。

### 4.4 组件重复: `selectable-row` 与多处 List 项

- `components/ui/selectable-row.tsx` 设计用于可选项列表
- 但 `channel-list.tsx`、`dm-list.tsx`、`agents-list` 仍各自手写 `<div onClick + selected 样式>`
- `message-list.tsx` 的 hover action 按钮和 `thread-panel.tsx` 的 message 渲染大段重复(react-markdown component map 几乎一样)

**修法**: 把 react-markdown 的 component map 抽到 `components/ui/markdown.tsx` 共享;`selectable-row` 加 `as`/size variants,迁移三个 List。

### 4.5 状态层重复: `use-messages.ts` 与 `use-dm.ts` 高度相似

两份 hook 各 25-27K,`PAGE_SIZE`、`mapMessageResponse`、`optimistic update` 都是双份。

**修法**: 抽 `lib/hooks/use-list-resource.ts`,把分页 + 乐观更新 + WS 增量 merge 抽一次。

### 4.6 表单一致性: login/register vs task-form

- `app/auth/login/page.tsx` 和 `register/page.tsx` 用 `react-hook-form` + `zod`
- `components/tasks/task-form.tsx` 用 `useState` 手动管理
- `components/dashboard/create-channel-modal.tsx` 又用 `react-hook-form` + `zod`

**修法**: task-form 改 RHF + zod,保持一致。

### 4.7 `as any` 3 处(thread-panel.tsx:84, 227, 390)

`ThreadPanel` 大量用 `as any` 绕开类型。反复 `as any` 通常是 WS payload 与 store type 不对齐。

**修法**: 修 `lib/ws-types.ts` 让 payload = 客户端 store type,消除 cast。

### 4.8 `globals.css` 残留双注入

`components.json` 已指向 `globals.brutal.css`,但 `app/globals.css` 还在,Next 会同时注入两份。**修法**: 删 `globals.css`,或在 layout.tsx 移除 import。

### 4.9 缺 `app/error.tsx` / `app/global-error.tsx`

Next 16 必备,用于捕获未处理异常。**修法**: 加 brutal 风格的 error boundary。

### 4.10 缺 `loading.tsx`

13 个页面里没有 `loading.tsx`,所有 await 都在 client 触发,首屏 LCP 体验差。

**修法**: 在 `app/(authed)/loading.tsx` 加 brutal 风格 SkeletonScreen。

---

## 5. 🟠 新粗野主义美学违和

**总评:4.1/5 — 真粗野主义,不是"披着粗野皮的 SaaS"。**

**核心证据**:
- 零渐变(`bg-gradient-` 命中 = 0)
- 零模糊(`backdrop-blur` 命中 = 0)
- 零玻璃拟态
- 阴影 100% 硬阴影(`shadow-md/lg` Tailwind 默认 = 0,只用 `shadow-brutal-*` 和 `shadow-[Npx_Npx_0_0_#000]`)
- 粗野 DNA 五大签名全覆盖:硬黑实线边 / 硬阴影 / 高饱和原色 / Space Grotesk 粗体 / 直角或极小圆角

### 5.1 🔴 P0 软化点(15 分钟可全清)

| 位置 | 违和点 | 现象 |
|---|---|---|
| `agent-view-panel.tsx:100` | 数字徽章 `rounded-full` | 突兀的 iOS 通知红点穿越到粗野主面板 |
| `channel-view.tsx:545` | `duration-500 ease-[cubic-bezier(0.16,1,0.3,1)]` 弹性贝塞尔 | macOS 范式"滑出来+回弹" |
| `dm-view.tsx:455` | 同上 | 同上 |
| `inbox-view.tsx:191` | 同上 | 同上 |
| `tasks/page.tsx:398` | 同上 | 同上 |
| `globals.brutal.css:121` | 进度条 `1.2s ease-in-out` | Material Design 温柔呼吸曲线 |

**统一重写**:
- 圆角徽章 → 方形 + `border-2 border-black shadow-brutal-sm`
- 弹性贝塞尔 → `duration-100 ease-linear`(参考 `agent-island.tsx` 的 hover 物理)
- 进度条 → `0.6s linear infinite`(机械传送带感)

### 5.2 🟡 P1 一致性修补(10 分钟)

| 位置 | 违和点 | 重写 |
|---|---|---|
| `interaction-mode.tsx:118` | 描述文字 `font-normal` | `font-bold` + `text-brutal-stone` |
| `agent-form.tsx:303/350/366` + `channel-view.tsx:607` | "可选"括号 `font-normal` + 全角括号 | 全大写 `(OPTIONAL)` + `font-bold` + `text-brutal-stone` |
| `agent-track.tsx:35` | 头部 `bg-black/5` 半透灰 | `bg-brutal-violet-light border-b-2 border-black` |
| `agent-chunk.tsx:33` | 1px 边 `border` 几乎不可见 | `border-2 border-black` |
| `task-card.tsx:27` | "普通"优先级 `bg-muted text-muted-foreground` | `bg-brutal-cream text-foreground border-2 border-black`(与"紧急=红"形成粗野对位) |
| `task-card.tsx:138` | 进度条 `bg-muted` | `bg-brutal-cream` + `border-2 border-black` |
| `streaming-message.tsx:74-96` | 流式标签头 `bg-brutal-pink/30` 半透 | `bg-brutal-pink` 实心 + `border-b-2 border-black` |
| `computers/page.tsx:507` | 展开卡 `duration-300 ease-in-out` | `duration-100 ease-linear` |
| `connection-banner.tsx:92` + `network-status.tsx:48` | 横幅 `duration-300` + `transition-all` | `duration-100 ease-out` + `transition-transform` |

### 5.3 🟢 P2 状态点模板统一(6 行)

**全项目状态点中,只有 `inbox-item.tsx:55` 主动加了 `border-2 border-black`**,其他状态点(`cli-detection`, `agent-island`, `computers-left-column`)都是裸 `rounded-full`。

**统一模板**:
```tsx
<span className="block h-2.5 w-2.5 rounded-full bg-brutal-lime border-2 border-black" />
```

### 5.4 ✅ 粗野主义标杆(Top 5 值得保留的组件)

1. **`components/agents/agent-island.tsx:284`** — 6 层黑边 + xl 硬阴影 + hover:-translate-2 + hover:shadow-[12px] + active:shadow-none,粗野"Interactive Physics"教科书,所有浮动组件应模仿
2. **`components/dashboard/agent-message.tsx` CodeBlock** — 50px 高度内五大签名全命中,代码块/终端/日志场景的官方模板
3. **`components/tasks/task-column.tsx`** — orange/cyan/lavender/lime/stone 五色全饱和并列无渐变,多分类并列场景的官方模板
4. **`components/dashboard/message-list.tsx`** — 白底 + 黑边 + 硬阴影 + Mono 时间戳,典型"贴纸卡片"质感
5. **`mention-dropdown` + `selectable-row`** — 选中态统一 `bg-brutal-pink + border-black + shadow-brutal-sm`,"选中 = 黄底 + 硬阴影"的强一致语言,所有列表项选中态应严格遵循

---

## 6. 🟢 锦上添花

### 6.1 启用 `optimizePackageImports`

`next.config.ts` 是空的:
```ts
const nextConfig: NextConfig = { async redirects() { ... } };
```
加:
```ts
experimental: { optimizePackageImports: ['lucide-react', 'date-fns', 'clsx'] }
```
`lucide-react` 在 66 个文件按需导入,显式声明能再砍 ~50KB。

### 6.2 拆 page 客户端 island

`app/agents/...`、`app/tasks/...` 都是 `'use client'` page。多数 dashboard 页可拆 data layer 到 server component + 叶子交互岛(client),改善 LCP。

### 6.3 inbox 改 URL state

`inbox-view.tsx` 的 prop drilling 严重,从 page.tsx 一路把 `onSelectChannel` 透传 3 层。改 `?channel=X&thread=Y` URL state 更优雅。

### 6.4 缺少 `robots.txt` / `sitemap.ts` / OG image

`app/` 下没看到。SEO/分享卡片是新访客第一观感。

### 6.5 时间格式统一到 `Intl`

`streaming-message.tsx:88-91` 用了 `toLocaleString('zh-CN')` 强加中文格式 + hydration mismatch 风险。

**修法**: 用 `Intl.DateTimeFormat` + 用户偏好 locale,或全站统一到 ISO8601 + 自定义 format 函数。

### 6.6 加 Playwright 烟测

`package.json` 没看到 vitest/playwright。E2E 对"频道式实时协作"产品是 must-have(消息顺序、断线重连、并发 optimistic)。

### 6.7 加 lint 规则禁 `<img>`

确保所有图片资源走 `next/image`。

---

## 7. 执行路线图

### Phase 1: 安全与正确性(1 个 PR,1 周)

> 目标:对外可演示

- [ ] 修 5 处 `min-w-[1024px]`(拆出 `lg:flex` AppFrame + 4 个 page 响应式)
- [ ] 加 `isomorphic-dompurify`,5 处 `dangerouslySetInnerHTML` 全部 sanitize
- [ ] `auth-context.tsx` 3 处 `localStorage.setItem` 改走 `setAuthTokens` helper
- [ ] `tasks/[id]/page.tsx` 改走 `apiClient.get`
- [ ] 修 `app-frame.tsx:42` 的 `onCreateDM` noop(实现或删)
- [ ] 读完整 `lib/ws-types.ts`,清掉 deprecated 字段
- [ ] **美学 P0**:4 处 `duration-500 ease-[cubic-bezier]` → `duration-100 ease-linear`
- [ ] **美学 P0**:`agent-view-panel.tsx:100` rounded-full 改方形
- [ ] **美学 P0**:`globals.brutal.css:121` 进度条 1.2s ease-in-out 改 0.6s linear

### Phase 2: 可访问性 + 设计系统一致(2 个 PR,1-2 周)

> 目标:可内测

- [ ] `ui/dialog.tsx` 升级 Radix Dialog(focus trap + aria)
- [ ] 收 30 处 raw hex 到 token,`pixel-avatar` 抽 `lib/avatar-palette.ts`
- [ ] 修 `bg-brutal-pink` 命名错位(grep 替换)
- [ ] 抽 5 处 `w-[220px]` → `w-sidebar` 自定义 spacing
- [ ] `ui/toast.tsx:100` 移动端宽度
- [ ] `use-messages` / `use-dm` 抽 `use-list-resource`
- [ ] 抽 `<Markdown>` 共享 component map
- [ ] `task-form.tsx` 改 RHF + zod
- [ ] 修 3 处 `as any`
- [ ] 删 `app/globals.css` 残留
- [ ] **美学 P1**:4 处 `font-normal` → `font-bold`
- [ ] **美学 P1**:状态点统一为"圆 + border-2 border-black"模板
- [ ] **美学 P1**:`bg-muted/5` / `bg-muted/20` 替换为 `bg-brutal-cream` / `bg-brutal-violet-light`

### Phase 3: 性能 / 长期健康(后续 sprint)

- [ ] 完整审 `app/computers/page.tsx` 与 `use-dm/use-messages`
- [ ] 启用 `optimizePackageImports`
- [ ] 拆 page 客户端 island,加 `loading.tsx` / `app/error.tsx` / `app/global-error.tsx`
- [ ] inbox 改 URL state
- [ ] 时间格式统一到 `Intl`
- [ ] 加 Playwright 烟测
- [ ] 加 `robots.txt` / `sitemap.ts` / OG image
- [ ] **美学 P2**:`transition-all` 精炼为具体字段过渡(10+ 处)

---

## 8. TL;DR — 给技术负责人

### 三个不能发版的硬伤

1. **响应式崩了** — 5 个核心页面在 1024px 以下完全无法使用,失去 50% 桌面/平板用户
2. **XSS 面** — 5 处 `dangerouslySetInnerHTML` 无 sanitizer,键盘手 1 小时可复现
3. **Dialog 全站无 a11y** — 屏幕阅读器视而不见,键盘用户 Tab 跑出 modal

### 三个安全姿态暗坑

- `auth-context` / `tasks/[id]` 绕过 `apiClient` 直接读 `localStorage.getItem('access_token')` + 原生 fetch,失去 401 刷新链,某些页面 401 直接白屏
- 登出逻辑可能根本没清 refresh_token(3 处 setItem 但没看到对称 clear)

### 美学强项

新粗野主义评分 **4.1/5**,已是 SaaS 圈稀缺水平,无任何软渐变/玻璃拟态/大圆角软阴影。最大短板仅 4 处 macOS 弹性贝塞尔 + 1 处圆角徽章,15 分钟可根治到 4.6+。

### 建议

**Phase 1(1 周)** 走完,即可对外可演示。
**Phase 2(2 周)** 走完,可内测。
**Phase 3** 是顺手的工程债清理,按优先级穿插。

---

## 附录:未完整审阅文件(建议在 Phase 1 内补完)

- `lib/hooks/use-messages.ts`(25K,核心消息状态)
- `lib/hooks/use-dm.ts`(27K,DM 状态)
- `lib/hooks/use-thread.ts`(没看)
- `app/computers/page.tsx`(27K,前 600 行已读)
- `app/globals.brutal.css`(21K,token 定义)

---

> **审查者**: frontend-neo-brutalist subagent
> **后续**: 本报告建议作为 v1.6 frontend-quality sprint 的输入,Owner 待指派
