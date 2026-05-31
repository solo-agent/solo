# Solo v1.3 -- Neubrutalist UI 视觉优化设计文档 v3.0

> 对比五个参考源与 Solo 当前设计系统，输出具体优化方案。
> 创建日期: 2026-05-17
> 更新日期: 2026-05-17 (v3.0: 新增 neobrutalism-components (ekmas) + neobrutal-ui (Bridgetamana) 深度对比; 补充 ImageCard/Marquee/Progress/Slider/Accordion/Drawer 评估; Button variant 对比; Card 子组件对比; 设计 token 对比)
> 对应迭代: v1.3 (Agent Team Collaboration)

---

## 1. 参考源概览

| 维度 | neubrutalism.com | brutalistui.site | neo-brutalism-ui-library (marieooq) | neobrutalism-components (ekmas) | neobrutal-ui (Bridgetamana) | Solo (当前) |
|------|------------------|------------------|--------------------------------------|----------------------------------|-----------------------------|-------------|
| 定位 | 设计理论权威指南 | npm 组件库 (CLI 安装) | React + Tailwind 组件集 (Copy-Paste) | shadcn/ui + Radix UI 组件集 | CLI 组件库 (`npx neobrutal add`) | SaaS 产品 (Agent 协作平台) |
| 技术栈 | 纯 CSS + HTML | React 18+ / TypeScript / Radix UI / Tailwind / Next.js 14 | React / Tailwind / Vite | React / Radix UI / Tailwind CSS | React / Tailwind | **Next.js 16 / Tailwind v4 / shadcn/ui** |
| 安装方式 | 无 (参考用) | `npx brutx@latest init` | 复制粘贴 | 复制粘贴 (shadcn/ui 模式) | `npx neobrutal add <c>` | 完整应用 |
| 暗色模式 | 无 | **有** (完整支持) | 无 | 无 | 无 | **无** |
| 组件数量 | N/A (理论) | 27 (完整文档) | 20 (Alert, Avatar, Badge, Button, Card, Checkbox, Dialog, Input, Label, Pagination, Popover, Progress, Radio Group, Select, Slider, Switch, Tabs, Textarea, Toast, Tooltip) | **19** (Accordion, Alert, Avatar, Badge, Button, Card, Checkbox, Dialog/Drawer, Dropdown, ImageCard, Input, Marquee, Modal, Newsletter, RadioGroup, Select, Tabs, Textarea, Tooltip) | **21** (Accordion, Alert, Avatar, Badge, Button, Card, Checkbox, Dialog, Input, Label, Pagination, Popover, Progress, Radio Group, Select, Slider, Switch, Tabs, Textarea, Toast, Tooltip) | 10 (shadcn/ui) + 17 (dashboard/tasks) |
| 无障碍 | WCAG 2.2 指导 | **Radix UI 基元** (A11y 内置) | 无明确声明 | **Radix UI 基元** (A11y 内置) | 无明确声明 | focus-visible + reduced-motion |
| 边框 | 3px | 3px (`border-3`) | 2px | 2px | 2px | 2px |
| 阴影策略 | 永久阴影 | 永久阴影 (4px, hover 6px) | hover 时出现 (2px) | **永久阴影 → hover 时消除** | 永久阴影 | 永久阴影 (5px, hover 7px) |
| 字重 | font-bold 700 | font-black 900 | 无明确定义 | font-bold 700 | font-bold 700 | font-heading 700 |
| 自定义配色前缀 | 无 | 无 | `xxx-` (xxx-violet, xxx-pink, xxx-red, xxx-orange, xxx-yellow, xxx-lime, xxx-cyan) | 无 | 无 | `brutal-` (brutal-pink, brutal-yellow 等) |

**关键发现:**

- **ekmas 是 Solo 的最近亲参考源** -- 同样使用 shadcn/ui + Radix UI + Tailwind CSS 技术栈，组件 API 高度兼容。它的 Button variant 设计 (default / noShadow / neutral / reverse) 和 Card 子组件拆分模式都值得直接参考。
- **marieooq 的自定义配色前缀 `xxx-`** 与 Solo 的 `brutal-` 前缀策略一致，均采用语义化命名 (violet/pink/red/orange/yellow/lime/cyan)，但 Solo 的颜色种类更丰富（8 色 vs 7 色 + light 变体）。
- **Bridgetamana 是参考源中组件覆盖最全的**（21 组件），且提供 CLI 安装方式，其组件清单可作为 Solo 组件完整度的对标基准。
- **ekmas 的阴影策略与其他所有源相反**：默认有阴影，hover 时 translate 到阴影位置使阴影消失，而非传统"抬升 + 阴影增大"。这是独特的"阴影吸收"隐喻。

---

## 2. 设计系统对比

### 2.1 色彩体系

| 角色 | neubrutalism.com | brutalistui.site | marieooq | ekmas | Bridgetamana | Solo |
|------|------------------|------------------|----------|-------|---------------|------|
| 背景 | `#FFFDF5` Off-White | `#FFFFFF` White | - | White | White | `#FFFAEF` Cream |
| 前景/文字 | `#000000` Black | `#000000` Black | `#000000` Black | `#000000` Black | `#000000` Black | `#000000` Black |
| 主色调 | `#FF6B6B` Coral Pink | `#FF6B6B` Coral Red | `#A6FAFF` Cyan | - (shadcn 默认) | - (通用) | `#fe7da8` Pink |
| 强调色 1 | `#FFD23F` Bold Yellow | `#FFE66D` Yellow | `#FFC29F` Peach | - | - | `#ffd440` Yellow |
| 强调色 2 | `#74B9FF` Sky Blue | `#4ECDC4` Teal | `#FF965B` Dark Peach | - | - | `#27ccf3` Cyan |
| 成功色 | `#88D498` Soft Green | N/A | - | - | - | `#a9d877` Lime |
| 危险色 | N/A (用 Pink) | `#FF6B6B` Coral (destructive) | - | - | - | `#f97264` Red |
| 配色前缀机制 | 无 | 无 | `xxx-` (7 色) | 无 | 无 | `brutal-` (8 色 + light 变体) |

**分析:**

- Solo 的色彩体系在数量和质量上均优于任何单个参考源（8 个语义颜色 + 对应的 light 变体），是最完整的设计系统级实现。
- marieooq 的 `xxx-` 前缀是唯一与 Solo 类似的配色前缀策略，但 Solo 额外提供了 light 变体和 stone 中性色。
- ekmas 和 Bridgetamana 均无独立配色体系，直接使用 Tailwind 默认色板或 shadcn/ui 的 CSS 变量。这意味着 Solo 的配色设计是差异化优势。
- 缺少: Solo 没有为结构化颜色分层级（如 `-100` / `-200` / `-300` 等级别），当需要更细粒度的色彩变体时只能依赖 opacity。

