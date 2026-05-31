# UI Brutalism Polish — Design Spec

> 创建日期: 2026-05-31
> 分支: `ui-brutalism-polish`
> 风格: Neubrutalist (参考 docs/v1.3/UI-DESIGN-v1.3.md)

---

## 1. 布局重构: 双栏 + 去右侧面板

```
Before: [Sidebar 240px]        [主内容区 flex-1]             [MemberList 224px]
After:  [NavBar 56px] [ChList 200px] [主内容区 flex-1]
```

### NavBar (56px)
- 极窄，仅图标竖排
- 从上到下: Workspace Logo → 频道( Hash) → 任务看板 → 团队 → Agent管理 → 电脑管理 → 设置 → 底部用户头像
- 底色: `bg-brutal-stone` (与 cream 主区形成对比)
- 图标按钮: `border-2 border-black bg-white shadow-brutal-sm`, hover lift, active press
- 当前激活项: `bg-brutal-pink`

### ChannelList (200px)
- 保留当前 Sidebar 上半部分: 频道列表 + DM 列表
- 去掉底部导航链接(已移至 NavBar)
- 保持现有样式

### 右侧成员面板 → 弹出
- 删除右侧固定 MemberList 面板
- 主内容区右上角成员数量图标改为可点击按钮
- 点击弹出 Popover/Sheet 承载成员列表 + 添加 Agent 功能

---

## 2. 像素头像系统

- 8 个预设像素角色, 复古 JRPG 风格
- 纯 CSS `box-shadow` 实现 (不使用图片/SVG)
- 每个像素 3px×3px, 颜色用 brutalist 色板
- 创建/编辑 Agent 时网格展示可选, 默认随机分配
- 消息列表中显示: `border-2 border-black shadow-brutal-sm` 包裹

---

## 3. Agent 回复多彩化

### 3a. Markdown 表格表头
- 表头行: `bg-brutal-yellow font-heading font-bold text-black`

### 3b. @mention 高亮
- `@用户名`: `bg-brutal-pink text-black font-bold px-1`

### 3c. 代码块语言标签
- 从 `bg-brutal-yellow/30` 改为全饱和 `bg-brutal-yellow`

### 3d. 链接和强调
- 链接: `text-brutal-cyan font-bold underline`
- 粗体: `font-black` (900)

---

## 4. 任务看板分组颜色

列头完整色条 (全饱和度):

| 状态 | 底色 |
|------|------|
| TODO | `bg-brutal-stone` |
| IN PROGRESS | `bg-brutal-yellow` |
| IN REVIEW | `bg-brutal-lavender` |
| DONE | `bg-brutal-lime` |
| CLOSED | `bg-brutal-red` |

卡片内状态 badge、认领按钮跟列同色。

---

## 5. 回复数显示

改为 badge 样式块:
- `badge-brutal bg-brutal-pink`
- 文字: `💬 N REPLIES` (font-mono, uppercase)
- hover lift 效果
- 未读: `border-brutal-pink`, 已读: `border-black`

---

## 6. Channel 底色

主内容区给 `bg-brutal-stone/15` 与 cream 侧边栏形成微妙区分。

---

## 影响文件

- `frontend/app/globals.brutal.css` — 像素头像 box-shadow CSS
- `frontend/app/dashboard/page.tsx` — 布局改为双栏
- `frontend/components/dashboard/sidebar.tsx` — 拆分为 NavBar + ChannelList
- `frontend/components/dashboard/channel-view.tsx` — 去右侧面板, 成员按钮弹出
- `frontend/components/dashboard/member-list.tsx` — 弹出式使用
- `frontend/components/dashboard/agent-message.tsx` — Markdown 渲染多彩化
- `frontend/components/dashboard/message-list.tsx` — 回复数 badge, 任务 header 颜色
- `frontend/components/tasks/task-column.tsx` — 列头全饱和色
- `frontend/components/agents/agent-form.tsx` — Agent 创建时头像选择
- `frontend/components/ui/avatar.tsx` — 支持像素头像渲染

## 不改变
- 粗野主义核心风格 (0 圆角, 硬阴影, 2px 黑边框, font-heading, Space Mono)
- 组件 API 接口
- 后端任何代码
