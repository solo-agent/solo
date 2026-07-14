# Solo 浅色皮肤 — Technical Design

> 创建日期：2026-07-14
> 范围：9 套固定浅色皮肤，保存在当前浏览器；不做深色、自定义调色或账号同步
> 视觉方向：保留 Solo 现有新粗野主义，只替换语义色

---

## 1. 目标

在现有设置页加入 3×3 皮肤选择区。用户点击任意皮肤后，整个 Solo 立即换色；选择写入 `localStorage`，刷新后在首屏绘制前恢复，避免闪回默认配色。

皮肤只改变语义色。黑色粗边框、白色卡片、硬偏移阴影、零圆角、Space Grotesk / Inter / Space Mono 字体和现有交互节奏全部保持不变。

## 2. 明确不做

- 深色皮肤与系统深色跟随
- 自定义颜色编辑器
- 数据库、API 或跨设备同步
- React Theme Provider
- 独立皮肤 CSS 文件或新增依赖
- 批量改写业务组件中的黑白结构色

## 3. 技术方案

### 3.1 切换机制

根节点使用 `data-skin`：

```html
<html data-skin="blueprint">
```

`globals.brutal.css` 为 `:root[data-skin="…"]` 定义语义色变量。Tailwind 现有 `bg-brutal-primary`、`text-brutal-danger` 等工具类继续读取同一组变量，所以业务组件不需要知道当前皮肤。

同一选择器同时支持设置页卡片预览：

```css
:root[data-skin="blueprint"],
[data-skin-preview="blueprint"] {
  --color-brutal-bg: #f3f7ff;
  --color-brutal-primary: #3b82f6;
  --color-brutal-accent: #ff5d8f;
}
```

### 3.2 浏览器持久化

- 存储键：`solo.skin`
- 存储值：稳定皮肤 ID，例如 `blueprint`
- 缺失、未知或读取异常：回退 `classic`
- 写入失败：当前页面仍换色，只是不跨刷新保存
- 根布局默认输出 `data-skin="classic"`
- `<head>` 内的小型同步脚本在 React hydration 前读取本地值并更新 `data-skin`
- `html` 使用 `suppressHydrationWarning`，允许首屏脚本在 hydration 前改变该属性

### 3.3 运行时 API

新增 `frontend/lib/theme.ts`：

```ts
export const themeStorageKey = 'solo.skin';
export const defaultThemeId = 'classic';
export const themeOptions = [
  { id: 'classic', labelKey: 'themeClassic' },
  { id: 'blueprint', labelKey: 'themeBlueprint' },
  { id: 'ultraviolet', labelKey: 'themeUltraviolet' },
  { id: 'seasalt', labelKey: 'themeSeasalt' },
  { id: 'tomato', labelKey: 'themeTomato' },
  { id: 'matcha', labelKey: 'themeMatcha' },
  { id: 'bubblegum', labelKey: 'themeBubblegum' },
  { id: 'lavender', labelKey: 'themeLavender' },
  { id: 'sky', labelKey: 'themeSky' },
] as const;
export type ThemeId = (typeof themeOptions)[number]['id'];

export function resolveThemeId(value: string | null | undefined): ThemeId;
export function getStoredTheme(): ThemeId;
export function setTheme(value: string): ThemeId;
```

`setTheme` 先通过白名单解析 ID，再更新 `document.documentElement.dataset.skin`，最后尽力写入 `localStorage` 并派发 `solo:theme-change` 事件。

## 4. 九套皮肤

每套都包含背景、主色、强调色、信息、成功、警告、危险、紫色辅助和弱化色；浅色变体用于输入框焦点与柔和背景。