### 2.2 阴影系统

| 级别 | neubrutalism.com | brutalistui.site | marieooq | ekmas | Bridgetamana | Solo |
|------|------------------|------------------|----------|-------|---------------|------|
| xs | -- | -- | -- | 2px (inline) | -- | -- |
| sm | 3px (badges, chips) | -- | 2px (hover only) | -- | -- | **3px** (`shadow-brutal-sm`) |
| md | 5px (cards, buttons, panels) | **4px** (`shadow-brutal`) | -- | **4px** (默认) | **4px** (默认) | **5px** (`shadow-brutal`) |
| lg | 8px (overlays, hero) | **6px** (hover) | -- | **6px** (hover) | -- | **7px** (`shadow-brutal-lg`) |
| xl | -- | -- | -- | -- | -- | **10px** (`shadow-brutal-xl`) |

**新增 -- ekmas 的"阴影吸收"hover 策略（与所有其他源相反）:**

```
/* ekmas Button — 默认有阴影 */
.neobrutalism-button {
  box-shadow: 4px 4px 0 0 #000;
}
/* hover: translate 到阴影位置 -> 阴影被元素"覆盖"而消失 */
.neobrutalism-button:hover {
  transform: translate(4px, 4px);  /* 向右下移动 = 盖住阴影 */
  box-shadow: none;                /* 阴影消失 */
}
```

| 交互阶段 | Solo | brutalistui.site | marieooq | ekmas |
|----------|------|------------------|----------|-------|
| 默认 | shadow 5px | shadow 4px | shadow 0 | shadow 4px |
| Hover | translate(-1,-1) + shadow 5→7px | translate(-0.5,-0.5) + shadow 4→6px | translate(0) + shadow 0→2px | **translate(4,4) + shadow 4px→none** |
| Active | translate(3,3) + shadow none | translate(0.5,0.5) + shadow none | bg color 加深 | translate(0,0) + shadow 4px (回默认) |

**阴影策略总结:**

| 策略名称 | 代表源 | 隐喻 | 适用场景 |
|----------|--------|------|----------|
| "始终可见 + 物理抬升" | Solo, brutalistui.site | 卡片浮起（阴影扩大，元素离开表面） | **产品级应用（推荐保持）** |
| "hover 揭示" | marieooq | 元素被"点亮"（阴影从无到有） | 内容站点、作品集 |
| "阴影吸收" | ekmas | 元素"压住"阴影（元素覆盖阴影） | 趣味性、游戏化界面 |
| "零交互" | neubrutalism.com (静态) | 纯结构，无隐喻 | 理论正典 |

**结论:** Solo 当前的"始终可见 + 物理抬升"策略是正确的产品级选择。ekmas 的"阴影吸收"策略虽然有趣但不适合 SaaS 产品（hover 时元素位置变化不可预测，影响布局稳定性）。Solo 的 4 级阴影分级仍然是所有参考源中最细的。

### 2.3 边框

| 属性 | neubrutalism.com | brutalistui.site | marieooq | ekmas | Bridgetamana | Solo |
|------|------------------|------------------|----------|-------|---------------|------|
| 标准宽度 | 3px | **3px** (`border-3`) | 2px | **2px** | 2px | 2px |
| 圆角 | 0 | 0 | rounded-md / rounded-full | **0** | 0 | 0 |
| 变体 | 无明确定义 | `border-3` 单工具类 | `border-2` 单工具类 | `border-2` 单工具类 | `border-2` 单工具类 | 3 级 (thin=1px / standard=2px / thick=3px) |

**分析:**

- ekmas 严格遵循 0 圆角正典，与 Solo 一致。这是相对于 marieooq 的圆角方案（`rounded-md` / `rounded-full`）的重要区分。
- Bridgetamana 同样使用 2px 标准边框 + 0 圆角，与 Solo 的默认配置一致。
- Solo 的 3 级边框变体 (1px/2px/3px) 在参考源中独一无二，提供了更大灵活性。

### 2.4 圆角

- **neubrutalism.com**: `0px` (square corners)
- **brutalistui.site**: `0px` (square corners)
- **marieooq**: `rounded-md` / `rounded-full` (软 neubrutalism -- 不推荐)
- **ekmas**: `0px` (square corners)
- **Bridgetamana**: `0px` (square corners)
- **Solo**: `0px` (square corners)

**结论:** 5 个参考源中 4 个使用 0 圆角，仅 marieooq 使用圆角。Solo 的 0 圆角策略获得最强共识支持，不应改变。

### 2.5 字体

| 角色 | neubrutalism.com | brutalistui.site | ekmas | Solo |
|------|------------------|------------------|-------|------|
| Display | **Syne** 800 | -- | -- | Syne (via CSS var fallback) |
| Heading | Space Grotesk 700 | **font-black (900)** + Inter | Inter 600-700 | Space Grotesk (primary) |
| Body | Inter 400 | Inter 400 | Inter 400 | Inter 400 |
| Mono | Space Mono 400 | Space Mono 400 | -- | Space Mono |

**新增 -- ekmas 的字体策略:**

ekmas 不使用独立的 display/heading 字体分层，统一使用 Inter 作为正文字体，标题通过 font-weight 区分层级。这与 shadcn/ui 默认策略一致，但比 Solo 的 Syne + Space Grotesk + Inter 三层体系更简单。

**分析:**

- Solo 的字体栈仍然是最丰富的（三层字体体系），但 Syne 的实际使用场景极其有限
- ekmas 的简化字体策略提醒: 不使用的 Display 字体是无用配置
- 建议: 要么在至少 3 处关键场景使用 Syne Display，要么从配置中移除

### 2.6 动画与微交互

| 模式 | neubrutalism.com | brutalistui.site | marieooq | ekmas | Bridgetamana | Solo |
|------|------------------|------------------|----------|-------|---------------|------|
| 按钮 hover | N/A (静态) | translate(-0.5,-0.5) + shadow 4→6px | shadow 0→2px | **translate(offset) + shadow→none** | translate lift + shadow 增大 | translate(-1,-1) + shadow 5→7px |
| 按钮 press | N/A | translate(0.5,0.5) + shadow-none | bg color 加深 | translate(0,0) + shadow 恢复 | translate press + shadow 移除 | translate(3,3) + shadow-none |
| 卡片 hover | N/A | N/A | shadow 出现 | translate(offset) + shadow→none | translate lift + shadow 增大 | translate(-1,-1) + shadow 5→7px |
| 跑马灯 (Marquee) | **有** | N/A | N/A | **有** (独立组件) | N/A | 无 |
| Loading | N/A | 4 变体 Spinner | 无 | 无 | 无 | 仅 Loader2 (lucide) + Skeleton |
| Accordion | N/A | N/A | N/A | **有** (Radix 基元) | **有** | 无 |
| Drawer | N/A | N/A | N/A | **有** (Dialog variant) | N/A | 无 |
| ImageCard | N/A | N/A | N/A | **有** | N/A | 无 |
| Progress | N/A | N/A | **有** (ProgressBar) | 无 | **有** | 无 |
| Slider | N/A | N/A | **有** | 无 | **有** | 无 |
| 主题切换 | N/A | sun/moon icon toggle | 无 | 无 | 无 | 无 |

