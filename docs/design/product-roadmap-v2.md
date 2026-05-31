# Solo 产品路线图 v1.0 -> v2.0

> 版本: 2.1
> 最后更新: 2026-05-12
> 变更: v2.0→v2.1 修正 UI 风格定位：从"暗色开发者终端"统一为 Neubrutalism（粗野主义），对齐 frontend-redesign-v2.md v3.0
> 负责人: pm1 (产品经理)
> 对齐文档: PRD.md v1.1, ARCHITECTURE.md v1.2, TASKS.md v1.0
> 来源: slock.ai 实地体验 + 竞品深度调研 (2026-05-11)

---

## 目录

1. [当前状态评估](#1-当前状态评估)
2. [已知问题深度分析](#2-已知问题深度分析)
3. [核心竞品差距分析：slock.ai](#3-核心竞品差距分析slockai)
4. [产品路线图概览](#4-产品路线图概览)
5. [v1.1 — 基础加固（当前迭代，4 周）](#5-v11--基础加固当前迭代4-周)
6. [v1.2 — Agent 能力升级（6 周）](#6-v12--agent-能力升级6-周)
7. [v1.3 — 协作与体验（6 周）](#7-v13--协作与体验6-周)
8. [v2.0 — 企业能力（8 周）](#8-v20--企业能力8-周)
9. [Agent 能力深度规划](#9-agent-能力深度规划)
10. [多 Agent 协作场景](#10-多-agent-协作场景)
11. [前端 UI 风格改造路线](#11-前端-ui-风格改造路线)
12. [产品优先级总排序](#12-产品优先级总排序)
13. [范围边界（本次路线图不涵盖）](#13-范围边界本次路线图不涵盖)
14. [附录：关键决策记录](#14-附录关键决策记录)
15. [附录 C：slock.ai 实地体验方法论](#15-附录-cslockai-实地体验方法论)

---

## 1. 当前状态评估

### 1.1 MVP 已交付功能（确认可用）

| 模块 | 状态 | 说明 |
|------|------|------|
| 认证系统 | 已交付 | 邮箱注册/密码登录/JWT access+refresh token |
| 频道管理 | 已交付 | 创建/编辑/删除频道，频道列表，成员管理 |
| 消息发送与实时推送 | 已交付 | WebSocket 实时推送，频道订阅/取消订阅 |
| 消息历史（游标分页） | 已交付 | 复合索引优化，limit 50/100 |
| 线程回复 | 已交付 | 右侧线程面板，Agent 线程上下文支持 |
| @提及 Agent | 已交付 | 自动完成下拉，后端 @解析，多 @提及 支持 |
| 私信（DM） | 已交付 | 创建/去重/列表/消息发送，Agent DM 响应 |
| Agent 管理 CRUD | 已交付 | 创建/编辑/删除，配置名称/system prompt/模型 |
| Agent 加入频道 | 已交付 | 频道成员列表展示，在线状态 |
| Agent 自动响应（流式） | 已交付 | SSE 流式推送，WebSocket 转发，thinking 状态 |
| Dashboard 主布局 | 已交付 | 左栏频道列表 + DM 列表 + 右栏消息视图 |
| 可观测性 | 已交付 | Prometheus 指标，健康检查，结构化日志 |
| Docker 部署 | 已交付 | docker-compose，Dockerfile，部署脚本 |

### 1.2 已知问题清单

通过代码审计和功能验证确认以下问题：

```
P0 — 影响核心体验，必须修复
P1 — 重要，影响可用性
P2 — 有更好，不阻塞
```

| ID | 问题 | 严重程度 | 影响范围 | 根因分析 |
|----|------|---------|---------|---------|
| K-01 | Agent system prompt 被 Claude Code 本地配置覆盖 | **P0** | 所有使用 Local Provider 的 Agent | `pkg/llm/local.go` 的 `buildPrompt()` 正确传入了 `SystemPrompt`，但 Claude Code CLI 在执行时仍读取 `CWD/CLAUDE.md`（或其他 claude 配置文件），这些项目级指令会叠加/覆盖 Solo 中定义的 system prompt。核心问题是：**没有为每个 Agent 创建隔离的执行环境** |
| K-02 | 所有本地 Agent 共享同一工作目录 | **P0** | 多 Agent 场景下的数据隔离 | Daemon 总是在同一目录下调用 `claude -p "..."`，无论触发哪个 Agent。Agent A 在 Claude Code CLI 中产生的 filesystem 变更对 Agent B 可见 |
| K-03 | Local Provider `CompleteStream` 无真正流式 | **P1** | 所有使用 Local Provider 的 Agent 的用户体验 | `local.go` 的 `CompleteStream()` 只是用 goroutine 包装了 `Complete()`，一次性返回完整结果。用户在使用 local Agent 时看不到逐字输出的对话体验 |
| K-04 | 前端 UI 风格偏通用 SaaS | **P1** | 产品差异化感知 | 当前使用 shadcn/ui slate 主题默认样式，与 slock.ai 的粗野主义（neubrutalist）风格形成明显差距。品牌辨识度不足 |
| K-05 | Agent 之间无 memory 隔离 | **P1** | Agent 个性化能力 | 每个 Agent 没有自己的长期记忆存储。Agent 的"个性"和"知识"完全依赖 system prompt 的提示词工程，无法积累经验 |
| K-06 | 无 Agent 工具/技能系统 | **P2** | Agent 能力边界 | 当前 Agent 只能做对话回复，无法调用外部工具（文件读写、搜索、代码执行等）。Agent 的使用场景受限 |
| K-07 | 本地 Agent 无法利用 Claude Code 的文件操作能力 | **P2** | 本地 Agent 价值 | Local Provider 通过 `claude -p` 传参调用，限制了 Claude Code CLI 的交互式能力（文件编辑、代码审查、PR 操作等） |

### 1.3 slock.ai 实地体验发现的 Solo 新增差距（2026-05-11）

通过 slock.ai 前端 JS bundle 逆向分析 + 路由结构反推 + UI 组件分析，发现以下 Solo 缺失但 slock.ai 已有的功能：

| ID | 差距 | 优先级 | 对应 slock.ai 实现 | 影响面 |
|----|------|--------|-------------------|--------|
| G-01 | **电脑/机器管理** — 管理运行 Agent 的机器/daemon | **高** | `/computers` 页面，workspace 扫描 | Agent 管理 |
| G-02 | **任务系统** — 内置任务分配和追踪 | **高** | `/tasks`, `/tasks/convert-message` | Agent 协作 |
| G-03 | **消息搜索** — 全局跨频道搜索 | **高** | `/messages/search` 独立搜索 API | 信息检索 |
| G-04 | **已保存消息** — 收藏重要消息 | **中** | `/channels/saved`, `/saved/check` | 信息管理 |
| G-05 | **统一收件箱** — 聚合未读/已处理消息 | **中** | `/channels/inbox`, `/channels/inbox/done` | 信息管理 |
| G-06 | **线程收件箱** — 追踪关注/未完成的线程 | **中** | ThreadsInbox 组件, `/channels/threads/*` | 线程管理 |
| G-07 | **消息翻译** — 多语言翻译 | **低** | `/message-translations:batch` API | 国际化 |
| G-08 | **推送通知** — 浏览器/桌面推送 | **中** | `/push/subscribe`, VAPID key | 实时感知 |
| G-09 | **文件上传/附件** | **高** | `/attachments/upload` 独立模块 | 协作基础 |
| G-10 | **多工作区/团队管理** + 邀请 | **高** | `/servers`, `/join/:code` 邀请链接 | 多人协作 |
| G-11 | **头像上传** | **低** | `/auth/me/avatar` 上传 | 个性化 |
| G-12 | **Agent 详情面板** — runtime、状态、头像 | **高** | AgentDetailPanel 组件含 runtime label | Agent 管理 |
| G-13 | **发布说明** | **低** | `/release-notes` 页面 | 用户沟通 |
| G-14 | **Billing/订阅管理** | **中** | `/billing/subscription` | 商业化 |
| G-15 | **已读/未读追踪**（频道级） | **中** | 频道列表含 unread_count, has_mention | 信息管理 |

> G 系列问题将在 v1.3 和 v2.0 中逐步补齐，详见各版本规划。

### 1.4 架构健康度评估

| 维度 | 评分 | 说明 |
|------|------|------|
| 代码质量 | A- | 清晰的模块划分，适当的抽象层级。适量复用 multica 已有能力 |
| 实时通信 | A | WebSocket Hub 架构健壮，SSE 流式通道稳定，断线重连机制完整 |
| 数据层 | A | 复合索引设计合理，游标分页性能良好，迁移流程规范 |
| Agent 架构 | B | 基本模式（Server Daemon 分离）正确，但执行隔离和工具系统缺失 |
| 前端架构 | B+ | 组件划分合理，WebSocket 客户端封装完整，但样式体系和交互细节待打磨 |
| 可扩展性 | B- | 单 Daemon 实例模式限制了 Agent 并发扩展，无 Redis 跨实例广播 |
| 可测试性 | B | 后端有单元测试覆盖，但 E2E 测试和集成测试需补充 |

---

## 2. 已知问题深度分析

### 2.1 K-01/K-02: Agent system prompt 与工作目录隔离

**现状代码路径**：

```
User send message
  -> server/service/agent.go TriggerAgentResponse()
    -> Loads agent config (system_prompt, model) from DB
    -> Builds daemonTaskRequest with SystemPrompt field
    -> Sends POST /internal/daemon/run to Daemon
      -> cmd/daemon/handler.go processTaskStreaming()
        -> llmReq.SystemPrompt = req.SystemPrompt  // 正确传递
        -> provider.CompleteStream(ctx, llmReq)
          -> pkg/llm/local.go buildPrompt()
            -> 把 systemPrompt + conversation 拼成文本
            -> exec.CommandContext(runCtx, "claude", "-p", prompt, "--print")
```

问题在 `local.go:40-42`：
```go
cmd := exec.CommandContext(runCtx, p.binary, "-p", prompt, "--print")
cmd.Env = append(os.Environ(), "CLAUDE_CODE_HEADLESS=true")
```

- `claude -p <prompt>` 在执行时，Claude Code CLI 会在 CWD 中查找 CLAUDE.md 并应用其中的系统指令
- 如果 CWD 中有一个配置了 "你是一个通用助手" 的 CLAUDE.md，这就会与 Solo 中定义的 system prompt 混合
- 更关键的是：Agent A 和 Agent B 在同一个 CWD 中执行，**没有任何隔离**

**修复方向**（详见 §9.1 Agent 工作空间）：
- 每个 Agent 创建独立的 `~/.solo/agents/<agent-id>/` 工作目录
- 在此目录中生成 Agent 专属的 CLAUDE.md 和 memory 文件
- Daemon 在调用 `claude` 时 `cd` 到该目录执行

### 2.2 K-03: Local Provider 流式输出

**现状**：`local.go:92-107`
```go
func (p *LocalProvider) CompleteStream(...) (<-chan StreamChunk, error) {
    ch := make(chan StreamChunk, 2)
    go func() {
        defer close(ch)
        resp, err := p.Complete(ctx, req)  // 等全部完成
        ch <- StreamChunk{Content: resp.Content}
        ch <- StreamChunk{Done: true}
    }()
    return ch, nil
}
```

- `claude -p` 以 `--print` 模式运行，一次性输出完结果后才退出
- 真正的流式需要通过管道读取 CLI 的标准输出（逐行读取，分块推送）

**修复方向**：
- 修改 `local.go`，用 `exec.Command` 的 `StdoutPipe()` 逐行读取输出
- 每读取到一定量的文本就推送一个 `StreamChunk`
- 这样 local Agent 也能像远程 Agent 一样逐字输出

### 2.3 K-04: 前端 UI 风格

**现状**：基于 shadcn/ui slate 主题的标准 SaaS 风格——卡片、圆角、毛玻璃效果、灰度色阶。

**目标风格**（基于 neubrutalism.com 规范 + 与 slock.ai 差异化）：
- **Neubrutalism（粗野主义）**：2px 黑边框、5px 5px 0 硬偏移阴影、零圆角
- **品牌色**：粉色 #fe7da8（与 slock 的黄色 #FFD700 差异化）
- **字体**：Space Grotesk（UI）+ Inter（正文）+ Space Mono（代码）
- **背景**：奶油白 #fffaef（亮色主题）
- **视觉语言**：高对比度、粗边框、硬阴影、无圆角、扁平填充色、无渐变

详见 §11 前端 UI 风格改造路线。详细设计系统规范见 `frontend-redesign-v2.md` v3.0。

### 2.4 K-05/K-06: Agent 独立能力

当前 Agent 的能力模型：
```
Agent
  ├── name (显示名称)
  ├── system_prompt (行为设定)
  ├── model_provider / model_name (LLM 后端)
  ├── temperature / max_tokens (推理参数)
  └── is_active (启用/停用)
```

Agent **没有**：
- 独立工作空间（filesystem isolation）
- 长期记忆（memory persistence）
- 工具/技能（tool execution）
- 文件知识库（RAG）
- 协作接口（agent-to-agent）

详见 §9 Agent 能力深度规划。

---

## 3. 核心竞品差距分析：slock.ai（实地体验版 — 2026-05-11）

> 分析来源：
> 1. **公开 landing page**（slock.ai）— 全量文案、功能描述、定价、团队介绍
> 2. **前端 JS bundle 逆向**（app.slock.ai `/assets/index-udNNoUrh.js`）— 提取路由结构、组件树、API 端点、功能特征
> 3. **市场评测** — 硅星人、腾讯新闻等第三方评测文章
> 4. 因未获得有效登录会话，未能体验 app 登录后功能。以下分析部分基于 JS bundle 中的组件名、路由路径和函数调用推断。

### 3.1 slock.ai 产品架构拆解（来自 JS bundle 分析）

#### URL 路由结构

```
app.slock.ai/
  ├── /                         # 根路径（可能重定向到 /s/...）
  ├── /s/:serverSlug/*          # 工作区主路由（s/ = server/team slug）
  │   ├── /channels             # 频道列表
  │   │   ├── /dm               # DM 频道索引
  │   │   ├── /inbox            # 收件箱（未读/已读/已处理）
  │   │   │   ├── /done         # 已处理
  │   │   │   └── /read-all     # 全部标已读
  │   │   ├── /saved            # 已保存消息
  │   │   │   └── /check        # 检查是否已保存
  │   │   ├── /threads          # 线程管理
  │   │   │   ├── /follow       # 关注线程
  │   │   │   ├── /unfollow     # 取消关注
  │   │   │   ├── /followed     # 已关注列表
  │   │   │   ├── /done         # 已处理
  │   │   │   └── /undone       # 未处理
  │   │   └── /unread           # 未读频道
  │   ├── /channel/:id          # 单个频道
  │   ├── /dm/:id               # 单个 DM（与用户 URL 一致）
  │   ├── /agent/:id            # Agent 详情
  │   ├── /agents               # Agent 列表
  │   ├── /computers            # 电脑/机器管理
  │   │   └── /:id/workspaces   # 工作区扫描
  │   ├── /members              # 成员管理
  │   ├── /tasks                # 任务系统
  │   │   ├── /convert-message  # 从消息创建任务
  │   │   └── /server           # 按服务器查看
  │   ├── /saved                # 已保存
  │   ├── /inbox                # 收件箱
  │   ├── /threads              # 线程
  │   ├── /search               # 搜索
  │   ├── /settings             # 工作区设置
  │   ├── /human/:id            # 人类成员详情
  │   └── /machine/:id          # 机器详情
  ├── /auth/...                 # 认证（login/register/verify/reset/logout）
  ├── /servers                  # 服务器管理
  │   └── /join-community       # 加入社区
  ├── /search                   # 全局搜索
  ├── /settings                 # 全局设置
  ├── /billing/subscription     # 订阅管理
  ├── /join/:code               # 邀请链接
  ├── /release-notes            # 发布说明
  └── /palette-audit            # 设计系统审计（内部工具）
```

#### 核心前端组件树（从 JS bundle 推断）

```
App Layout
  ├── Sidebar.tsx               # 侧栏导航
  │   ├── selected channel item # 当前选中高亮
  │   ├── unread count badges   # 未读数量标记
  │   └── ThreadsInbox.tsx      # 线程收件箱入口
  ├── ChatPanel.tsx             # 主聊天面板
  │   ├── panel header          # 频道/DM/Agent 标题+图标
  │   └── human DM header avatar
  ├── MessageItem.tsx           # 消息条目
  │   ├── #channel inline ref   # 频道内联引用
  │   ├── thread ref inline     # 线程引用
  │   └── @mention inline       # @提及链接
  ├── ChannelMembers.tsx        # 频道成员列表（含人类小头像）
  ├── AgentDetailPanel.tsx      # Agent 详情面板
  │   ├── 大 Agent 头像
  │   ├── runtime label badge   # 运行时标签
  │   └── Delete Agent          # 删除操作
  ├── ThreadsInbox.tsx          # 线程收件箱面板
  │   └── in-review task badge  # 审查中的任务标签
  ├── SettingsPanel.tsx         # 设置面板
  │   └── warning banner        # 设置警告横幅
  ├── MentionLink.tsx           # @提及链接
  └── MentionHoverCard.tsx      # @提及悬停卡片
```

#### 数据模型（从代码推断）

```
Server (工作区)
  ├── slug (URL 唯一标识)
  └── name

Channel
  ├── id (UUID)
  ├── server_id
  ├── type (channel / dm / inbox)
  ├── pinned_order[]
  ├── pinned_channel_ids[]
  ├── pinned_agent_ids[]
  ├── hidden_dm_ids[]
  ├── unread_count
  ├── first_unread_message_id
  ├── has_mention (boolean)
  └── dm_sort_mode

Message
  ├── id
  ├── channel_id
  ├── author_id
  ├── content
  ├── created_at
  └── ... (translations, reactions, mentions)

Agent
  ├── id
  ├── name
  ├── avatar
  ├── runtime_label (模型/provider)
  ├── computer_id
  └── ...

Computer (机器)
  ├── id
  ├── name
  ├── daemon_url
  ├── status (online/offline)
  ├── workspaces[] (扫描的代码库)
  └── last_heartbeat

Task
  ├── id
  ├── title
  ├── status
  ├── assignee_id (agent 或 human)
  ├── source_message_id
  └── server_id

Member
  ├── id
  ├── server_id
  ├── role
  └── ...
```

#### 核心功能流

```
用户打开 slock.ai
  → /s/{workspace} 自动路由
  → Sidebar 显示:
      已置顶频道 | Agent | DM（排序: 最近/未读）
      Channel 未读 + @提及 计数
      计算机/机器在线状态
  → 选择频道/DM → ChatPanel 加载消息列表
  → MessageItem 支持:
      @mention（MentionLink + MentionHoverCard）
      线程回复（thread ref inline + 计数）
      已保存/收藏（/channels/saved/check）
      翻译（/message-translations:batch）
      附件（/attachments/upload）
  → 输入框支持:
      普通消息
      @提及自动完成
      文件上传拖拽
  → Inbox 聚合所有未读/已处理
  → Tasks 管理 Agent 任务分配和进度
  → Search 跨消息全局搜索
```

### 3.2 宣传页分析

#### 登陆页观察到的主要内容

**整体布局**：
- 导航栏：左侧 Logo（黄色闪电锁图标 + "Slock" 字样），右侧 Pricing | Blog | Sign in
- Hero 区：深背景展示聊天界面模拟——频道消息列表，显示用户头像、名称、消息时间和消息内容
- 聊天模拟中展示了典型的工程协作场景：`@Tenny how's CI on PR #982?` → `All green. Merging once @tygg signs off.`
- 品牌色：黄色 (#FFD700)、奶油白、黑色。风格为粗野主义（brutalist）——粗边框、高对比度、无圆角
- 字体：Space Grotesk（UI 文字）、Space Mono（等宽代码文字）
- 引用语：`"The future of work isn't humans using AI tools. It's humans and AI agents building together."`

**功能宣传**：
1. "Chat is the workspace" — Channels, DMs, threads, every interaction is a message. Zero overhead.
2. "Long-running agents" — Each agent is a persistent process with its own memory. Drop a task and they pick up where they left off.
3. "Your computers, your agents" — `npx @slock-ai/daemon`, full privacy, local execution.

**定价**：
- Hobby: $0 forever（限时无限量，无上限：2 computers → unlimited，5 agents → unlimited，5 channels → unlimited，30天历史 → unlimited）
- Team: Coming soon
- Business: Coming soon

**团队展示**：
- 混合团队页面：人类（Richard, Tenny）和 Agent（XX, Bugen, Noel, DD 猫）作为平级队友展示
- 每个成员有独立角色描述和照片/头像
- Agent 有明确名称、角色标签和个性描述

**登录页 (app.slock.ai)**：
- 简洁登录界面：Email + Password 登录
- 社交登录：Continue with Google, Continue with GitHub
- 基础功能：Forgot password, Create one (注册)

**上手流程**：
```
1. Create a server
2. Connect a computer — npx @slock-ai/daemon
3. Spawn agents
4. Collaborate
```

### 3.3 按维度详细分析

#### 3.3.1 导航与信息架构

| 维度 | slock.ai 实际情况 | Solo 状态 | 差距评估 |
|------|------------------|-----------|---------|
| 侧栏结构 | 频道列表 + DM 列表 + 任务系统，标准三栏布局 | 频道列表 + DM 列表 | `🔶` Solo 缺少任务系统入口 |
| 频道列表 | 频道分组展示，支持通知标记 | 平铺频道列表 | `🔶` Solo 缺少频道分组 |
| DM 列表 | 用户 + Agent DM 混合展示 | 用户 + Agent DM | `✅` 基本一致 |
| Server/Workspace 切换 | 未发现多 Server 切换能力（可能是单工作区模式） | 单工作区 | `✅` 一致 |
| 搜索功能 | 官网未明确展示搜索 UI，但消息历史功能暗示存在 | **缺失** | `❌` Solo 缺失搜索 |
| 任务系统入口 | 侧栏有独立"任务"导航项 | **缺失** | `❌` Solo 缺失任务系统 |

#### 3.3.2 消息系统

| 维度 | slock.ai 实际情况 | Solo 状态 | 差距评估 |
|------|------------------|-----------|---------|
| 消息展示格式 | 标准聊天格式：头像 + 名称 + 时间 + 消息内容 | 标准聊天格式 | `✅` 基本一致 |
| 消息发送/编辑/删除 | 标准消息操作 | 发送✅ 编辑/删除❌ | `❌` Solo 缺编辑删除 |
| 文件/图片附件 | 未从公开页面确认，但作为协作平台大概率支持 | **缺失** | `🔶` 待确认后评估 |
| 消息回复/线程 | 有 Threads（"Channels, DMs, threads"） | 已交付 | `✅` Solo 已有 |
| @提及 | Hero 区展示 "@Tenny"，交互内嵌 | 已交付 | `✅` Solo 已有 |
| 表情/反应 | 未从公开页面确认 | **缺失** | `🔶` 待确认 |
| 消息搜索 | 消息历史支持搜索，Hobby 版 30天 | **缺失** | `❌` Solo 缺失 |

#### 3.3.3 Agent 交互

| 维度 | slock.ai 实际情况 | Solo 状态 | 差距评估 |
|------|------------------|-----------|---------|
| Agent 展示方式 | Agent 有独立头像/照片，和人类并列显示 | Agent 有列表展示 | `✅` 基本一致 |
| Agent 消息 vs 用户消息 | 视觉上 Agent 消息可能通过颜色/标记区分 | 有区分 | `✅` 基本一致 |
| Agent 触发方式 | @提及 + 频道内自动响应 | @提及 | `🔶` Solo 缺主动触发 |
| Agent 回复格式 | 标准消息流 | 标准消息流（含流式） | `✅` 基本一致 |
| Agent "思考中"状态 | 确认有（typing/thinking indicator） | 已交付 | `✅` 一致 |
| 创建/配置 Agent | 通过 UI 或 npx spawn | Agent 管理页 | `✅` 基本一致 |
| **Agent 持久记忆** | **核心差异化：每个 Agent 有独立持久内存** | **缺失** | `❌` 核心差距 |
| **多 Agent 同频道协作** | **多个 Agent 可同时看到并讨论同一消息** | **缺失** | `❌` 核心差距 |
| **Agent 间调用** | Agent 之间可互相讨论和响应 | **缺失（计划中）** | `❌` 核心差距 |

#### 3.3.4 频道管理

| 维度 | slock.ai 实际情况 | Solo 状态 | 差距评估 |
|------|------------------|-----------|---------|
| 创建频道 | UI 创建 | UI 创建 | `✅` 一致 |
| 频道设置 | 有名称、描述等设置 | 名称/描述 | `✅` 基本一致 |
| 频道成员管理 | 支持添加/移除人/Agent | 已交付 | `✅` 基本一致 |
| 公开/私有频道 | 未从公开页面确认 | 未实现 | `🔶` 待确认 |

#### 3.3.5 DM 私信

| 维度 | slock.ai 实际情况 | Solo 状态 | 差距评估 |
|------|------------------|-----------|---------|
| DM 列表展示 | 用户 + Agent DM 混合列表 | 已交付 | `✅` 一致 |
| 发起新 DM | UI 发起 | 已交付 | `✅` 一致 |
| DM 和频道 UI 区别 | 标准区分（私信 vs 群组） | 已交付 | `✅` 一致 |

#### 3.3.6 其他功能

| 维度 | slock.ai 实际情况 | Solo 状态 | 差距评估 |
|------|------------------|-----------|---------|
| **任务/Issue 系统** | **有独立的 Tasks 模块，可分配任务给 Agent** | **缺失** | `❌` 核心功能缺失 |
| Reminder 提醒 | 未确认 | **缺失** | `🔶` 待确认 |
| Activity Log | 未确认 | **缺失** | `🔶` 待确认 |
| Settings/Profile | 登录页面有 Forgot password | 有用户设置 | `✅` 基本一致 |
| 主题切换 | 官网为亮色（黄/白/黑），app 登录页为深色 | 暗色模式 | `✅` 基本一致 |
| OAuth 登录 | Google + GitHub 登录 | **缺失** | `❌` Solo 缺失 |

#### 3.3.7 用户体验

| 维度 | slock.ai 实际情况 | Solo 状态 | 差距评估 |
|------|------------------|-----------|---------|
| Onboarding | "1. Create server 2. npx daemon 3. Spawn agents 4. Collaborate" | 有部署文档 | `🔶` Solo 缺少产品内引导 |
| 空状态设计 | 未确认 | 基础 | `🔶` 待确认 |
| 加载状态 | 未确认 | 基础 | `🔶` 待确认 |
| 快捷键 | 未确认 | **缺失** | `🔶` 待确认 |

### 3.4 功能对标矩阵（v2.0 更新版）

**符号说明**：
- ✅ Solo 已有
- 🔶 Solo 部分有/需增强
- ❌ Solo 缺失，建议吸收
- ❌⚠ Solo 缺失且为关键差距
- ➖ 不适合 Solo / 非竞品维度

| 功能维度 | slock.ai 实际能力 | Solo v1.0 | Solo v2.0 目标 | 优先级 | 代号 |
|---------|------------------|-----------|---------------|--------|------|
| **基础协作** | | | | | |
| 频道（Channel） | 核心功能 | ✅ 已交付 | 频道分组、归档、固定 | P2 | F-CH |
| 私信（DM） | 核心功能 | ✅ 已交付 | DM 群组（3-5 人） | P2 | F-DM |
| 线程（Thread） | 核心功能 | ✅ 已交付 | 线程通知改进 | P1 | F-TH |
| 消息编辑/删除 | 标准功能 | ❌ 缺失 | v1.1 交付 | P0 | F-ED |
| 文件上传/分享 | 预计支持 | ❌ 缺失 | v1.3 交付 | P1 | F-FU |
| 消息搜索 | 标准功能（含历史搜索） | ❌ 缺失 | v1.3 交付 | P1 | F-SR |
| 表情/反应 | 未确认 | ❌ 缺失 | 待评估 | P3 | F-RE |
| **任务管理系统** | | | | | |
| **任务/Issue 系统** | **核心差异化功能** | **❌ 缺失** | **v1.2/v1.3 启动** | **P1** | **F-TK** |
| 任务分配 | 支持分配给 Agent | ❌ 缺失 | 同任务系统 | P1 | F-TA |
| 进度跟踪 | 支持监控 Agent 执行 | ❌ 缺失 | 同任务系统 | P1 | F-TP |
| **Agent 能力** | | | | | |
| Agent 自定义 prompt | 核心能力 | ✅ 已交付（被覆盖） | v1.1 修复隔离 | P0 | F-PR |
| Agent 工具/技能 | 核心能力（通过 daemon） | ❌ 缺失 | v1.2 交付 | P1 | F-TO |
| **Agent 持久记忆** | **核心差异化能力** | **❌ 缺失** | **v1.2 交付** | **P0** | **F-ME** |
| **多 Agent 同频道协作** | **核心差异化能力** | **❌ 缺失** | **v1.3 交付** | **P1** | **F-MA** |
| Agent 间调用互触发 | 核心能力 | ❌ 缺失 | v1.3 交付 | P1 | F-AA |
| Agent 本地 Daemon 执行 | 核心模式（npx daemon） | ✅ 已交付 | 加固 | P1 | F-DA |
| Agent 市场/模板 | 路线图中 | ❌ 缺失 | v2.0 交付 | P2 | F-MK |
| **体验与设计** | | | | | |
| 品牌视觉 | **粗野主义（brutalist）黄/白/黑** | 通用 SaaS | v1.1 启动改造 | P1 | F-UI |
| 类终端字体/代码展示 | Space Mono 等宽 | 通用字体 | 字体统一 | P1 | F-TY |
| 消息富文本/Markdown | 标准渲染 | ✅ 已支持 | 完善 | P1 | F-MD |
| 暗色模式 | 亮色官网，深色 app | ✅ 已支持 | N/A | - | |
| Onboarding 引导 | 简洁 4 步流程 | ❌ 缺失 | v1.3 交付 | P2 | F-OB |
| 快捷键 | 未确认 | ❌ 缺失 | v1.3 交付 | P2 | F-KB |
| **企业能力** | | | | | |
| 用量统计 | 通过 Hobby/Pricing 暗示 | ❌ 缺失 | v1.3 交付 | P2 | F-US |
| 审批流 | 未确认 | ❌ 缺失 | v2.0 交付 | P2 | F-AP |
| 定时任务 | 未确认 | ❌ 缺失 | v2.0 交付 | P2 | F-CR |
| OAuth/SSO | Google + GitHub 登录 | ❌ 缺失 | v2.0 交付 | P2 | F-OA |
| 多租户 | 未确认 | ❌ 缺失 | 远期 | P3 | |

### 3.5 从 slock.ai 实地体验中提取的关键洞察

#### 洞察 1：任务系统是 slock.ai 的核心差异化之一

Slock 不只是"聊天 + Agent"，它有**独立的任务管理模块**。用户可以创建任务、分配给 Agent、跟踪进度。这使其超越纯聊天工具，成为真正的工作平台。Solo 当前完全没有任务系统。

> **建议吸收**：在 v1.2 或 v1.3 增加轻量级任务系统，作为 Agent 协作的核心载体。

#### 洞察 2：Slock 的 UI 风格是粗野主义（Brutalist），而非复古终端

当前路线图 §11 误将 slock.ai 风格描述为"复古终端/像素风（retro-terminal / pixel-art aesthetic）"。实际观察到的风格特征为：
- **品牌色**：黄色 (#FFD700) 为主色调，搭配奶油白(#FFF8E7)、黑色
- **设计语言**：粗边框（border-2）、无圆角、高对比度、极简
- **字体**：Space Grotesk（UI）、Space Mono（代码）
- **整体感觉**：粗野主义 + 极简北欧风的混合，而非仿古终端
- Logo：带闪电的锁形图标

> **Solo UI 方向决策**：Solo 采用与 slock 一致的 **Neubrutalism** 设计体系（基于 neubrutalism.com 规范），同时通过品牌色差异化——Solo 用粉色 #fe7da8，slock 用黄色 #FFD700。Solo 不是暗色终端/开发者风格，而是**亮色粗野主义**：奶油白背景 #fffaef、2px 黑边框、5px 硬阴影、零圆角、Space Grotesk UI 字体。详见 `frontend-redesign-v2.md` v3.0。

#### 洞察 3：Slock 的团队展示方式值得借鉴

Slock 官网的"Meet the team"页面列出了 6 个成员（3 人类 + 3 Agent + 1 猫），每个都有照片、角色和个性描述。这强化了"Agent 是平等队友"的品牌叙事。

> **建议 Solo 参考**：在 Agent 管理页面增加角色标签、个性描述和头像自定义功能。

#### 洞察 4：Agent 持久记忆可能是最关键的单点差距

Slock 的核心卖点是 **"Long-running agents with persistent memory"**。Agent 记住代码偏好、历史对话，分配新任务后可以从上次中断处继续。Solo 的 Agent 每次对话都是独立的。

> **建议提升优先级**：Agent 持久记忆从 P1 提升到 P0，作为 v1.2 的核心交付物。

#### 洞察 5：多 Agent 同频道协作和 Slock 存在的批评

Slock 的产品理念验证和批评并存：
- **正面**（新浪/硅星人）：抓住了多 Agent 管理的真问题——"Agent 多起来后，最先崩掉的不是能力，而是管理"
- **负面**（163.com 深度批评）："AI 群聊不成立，多 Agent 协作只是生成了更多冗余信息"

> **Solo 的应对策略**：不是简单地做"Agent 群聊"，而是用**任务驱动**的 Agent 协作——Agent 在频道中响应任务，而不是在所有对话中都参与。通过 `@提及` 精确触发，减少信息冗余。

### 3.6 Solo 差异化优势（更新版）

| 优势 | 现状 | 加固方向 | 与 slock 对比 |
|------|------|---------|-------------|
| **开源可自托管** | 已实现 | 完善部署文档 + Helm Chart | 差异点：Slock 是闭源 SaaS |
| **模型无关** | 支持 OpenAI/Anthropic/本地 | Ollama 集成 + UI 切换 | 差异点：Slock 依赖 daemon 本地模型 |
| **本地 Claude Code CLI 深度集成** | 存在但受限 | v1.2 交互模式 | 差异点：Slock 的 daemon 自己管理 Agent 执行 |
| **已有 multica 对标基础** | 代码复用良好 | 持续优化 | 差异点：multica 的 issue+agent 模式更贴近 Solo 的任务系统方向 |
| **无 API Key 也可用**（本地模式） | Claude Code CLI | Ollama/本地模型 | 相同点：都支持本地执行 |

### 3.7 Agent 能力深度差距分析

**Slock 视角下的 Agent 心智模型**：
```
Agent = 持久的「数字同事」
  ├── 独立身份（名称 + 头像 + 角色）
  ├── 独立记忆（记忆代码库偏好 + 历史对话）
  ├── 和人类平级（在同一频道中）
  ├── 任务驱动（分配任务 = 分配工作）
  └── 多 Agent 协作（互相讨论/接力）
```

**Solo 当前的 Agent 心智模型**：
```
Agent = 可配置的「对话机器人」
  ├── 名称 + System Prompt
  ├── LLM 后端配置
  ├── 被 @ 时激活
  ├── 独立响应
  └── 对话无记忆
```

**核心差距**不在功能数量，而在产品**世界观**——Slock 以 Agent 为"同事"，Solo 以 Agent 为"工具"。路线图中需要优先解决这个心智模型转变。

### 3.8 关键差距：需优先弥补的 5 个缺口（更新版）

| 优先级排名 | 差距 | 影响 | 方案 | 建议版本 |
|-----------|------|------|------|---------|
| 1 | **Agent 持久记忆** | Agent 无法积累上下文 | v1.2 Memory 系统（§9.2） | v1.2 |
| 2 | **消息编辑/删除** | 基础功能缺失影响体验 | 后端 API + 前端 UI | v1.1 |
| 3 | **任务系统** | 缺乏组织 Agent 工作的载体 | 轻量任务 + Agent 分配 | v1.2-1.3 |
| 4 | **Agent 工作目录隔离** | System prompt 被覆盖 | Workspace 系统（§9.1） | v1.1 |
| 5 | **视觉品牌辨识度** | 产品无记忆点 | Neubrutalist 风格 UI 改造 | v1.1-1.3 |

---

## 4. 产品路线图概览

```
2026年5月        2026年6月        2026年7月        2026年8月
─────────────────────────────────────────────────────────────►

v1.1 (4周)       v1.2 (6周)       v1.3 (6周)       v2.0 (8周)
┌─────────┐     ┌───────────┐    ┌───────────┐    ┌──────────────┐
│ 基础加固 │      │ Agent升级 │    │ 协作+体验 │    │ 企业能力      │
│         │      │  + 任务系统│   │           │    │              │
│ K-01    │      │ K-05     │    │ 消息搜索  │    │ 审批流       │
│ K-02    │      │ K-06     │    │ 文件上传  │    │ 定时任务     │
│ K-04    │      │ K-03     │    │ 多 Agent  │    │ 插件系统     │
│ K-07    │      │ K-07 深挖 │   │ 协作场景  │    │ Agent 市场   │
│ ──────  │      │ ──────    │    │ Agent调用链│   │ OAuth/SSO   │
│ 消息编辑 │      │ Agent记忆 │    │ 触发起任务│    │ ──────       │
│ 删除     │      │ 工具系统  │    │ 用量统计  │    │ 移动端       │
│ 多模型UI │      │ Workspace│   │ Onboarding│   │ 多租户（远期）│
│         │      │ ──────    │    │ 快捷键    │    │              │
│         │      │ ★任务系统  │    │ ──────    │    │              │
│         │      │ (Issue/   │    │ 频道归档  │    │              │
│         │      │  Agent分配)│   │ UI 打磨   │    │              │
└─────────┘     └───────────┘    └───────────┘    └──────────────┘

P0 修复         P0-P1 新能力      P1 体验         P2 扩展
```

### 交付节奏策略

| 版本 | 核心主题 | 交付物 | 目标用户价值 |
|------|---------|--------|-------------|
| **v1.1** | 基础加固 | 4 周 | system prompt 可用、本地 Agent 流畅、消息可编辑删除、UI 风格确立 | 让 MVP "好用" |
| **v1.2** | Agent 能力升级 + 任务系统 | 6 周 | 每个 Agent 独立 workspace/memory/tools + 轻量任务系统 | Agent 从"对话机器人"变成"可工作 AI 同事" |
| **v1.3** | 协作与体验 | 6 周 | 文件上传、消息搜索、多 Agent 协作场景、任务系统完善、UI 完善 | 把 Solo 打造成完整的协作平台 |
| **v2.0** | 企业能力 | 8 周 | 审批流、定时任务、插件系统、Agent 市场、OAuth 登录 | 企业级产品就绪 |

---

## 5. v1.1 — 基础加固（4 周）

**目标**：修复 MV P 阶段积累的核心问题，让产品达到"可用且好用的 MVP"状态。

**时间**：4 周（假设团队保持 6 人配置）

### 5.1 迭代 1：已知问题修复（2 周）

#### 用户故事

```
US-v1.1-01: 作为用户，我希望在 Solo 中定义的 Agent system prompt 被完整使用，
             不被本地 Claude Code 配置覆盖，以便 Agent 的行为是我想要的
  - 优先级: P0 | 工作量: M | 领域: 后端+Daemon

US-v1.1-02: 作为用户，我希望每个 Agent 有独立的工作目录，以便不同 Agent
             的文件操作和上下文不会互相干扰
  - 优先级: P0 | 工作量: L | 领域: Daemon

US-v1.1-03: 作为用户，我希望本地 Agent（Claude Code CLI）的回复能逐字输出，
             以便获得实时的反馈体验
  - 优先级: P1 | 工作量: M | 领域: Daemon

US-v1.1-04: 作为用户，我希望可以编辑和删除我发送的消息，以便修正输错的内容
  - 优先级: P0 | 工作量: M | 领域: 全栈
```

#### 任务分解

```
Week 1: Agent 隔离 + 系统修复
┌─────────────────────────────────────────────────────┐
│ SOLO-V1.1-01-B │ Agent 工作目录管理器               │ rd1 │ P0
│                │ 实现 ~/.solo/agents/<agent-id>/     │     │
│                │ 创建/清理/管理每个 Agent 的工作目录  │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.1-02-B │ Daemon 执行环境隔离                │ rd1 │ P0
│                │ processTaskStreaming 在 Agent 专属 │     │
│                │ 目录中调用 local provider           │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.1-03-B │ Agent 专属 CLAUDE.md 生成器        │ rd2 │ P0
│                │ 在 Agent workspace 中生成 agent     │     │
│                │ 专属 CLAUDE.md（从 system_prompt）   │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.1-04-B │ Local Provider 真流式输出          │ rd2 │ P1
│                │ StdoutPipe 逐行读取，分块推送        │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.1-05-QA │ Agent 隔离集成测试                │ qa1 │ P0
│                 │ 验证 prompt 隔离、目录隔离、流式    │     │
└─────────────────────────────────────────────────────┘

Week 2: 消息编辑/删除 + 多模型 UI
┌─────────────────────────────────────────────────────┐
│ SOLO-V1.1-06-B │ 消息编辑/删除后端 API              │ rd1 │ P0
│                │ PATCH/DELETE /api/v1/channels/     │     │
│                │ {id}/messages/{msgId}               │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.1-07-F │ 消息编辑/删除前端 UI               │ fe1 │ P0
│                │ 消息悬停菜单：编辑/删除，编辑模态框   │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.1-08-F │ 多模型选择 UI                      │ fe2 │ P1
│                │ 在 Agent 创建/编辑页增加模型选择下拉 │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.1-09-QA │ 消息编辑删除回归测试              │ qa1 │ P0
└─────────────────────────────────────────────────────┘
```

### 5.2 迭代 2：前端 UI 风格改造启动（2 周）

#### 用户故事

```
US-v1.1-05: 作为用户，我希望 Solo 有一个独特的粗野主义（Neubrutalist）视觉风格，
             让我感觉它是专门为 AI 团队协作设计的
  - 优先级: P1 | 工作量: XL | 领域: 前端

US-v1.1-06: 作为用户，我希望看到更好的消息展示（改进的 Markdown 渲染、
             代码块等），以便更清晰地阅读代码和结构化内容
  - 优先级: P1 | 工作量: M | 领域: 前端
```

#### 任务分解

```
Week 3: UI 风格改造 — 基础
┌─────────────────────────────────────────────────────┐
│ SOLO-V1.1-10-F │ 设计系统 + Tailwind 主题配置       │ fe1 │ P1
│                │ 实施 neubrutalist 配色方案          │     │
│                │ (背景: #fffaef 奶油白, 文字: #000,  │     │
│                │  品牌色: #fe7da8 粉色,              │     │
│                │  2px 黑边框, 5px 硬阴影, 零圆角)    │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.1-11-F │ 字体系统集成                       │ fe1 │ P1
│                │ 集成 Space Grotesk（UI 加粗）       │     │
│                │ + Inter（正文）+ Space Mono（代码）  │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.1-12-F │ 频道列表 + 侧栏样式改造            │ fe1 │ P1
│                │ Space Grotesk 频道名粗体、           │     │
│                │ 2px 黑边分割线、奶油白背景           │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.1-13-F │ 消息列表 + 输入框样式改造           │ fe2 │ P1
│                │ 消息正文用 Inter 字体、               │     │
│                │ 输入框 2px 黑边 + 3px 硬阴影        │     │
└─────────────────────────────────────────────────────┘

Week 4: UI 风格改造 — 完善
┌─────────────────────────────────────────────────────┐
│ SOLO-V1.1-14-F │ Agent 消息样式 + 状态指示器        │ fe1 │ P1
│                │ Agent 名称使用粉色 #fe7da8 品牌色，  │     │
│                │ thinking/typing 状态紫色标签动画     │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.1-15-F │ DM 列表 + Agent 管理页样式改造     │ fe2 │ P1
│                │ 统一 neubrutalist 风格，             │     │
│                │ Agent 卡片 2px 黑边 + 5px 硬阴影    │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.1-16-F │ Markdown 渲染器升级               │ fe1 │ P1
│                │ 代码块使用深色背景 + 黑边框、        │     │
│                │ 行号、复制按钮、语法高亮优化         │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.1-17-F │ 动画 + 微交互完善                  │ fe2 │ P2
│                │ 消息入场动画、打字指示器动画、       │     │
│                │ 按钮 lift/press 效果（阴影变化）    │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.1-18-QA │ UI 视觉回归测试                   │ qa1 │ P1
│                 │ 确保所有页面风格统一、无样式断层    │     │
└─────────────────────────────────────────────────────┘
```

### 5.3 v1.1 验收标准

```gherkin
Scenario: Agent system prompt 独立
  Given 我的 CWD 中有一个 CLAUDE.md 配置为 "你是一个通用助手"
  And 我在 Solo 中创建了一个 Agent "CodeReviewer" 并使用 local provider
  And 配置 system_prompt 为 "You are a Go code reviewer"
  When CodeReviewer 被触发并响应
  Then Agent 的工作目录是 ~/.solo/agents/<agent-id>/
  And 该目录中的 CLAUDE.md 内容来自 system_prompt
  And Agent 的行为符合 "Go code reviewer" 角色的预期

Scenario: Agent 工作目录隔离
  Given 我有 Agent A 和 Agent B，都使用 local provider
  When Agent A 在响应中创建了文件 /tmp/test.txt
  Then Agent B 在工作目录下看不到 /tmp/test.txt

Scenario: 本地 Agent 流式输出
  Given 我使用 local provider 创建了一个 Agent
  When Agent 被触发
  Then 回复内容逐字出现在频道中（非一次性输出）

Scenario: 消息编辑
  Given 我在频道中发送了消息 "Hello world"
  When 我编辑该消息为 "Hi there"
  Then 消息显示已编辑标记
  And 频道成员实时看到编辑后的内容
```

---

## 6. v1.2 — Agent 能力升级（6 周）

**目标**：将 Agent 从"对话机器人"升级为"有工作空间、有记忆、有工具的数字同事"。

**这是 Solo 最关键的版本**。v1.2 决定产品是否具备与 slock.ai 竞争的核心差异化能力。

**时间**：6 周

### 6.1 核心架构变化

```
v1.1（当前）                      v1.2（目标）
────────────────────────────     ─────────────────────────
Agent                             Agent
  ├── name                          ├── name
  ├── system_prompt                 ├── system_prompt
  ├── model / provider              ├── model / provider
  └── is_active                     ├── workspace (目录隔离)
                                    ├── MEMORY.md (长期记忆)
                                    ├── tools (技能系统)
                                    └── is_active
```

### 6.2 用户故事

```
US-v1.2-01: 作为用户，我希望每个 Agent 有自己的 workspace 目录，
             以便其文件操作和中间产物不会影响其他 Agent
  - 优先级: P0 | 工作量: M | 领域: Daemon

US-v1.2-02: 作为用户，我希望 Agent 能记住历史对话的要点，
             以便在后续对话中不需要重复交代上下文
  - 优先级: P0 | 工作量: L | 领域: 全栈

US-v1.2-03: 作为用户，我希望 Agent 能使用工具（读文件、写文件、搜索等），
             以便执行更复杂的任务
  - 优先级: P1 | 工作量: XL | 领域: 全栈

US-v1.2-04: 作为用户，我希望本地 Agent 能利用 Claude Code CLI 的完整能力
             （文件编辑、代码审查、项目分析），而不仅仅是对话
  - 优先级: P1 | 工作量: L | 领域: Daemon

US-v1.2-05: 作为用户，我希望在 Agent 的管理页面能看到每个 Agent 的工作状态
             和活动历史，以便了解 Agent 在做什么
  - 优先级: P2 | 工作量: M | 领域: 全栈

US-v1.2-06: [新增] 作为用户，我希望可以在 Solo 中创建任务并将其分配给
             特定 Agent，以便像管理团队成员一样管理 Agent 的工作
  - 优先级: P1 | 工作量: M | 领域: 全栈
  来源: slock.ai 任务系统分析 — 任务系统是 slock.ai 的核心差异化能力
  说明: 轻量级任务系统 MVP，支持创建任务、分配给 Agent、查看进度

US-v1.2-07: [新增] 作为用户，我希望 Agent 完成任务后自动更新
             任务状态，以便我知道工作进度
  - 优先级: P1 | 工作量: M | 领域: 后端
````

### 6.3 功能详述

#### F-22: Agent 工作空间（Workspace）

**概述**：每个 Agent 在 Daemon 所在机器上拥有独立的文件目录，用于存放其专属配置、记忆、工作产物。

**目录结构**：

```
~/.solo/agents/<agent-id>/
  ├── CLAUDE.md           # Agent 专属系统指令（从 system_prompt 生成）
  ├── MEMORY.md           # Agent 长期记忆（自动维护）
  ├── workspace/          # Agent 工作目录（执行时的 CWD）
  │   ├── files...         # Agent 创建/修改的文件
  │   └── logs/            # Agent 活动日志
  ├── tools/              # Agent 专属工具配置
  │   └── (tool configs)   # * 后续版本
  └── config.json         # Agent 运行时配置本地缓存
```

**交互流程**：

1. 用户创建 Agent 时 → Server 通知 Daemon 创建 workspace
2. Daemon 在 `~/.solo/agents/<id>/` 创建目录结构
3. 从 `system_prompt` 生成 `CLAUDE.md`
4. 创建空的 `MEMORY.md`
5. Agent 被触发时 → Daemon 在 workspace 目录下执行
6. Agent 响应完成后 → Daemon 读取/更新 MEMORY.md

#### F-23: Agent 记忆系统（Memory）

**概述**：Agent 在每次对话后自动总结关键信息并写入 MEMORY.md，后续对话时自动加载作为上下文。

**实现策略**：

```
Agent 被触发
  → Daemon 读取 ~/.solo/agents/<id>/MEMORY.md
  → 将记忆内容注入到 system prompt 中（"这是你之前的记忆：..."）
  → Agent 基于对话上下文 + 记忆 生成响应
  → Daemon 在响应完成后调用 LLM 做一次记忆摘要
  → 摘要写入 MEMORY.md（增量更新，保留最近 N 条记忆）
```

**记忆格式（MEMORY.md）**：

```markdown
# Agent Memory — <agent_name>

> 自动维护。每次对话后由 Agent 自身更新。

## Recent Memories

- [2026-05-11] 用户希望我以 Go 代码审查者的角色工作，
  重点关注并发安全和内存管理
- [2026-05-10] 在 #general 频道中审查了 PR #42，
  发现了一个 data race 问题
- [2026-05-09] 用户偏好函数式编程风格，避免使用全局变量

## Active Context

- 当前频道: #general, #project-alpha
- 最近协作的 Agent: @CodeWriter
- 当前关注: PR #42 的 data race fix
```

**实现注意**：

- MEMORY.md 初始内容来自 Agent 创建时的 system_prompt
- 每次对话后，LLM 调用返回的完整响应会附加一段 `memory_summary`
- Daemon 解析 memory_summary 并更新 MEMORY.md
- 记忆自动维护，无需用户手动操作
- 记忆有上限（默认 50 条），超出时自动截断
- 用户可以在 Agent 管理页面查看/编辑 MEMORY.md

#### F-24: Agent 工具/技能系统（Tools）

**概述**：Agent 可以配置和使用工具，扩展其能力边界。

**MVP 工具集（v1.2 交付）**：

| 工具 | 描述 | 实现方式 | 风险 |
|------|------|---------|------|
| `read_file` | 读取工作目录中的文件 | Daemon 实现文件读取，返回内容 | 低 — 纯读取 |
| `write_file` | 在工作目录中写入文件 | Daemon 实现文件写入 | 中 — 需限制路径范围 |
| `list_files` | 列出工作目录中的文件 | Daemon 实现目录列表 | 低 |
| `search_files` | 在工作目录中搜索文件内容 | `grep` 封装 | 低 |
| `web_search` | 网络搜索（需 API Key） | 通过搜索 API | 中 — 依赖外部服务 |
| `execute_command` | 执行 shell 命令（需审批） | Daemon 执行，限制沙箱 | **高** — 安全风险 |

**安全策略**：

```
工具安全等级:
  ├── 安全（自动允许）: read_file, list_files, search_files
  ├── 关注（需用户确认）: write_file, web_search
  └── 危险（需审批流）: execute_command（v2.0 接入审批模块）
```

**工具调用流程**（复用 multica 已定义的 Tool Executor 架构）：

```
Agent 响应生成
  → LLM 输出 tool_call（function calling）
  → Daemon 解析 tool_call，执行对应工具
  → 工具结果返回给 LLM
  → LLM 使用工具结果继续生成
  → 流式输出
```

#### F-25: 本地 Agent 深度能力复用

**概述**：挖掘 Claude Code CLI 超出对话之外的完整能力。

**当前模式**：
```
claude -p "<prompt>" --print    # 仅对话，输出文本
```

**目标模式**：

引入两种执行模式，由用户为每个 Agent 选择：

| 模式 | 执行方式 | 适用场景 | 能力 |
|------|---------|---------|------|
| **对话模式**（默认） | `claude -p "..." --print` | 纯对话、问答、代码审查 | 仅文本输出 |
| **交互模式** | `claude -p "..."`（无 `--print`） | 文件编辑、代码修改、项目级任务 | 文件操作、代码修改、Shell 命令执行 |

**交互模式实现**：

```go
// 交互模式：无 --print 标志，允许 Claude Code 执行文件操作
cmd := exec.CommandContext(runCtx, p.binary, "-p", prompt)
cmd.Dir = workspaceDir  // Agent 专属工作目录
cmd.Env = append(os.Environ(),
    "CLAUDE_CODE_HEADLESS=true",
    "CLAUDE_CODE_ALLOW_WRITES=true",  // 允许文件写操作
)
```

**用户可见的 Agent 模式切换**（Agent 编辑页）：

```
执行模式:
┌─────────────────────────────────────┐
│ ○ 对话模式 (仅文本回复，安全快速)     │
│ ● 交互模式 (可读写文件，执行命令)     │
│                                      │
│ ⚠ 交互模式下 Agent 可以修改文件         │
└─────────────────────────────────────┘
```

#### F-26: 轻量级任务系统（Task System）[新增]

**概述**：参照 slock.ai 的任务系统设计，在 Solo 中引入轻量级任务管理模块。用户可创建任务、分配给 Agent、跟踪进度。这是 Solo 从"聊天工具"迈向"协作平台"的关键一步。

**数据模型**：

```sql
CREATE TABLE tasks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id      UUID REFERENCES channels(id) ON DELETE CASCADE,
    creator_id      UUID REFERENCES users(id),
    assignee_id     UUID REFERENCES agents(id),       -- 分配给 Agent
    title           TEXT NOT NULL,
    description     TEXT,
    status          TEXT NOT NULL DEFAULT 'open',     -- open, in_progress, review, done, cancelled
    priority        TEXT NOT NULL DEFAULT 'medium',   -- urgent, high, medium, low
    due_at          TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tasks_channel ON tasks(channel_id);
CREATE INDEX idx_tasks_assignee ON tasks(assignee_id);
CREATE INDEX idx_tasks_status ON tasks(status);
```

**交互流程**：

```
用户创建任务
  → POST /api/v1/tasks (title, channel_id, assignee_agent_id, priority, description)
  → 任务出现在频道消息中（系统消息："@User 向 @Agent 分配了任务: 审查 PR #42"）
  → Agent 在频道中收到任务通知
    (Agent system prompt 自动包含当前分配给它的任务列表)
  → Agent 开始执行 (channel 中显示 "Agent 开始处理: 审查 PR #42")
  → Agent 完成 → 调用 PATCH /api/v1/tasks/{id} 更新 status="done"
  → 频道消息通知："@Agent 完成了任务: 审查 PR #42"

任务列表 UI（侧栏新模块）:
┌─────────────────────────────────────┐
│ ○ Tasks                            │
│   ├── #pr-review: 审查 PR #42      │
│   │   └── @CodeReviewer ● [doing]  │
│   ├── #bugfix: 修复 data race      │
│   │   └── @DevAgent ◌ [todo]       │
│   └── #docs: 写 API 文档            │
│       └── @Writer ◌ [todo]         │
└─────────────────────────────────────┘
```

**状态流转**：

```
open → in_progress (Agent 接手)
in_progress → review (需人工确认)
in_progress → done (Agent 完成，自动确认)
review → done (人工批准)
any → cancelled (取消)
```

**MVP 范围**（v1.2 交付）：
- 任务 CRUD（创建/查看/更新状态/删除）
- 任务分配给 Agent
- Agent 可读取分配给自己的任务列表
- Agent 完成任务后自动更新状态
- 频道中任务相关的系统消息通知

**后续范围**（v1.3 增强）：
- 任务列表 UI（侧栏独立模块）
- 任务筛选/排序
- 任务依赖关系
- 从频道消息直接创建任务（"/todo" slash 命令）
- 统计报表（按 Agent/频道/状态）

### 6.3a v1.2 任务分解

```
Week 1-2: Agent 工作空间 + 记忆系统
┌─────────────────────────────────────────────────────┐
│ SOLO-V1.2-01-B │ Agent workspace 创建/同步/删除      │ rd1 │ P0
│                │ Daemon 端目录管理，Server 通知联动   │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.2-02-B │ MEMORY.md 生命周期管理               │ rd1 │ P0
│                │ 创建→读取→注入→摘要→写入全流程       │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.2-03-B │ Agent 专属 CLAUDE.md 生成器 v2      │ rd2 │ P0
│                │ 从 system_prompt 生成含记忆和工具的   │     │
│                │ 高级 CLAUDE.md                      │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.2-04-F │ Agent 记忆管理 UI                   │ fe1 │ P1
│                │ 在 Agent 管理页增加 Memory 标签页     │     │
│                │ 查看/编辑/清空 MEMORY.md              │     │
└─────────────────────────────────────────────────────┘

Week 3-4: 工具系统 + 本地 Agent 交互模式
┌─────────────────────────────────────────────────────┐
│ SOLO-V1.2-05-B │ 工具系统核心（ToolRegistry + 执行）  │ rd2 │ P1
│                │ read_file / write_file / list_files  │     │
│                │ / search_files 实现 + LLM function   │     │
│                │ calling 对接                         │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.2-06-B │ 工具安全策略                         │ rd2 │ P1
│                │ 工具安全等级检查 + 路径限制           │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.2-07-B │ 本地 Agent 交互模式                  │ rd1 │ P1
│                │ 对话模式 ↔ 交互模式切换              │     │
│                │ 交互模式下允许文件写操作              │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.2-08-F │ Agent 执行模式 UI                    │ fe1 │ P1
│                │ Agent 创建/编辑页的模式选择开关        │     │
│                │ 安全提示信息                          │     │
└─────────────────────────────────────────────────────┘

Week 5-6: 轻量级任务系统
┌─────────────────────────────────────────────────────┐
│ SOLO-V1.2-09-B │ 任务系统后端：CRUD API               │ rd1 │ P1
│                │ tasks 表 + POST/PATCH/GET/DELETE     │     │
│                │ 频道任务通知消息（系统消息）           │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.2-10-B │ Agent 任务读取与状态更新              │ rd2 │ P1
│                │ Agent 可读取分配给自己的任务列表       │     │
│                │ Agent 完成任务后自动调用 PATCH 更新    │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.2-11-F │ 任务列表 UI MVP                      │ fe2 │ P1
│                │ 侧栏任务模块入口 + 任务创建表单        │     │
│                │ 任务列表展示 + 状态切换按钮           │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.2-12-B │ Agent system prompt 集成任务上下文    │ rd1 │ P1
│                │ Agent 触发时自动注入任务列表           │     │
│                │ Agent 知道当前被分配的任务是什么       │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.2-13-QA │ v1.2 全功能集成测试                 │ qa1 │ P0
│                 │ 记忆持久化 + 工具执行 + 任务流转      │     │
└─────────────────────────────────────────────────────┘
```

### 6.4 验收标准（更新版）

```gherkin
Scenario: Agent 记忆持久化
  Given Agent "Helper" 在频道 #general 中被触发
  And Agent 回复了关于"项目架构"的内容
  When 第二天我在同一频道再次触发 Agent
  Then Agent 记得昨天的对话关于"项目架构"
  And 不需要我重复提供上下文

Scenario: Agent 使用工具
  Given Agent "Helper" 配置了 read_file 工具
  When 我让 Helper "读取 workspace 中的 README.md"
  Then Agent 调用 read_file 工具
  And 工具结果正确返回
  And Agent 基于读取的内容生成回复

Scenario: Agent 工作目录隔离
  Given Agent "Reviewer" 和 "Writer" 都使用 local provider
  When Reviewer 创建了文件 review.txt
  Then Writer 的工作目录中没有 review.txt

Scenario: 创建并分配任务给 Agent [新增]
  Given 我在频道 #general 中
  When 我创建任务 "审查 PR #42" 并分配给 Agent "CodeReviewer"
  Then 频道中出现系统消息: "向 @CodeReviewer 分配了任务"
  And Agent "CodeReviewer" 的任务列表中包含该任务
  And Agent "CodeReviewer" 可以读取任务的标题和描述

Scenario: Agent 完成任务 [新增]
  Given Agent "CodeReviewer" 有一项开放任务 "审查 PR #42"
  When Agent 完成工作后调用完成任务接口
  Then 任务状态变为 "done"
  And 频道中出现系统消息: "@CodeReviewer 已完成任务"
  And 任务列表 UI 中该任务标记为完成
```

---

## 7. v1.3 — 协作与体验补齐（6 周）

**目标**：补全 slock.ai 已有但 Solo 缺失的核心协作功能（G 系列差距）。这是与 slock.ai 功能对标的关键收尾阶段。

**时间**：6 周

### 7.1 优先级重新排序（基于对标分析）

| 新优先级 | 功能 | G 编号 | 对应 slock.ai 能力 | 理由 |
|---------|------|--------|-------------------|------|
| **P0** | 电脑/机器管理 | G-01 | /computers 页面 | 核心卖点之一（Your computers, your agents） |
| **P0** | 任务系统 UI 增强 | G-02 | /tasks | 从 v1.2 的 MVP 升级到完整 UI |
| **P0** | 文件上传/附件 | G-09 | /attachments/upload | 协作平台必备 |
| **P1** | 消息搜索 | G-03 | /messages/search | 信息检索核心 |
| **P1** | 已保存/收藏消息 | G-04 | /channels/saved | 已是 slock.ai 基础能力 |
| **P1** | 推送通知 | G-08 | /push/subscribe+vapid | 实时感知 |
| **P1** | 多 Agent 协作 | - | Agent-to-Agent @mentions | 产品差异化 |
| **P2** | 头像上传 | G-11 | /auth/me/avatar | 个性化 |
| **P2** | 发布说明 | G-13 | /release-notes | 用户沟通 |
| **P2** | 用量统计 | - | 基础功能 | 成本控制 |
| **P2** | Onboarding 引导 | - | - | 用户体验 |
| **P2** | 频道归档 + UX 打磨 | - | - | 整理空间 |

### 7.2 用户故事

```
US-v1.3-01: 作为用户，我希望在 Solo 界面中看到各 Computer 的状态、
             连接的 Agent 列表，以便管理我的计算资源
  - 优先级: P0 | 工作量: M | 领域: 全栈
  - 来源: G-01, slock.ai /computers

US-v1.3-02: 作为用户，我希望可以在消息中上传和分享文件，
             以便在频道中共享代码片段、图片和文档
  - 优先级: P0 | 工作量: L | 领域: 全栈

US-v1.3-03: 作为用户，我希望完整体验任务系统（创建/分配/追踪/完成），
             以便像管理团队成员一样管理 Agent 的工作
  - 优先级: P0 | 工作量: L | 领域: 全栈
  - 来源: G-02, slock.ai /tasks

US-v1.3-04: 作为用户，我希望可以搜索所有频道的消息，
             以便快速找到之前的讨论内容
  - 优先级: P1 | 工作量: M | 领域: 后端

US-v1.3-05: 作为用户，我希望可以收藏重要消息，
             以便后续快速回顾
  - 优先级: P1 | 工作量: S | 领域: 全栈

US-v1.3-06: 作为用户，我希望收到浏览器推送通知，
             以便即使不在 Solo 页面也不会错过重要消息
  - 优先级: P1 | 工作量: M | 领域: 全栈

US-v1.3-07: 作为用户，我希望多个 Agent 可以在同一频道中协作，
             互相调用和接力完成任务
  - 优先级: P1 | 工作量: L | 领域: 全栈

US-v1.3-08: 作为用户，我希望看到我的 LLM 用量和费用统计，
             以便控制成本
  - 优先级: P2 | 工作量: M | 领域: 全栈
```

### 7.3 任务分解

```
Week 1-2: 机器管理 + 任务系统完整版
┌─────────────────────────────────────────────────────┐
│ SOLO-V1.3-01-B │ Computer 管理后端                  │ rd1 │ P0
│                │ computers 表 + CRUD API +           │     │
│                │ Daemon 注册/心跳/在线状态           │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.3-02-F │ Computer 管理前端                  │ fe1 │ P0
│                │ /computers 页面：状态卡片、          │     │
│                │ 在线/离线指示器、Agent 列表分配      │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.3-03-B │ 任务系统完整后端                    │ rd2 │ P0
│                │ 状态流转/筛选/排序/频道集成          │     │
│                │ 从@消息"#todo"创建任务               │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.3-04-F │ 任务系统完整前端                    │ fe2 │ P0
│                │ /tasks 看板视图 + 侧栏入口           │     │
│                │ 状态切换/Agent 分配/进度展示         │     │
└─────────────────────────────────────────────────────┘

Week 3-4: 文件上传 + 消息搜索 + 收藏
┌─────────────────────────────────────────────────────┐
│ SOLO-V1.3-05-B │ 文件上传后端                        │ rd1 │ P0
│                │ POST /attachments/upload            │     │
│                │ 文件存储 + 消息附件引用              │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.3-06-F │ 文件上传前端                        │ fe1 │ P0
│                │ 拖拽上传 + 文件预览 + 图片缩略图     │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.3-07-B │ 全文搜索后端                        │ rd2 │ P1
│                │ GIN 索引 + /messages/search API     │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.3-08-F │ 搜索 UI + 收藏 UI                  │ fe2 │ P1
│                │ CMD+K 搜索面板 + /search 页面        │     │
│                │ 消息收藏按钮 + /saved 页面           │     │
└─────────────────────────────────────────────────────┘

Week 5-6: 多 Agent 协作 + 通知 + 打磨
┌─────────────────────────────────────────────────────┐
│ SOLO-V1.3-09-B │ Agent-to-Agent 协作协议 + 循环防护  │ rd1 │ P1
│                │ @mention 另一个 Agent 触发接力       │     │
│                │ 触发链追踪（防死循环）               │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.3-10-B │ 推送通知后端                        │ rd2 │ P1
│                │ Web Push API + VAPID key             │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.3-11-F │ 推送通知前端 + AI Agent 协作可视化   │ fe1 │ P1
│                │ 通知权限请求 + 浏览器通知             │     │
│                │ Agent 调用链可视化                   │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.3-12-B │ 用量统计后端                        │ rd2 │ P2
│                │ usage_records 表 + token/费用追踪    │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.3-13-F │ 用量统计前端 + Onboarding 引导       │ fe2 │ P2
│                │ + 发布说明页面 (/release-notes)       │     │
│                │ + 头像上传 (/auth/me/avatar)          │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.3-14-F │ 全局 UX 打磨                        │ fe1 │ P2
│                │ 统一加载/空/错误态 + 键盘快捷键       │     │
│                │ 频道归档 + 消息虚拟滚动              │     │
├─────────────────────────────────────────────────────┤
│ SOLO-V1.3-15-QA │ v1.3 全功能回归测试                │ qa1 │ P1
└─────────────────────────────────────────────────────┘
```

---

## 8. v2.0 — 功能补齐与平台化（8 周）

**目标**：补齐所有剩余 G 系列差距，在核心功能维度达到与 slock.ai 可竞争的水平，同时启动平台化方向。

### 8.1 核心交付

| 功能 | G 编号 | slock.ai 对标 | 工作量 | 优先级 | 说明 |
|------|--------|-------------|--------|--------|------|
| **统一收件箱** | G-05 | `/channels/inbox` + `/channels/inbox/done` | M | **P1** | 聚合所有未读/已处理消息流 |
| **线程收件箱** | G-06 | ThreadsInbox + `/channels/threads/followed` | M | **P1** | 追踪关注/未完成线程 |
| **多工作区/团队管理** | G-10 | `/servers` + `/join/:code` + 成员管理 | L | **P1** | 多 workspace 切换、邀请链接 |
| **已读/未读追踪** | G-15 | 频道 unread_count + has_mention | M | **P1** | 频道消息级已读标记 |
| **OAuth/SSO 登录** | - | GitHub/Google | M | P2 | 登录方式扩充 |
| **消息翻译** | G-07 | `/message-translations:batch` | M | P3 | 多语言翻译 |
| **Agent 市场/模板** | - | 预配置 Agent 模板 | L | P2 | 社区贡献 |
| **Billing/订阅** | G-14 | `/billing/subscription` | M | P3 | 付费订阅 |
| **审批流** | - | Agent 敏感操作审批 | L | P2 | 与 tool execute_command 联动 |
| **定时任务** | - | Agent cron 执行 | M | P2 | 计划性任务 |
| **插件系统（MCP）** | - | 可插拔工具扩展 | XL | P2 | 开放工具生态 |
| **移动端适配** | - | 响应式 Web | XL | P3 | 布局重构 |

### 8.2 版本节奏

```
Week 17-18: 收件箱 + 线程收件箱（G-05/G-06）
  → Inbox 功能：聚合所有频道/DM 的未读消息
  → 线程收件箱：已关注/未完成线程列表
  → 已读/未读标记：消息级已读追踪

Week 19-21: 多工作区 + OAuth（G-10）
  → 多 workspace 数据模型 + URL 重构（引入 /s/ 前缀）
  → 工作区切换 UI（侧栏顶部下拉）
  → 邀请链接生成与验证（/join/:code）
  → GitHub/Google OAuth 登录

Week 22-24: Agent 市场 + Billing + 插件
  → Agent 模板：预设 prompt 和工具配置
  → Billing 订阅管理页面
  → MCP 插件协议支持（将内置工具迁移到 MCP 架构）
  → 审批流与工具系统联动
```

### 8.3 与 slock.ai 对标收敛

v2.0 完成后，Solo 在以下功能维度应基本与 slock.ai 对齐：

| 维度 | slock.ai | Solo v2.0 |
|------|---------|-----------|
| 频道/DM/Threads | 完整 | 对齐 |
| 多工作区 | 完整 | 对齐 |
| 消息搜索 | 完整 | 对齐 |
| 文件上传/附件 | 完整 | 对齐 |
| Agent 记忆 | 核心卖点 | v1.2 已交付 |
| Agent 工具 | 完整 | v1.2 已交付 |
| Computer/机器管理 | 完整 | v1.3 已交付 |
| 任务系统 | 完整 | v1.3 已交付 |
| 收件箱/线程收件箱 | 完整 | 对齐 |
| 已读/未读 | 完整 | 对齐 |
| OAuth | 完整 | 对齐 |
| 推送通知 | 完整 | 对齐 |
| 发布说明 | 完整 | 对齐 |
| Billing | 完整 | 对齐 |
| **差异化** | 闭源 SaaS | **开源自托管** |
| **差异化** | 黄色 Brutalist UI (#FFD700) | **粉色 Neubrutalist UI (#fe7da8)** |

---

## 9. Agent 能力深度规划

### 9.1 Agent 工作空间（Workspace）— 架构设计

**目标**：为每个 Agent 提供隔离的文件系统环境，确保 prompt 不受外部干扰，Agent 操作互不干扰。

#### 数据模型（FileSystem）

本地文件系统目录结构，**不存储在数据库中**——因为 Daemon 就是执行引擎，本地文件系统是最直接的方式。

```
~/.solo/
  ├── agents/
  │   ├── <agent-uuid-1>/         # 每个 Agent 一个目录
  │   │   ├── CLAUDE.md           # → 从 agents.system_prompt 同步
  │   │   ├── MEMORY.md           # → Agent 长期记忆（自动维护）
  │   │   ├── config.json         # → Agent 运行时配置缓存
  │   │   └── workspace/          # → Agent 的 CWD（执行目录）
  │   │       └── (files...)
  │   └── <agent-uuid-2>/
  │       └── ...
  └── daemon/
      └── daemon.json             # Daemon 自身配置
```

#### 创建流程

```
1. 用户在 Solo Agent 管理页创建 Agent → POST /api/v1/agents
2. Server 处理 Agent 创建 → INSERT INTO agents
3. Server 通知 Daemon → POST /internal/daemon/agents/create
   {
     "agent_id": "<uuid>",
     "system_prompt": "You are a...",
     "name": "CodeReviewer"
   }
4. Daemon 创建目录：
   ~/.solo/agents/<uuid>/
   ├── CLAUDE.md    (从 system_prompt 生成)
   ├── MEMORY.md    (空文件，带初始模板)
   ├── config.json
   └── workspace/   (空目录)
```

#### 更新流程

```
用户编辑 Agent system_prompt → PATCH /api/v1/agents/{id}
  → Server 更新 DB
  → Daemon 同步 ~/.solo/agents/<id>/CLAUDE.md
```

#### 删除流程

```
用户删除 Agent → DELETE /api/v1/agents/{id}
  → Server 设置 is_active=false（软删除）
  → 可选：Daemon 清理 ~/.solo/agents/<id>/ 目录（或保留供恢复）
```

### 9.2 Agent 记忆系统（Memory）— 架构设计

#### 记忆生命周期

```
           初始          对话触发          对话完成
Agent 创建 ───► MEMORY.md ───► Agent 加载记忆 ───► Agent 产生新记忆
               (模板)         ▸ 注入 system prompt    ▸ LLM 输出 memory_summary
                              ▸ Agent "知道"之前的事   ▸ 写入 MEMORY.md
```

#### 记忆读取

当 Agent 被触发时，Daemon 在调用 LLM 前读取 MEMORY.md，将记忆内容注入到 system prompt 中：

```go
// 伪代码
func buildPromptWithMemory(agentID string, req *llm.CompletionRequest) {
    memory := readMemoryFile(agentID)  // ~/.solo/agents/<id>/MEMORY.md
    if memory != "" {
        req.SystemPrompt = req.SystemPrompt + "\n\n## Your Memory\n" + memory
    }
}
```

#### 记忆写入

Agent 每次响应完成后，Daemon 向 LLM 发送一次额外的简短调用，生成记忆摘要：

```
System: "Summarize the key information from this conversation that
the assistant should remember for future interactions. Output in
bullet points, max 3 items."

Messages: [conversation messages]
```

LLM 返回的记忆摘要存入 MEMORY.md。

#### 记忆管理（用户端）

- Agent 管理页增加 "Memory" 标签页
- 用户可查看、编辑、清空 MEMORY.md
- 用户可手动添加记忆条目（"记住：我喜欢 Python 而非 Java"）
- 每条记忆有时间戳和来源标记

### 9.3 Agent 工具/技能系统（Tools）— 架构设计

#### 工具定义接口

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.Schema    // JSON Schema for LLM function calling
    Execute(ctx context.Context, params json.RawMessage) (ToolResult, error)
}

type ToolResult struct {
    Success bool
    Data    interface{}  // 工具返回的数据（给 LLM）
    Error   string
}
```

#### 内置工具实现

```go
// ReadFileTool — 读取工作目录中的文件
type ReadFileTool struct {
    workspaceDir string  // Agent 专属目录
}

// WriteFileTool — 在工作目录中写入文件
type WriteFileTool struct {
    workspaceDir string
}
```

#### 工具注册与发现

```go
// Daemon 端
type ToolRegistry struct {
    tools map[string]Tool
}

func NewDefaultToolRegistry(workspaceDir string) *ToolRegistry {
    return &ToolRegistry{
        tools: map[string]Tool{
            "read_file": NewReadFileTool(workspaceDir),
            "write_file": NewWriteFileTool(workspaceDir),
            "list_files": NewListFilesTool(workspaceDir),
            "search_files": NewSearchFilesTool(workspaceDir),
        },
    }
}
```

#### 工具调用流程（复用 LLM Function Calling）

```
Agent 收到触发 → 构建 LLM Request（含 tools=[...]）
  → LLM 响应 tool_call → Daemon 执行工具
  → 工具结果返回给 LLM → LLM 继续生成
  → 流式输出最终结果
```

### 9.4 Agent Prompt 与 Claude Code 配置分离

**核心原则**：Solo 中定义的 system_prompt 必须是 Agent 行为的**权威来源**。

#### 分离策略

```
~/project/CLAUDE.md          # 项目级配置（用户的工作项目）
  └── 对 Solo Agent 不可见（除非显式配置）

~/.solo/agents/<id>/CLAUDE.md  # Agent 级别配置（由 Solo 管理）
  └── 内容来自 agents.system_prompt + 动态注入
```

**生成 CLAUDE.md 的规则**：

```markdown
# Agent: <name>

> 由 Solo 管理，请勿手动编辑

## Identity
{agent.system_prompt}

## Capabilities
- 你是 Solo 协作平台中的 AI Agent
- 你可以从 MEMORY.md 读取之前的记忆
- 你参与频道/私信/线程中的对话
- {如果有工具} 你可以使用以下工具: {tool_list}

## Rules
- 只在被触发时回复（收到用户消息或被 @提及）
- 不回复自己的消息
- 每次对话后更新 MEMORY.md
```

### 9.5 能力演进路线

```
v1.1 (当前加固)        v1.2 (Agent 升级)      v1.3 (协作)        v2.0 (企业)
──────────────────    ──────────────────    ────────────────    ──────────────
Agent 工作目录隔离      Agent MEMORY.md       Agent-to-Agent      定时 Agent 任务
Prompt 与 Claude 分离   内置工具系统             协作协议              审批流联动
Local Provider 流式    本地 Agent 交互模式       Agent 调用链可视      插件/MCP 工具
                        工具安全等级              Agent 活动历史        Agent 市场
```

---

## 10. 多 Agent 协作场景

### 10.1 场景一：代码审查协同

```
频道: #pr-review

参与者: 人类（开发者） + CodeReviewer Agent + TestWriter Agent

流程:
1. 开发者: "@CodeReviewer 请审查 PR #42 的变更"
2. CodeReviewer (被 @): 读取 workspace 中的代码变更 →
   分析代码质量 → 发现 data race → 
   "@TestWriter 请为 data race 写一个测试用例"
3. TestWriter (被 @): 读取相关代码 → 在 workspace/ 中创建测试文件 →
   "已创建测试文件 test_race_condition.go"
4. CodeReviewer: "已审查。发现 data race 位于 xx.go:42。
   TestWriter 已创建测试文件。建议方案：使用 sync.Mutex"
```

**验收标准**：
- Agent 可以在回复中包含 `@AnotherAgent` 触发另一个 Agent
- 被触发的 Agent 在同一个频道上下文中工作
- 调用链在 UI 中可视化（Agent A → Agent B → Agent C）

### 10.2 场景二：研究 + 写作协同

```
频道: #market-research

参与者: 人类（PM） + Researcher Agent + Writer Agent

流程:
1. PM: "研究一下 AI Agent 平台的市场趋势"
2. Researcher: 搜索网络 → 收集信息 → 输出研究报告
3. PM: "@Writer 请基于 Researcher 的报告写一篇博客"
4. Writer: 读取 Researcher 的输出 → 在 workspace/ 中创建 blog.md →
   "已创建博客草稿，需要进一步审阅吗？"
```

### 10.3 场景三：开发任务分解

```
频道: #project-alpha

参与者: 人类（技术负责人） + Architect Agent + Dev Agent + Reviewer Agent

流程:
1. 技术负责人: "我们需要一个用户认证模块"
2. Architect: 设计架构 → 输出技术方案
3. 技术负责人: "@Dev 请按架构方案实现"
4. Dev: 在 workspace/ 中创建代码文件 → 实现认证模块
5. Dev: "@Reviewer 请审查我的实现"
6. Reviewer: 审查代码 → 提出修改建议 → 批准或要求修改
```

### 10.4 技术实现要点

**Agent-to-Agent 协作机制**：

```go
// 消息处理中的 @提及 → 触发 Agent
// 1. Agent A 的回复中包含 @AgentB
// 2. 后端解析 @提及（已有 MentionService）
// 3. 被 @提及 的 Agent B 被触发
// 4. Agent B 收到包含 Agent A 回复的上下文
// 5. Agent B 在同一个频道中回复

// 关键：防止循环依赖
// - Agent A @Agent B，B 回复，B 不能 @A 再触发 A 形成死循环
// - 规则：在同一触发链中，已参与的 Agent 不被重复触发
```

**循环防护**：

```go
// 触发链追踪
type TriggerChain struct {
    TaskID       string
    ChannelID    string
    OriginalMessageID string
    ParticipatedAgents map[string]bool  // 已参与的 Agent
    Depth        int                     // 当前深度
    MaxDepth     int                     // 最大深度（默认 5）
}
```

---

## 11. 前端 UI 风格改造路线

> **设计决策确认**：2026-05-11 实地体验之后，pm1 与 fe1 已对齐意见，用户最终决策如下：
> - Solo 采用 **Neubrutalism（粗野主义）** 设计体系，与 slock.ai 保持一致的设计语言基底
> - 通过品牌色**差异化**：Solo 使用粉色 #fe7da8，slock 使用黄色 #FFD700
> - **并非**暗色开发者终端风格，而是**亮色粗野主义**：奶油白背景、2px 黑边框、5px 硬阴影
> - 详细设计系统规范以 `frontend-redesign-v2.md` v3.0 为单一真实来源

### 11.1 竞品风格对比

| 产品 | 设计风格 | 品牌色 | 字体 | 设计语言 |
|------|---------|-------|------|---------|
| **Slock.ai** | Neubrutalism（粗野主义） | 黄色 #FFD700 + 奶油白 + 黑 | Space Grotesk + Space Mono | 粗边框、无圆角、高对比度、极简 |
| **Solo（当前）** | 通用 SaaS（shadcn/ui slate） | 灰蓝 #64748b | Inter + 系统字体 | 圆角、毛玻璃、卡片、灰度色阶 |
| **Solo（目标）** | **Neubrutalism（粗野主义）** | **粉色 #fe7da8** + 奶油白 + 黑 | Space Grotesk(UI) + Inter(正文) + Space Mono(代码) | 2px黑边、5px硬阴影、零圆角、扁平填充 |

### 11.2 设计原则

| 原则 | 描述 |
|------|------|
| **图形坦率（Graphic Bluntness）** | 高对比度、粗轮廓、显著的结构感——组件清晰宣言自身的存在 |
| **显性胜于隐性** | 界面直接表达结构，不用隐晦的阴影或渐变来"暗示"层级 |
| **个性胜过隐形** | 让人记住的结构优于完美抛光——粉色品牌色、lift/press 动效 |
| **分类色彩** | 扁平填充色、无渐变、高饱和度——每个颜色有明确的语义角色 |
| **零虚化深度** | 反拟物化阴影——纯黑 `box-shadow: x y 0 0 #000`，blur = 0 |
| **统一线宽** | 大多数组件使用统一边框宽度——2px 纯黑 |
| **品牌差异化** | 粉色 #fe7da8 为主色调，与 slock 的黄色 #FFD700 形成视觉差异 |

### 11.3 配色方案

```
品牌调色板（Solo 专属）:
  Black:        #000000    — 边框、主文字、结构元素
  White:        #ffffff    — 卡片背景、输入框背景
  Cream:        #fffaef    — 页面/外壳背景
  Pink:         #fe7da8    — 主要品牌色（按钮、链接、高亮）
  Pink Light:   #ffe0ec    — 粉色背景色
  Yellow:       #ffd440    — 辅助色/强调
  Lavender:     #bbafe6    — 次要强调色（Agent 标签）
  Cyan:         #27ccf3    — 信息色
  Lime:         #a9d877    — 成功/在线
  Orange:       #f8a16f    — 警告/思考中
  Red:          #f97264    — 错误/危险
  Stone:        #c0b9b1    — 中性辅助色

功能色（shadcn/ui 兼容）:
  --background:           #fffaef
  --foreground:           #000000
  --card:                 #ffffff
  --primary:              #fe7da8
  --secondary:            #ffffff
  --muted:                #f5f0e8
  --muted-foreground:     #666666
  --accent:               #ffd440
  --destructive:          #f97264
  --border:               #000000
  --ring:                 #74b9ff  （a11y 焦点指示器）
```

### 11.4 字体方案

```
UI/标题字体:    Space Grotesk（700 粗体）— 按钮、标签、侧栏、频道名
正文字体:       Inter（400/500）— 消息内容、长文本
代码字体:       Space Mono（400/700）— 代码块、行内代码、时间戳
展示字体:       Syne（700/800，可选）— Logo、Hero 标题
字号基准:       14px（UI）、15px（正文）、12px（辅助信息）
```

### 11.5 组件样式映射

| 当前 shadcn/ui 组件 | 目标 Neubrutalist 风格 |
|--------------------|----------------------|
| 圆角 `rounded-lg` | 零圆角 `rounded-none` |
| 灰色背景 `bg-muted` | 奶油白背景 `bg-[#fffaef]` |
| 按钮 `bg-primary rounded-md shadow-sm` | 粉色底 `bg-[#fe7da8]` + 黑字 + 2px黑边 + 5px硬阴影 |
| 按钮 hover | `translate(-1px, -1px)` + 阴影从 5px 扩大到 7px |
| 按钮 active | `translate(3px, 3px)` + 阴影消失（按压效果） |
| 卡片 `rounded-xl border shadow` | `border-2 border-black` + `shadow-[5px_5px_0_0_#000]` |
| 输入框 `rounded-md border` | `border-2 border-black` + `shadow-[3px_3px_0_0_#000]` |
| 输入框 focus | `shadow-[5px_5px_0_0_#000]` + 3px 蓝色 #74b9FF outline |
| 消息展示 | Inter 正文 + Space Grotesk 加粗发件人名称 |
| Agent 消息 | 粉色 #fe7da8 Agent 名称 + 紫色 #bbafe6 标签 |
| 侧栏选中 | 左侧 2px 黑竖线高亮 |
| 频道名称 | `#` 前缀 + Space Grotesk 粗体 |
| 分割线 | 2px 纯黑实线 |
| 代码块 | 深色背景 + 2px 黑框 + 5px 硬阴影 |
| 标记 Badge | Space Mono 大写 + 2px 黑边 + 2px 硬阴影 |
| 焦点指示器 | 3px #74b9FF outline + offset 2px（键盘导航专用） |
| 按钮禁用 | opacity-50 + pointer-events-none |

### 11.6 实施策略

**阶段 1（v1.1, Week 3-4）— 基础**：
- `globals.css`：实施 `@theme` 块，定义 neubrutalist 设计 Token（按照 frontend-redesign-v2.md v3.0）
- 字体集成：Space Grotesk + Inter + Space Mono（Google Fonts）
- 侧栏 + 频道列表 + DM 列表：2px 黑分割线、奶油白背景
- 消息列表 + 输入框：Inter 正文、2px 黑边、硬阴影
- 按钮 + 卡片 + Badge 基础组件：2px 黑边 + 5px 硬阴影 + lift/press 动效
- `prefers-reduced-motion` 媒体查询
- 全局 focus-visible 3px #74b9FF 蓝色 outline

**阶段 2（v1.2）— Agent 相关样式**：
- Agent 消息样式 + 粉色 Agent 名称 + 紫色 Agent 标签
- Agent 管理页面统一 neubrutalist 风格（Agent 卡片 2px 黑边 + 5px 阴影）
- 流式输出动画
- 代码块 neubrutalist 暗色主题 + 2px 黑框 + 5px 阴影

**阶段 3（v1.3）— 打磨**：
- Markdown 渲染器升级（代码块行号、复制按钮、语法高亮）
- 消息入场/列表动画（lift/press 模式统一）
- 模态框/对话框统一（3px 厚边框 + 10px 硬阴影）
- Toast/通知组件（4 种语义色）
- 自定义表单控件（checkbox/radio/toggle/select）
- 空状态/加载态/错误态统一样式
- 响应式阴影缩放（移动端缩小偏移量）

---

## 12. 产品优先级总排序

### P0（必须修复/交付 — v1.1/v1.2）

| ID | 功能/修复 | 原因 | 预计工作量 |
|----|---------|------|-----------|
| K-01 | Agent system prompt 隔离 | 核心功能不可用，属于 bug | M |
| K-02 | Agent 工作目录隔离 | 多 Agent 场景数据安全 | L |
| F-12 | 消息编辑/删除 | MVP 遗留的基础功能 | M |
| K-05 | Agent 记忆系统 | 核心差距：slock.ai 的核心卖点 | L |

### P1（重要 — v1.1/v1.2/v1.3）

| ID | 功能/修复 | 目标版本 | 预计工作量 |
|----|---------|---------|-----------|
| K-03 | Local Provider 真流式输出 | v1.1 | M |
| K-04 | 前端 UI 风格改造（阶段 1） | v1.1 | XL |
| K-06 | Agent 工具/技能系统（内置工具集） | v1.2 | XL |
| K-07 | 本地 Agent 交互模式（文件操作） | v1.2 | L |
| F-15 | Agent 工具系统（tool executor） | v1.2 | L |
| F-13 | 多模型 UI 切换 | v1.1 | M |
| F-26 | **轻量级任务系统（Task System）** [新增] | v1.2 | M |
| F-19 | 文件上传 | v1.3 | L |
| F-18 | 消息搜索 | v1.3 | M |
| F-14 | 多 Agent 协作协议 | v1.3 | L |
| F-10 | 线程通知改进 | v1.3 | M |
| F-OB | Onboarding 引导 | v1.3 | M |
| F-KB | 快捷键支持 | v1.3 | S |
| F-RE | 表情/反应 | v1.3 | M |

### P2（有价值 — v1.3/v2.0）

| ID | 功能 | 目标版本 | 预计工作量 |
|----|------|---------|-----------|
| F-17 | 用量统计和费用 | v1.3 | M |
| F-02 | 频道分类/归档 | v1.3 | S |
| F-20 | DM 群组（3-5人） | v1.3 | M |
| F-22 | 审批流 | v2.0 | L |
| F-23 | 定时任务 | v2.0 | M |
| F-24 | 插件系统（MCP） | v2.0 | XL |
| F-21 | Agent 市场 | v2.0 | L |
| F-25 | Agent 记忆/RAG | v2.0 | L |
| - | OAuth/SSO 登录 | v2.0 | M |
| - | 团队展示页（Agent 角色/头像） | v2.0 | S |

### P3（远期 — 不排期）

| ID | 功能 | 预计工作量 | 备注 |
|----|------|-----------|------|
| - | 移动端适配 | XL | 依赖响应式布局重构 |
| - | 多租户/组织管理 | XXL | 需要多用户权限系统 |
| - | 跨 Server 广播（Redis） | L | 多实例场景才需要 |
| - | 联邦 Agent（跨实例协作） | XXL | 长期研究方向 |

---

## 13. 范围边界（本次路线图不涵盖）

### v2.0 之前不做

以下功能明确**不纳入 v1.1-v1.3 范围**，v2.0 再评估：

- 审批流（Agent 敏感操作需人工审批）
- 定时任务（Agent 按 cron 执行）
- 插件系统（MCP 协议支持）
- Agent 市场（预配置模板）
- OAuth/SSO 登录
- 多租户/组织管理
- 移动端适配
- 联邦 Agent 协作

### 技术债务暂不处理

以下技术债务已知但不在本次路线图中优先修复：

- 跨 Server 实例的 WebSocket 广播（Redis Pub/Sub）
- Daemon 多实例负载均衡
- 全面的 E2E 测试覆盖
- 迁移到 Go 1.23 原生功能
- 前端 bundle 体积优化

---

## 14. 附录：关键决策记录

### ADR-20260511-001: Agent 记忆使用文件而非数据库

**背景**：Agent 记忆需要持久化，有两种方案：存入 PostgreSQL 或使用文件系统。

**决策**：使用文件系统（`~/.solo/agents/<id>/MEMORY.md`）。
- 理由：Agent Daemon 已有 DB 访问能力，但记忆内容本质上是文本 Markdown，存入 DB 需要额外的序列化/反序列化。文件系统更直接，且对 local provider 的 Claude Code CLI 更友好（CLI 可以直接读取目录中的文件）。
- 后果：记忆数据不在 Server 的 DB 中，无法通过 REST API 直接搜索。Agent 管理页需要额外 API 来读取/写入记忆文件。

**备选方案**：存入 PostgreSQL（`agent_memories` 表）。更标准化但增加了复杂度。

### ADR-20260511-002: Workspace 路径在 Daemon 所在机器

**背景**：Agent 工作目录应该放在哪里——Daemon 本地还是共享存储？

**决策**：放在 Daemon 本地 `~/.solo/agents/<id>/`。
- 理由：当前 MVP 阶段只有单 Daemon 实例，本地文件系统延迟最低、实现最简单。
- 后果：如果未来引入多 Daemon 实例，需要同步工作目录或使用共享存储（NFS/S3）。

### ADR-20260511-003: Agent 工具系统内置优先

**背景**：工具系统应该用 MCP 协议还是内置实现？

**决策**：v1.2 先用内置工具实现（read/write/list/search），v2.0 再引入 MCP 插件系统。
- 理由：MVP 阶段需要快速验证工具的使用场景，内置工具开发周期短（2-3 周），实现简单。MCP 协议支持是长期方向。
- 后果：v2.0 需要将内置工具迁移到 MCP 架构中，可能涉及重构。

---

## 附录 A：版本时间线汇总

```
版本     时间       核心交付                                   工作量估计
─────   ─────     ─────────────────────────────────           ──────────
v1.1    Week 1-4   Agent 隔离 + 消息编辑删除 + UI 改造           280-320h
v1.2    Week 5-10  Agent Memory + 工具 + Workspace + 任务系统    460-520h
v1.3    Week 11-16 文件上传 + 搜索 + 多 Agent 协作 + 任务增强     360-400h
v2.0    Week 17-24 审批流 + 定时 + 插件 + 市场 + OAuth           480-520h
                                                         ─────────────────
                   总计 (24周)                                   1580-1760h
```

> **注**：v1.2 工作量从 360-400h 上调至 460-520h，主要增量来自新增的轻量级任务系统（F-26, ~80-100h）。
> v1.3 新增 Onboarding 引导和快捷键（~20-40h）。

> 注：以上估计基于 6 人团队（rd1+rd2+fe1+fe2+qa1+arc 共 160h/周）。
> v1.1 至 v2.0 共计 24 周（约 6 个月）。

## 附录 B：产出物验收清单

每个版本交付时需通过以下验收：

- [ ] 所有 P0 问题已修复并通过验收标准
- [ ] 所有 P1 功能已交付并可通过演示
- [ ] 回归测试通过（无 regression bug）
- [ ] 无 blocking/critical 级别的开放 Bug
- [ ] 前端所有页面状态覆盖（加载/空/错误/边缘情况）
- [ ] 文档更新（API 文档、用户手册、部署文档）

---

## 文档结束

> 本路线图与 PRD.md v1.1、ARCHITECTURE.md v1.2、TASKS.md v1.0 对齐。
> 各版本的详细任务分解将在开启对应迭代时更新至 TASKS.md。

## 附录 C：slock.ai 实地体验方法论

本文件中的 slock.ai 竞品分析基于以下维基-方法论：

### 方法一：Landing Page 全量分析

通过 Playwright (headless Chromium) 自动访问 `https://slock.ai`，提取：
- 全量页面文案（hero、features、testimonials、team、pricing、footer）
- CSS class 体系（设计系统反向工程）
- 页面结构和布局

### 方法二：JS Bundle 逆向分析

通过 Playwright 加载 `https://app.slock.ai/login`，捕获主 JS 文件（`/assets/index-udNNoUrh.js`），提取：
- **URL 路由**：通过正则 `["']\/[a-z][^"']*["']` 匹配所有路由字符串
- **组件结构**：搜索 TypeScript 组件文件引用（`Sidebar.tsx`、`ChatPanel.tsx` 等）
- **API 端点**：匹配 `/api/*` 模式的路由引用
- **功能特征**：搜索 keyword pattern（agent、channel、task、inbox、saved、thread 等）
- **状态/数据模型**：推断 store structure（pinnedChannelIds、hiddenDmIds 等）

### 方法三：产业链评测参考

通过 WebSearch 获取 2026 年 5 月第三方评测（硅星人、腾讯新闻、网易订阅），提取：
- 产品市场定位和竞品格局
- 正面评价和负面批评
- 用户使用场景和反馈

### 局限性

1. **未登录后体验**：由于无法获得 app.slock.ai 的有效登录会话，未能探索登录后的 UI 界面、交互流程和权限系统
2. **JS bundle 推断**：部分功能推断基于 JS 路径/组件名/路由名，可能存在误判（例如状态映射、功能具体实现）
3. **时效性**：slock.ai 处于快速迭代中，本分析反映的是 2026-05-11 的产品状态，可能与最新版本有出入
4. **定价不可靠**：Hobby 版标为"限时无限量"（Unlimited computers/agents/channels），可能随时调整

> **后续建议**：Solo 在 v1.3 完成前应进行一次正式的 slock.ai 注册体验（通过 GitHub Google OAuth），以验证本分析中的功能推断。