| ID | 名称 | 背景 | 主色 | 强调 | 信息 | 成功 | 警告 | 危险 | 紫色 | 弱化 |
|---|---|---|---|---|---|---|---|---|---|---|
| `classic` | 经典日光 | `#FFFAEF` | `#FFD23F` | `#FF6B6B` | `#74B9FF` | `#88D498` | `#F8A16F` | `#F97264` | `#BBAFE6` | `#C0B9B1` |
| `blueprint` | 蓝图汽水 | `#F3F7FF` | `#3B82F6` | `#FF5D8F` | `#38CFE8` | `#79D58C` | `#FFBC51` | `#FF5F68` | `#A78BFA` | `#B9C5D3` |
| `ultraviolet` | 紫外线工作室 | `#FBF6FF` | `#B47CFF` | `#51D6BE` | `#6CA8FF` | `#82D38B` | `#FFC857` | `#FF6B75` | `#8D74D8` | `#C9BDD6` |
| `seasalt` | 海盐橙 | `#F1FFF9` | `#36C9A5` | `#FF7A59` | `#5A8DEE` | `#75C56B` | `#FFCB57` | `#F45B69` | `#8E7DF2` | `#ADC9BF` |
| `tomato` | 番茄电台 | `#FFF5EF` | `#FF624A` | `#4F9DFF` | `#56D6C9` | `#75C76D` | `#FFC24C` | `#E9435C` | `#A889E8` | `#C8B7AF` |
| `matcha` | 抹茶蓝 | `#F6FFE9` | `#A4D936` | `#5577FF` | `#4FC3D7` | `#60BF70` | `#FFC857` | `#F25F5C` | `#9B85E8` | `#B8C39B` |
| `bubblegum` | 泡泡糖 | `#FFF2F8` | `#FF78B7` | `#4FD1C5` | `#70A5FF` | `#78CE86` | `#FFCA58` | `#F05268` | `#B794F4` | `#D1B5C4` |
| `lavender` | 薰衣草柠檬 | `#F8F5FF` | `#9B82F3` | `#FFDB4D` | `#5EB9E8` | `#7BC987` | `#FFB357` | `#F3616D` | `#C0A9FF` | `#BFB4D4` |
| `sky` | 晴空橘 | `#EFFAFF` | `#55BEEB` | `#FF8A4C` | `#6388FF` | `#67C78A` | `#FFD05C` | `#F2545B` | `#9D8DF1` | `#ADC7D3` |

所有大面积背景和主要按钮继续使用黑色文字。实现前用对比度脚本验证主色、强调色、状态色与黑色文字满足 WCAG AA；不满足的颜色必须在同一色相内调亮。

## 5. 设置页交互

皮肤区域放在现有“语言”区域之后，不新增设置侧栏、弹窗或二级页面。

- 桌面：3 列；共 3 行
- 窄屏：1 列
- 每张卡片是原生 `<button>`
- 卡片顶部显示 4 个语义色色块，底部显示皮肤名称
- 选中态同时使用勾选、`aria-pressed="true"`、粗边与硬阴影，不只依赖颜色
- 点击立即调用 `setTheme`，无需保存按钮
- 皮肤名称与说明通过现有 i18n 提供中英文

## 6. 文件改动

| 路径 | 改动 |
|---|---|
| `frontend/lib/theme.ts` | 新增 9 个 ID、解析、本地存储和应用逻辑 |
| `frontend/scripts/assert-theme-skins.mjs` | 新增无框架运行检查 |
| `frontend/app/globals.brutal.css` | 增加 9 套语义变量选择器 |
| `frontend/app/layout.tsx` | 默认属性与首屏恢复脚本 |
| `frontend/app/settings/page.tsx` | 3×3 皮肤按钮区域 |
| `frontend/lib/i18n.ts` | 中英文皮肤文案 |

## 7. 验证

### 自动检查

- 先运行 `node scripts/assert-theme-skins.mjs` 并确认因功能缺失而失败
- 实现后再次运行并通过
- `npm run build`
- `graphify update .`

### 浏览器端到端

在真实浏览器中登录并打开 `/settings`：

1. 确认显示 9 个皮肤按钮
2. 依次点击 9 个按钮，检查 `html[data-skin]` 与 `localStorage['solo.skin']`
3. 每次读取关键语义色，确认 9 套结果不同
4. 选择非默认皮肤后刷新，确认选择和实际颜色保持
5. 检查 Dashboard、Tasks、Teams 的主要表面仍为粗边、硬阴影和可读文本
6. 检查浏览器控制台无 hydration、React 或运行时错误

## 8. 成功标准

- 设置页恰好有 9 套固定浅色皮肤
- 点击后全局即时生效
- 当前浏览器刷新后保持选择
- 未知存储值安全回退经典日光
- 不改后端，不新增依赖，不批量重构业务组件
- 自动检查、生产构建和浏览器端到端验证全部通过