**新发现 -- ekmas 的 Marquee 组件:**

ekmas 是唯一提供 Marquee（跑马灯）组件的参考源。实现方式:
- 水平无限滚动 (`animate-marquee`)
- 支持 `reverse`、`pauseOnHover`、`vertical` props
- 适用场景: Solo 的 Agent 状态条 ("Agent Alpha is thinking... Agent Beta just replied...")

**新发现 -- ekmas 的 Drawer 组件:**

ekmas 以 Dialog variant 的形式实现 Drawer（侧边抽屉），而非独立组件。Solo 当前的面板（ThreadPanel、TaskDetailPanel）本就是 Drawer 的变体，可考虑统一。

---

## 3. 组件库对比

### 3.1 Solo 现有组件清单

**shadcn/ui 基元 (10):**
Button, Card, Input, Textarea, Label, Dialog, ScrollArea, Skeleton, Toast, Avatar

**业务组件 (17):**
Sidebar, ChannelList, ChannelView, DMList, DMView, MessageList, MessageInput, AgentMessage, StreamingMessage, ThreadPanel, MemberList, MentionDropdown, TypingIndicator, CreateChannelModal, CreateDMModal, AddAgentModal, DeleteChannelDialog

**任务组件 (7):**
TaskBoard, TaskColumn, TaskCard, TaskDetailPanel, TaskForm, TaskList, CreateTaskModal

### 3.2 五源组件覆盖矩阵

| 组件 | brutalistui.site | marieooq | ekmas | Bridgetamana | Solo | ekmas 技术栈匹配度 |
|------|-----------------|----------|-------|---------------|------|---------------------|
| **Accordion** | -- | -- | **有** (Radix) | **有** | 无 | **高** (Radix 基元) |
| **Alert** | 有 | 有 | 有 | 有 | 内联错误状态 | **高** (shadcn 模式) |
| **Avatar** | 有 | 有 | 有 | 有 | **有** (shadcn/ui) | 已有 |
| **Badge** | 有 | 有 | 有 | 有 | **有** (badge-brutal) | 已有 CSS 类 |
| **Button** | 有 (9 variant) | 有 | 有 (4 variant) | 有 | **有** (shadcn/ui) | 已有 |
| **Calendar** | 有 | -- | -- | -- | 无 | -- |
| **Card** | 有 | 有 | **有** (6 子组件) | 有 | **有** (shadcn/ui) | 已有 |
| **Checkbox** | 有 | 有 | 有 | 有 | 无 | **高** |
| **Combobox** | 有 | -- | -- | -- | MentionDropdown (变体) | -- |
| **Command** | 有 | -- | -- | -- | 无 | -- |
| **Dialog** | 有 | 有 | **有 (含 Drawer)** | 有 | **有** (shadcn/ui) | 已有 |
| **Drawer** | -- | -- | **有** (Dialog variant) | -- | Thread/TaskPanel (变体) | **高** |
| **DropdownMenu** | 有 | -- | **有** | -- | 无 | **高** |
| **ImageCard** | -- | -- | **有** (特有) | -- | 无 | **高** |
| **Input** | 有 | 有 | 有 | 有 | **有** (shadcn/ui) | 已有 |
| **Label** | 有 | 有 | -- | 有 | **有** (shadcn/ui) | 已有 |
| **Marquee** | -- | -- | **有** (特有) | -- | 无 | **高** |
| **Modal** | -- | -- | 有 | -- | 通过 Dialog | 已有替代 |
| **Newsletter** | -- | -- | 有 (特有) | -- | 无 | 低 (非核心) |
| **Pagination** | 有 | 有 | -- | 有 | 无 | 中 |
| **Popover** | 有 | 有 | -- | 有 | 无 | 中 |
| **Progress** | -- | **有** | -- | **有** | 无 | 中 |
| **Radio Group** | -- | 有 | 有 | 有 | 无 | **高** |
| **ScrollArea** | 有 | -- | -- | -- | **有** (shadcn/ui) | 已有 |
| **Select** | 有 | 有 | 有 | 有 | 无 | **高** |
| **Separator** | 有 | -- | -- | -- | 内联 border | 中 |
| **Skeleton** | 有 | -- | -- | -- | **有** (shadcn/ui) | 已有 |
| **Slider** | -- | **有** | -- | **有** | 无 | 中 |
| **Spinner** | 有 (4 变体) | -- | -- | -- | Loader2 (lucide) | -- |
| **SubmitButton** | 有 | -- | -- | -- | Button + disabled | -- |
| **Switch** | 有 | 有 | -- | 有 | 无 | 中 |
| **Table** | 有 (复合) | -- | -- | -- | 仅 Markdown 表格 | -- |
| **Tabs** | 有 | 有 | 有 | 有 | 嵌入在 ChannelView | **高** |
| **Textarea** | 有 | 有 | 有 | 有 | **有** (shadcn/ui) | 已有 |
| **Toast** | 有 | 有 | -- | 有 | **有** (shadcn/ui) | 已有 |
| **Tooltip** | 有 | 有 | **有** | 有 | 无 (仅有 title 属性) | **高** |

### 3.3 Button Variant 对比: Solo vs ekmas

ekmas 的技术栈 (shadcn/ui + Radix UI + Tailwind CSS) 与 Solo 最接近，其 Button variant 设计值得重点参考。

| 维度 | Solo | ekmas | 差异分析 |
|------|------|-------|----------|
| **variant 数量** | 6 (default/destructive/outline/secondary/ghost/link) | 4 (default/noShadow/neutral/reverse) | Solo 偏向语义化; ekmas 偏向视觉表现 |
| **default** | `btn-brutal-pink` (粉底黑字) | 白底黑字 + 4px shadow | Solo 的主色填充 vs ekmas 的白色中性 |
| **noShadow** | 无对应 | 白底黑字 + 无阴影 | Solo 可用 `shadow-none` 实现 |
| **neutral** | `btn-flat` (无边框无阴影无色) | 灰底黑字 + 无阴影 | Solo 的 ghost 更轻量 |
| **reverse** | 无对应 | 黑底白字 + 4px shadow | Solo 无反转色 variant |
| **destructive** | `bg-brutal-red` + 黑字 | 无独立 variant (通过 className 覆盖) | Solo 更语义化 |
| **hover 策略** | translate(-1,-1) + shadow 5→7px | **translate(4,4) + shadow 4px→none** | ekmas 的"阴影吸收" vs Solo 的"物理抬升" |
| **active 策略** | translate(3,3) + shadow-none | translate(0,0) + shadow 4px (回默认) | 截然相反的按压隐喻 |
| **border** | 2px | 2px | 一致 |
| **圆角** | 0 | 0 | 一致 |
| **font-weight** | 700 | 600-700 | 基本一致 |
| **asChild** | 无 | **Radix UI Slot** | ekmas 支持，Solo 缺少 |

**分析:**

ekmas 的 4 个 Button variant 是视觉表现维度的分类（"有没有阴影？什么颜色？"），而 Solo 的 6 个 variant 是语义维度的分类（"这个按钮是什么意图？"）。两种思路各有优劣:

- 语义 variant (Solo): 适合产品开发，意图清晰，易于维护一致性
- 视觉 variant (ekmas): 更灵活，组合自由度高，但需要更多设计约束

**建议:**
- Solo 保持当前的语义 variant 方向，这是产品级应用的正确选择
- 可参考 ekmas 的 `reverse` variant 新增 `variant="reverse"` (黑底白字)，用于 hero CTA / dark sections
- 可参考 ekmas 的 `noShadow` 概念，但通过 size/style 组合实现而非独立 variant

### 3.4 Card 子组件对比: Solo vs ekmas

ekmas 与 Solo 的 Card 均为 shadcn/ui 风格的复合组件模式，拆分子组件:

```
ekmas Card 结构:          Solo Card 结构:
Card                      Card          (match)
├── CardHeader            CardHeader    (match)
├── CardTitle             CardTitle     (match)
├── CardDescription       CardDescription (match)
├── CardContent           CardContent   (match)
└── CardFooter            CardFooter    (match)
```

| 维度 | Solo | ekmas | 差异 |
|------|------|-------|------|
| 子组件数量 | 6 | 6 | **完全一致** |
| 命名 | Card/Header/Title/Description/Content/Footer | Card/Header/Title/Description/Content/Footer | **完全一致** |
| 默认 border | 2px | 2px | 一致 |
| 默认 shadow | 5px | 4px | Solo 略大 |
| hover 行为 | translate(-1,-1) + shadow 7px | 无 hover 效果 (静态) | Solo 有抬升，ekmas 无 |
| 圆角 | 0 | 0 | 一致 |
| Header padding | p-6 | p-6 | 一致 |
| Content padding | p-6 pt-0 | p-6 pt-0 | 一致 |

**结论:** Solo 与 ekmas 的 Card 组件在结构层面**完全一致**（均为 shadcn/ui 标准模式）。唯一的风格差异是 hover 行为（Solo 有抬升动画，ekmas 无）和阴影偏移量（5px vs 4px）。

### 3.5 遗漏组件深度评估

#### Marquee (ekmas)

**描述:** 水平无限滚动跑马灯组件。
**ekmas 实现:**
- `animate-marquee` 关键帧动画
- 支持 `reverse`、`pauseOnHover`、`vertical` props
- 通过复制子元素实现无缝循环

**Solo 适用场景:**
- Agent 状态条: "Agent Alpha 正在分析... | Agent Beta 刚回复了 #general | Agent Gamma 空闲"
- 频道顶部公告栏
- 全局通知条

**优先级: P1** — 提升 Agent 协作感知的差异化功能。

#### ImageCard (ekmas)

**描述:** 图文卡片组件，图片 + 标题 + 描述的复合布局。
**ekmas 实现:**
- 图片带 `border-2` + `shadow`
- 下方标题 + 描述文本
- hover 有 translate 效果

**Solo 适用场景:**
- Agent 配置页的 Agent 头像卡片
- 文件/图片消息的预览卡片
- Task 的附件缩略图卡片

**优先级: P2** — 非阻塞功能，丰富内容呈现。

#### Accordion (ekmas, Bridgetamana)

**描述:** 可折叠展开的内容面板。
**ekmas 实现:**
- Radix Accordion 基元 (`@radix-ui/react-accordion`)
- `border-2` + `shadow`
- 支持 `type="single"` / `type="multiple"`

**Solo 适用场景:**
- Agent 配置面板 (Tools 列表折叠 / Runtime 参数折叠)
- 设置页面的分组
- FAQ / 帮助页面

**优先级: P1** — Agent 配置表单有明确的折叠需求。

#### Drawer (ekmas — Dialog variant)

**描述:** 侧边滑出面板。ekmas 将其作为 Dialog 的 variant 实现（而非独立组件）。
**ekmas 实现:**
- 基于 Radix Dialog
- `direction="left"` / `direction="right"` prop
- `border-2` + `shadow`

**Solo 对照:**
- Solo 的 ThreadPanel 本质上是右侧 Drawer (`animate-slide-in-from-right`)
- Solo 的 TaskDetailPanel 也是右侧 Drawer
- 两者当前未统一为 Drawer 组件，各自独立实现

**优先级: P1** — 可将 ThreadPanel 和 TaskDetailPanel 统一到 Drawer 基元上，减少重复代码。

#### Progress (marieooq, Bridgetamana)

**描述:** 进度条，色块填充 + 边框。
**marieooq 实现:**
- 内部 `div` 宽度百分比
- `border-2` + 0 圆角
- 支持 `value` (0-100) + `color` props

**Solo 适用场景:**
- Agent 任务执行进度
- Agent 流式输出进度 (token 计数)
- 文件上传进度

**优先级: P2** — 有明确场景但当前无阻塞。

#### Slider (marieooq, Bridgetamana)

**描述:** 范围滑块输入。
**实现:**
- 使用 `input[type="range"]` 自定义样式
- 或 Radix Slider 基元
- `border-2` thumb + track

**Solo 适用场景:**
- Agent 温度 (temperature) 参数调节
- 消息历史长度设置
- 通知频率设置

**优先级: P2** — Agent 参数配置场景需要，但 MVP 可用 Input number 替代。

---

## 4. 视觉层次分析

### 4.1 频道消息列表

**当前状态:**
- User 消息: `message-bubble` (white bg + 2px border + 3px shadow)
- Agent 消息: `border-l-brutal-pink` (3px left border pink) + cream bg + Bot icon
- Streaming 消息: 同上 + pink blink cursor
- Failed 消息: red-tinted bg + error text
- Hover actions: absolute positioned, group-hover 显示

**问题:**
1. Agent 消息与 User 消息的视觉对比不够强烈。Agent 仅靠左粉色边框 + cream 背景区分。brutalistui.site / neubrutalism.com 倾向于用完整色块区分角色类型。
2. Hover actions 在消息右侧弹出，容易溢出视口（尤其是窄屏）。brutalistui.site 使用 DropdownMenu 解决此问题。
3. "Channel beginning" 分割线使用 `border-t-2`，与 `divider-brutal` 类不一致。brutalistui.site 有专门的 Separator 组件。
4. 流式消息的 "..." loading dots 使用 `rounded-full`（圆形），与 neubrutalism 的 `border-radius: 0` 哲学冲突。

### 4.2 TaskBadge 视觉区分度

**当前状态:**
- `TASK_BADGE_CONFIG` (message-list.tsx): 左色条 + 浅色背景 + 2px border + 2px shadow
- `STATUS_COLUMN_CONFIG` (task-column.tsx): 全色背景 + 2px border + 2px shadow
- `STATUS_CONFIG` (task-card.tsx): 全色背景 + badge-brutal 类

**问题 (P0):**
1. **三套配置不一致**: 同一个 "in_progress" 状态在 MessageList 中是 `border-l-cyan` + `bg-cyan-light`，在 TaskColumn 中是 `bg-yellow`，在 TaskCard 中也是 `bg-yellow`。用户会困惑。
2. **颜色语义混乱**: 多处同时使用不同颜色表示相同状态。
3. **TaskBadge 状态 vs 进度指示**: 左色条表示状态分类，但色条本身不承载状态含义（被其他 UI 元素覆盖）。

### 4.3 Thread Panel 视觉一致性

**当前状态:**
- ThreadPanel: `border-2` + `shadow-brutal-sm` + `animate-slide-in-from-right`
- TaskDetailPanel: `border-l-2` + 无 shadow + `animate-slide-in-from-right`

**问题 (P1):**
1. 两个面板的外观不一致：ThreadPanel 有完整边框和阴影，TaskDetailPanel 只有左边框无阴影。
2. 均使用 `animate-slide-in-from-right` 但 ThreadPanel 在 `bg-cream` wrapper 内，TaskDetailPanel 直接在白色背景上。
3. 关闭按钮样式一致（`btn-brutal btn-brutal-sm`）-- 良好。
4. Header 结构一致（border-b-2 + h-14）-- 良好。

### 4.4 暗色模式支持

Solo **没有任何暗色模式支持**。brutalistui.site 通过 Tailwind dark mode 实现完整支持：

从 brutalistui.site 的实现中提取的关键 dark token:
- 背景: `dark:bg-gray-950`
- 文字: `dark:text-white`
- 边框: `dark:border-white`
- 阴影: `dark:shadow-[4px_4px_0px_0px_#FFFFFF]`
- 卡片: `dark:bg-gray-900`
- 表头: `bg-[#FFE66D]` (保持黄色不变)
- Sidebar 分隔: `dark:border-gray-700`

Neubrutalism 在暗色模式下的挑战:
- 黑色边框在暗色背景下不可见，需要反色（白色边框）
- 黑色阴影在暗色背景下不可见，需要改为亮色阴影
- 需要定义 `dark:` 变体下的颜色映射

**这是 v1.3 最大的设计缺口，但不一定是最优先的（取决于产品策略）。**

---

## 5. 优化建议

### P0 -- 本迭代必须修复（影响一致性与可用性）

#### P0-1: 统一 Status Badge 色彩配置

**现状:** `TASK_BADGE_CONFIG`、`STATUS_COLUMN_CONFIG`、`STATUS_CONFIG` 三套配置各自定义状态颜色映射。

**方案:** 统一为一个 `TASK_STATUS_DESIGN_TOKENS` 常量，包含三种显示模式：

```typescript
// lib/design-tokens.ts (建议新增)
export const TASK_STATUS_TOKENS = {
  todo:        { bg: 'bg-brutal-stone',     border: 'border-brutal-stone',     text: 'text-black' },
  in_progress: { bg: 'bg-brutal-yellow',    border: 'border-brutal-yellow',    text: 'text-black' },
  in_review:   { bg: 'bg-brutal-lavender',  border: 'border-brutal-lavender',  text: 'text-black' },
  done:        { bg: 'bg-brutal-lime',      border: 'border-brutal-lime',      text: 'text-black' },
  closed:      { bg: 'bg-brutal-red',       border: 'border-brutal-red',       text: 'text-white' },
} as const;
```

**影响面:** `message-list.tsx`, `task-column.tsx`, `task-card.tsx`, `task-detail-panel.tsx`

#### P0-2: Thread Panel 与 TaskDetailPanel 视觉统一

**方案:**
- TaskDetailPanel 添加 `border-2` (替代当前 `border-l-2`) + `shadow-brutal-sm`
- 统一 slide-in 动画和背景色（均在 `bg-white` 上，消除 wrapper 中的 `bg-cream` 差异）
- 长期: 统一为 Drawer 组件（参考 ekmas 的 Dialog Drawer variant）

#### P0-3: 补充缺失的 shadcn/ui 基元组件

**方案:** 新增以下 brutalist 风格基元组件。优先参考 **ekmas**（技术栈完全匹配）和 **brutalistui.site**（实现最完整）。

**3a. Select** -- 下拉选择器（P0，Agent 配置表单需要）
- 参考 ekmas: Radix Select + `border-2` + `shadow-brutal-sm` + `font-bold`
- 支持 placeholder, disabled options

**3b. Checkbox** -- 勾选框（P0，任务筛选需要）
- 参考 ekmas: `appearance-none` 自定义样式
  - `border-2` + `shadow-brutal-sm` + 0 radius
  - Checked: 背景色变化 + `after:` 伪元素 checkmark

**3c. Tooltip** -- 悬停提示（P0，图标按钮无障碍需要）
- 参考 ekmas Radix Tooltip + `border-2` + `shadow-brutal-sm` + `font-bold text-sm`

**3d. DropdownMenu** -- 右键/更多菜单（P0，Message hover actions 替代方案）
- 参考 ekmas: Radix DropdownMenu + `border-2` + `shadow-brutal` + `font-bold`
- 支持 separator, disabled items, keyboard navigation

### P1 -- 本迭代建议改进（提升品质）

#### P1-1: 新增 Loading Spinner 组件

**方案:** 参考 brutalistui.site 的 4 变体方案（Solo 可先实现 2-3 种）：
- `SpinnerBrutal` -- 几何图形旋转（方形旋转替代圆形，符合 neubrutalism）
- `SpinnerBlock` -- 方块脉冲（4 个方块 2x2 网格依次脉冲）
- `SpinnerDots` -- 3 个方块依次闪烁

每个支持 `size` (sm/md/lg) 和 `color` (pink/yellow/cyan/black) props。

**影响面:** 全局 loading 状态替换 Loader2 图标

#### P1-2: 新增 Tabs 通用组件

**现状:** ChannelView 中的"消息 / 任务"切换是嵌入的 inline tabs。

**方案:** 抽离为 shadcn/ui 风格的 `tabs.tsx` 组件，参考 ekmas 的实现（Radix Tabs 基元）：
- 水平标签页 (underline variant)
- 垂直标签页 (用于 Settings 页面)
- brutalist 样式: `border-b-2` 活动指示器 + `font-bold` / `font-black` heading

#### P1-3: 新增 Alert / Banner 组件

**方案:** 4 种变体 (info / success / warning / error)，参考 ekmas + brutalistui.site：
- 左色条 (3px，参考现有 Agent message 的 `border-l-brutal-pink` 模式)
- 可选关闭按钮 + 图标
- `font-bold` 标签 + `font-body` 内容
- `border-2` + `shadow-brutal-sm`

#### P1-4: 新增 Popover 组件

**方案:**
- 用户信息卡片 (hover 头像显示)
- @mention 的用户信息预览
- 富文本输入工具栏
- 参考 brutalistui.site: Radix Popover + `border-2` + `shadow-brutal`

#### P1-5: btn-flat 增强为 brutalist ghost

**现状:** `btn-flat` 无边框无阴影，失去结构感。

**方案:** 参考 brutalistui.site ghost variant + ekmas neutral variant，保留 2px 透明边框（hover 时变黑），维持结构锚点：
```css
.btn-ghost-brutal {
  border: 2px solid transparent;  /* 保持布局稳定 */
  background: none;
  color: #000;
}
.btn-ghost-brutal:hover {
  border-color: #000;
  background: rgba(0, 0, 0, 0.05);
}
```

#### P1-6: 流式消息 loading dots 去圆形化

**现状:** 流式消息的 loading dots 使用 `rounded-full`。

**方案:** 改为小方块 (`rounded-none`)，或者使用 Space Mono 的 `...` 字符闪烁：
```html
<span class="font-mono text-brutal-pink animate-pulse">...</span>
```

#### P1-7: Message hover actions 改为 DropdownMenu

**现状:** 4 个操作按钮 (reply/edit/delete/asTask) 在 group-hover 时显示，在窄屏时溢出。

**方案:** 保留 reply 为独立按钮（最高频操作），将其余 3 个操作收进 "..." DropdownMenu。

#### P1-8: Button variant 补充 success / accent 语义

**现状:** Solo 有 `btn-brutal-pink` 自定义 variant，但缺少 semantic intent variant。

**方案:** 为 Button 添加：
- `variant="success"` -- 使用 `bg-brutal-lime` + `text-black`
- `variant="accent"` -- 使用 `bg-brutal-yellow` + `text-black`
- `variant="reverse"` -- 使用 `bg-black text-white` (参考 ekmas reverse)

#### P1-9: Combobox 组件（替代当前 MentionDropdown 逻辑）

**现状:** `MentionDropdown` 是手动实现的 @mention 选择器，类似 combobox 行为。

**方案:** 参考 brutalistui.site Combobox (Popover + Command 复合)，重构 MentionDropdown：
- 支持键盘导航 (arrow up/down, enter)
- 支持 multi-select（如"选择多个 Agent"场景）
- 可复用于 Agent 选择器、搜索建议

#### P1-10: 新增 Accordion 组件

**方案:** 参考 ekmas（Radix Accordion 基元 + brutalist 样式）：
- `border-2` + `shadow-brutal-sm` 折叠面板
- 支持 `type="single"` / `type="multiple"`
- 应用于: Agent 配置面板的 Tools/Runtime 折叠区

#### P1-11: 统一 Thread/Task Panel 为 Drawer 组件

**方案:** 参考 ekmas（Radix Dialog Drawer variant）：
- 统一 `direction="right"` 面板
- `border-l-2` + `shadow-brutal-lg` 左侧硬阴影
- 替代当前 ThreadPanel 和 TaskDetailPanel 的独立实现

### P2 -- 后续迭代（提升完整度）

#### P2-1: 暗色模式支持

**方案:** 分阶段实现，参考 brutalistui.site 的 Tailwind dark mode 实现：

- Phase A: 定义 dark color tokens
  - 背景 `#FFFAEF` -> `#1a1a2e` (深蓝黑)
  - 前景 `#000` -> `#f0f0f0`
  - 边框 `#000` -> `#ffffff` (白色边框替代黑色)
  - 阴影 `#000` -> `rgba(255, 255, 255, 0.3)` 或 `#ffffff` offset
  - 卡片 dark: `bg-gray-900` / `dark:bg-gray-900`
  - 页面 dark: `bg-gray-950` / `dark:bg-gray-950`
- Phase B: 实现 ThemeProvider + `<html class="dark">` 切换
- Phase C: 持久化用户偏好 (localStorage `theme` key)

#### P2-2: 新增 Table 组件

**方案:** 参考 brutalistui.site 复合组件模式：
- `Table` + `TableHeader` + `TableBody` + `TableHead` + `TableRow` + `TableCell`
- `border-2` 外边框 + `border-b-2` 行分隔
- 黄色表头 `bg-brutal-yellow` 或其 light variant
- `font-black` 表头文字

#### P2-3: 新增 Calendar 组件

**方案:** 参考 brutalistui.site Calendar (基于 Radix)：
- 日期网格 + `border-2` + `shadow-brutal`
- 当天高亮: `bg-brutal-yellow`
- 选中日: `bg-brutal-pink` text-white
- 应用于: 消息搜索按日期过滤、任务截止日期选择

#### P2-4: 新增 Command 组件（Cmd+K 面板）

**方案:** 参考 brutalistui.site Command (基于 cmdk)：
- `border-2` + `shadow-brutal-lg` + 搜索输入框
- 分组快捷键提示
- 键盘导航

#### P2-5: 新增 Pagination 组件

**方案:**
- "上一页 / 下一页" + 页码方块
- 当前页：黑色填充 + 白色文字 (参考 ekmas reverse variant)
- 其他页：白色填充 + 黑色边框
- `border-2` + `shadow-brutal-sm` 基础

#### P2-6: Display 字体 (Syne) 的实际运用

**现状:** 配置了 `--font-display: Syne` 但几乎没有实际使用场景。

**方案:** 在以下场景使用 Syne 800：
- Workspace 名称 (Sidebar top)
- Task detail 大标题
- Agent onboarding hero
- Empty states 的大标题

#### P2-7: Agent 状态跑马灯 (Marquee)

**方案:** 参考 ekmas Marquee 组件，在以下场景使用：
- 频道顶部 Agent 活动状态条: "Agent Alpha is thinking..."
- Member list 中的 Agent 在线动态
- 全局通知横幅

#### P2-8: 新增 Separator 组件

**方案:** 统一当前代码中分散的 `border-t-2` 为 Separator 组件，参考 brutalistui.site：
- 水平/垂直方向
- `border-2` + `border-black`

#### P2-9: 新增 ProgressBar 组件

**方案:** 参考 marieooq ProgressBar：
- 色块填充 + `border-2` + 0 圆角
- 支持 `value` (0-100) + `color` props
- 应用于: Agent 任务进度、文件上传进度

#### P2-10: 新增 Slider 组件

**方案:** 参考 marieooq / Bridgetamana：
- 自定义 `input[type="range"]` 样式或 Radix Slider 基元
- `border-2` thumb + track
- 应用于: Agent temperature 参数、通知频率

#### P2-11: 新增 ImageCard 组件

**方案:** 参考 ekmas ImageCard：
- 图片 + 标题 + 描述的复合卡片
- `border-2` + `shadow-brutal`
- 应用于: Agent 头像卡片、文件预览

---

## 6. 设计 Token 对比与调整建议

### 6.1 阴影动效策略对比

| 策略维度 | Solo | brutalistui.site | ekmas | 建议 |
|----------|------|------------------|-------|------|
| 默认阴影 | 5px | 4px | 4px | 保持 5px（产品级应用需要更强的视觉锚点） |
| Hover 行为 | translate(-1,-1) + shadow 5→7px | translate(-0.5,-0.5) + shadow 4→6px | translate(4,4) + shadow→none | 保持当前 lift 策略 |
| Active 行为 | translate(3,3) + shadow-none | translate(0.5,0.5) + shadow-none | translate(0,0) + shadow 恢复 | 保持当前 press 策略 |
| 阴影分级 | 4 级 (3/5/7/10px) | 2 级 (4/6px) | 无明确分级 | 保持 4 级；考虑新增 `shadow-brutal-xs` (2px) |

### 6.2 Hover 行为设计 Token

```css
/* ---- 当前 Solo hover 策略 (推荐保持) ---- */
.btn-brutal:hover {
  transform: translate(-1px, -1px);   /* 左上抬升 */
  box-shadow: 7px 7px 0 0 #000;       /* 阴影扩大 */
}
/* 隐喻: "元素从表面浮起" -- 适合产品级应用 */

/* ---- ekmas hover 策略 (不推荐全局采用) ---- */
/* transform: translate(4px, 4px);    向右下移动 */
/* box-shadow: none;                   阴影消失 */
/* 隐喻: "元素压住阴影" -- 适合游戏化/趣味界面，但布局不可预测 */
```

### 6.3 配色方案 Token 对比

| 维度 | Solo | marieooq | 建议 |
|------|------|----------|------|
| 前缀策略 | `brutal-` | `xxx-` | 保持 `brutal-` |
| 颜色数量 | 8 语义色 + 8 light 变体 + 1 stone | 7 语义色 (violet/pink/red/orange/yellow/lime/cyan) | Solo 更完整 |
| 色阶体系 | 无 (仅 single + light) | 无 | 暂不需要，当前产品规模不需要 50-900 色阶 |
| 暗色变体 | 无 | 无 | P2 规划 |

### 6.4 阴影 Token (微调建议)

```css
/* 当前 */
.shadow-brutal { box-shadow: 5px 5px 0 0 #000; }
.shadow-brutal-lg { box-shadow: 7px 7px 0 0 #000; }

/* 建议 --- 匹配 neubrutalism.com 正典 (8px) */
.shadow-brutal-lg { box-shadow: 8px 8px 0 0 #000; }  /* 7px -> 8px */

/* 新增 --- inline 元素阴影 (chips, badges, keycaps) */
.shadow-brutal-xs { box-shadow: 2px 2px 0 0 #000; }
```

### 6.5 字重 Token

```css
/* 当前使用 */
.font-heading { font-weight: 700; }  /* Space Grotesk 700 */

/* brutalistui.site 使用 font-black (900) 作为标题基准 */
/* Solo 可以在特定场景采用 font-black 增强冲击力 */
.font-heading-heavy { font-weight: 900; }  /* 新增，用于 card title、hero text */

/* neubrutalism.com 推荐 */
Display: Syne 800  /* 当前已有字体但未使用此字重 */
Heading: Space Grotesk 700  /* 一致 */
Body: Inter 400  /* 一致 */
Mono: Space Mono 400/700  /* 一致 */
```

### 6.6 Press 效果 Token (已对齐)

```css
/* Solo 当前的 press 策略已经与 brutalistui.site 正典完全对齐: */
.btn-brutal:active {
  transform: translate(3px, 3px);  /* 被按下的方向 */
  box-shadow: none;                 /* 阴影完全消失 */
}
/* 不需要调整为 ekmas 的相反方案。 */
```

---

## 7. 实施计划

| 阶段 | 内容 | 预估工时 | 依赖 | 主要参考源 |
|------|------|----------|------|------------|
| P0-1 | 统一 Status Badge 色彩 | 2h | 无 | -- |
| P0-2 | Thread/TaskPanel 视觉统一 | 1h | 无 | -- |
| P0-3a | Select 组件 | 2h | 无 | ekmas + brutalistui.site |
| P0-3b | Checkbox 组件 | 1h | 无 | ekmas |
| P0-3c | Tooltip 组件 | 1.5h | 无 | ekmas |
| P0-3d | DropdownMenu 组件 | 2h | 无 | ekmas |
| P1-1 | Spinner 组件 (2-3 变体) | 1.5h | 无 | brutalistui.site |
| P1-2 | Tabs 组件 (抽离) | 1.5h | 无 | ekmas |
| P1-3 | Alert 组件 | 1h | 无 | ekmas + brutalistui.site |
| P1-4 | Popover 组件 | 2h | 无 | brutalistui.site |
| P1-5 | btn-ghost 增强 | 0.5h | 无 | brutalistui.site + ekmas |
| P1-6 | Loading dots 方形化 | 0.5h | 无 | -- |
| P1-7 | Message hover dropdown | 1.5h | P0-3d | -- |
| P1-8 | Button success/accent/reverse variant | 0.5h | 无 | brutalistui.site + ekmas |
| P1-9 | Combobox 组件 | 2h | 无 | brutalistui.site |
| P1-10 | Accordion 组件 | 1.5h | 无 | ekmas |
| P1-11 | Drawer 组件 (统一 Panel) | 2h | P0-2 | ekmas |
| P2-1 | 暗色模式 Phase A (tokens) | 3h | P0 全部完成 | brutalistui.site |
| P2-2 | Table 组件 | 2h | 无 | brutalistui.site |
| P2-3 | Calendar 组件 | 2h | 无 | brutalistui.site |
| P2-4 | Command 组件 (Cmd+K) | 2h | 无 | brutalistui.site |
| P2-5 | Pagination 组件 | 1.5h | P2-2 | brutalistui.site |
| P2-6 | Syne Display 运用 | 1h | 无 | neubrutalism.com |
| P2-7 | Marquee (Agent 状态跑马灯) | 1.5h | 无 | ekmas |
| P2-8 | Separator 组件 | 0.5h | 无 | brutalistui.site |
| P2-9 | ProgressBar 组件 | 1h | 无 | marieooq |
| P2-10 | Slider 组件 | 1h | 无 | marieooq / Bridgetamana |
| P2-11 | ImageCard 组件 | 1h | 无 | ekmas |

**P0 合计:** ~9.5h
**P1 合计:** ~14.5h (新增 P1-10 Accordion + P1-11 Drawer = +3.5h)
**P2 合计:** ~16.5h (新增 P2-10 Slider + P2-11 ImageCard = +2h)
**总计:** ~40.5h

---

## 8. 附录: 关键代码片段

### 8.1 统一 Status Token (P0-1)

```typescript
// lib/design-tokens.ts (建议新增)
export const TASK_STATUS = {
  todo:        { label: 'TODO',        bg: 'bg-brutal-stone',    text: 'text-black', border: 'border-l-brutal-stone' },
  in_progress: { label: 'IN PROGRESS', bg: 'bg-brutal-yellow',   text: 'text-black', border: 'border-l-brutal-yellow' },
  in_review:   { label: 'IN REVIEW',   bg: 'bg-brutal-lavender', text: 'text-black', border: 'border-l-brutal-lavender' },
  done:        { label: 'DONE',        bg: 'bg-brutal-lime',     text: 'text-black', border: 'border-l-brutal-lime' },
  closed:      { label: 'CLOSED',      bg: 'bg-brutal-red',      text: 'text-white', border: 'border-l-brutal-red' },
} as const;
```

### 8.2 工具类增强 (P0-3, P1-5)

```css
/* globals.brutal.css 新增 */

/* 超小阴影 (inline elements) */
.shadow-brutal-xs {
  box-shadow: 2px 2px 0 0 #000;
}

/* Ghost button -- 保留结构锚点 (参考 brutalistui.site + ekmas neutral) */
.btn-ghost-brutal {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-family: var(--font-heading);
  font-weight: 700;
  border: 2px solid transparent;
  background: none;
  color: #000;
  cursor: pointer;
  transition: border-color 0.1s ease, background 0.1s ease;
}
.btn-ghost-brutal:hover {
  border-color: #000;
  background: rgba(0, 0, 0, 0.05);
}

/* Spinner -- 几何旋转 (方形替代圆形) */
@keyframes spin-brutal {
  to { transform: rotate(90deg); }
}
.spinner-brutal {
  display: inline-block;
  width: 16px;
  height: 16px;
  border: 2px solid #000;
  animation: spin-brutal 0.6s steps(4) infinite;
}

/* Spinner -- 脉冲方块 (Block Spinner, 参考 brutalistui.site) */
@keyframes pulse-brutal {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.2; }
}
.spinner-pulse span {
  display: inline-block;
  width: 6px;
  height: 6px;
  background: #000;
  animation: pulse-brutal 1s ease-in-out infinite;
}
```

### 8.3 阴影级别语义修正

```css
/* 当前: shadow-brutal-lg = 7px */
/* 建议: shadow-brutal-lg = 8px (匹配 neubrutalism.com 正典) */
.shadow-brutal-lg {
  box-shadow: 8px 8px 0 0 #000;
}
```

### 8.4 Button reverse variant (参考 ekmas)

```css
.btn-brutal-reverse {
  background: #000;
  color: #fff;
  border: 2px solid #000;
  box-shadow: 5px 5px 0 0 #000;
}
.btn-brutal-reverse:hover {
  transform: translate(-1px, -1px);
  box-shadow: 7px 7px 0 0 #000;
}
.btn-brutal-reverse:active {
  transform: translate(3px, 3px);
  box-shadow: none;
}
```

### 8.5 新增字重工具类

```css
.font-heading-heavy {
  font-family: var(--font-heading);
  font-weight: 900;  /* brutalistui.site 标准 */
}
```

### 8.6 ekmas Marquee 动画参考

```css
/* ekmas 风格的跑马灯关键帧 */
@keyframes marquee {
  from { transform: translateX(0); }
  to { transform: translateX(-50%); }
}
.animate-marquee {
  animation: marquee 20s linear infinite;
}
.animate-marquee:hover {
  animation-play-state: paused;  /* pauseOnHover */
}
```

---

## 变更记录

| 日期 | 版本 | 变更 |
|------|------|------|
| 2026-05-17 | v1.0 | 初始版本，完成三个参考源的对比分析和优化建议 |
| 2026-05-17 | v2.0 | 新增 brutalistui.site/docs 和 neo-brutalism-ui-library 深度对比；新增 2 个参考源到概览表；扩充组件对比（27 vs 9 vs Solo）；新增 Combobox/Calendar/Command/ProgressBar/SubmitButton/Separator 组件建议；补充 Button 9 variants 对比、Spinner 4 变体对比、Table 复合组件分析、font-black 字重对比、Press 效果对比、暗色模式 token 提取；P1 新增 P1-8 (Button variant)、P1-9 (Combobox)；P2 新增 P2-3 (Calendar)、P2-4 (Command)、P2-8 (Separator)、P2-9 (ProgressBar) |
| 2026-05-17 | v3.0 | 新增 neobrutalism-components (ekmas) 和 neobrutal-ui (Bridgetamana) 两个参考源；概览表扩展至 5 源对比；新增 2.6 动画策略对比表（4 种 hover 策略）；新增 3.2 五源组件覆盖矩阵（35 组件横向对比）；新增 3.3 Button variant 对比 (Solo vs ekmas, 6 vs 4 variants)；新增 3.4 Card 子组件对比 (Solo vs ekmas, 结构完全一致)；新增 3.5 遗漏组件深度评估 (Marquee/ImageCard/Accordion/Drawer/Progress/Slider)；P1 新增 P1-10 (Accordion)、P1-11 (Drawer 统一)；P2 新增 P2-7 (Marquee 改为 ekmas 参考)、P2-10 (Slider)、P2-11 (ImageCard)；新增 6.1-6.6 设计 token 对比 (阴影动效策略 / hover 行为 / 配色方案 / 字重 / press 效果)；新增 8.4 Button reverse variant 代码片段、8.6 Marquee 动画参考；工时总计从 35h 更新至 40.5h |
