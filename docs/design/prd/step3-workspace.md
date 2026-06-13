# PRD — Step 3: Workspace（多层工作区）

> 版本: v1.0 | 日期: 2026-06-13 | 状态: Draft
> 前置依赖: [Step 1 — Foundation](step1-foundation.md) · [Step 2 — Coordination](step2-coordination.md)
> 关联文档: [Agent 协作路线图](../agent-collaboration-roadmap.md)

---

## 1. 产品背景与目标

### 1.1 现状（Step 2 完成后）

Agent 们可以正式委托任务，看得到依赖关系，频道内共享记忆也在积累。但所有 Agent 都工作在**自己的个人工作区**（`~/.solo/agents/<id>/workspace/`）里。当一个频道有多名 Agent 协作同一个项目时，每个人的工作区是隔离的——代码不共享、文件不同步、修改互相不可见。

### 1.2 目标

建立**三层工作区**模型，让 Agent 的协作从"消息层面"升级到"代码层面"：

1. **频道共享工作区**：频道绑定 Git 仓库，所有 Agent 共享同一份代码
2. **项目绑定**：管理员将频道与仓库关联，Agent 加入频道自动获得代码访问权
3. **Git Worktree 任务隔离**：并行任务需要隔离时，自动创建临时 worktree，完成后合并清理

### 1.3 Step 3 完成后的协作体验

```
频道 #frontend 绑定项目 https://github.com/org/webapp 后:

1. Backend Agent 加入频道 #frontend
   -> 自动获得共享工作区访问权
   -> ~/.solo/channels/<id>/workspace/repo/ 已有 checkout 的代码

2. Backend Agent 在频道内执行命令
   -> 自动使用共享 repo 作为工作目录
   -> 修改的文件对其他 Agent 可见

3. Frontend Agent 同时开始 Task #5（修改 CSS）
   -> 检测到 Backend Agent 也在修改 JS 文件
   -> 系统建议任务隔离
   -> solo task isolate -n 5
   -> 自动创建 worktree: ~/.solo/agents/<id>/worktrees/task-5/
   -> 两个 Agent 各自在隔离环境中工作，互不污染

4. Frontend Agent 完成任务
   -> git commit + push
   -> worktree 自动合并到共享工作区
   -> 临时 worktree 被清理
```

---

## 2. 用户故事地图

### 2.1 频道共享工作区

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| W1 | P0 | 作为频道管理员，我希望将频道绑定到一个 Git 项目仓库，以便所有 Agent 共享同一份代码 | M |
| W2 | P0 | 作为 Agent，加入频道后自动获得共享工作区访问权，以便无需手动 clone 项目 | M |
| W3 | P0 | 作为 Agent，在频道内执行 shell 命令时自动使用共享 repo 作为工作目录，以便上下文切换零成本 | M |
| W4 | P1 | 作为 Agent，我希望共享工作区支持常见的 Git 操作（pull/commit/push），以便正常开发流程不受影响 | M |

### 2.2 项目绑定管理

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| W5 | P0 | 作为频道管理员，我希望在频道设置中配置绑定的仓库 URL 和分支，以便灵活切换项目 | S |
| W6 | P1 | 作为管理员，我希望解绑或更换绑定的仓库，以便频道可以切换项目 | S |
| W7 | P2 | 作为 Agent，我希望查看频道当前绑定的仓库信息，以便了解项目上下文 | S |

### 2.3 Git Worktree 任务隔离

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| W8 | P1 | 作为 Agent，我希望请求为某个任务创建隔离的 worktree，以便不被其他 Agent 的文件修改干扰 | L |
| W9 | P1 | 作为系统，当检测到多 Agent 同时修改同文件时，建议进行任务隔离，以便避免冲突 | M |
| W10 | P1 | 作为 Agent，任务完成后自动将 worktree 的修改合并回共享工作区，以便结果被团队共享 | M |
| W11 | P2 | 作为系统，任务完成或取消后自动清理临时 worktree，以便不浪费磁盘空间 | S |

---

## 3. 功能详述

### 3.1 三层工作区模型

```
Agent 级（已有）
  ~/.solo/agents/<agent-id>/workspace/
    ├── notes/          # 个人笔记
    ├── memory/         # 个人记忆
    └── projects/       # 个人项目

频道共享（新增）
  ~/.solo/channels/<channel-id>/workspace/
    ├── repo/           # git clone 的工作副本
    ├── .solo/
    │   └── channel-context.md
    └── shared/         # 频道内共享文件

任务隔离（新增，按需）
  ~/.solo/agents/<agent-id>/worktrees/task-<n>/
    └── (git worktree of repo/)
```

### 3.2 频道共享工作区

#### 绑定流程

```
管理员操作:
  solo channel bind-project --channel '#frontend' --repo 'https://github.com/org/webapp' --branch main

系统执行:
  1. 校验仓库 URL 可访问（git ls-remote）
  2. 在 ~/.solo/channels/<id>/workspace/repo/ 执行 git clone
  3. 保存绑定信息到频道配置
  4. 通知所有频道成员 "频道 #frontend 已绑定项目 webapp (main)"

Agent 加入频道时:
  1. 检测频道已绑定项目
  2. 在 Agent 的 shell executor 中注册共享工作区路径
  3. System Prompt 注入工作区信息
```

#### 工作目录切换规则

Agent 在频道内执行命令时，工作目录自动切换为共享 repo：

```
# 在 #frontend 频道内
$ solo exec "ls"                    -> 在 ~/.solo/channels/<id>/workspace/repo/ 执行
$ solo exec "cat package.json"      -> 同上
$ solo exec "git status"            -> 同上
$ solo exec "cd /tmp && ls"         -> 仍可用绝对路径跳出
```

Agent 在自己的 Agent 级空间执行命令时，不走共享工作区：
```
# Agent 级上下文（非频道内）
$ solo exec "ls"                    -> 在 ~/.solo/agents/<id>/workspace/ 执行
```

#### 业务规则

- 一个频道同一时间只能绑定一个仓库
- 绑定仓库时如果共享工作区已有 clone，询问是否覆盖（`--force`）
- Agent 离开频道后不再有共享工作区访问权
- 共享工作区的 Git 身份使用频道的默认 Git 配置（可配置 `solo channel git-config`）
- 共享工作区只维护一个分支（绑定时的默认分支），Agent 应通过 worktree 在其他分支工作

### 3.3 项目绑定管理

#### CLI

```bash
# 绑定项目
solo channel bind-project --channel '#frontend' --repo 'https://github.com/org/webapp'
solo channel bind-project --channel '#frontend' --repo 'git@github.com:org/webapp.git' --branch develop

# 查看绑定
solo channel info '#frontend'
  # 输出包含:
  #   Project: https://github.com/org/webapp (main)
  #   Workspace: ~/.solo/channels/xxx/workspace/repo/
  #   Last updated: 2026-06-13 14:30

# 解绑
solo channel unbind-project --channel '#frontend'
  # 警告：共享工作区将被删除。确认？(y/N)
  # --keep-workspace 保留本地文件但解除绑定

# 更换绑定
solo channel bind-project --channel '#frontend' --repo 'https://github.com/org/new-project' --force
  # 强制覆盖现有绑定，旧工作区归档到 workspace.archive/
```

#### 前端 — 频道设置页

```
┌─────────────────────────────────────────────────────────┐
│ 频道设置: #frontend                            [返回]   │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ★ 基本信息                                             │
│  ┌─────────────────────────────────────────────────┐   │
│  │ 名称: frontend                         [修改]    │   │
│  │ 描述: 前端团队协作频道                  [修改]    │   │
│  └─────────────────────────────────────────────────┘   │
│                                                         │
│  ★ 项目绑定                          <-- 新增 section  │
│  ┌─────────────────────────────────────────────────┐   │
│  │ 仓库: https://github.com/org/webapp              │   │
│  │ 分支: main                                       │   │
│  │ 工作区: ~/.solo/channels/.../workspace/repo/     │   │
│  │ 最后更新: 2026-06-13 14:30                       │   │
│  │                                                   │   │
│  │ [更换仓库] [解绑]                                 │   │
│  └─────────────────────────────────────────────────┘   │
│                                                         │
│  ★ 成员管理                                             │
│  ...                                                    │
└─────────────────────────────────────────────────────────┘
```

### 3.4 Git Worktree 任务隔离

#### 触发方式

**方式 1：Agent 主动请求**
```bash
solo task isolate -n 5
# 为 Task #5 创建隔离 worktree
# 输出: Worktree created at ~/.solo/agents/<id>/worktrees/task-5/
```

**方式 2：系统检测建议（冲突检测）**
```
检测条件:
  - 两个 Agent 同时 claim 了同一频道的任务
  - 两个任务都涉及对共享工作区的文件修改
  - 两个 Agent 可能修改相同的文件路径

触发时:
  - 向两个 Agent 发送提示: "检测到 @Bob 也在修改 src/auth/login.go，建议使用 solo task isolate 隔离工作区"
  - 不强制，Agent 可忽略
```

#### 生命周期

```
1. 创建隔离
   $ solo task isolate -n 5
   -> 在频道共享 repo 上创建 git worktree
   -> 路径: ~/.solo/agents/<agent-id>/worktrees/task-5/
   -> Agent 后续在该任务上下文中的命令自动使用此 worktree

2. 工作中
   -> Agent 在 worktree 中自由修改、commit
   -> 工作目录自动指向 worktree 而非共享 repo

3. 完成合并
   $ solo task complete -n 5
   -> 自动 git push worktree 的修改到远程
   -> 共享 repo 执行 git pull 同步
   -> 清理 worktree: git worktree remove + rm -rf
   -> 任务上下文恢复为共享 repo

4. 取消废弃
   $ solo task cancel -n 5
   -> 丢弃 worktree 的修改（git worktree remove --force）
   -> 清理目录
   -> 任务上下文恢复为共享 repo
```

#### 隔离模式下 Agent 的工作目录

```
Task #5 (隔离模式):
  $ solo exec "pwd"
  -> /home/solo/.solo/agents/backend-bot/worktrees/task-5/

  $ solo exec "git branch"
  -> * task-5  (自动创建的分支)

Task #7 (无隔离):
  $ solo exec "pwd"
  -> /home/solo/.solo/channels/frontend/workspace/repo/
```

#### 业务规则

- worktree 只从频道共享 repo 创建（不从个人工作区创建）
- 一个 Agent 同一时间可有多个活跃 worktree（每个对应一个 isolated task）
- worktree 分支命名规则：`solo/task-<n>/<agent-slug>`
- 任务完成后 worktree 的提交 squash 合并到主分支（避免污染 Git 历史）
- 若 worktree 中有未提交的修改，任务完成时自动 `git stash` + 提示 Agent
- 磁盘限额：单个 Agent 最多 5 个并发 worktree，超出时提示先完成或取消

---

## 4. 验收标准

### 4.1 频道共享工作区

**W1 — 绑定项目仓库**

```
GIVEN 频道 #frontend 存在且无绑定
WHEN 管理员执行
  solo channel bind-project --channel '#frontend' --repo 'https://github.com/org/webapp'
THEN 终端输出 "Cloning https://github.com/org/webapp..."
  AND clone 完成后输出 "频道 #frontend 已绑定项目 webapp (main)"
  AND ~/.solo/channels/<id>/workspace/repo/ 包含完整仓库
  AND 所有频道成员收到通知
```

**W1b — 绑定已存在的频道**

```
GIVEN 频道 #frontend 已绑定 webapp
WHEN 管理员执行 bind-project --repo 'https://github.com/org/other'
THEN 终端输出 "频道已绑定 webapp，使用 --force 覆盖"
  AND 退出码非 0
```

**W2 — Agent 加入频道自动获得访问权**

```
GIVEN 频道 #frontend 已绑定 webapp，共享 repo 已 clone
WHEN 新 Agent @bob 加入频道 #frontend
THEN Bob 获得共享工作区 ~/.solo/channels/<id>/workspace/repo/ 的读取权限
  AND Bob 的 System Prompt 包含工作区路径信息
  AND Bob 在频道内执行命令时默认工作目录为共享 repo
```

**W3 — 频道内命令使用共享工作区**

```
GIVEN Agent @bob 是频道 #frontend 的成员，共享工作区存在
WHEN Bob 在频道上下文中执行 solo exec "ls"
THEN 命令在 ~/.solo/channels/<id>/workspace/repo/ 执行
  AND 输出为共享 repo 的文件列表
```

**W4 — Git 操作**

```
GIVEN Agent @bob 在频道 #frontend 共享工作区中修改了文件
WHEN Bob 执行 git add + git commit + git push
THEN 修改被推送到远程仓库
  AND 其他 Agent 可通过 git pull 获取更新
```

### 4.2 项目绑定管理

**W5 — 配置绑定参数**

```
GIVEN 频道 #frontend 未绑定
WHEN 管理员执行
  solo channel bind-project --channel '#frontend' --repo 'git@github.com:org/webapp.git' --branch develop
THEN 共享工作区 checkout 到 develop 分支
  AND 频道配置保存 repo URL 和 branch
```

**W6 — 解绑**

```
GIVEN 频道 #frontend 已绑定 webapp
WHEN 管理员执行 solo channel unbind-project --channel '#frontend'
THEN 终端提示 "共享工作区将被删除。确认？(y/N)"
WHEN 管理员输入 y
THEN 共享工作区目录被删除
  AND 频道配置中移除绑定信息
  AND 频道成员收到通知 "频道 #frontend 已解除项目绑定"
```

**W7 — 查看绑定信息**

```
GIVEN 频道 #frontend 已绑定 webapp
WHEN 管理员执行 solo channel info '#frontend'
THEN 输出包含:
  - Project: https://github.com/org/webapp (main)
  - Workspace: ~/.solo/channels/<id>/workspace/repo/
```

### 4.3 Git Worktree 任务隔离

**W8 — 主动创建隔离 worktree**

```
GIVEN Task #5 属于频道 #frontend（已绑定 repo）
WHEN Agent 执行 solo task isolate -n 5
THEN 系统在共享 repo 上创建 git worktree
  AND worktree 位于 ~/.solo/agents/<agent-id>/worktrees/task-5/
  AND 分支名为 solo/task-5/<agent-slug>
  AND 终端输出 "Worktree created. 后续命令将在隔离环境中执行。"
```

**W9 — 冲突检测建议**

```
GIVEN Task #3 (Agent @alice) 和 Task #5 (Agent @bob) 都涉及文件 src/auth/login.go
WHEN Agent @bob 开始 Task #5
THEN Bob 收到提示 "检测到 @alice (Task #3) 也在修改 src/auth/login.go，建议 solo task isolate -n 5"
```

**W10 — 完成合并**

```
GIVEN Task #5 处于隔离模式，worktree 中有 2 个新 commit
WHEN Agent 执行 solo task complete -n 5
THEN worktree 的修改被 git push 到远程
  AND 共享 repo 自动 git pull 更新
  AND worktree 被 git worktree remove 清理
  AND 目录被删除
  AND Agent 工作目录恢复为共享 repo
  AND 终端输出 "Task #5 完成，修改已合并，worktree 已清理。"
```

**W11 — 自动清理**

```
GIVEN Task #5 被取消（solo task cancel -n 5）
WHEN 任务状态变为 cancelled
THEN worktree 修改被丢弃
  AND worktree 被 git worktree remove --force
  AND 目录被删除
  AND 终端输出 "Task #5 已取消，worktree 修改已丢弃。"
```

**W11b — 并发限制**

```
GIVEN Agent 已有 5 个活跃 worktree
WHEN Agent 尝试 solo task isolate -n 6
THEN 终端输出 "已达到最大 worktree 数量 (5)，请先完成或取消现有隔离任务"
  AND 退出码非 0
```

---

## 5. UI/UX 草图

### 5.1 频道设置 — 项目绑定 section

```
┌─────────────────────────────────────────────────────────┐
│ 频道设置: #frontend                                     │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ★ 项目绑定                                             │
│  ┌─────────────────────────────────────────────────┐   │
│  │                                                  │   │
│  │  📦 仓库                                        │   │
│  │  ┌──────────────────────────────────────────┐   │   │
│  │  │ https://github.com/org/webapp      [修改] │   │   │
│  │  └──────────────────────────────────────────┘   │   │
│  │                                                  │   │
│  │  🌿 分支                                        │   │
│  │  ┌──────────────────────────────────────────┐   │   │
│  │  │ main                                 [▾] │   │   │
│  │  └──────────────────────────────────────────┘   │   │
│  │                                                  │   │
│  │  📂 工作区路径                                   │   │
│  │  ~/.solo/channels/a1b2c3/workspace/repo/        │   │
│  │                                                  │   │
│  │  ⏱️ 最后更新: 2026-06-13 14:30:00              │   │
│  │                                                  │   │
│  │  [更换仓库]  [解绑]                              │   │
│  └─────────────────────────────────────────────────┘   │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### 5.2 Agent 任务执行上下文指示器

```
┌─────────────────────────────────────────┐
│ 🖥️ 当前执行上下文                       │
├─────────────────────────────────────────┤
│ 频道: #frontend                         │
│ 任务: #5 修改登录页面样式                │
│ 模式: 🟡 隔离 (worktree)                 │
│ 路径: ~/.solo/agents/bob/worktrees/task-5│
│                                         │
│ [切换任务] [退出隔离]                    │
└─────────────────────────────────────────┘
```

### 5.3 频道详情页 — 工作区状态卡片

```
┌─────────────────────────────────────────────────────────┐
│ 频道 #frontend                          [设置] [成员]   │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌─── 工作区状态 ──────────────────────────────────┐   │
│  │                                                  │   │
│  │  📦 webapp (main)  🟢 同步                       │   │
│  │  ⏱️ 最后更新: 5 分钟前                           │   │
│  │                                                  │   │
│  │  活跃 Agent:                                     │   │
│  │  🟢 @alice — Task #3 (共享工作区)                │   │
│  │  🟢 @bob   — Task #5 (🟡 隔离 worktree)          │   │
│  │  ⚫ @carol — 无活跃任务                          │   │
│  │                                                  │   │
│  │  [查看仓库] [强制同步]                            │   │
│  └──────────────────────────────────────────────────┘   │
│                                                         │
│  ┌─── 共享记忆 ────────────────────────────────────┐   │
│  │ CHANNEL.md (最后修改: @alice, 2 小时前)          │   │
│  │ decisions.md (3 条决策记录)                      │   │
│  └──────────────────────────────────────────────────┘   │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

---

## 6. 非功能需求

### 6.1 性能

| 指标 | 目标 |
|------|------|
| git clone 大型仓库 (100MB) | < 30s（取决于网络） |
| git worktree 创建 | < 2s（本地操作） |
| 工作目录切换（共享 <-> 隔离） | < 100ms（只需更新环境变量） |
| 冲突检测扫描 | < 500ms（只扫描当前活跃任务的涉及文件） |

### 6.2 安全性

- 频道共享工作区中的 Git 凭证从系统级 Git credential helper 获取，不在 Solo 中存储
- Agent 对共享工作区的写入需校验 Agent 是频道成员
- worktree 隔离后，Agent 只能在自己的 worktree 中操作，不可读写其他 Agent 的 worktree
- 频道解绑时共享工作区默认删除，防止敏感代码泄露

### 6.3 可靠性

- 共享工作区的 Git 操作失败时（如冲突），不阻塞 Agent 工作，提示 Agent 手动解决
- worktree 清理失败时不影响任务完成状态，仅记录 warning 日志
- 频道删除时级联清理共享工作区和工作区内 worktree

---

## 7. 范围边界

### 本次做

- 频道共享工作区（绑定 repo + git clone + Agent 自动访问）
- 频道内命令自动定向到共享工作区
- 项目绑定 CLI（bind/unbind/info）
- Git Worktree 任务隔离（主动创建 + 完成合并 + 取消清理）
- 冲突检测建议（多 Agent 同文件）
- 工作区上下文指示器（前端）
- 并发 worktree 数量限制

### 本次不做

- 频道共享工作区的 Web IDE（不在此路线图中）
- 自动 rebase / 自动解决 Git 冲突
- 跨频道的共享工作区（一个频道一个项目）
- 在 worktree 中运行测试套件
- CI/CD 集成
- 项目模板脚手架

---

## 8. 依赖与风险

### 依赖

| 依赖项 | 说明 | 风险等级 |
|--------|------|----------|
| 频道系统 | 频道创建、成员管理必须可用 | 低 — 已有 |
| Git | 服务器需安装 Git 2.30+ | 低 — 标准化 |
| Agent shell executor | Agent 执行命令需支持动态工作目录 | 中 — 需改造现有 executor |
| 文件系统权限 | Agent 进程需对 ~/.solo/channels/ 有读写权限 | 低 |

### 风险

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 多 Agent 在共享工作区同时 commit 导致 Git 冲突 | 工作阻塞 | 引导高频冲突场景使用 worktree 隔离；冲突时提供清晰的解决指引 |
| 大仓库 clone 耗时长、占磁盘 | 频道初始化体验差 | 支持 shallow clone（--depth 1）；首次绑定提示预估空间 |
| worktree 清理失败残留大量临时目录 | 磁盘耗尽 | 定时任务清理 7 天前的孤立 worktree；磁盘使用率监控 |
| Agent 在共享工作区执行危险命令（如 rm -rf） | 代码丢失 | 共享工作区只允许 Git 操作和文件编辑，禁止 rm -rf / git push --force / git reset --hard 等 |
| 项目绑定凭证泄露 | 安全问题 | 不在 Solo 配置中存储 Git 凭证，走系统 credential helper |

---

## 9. 成功指标

### 上线验证

- [ ] 频道绑定项目 repo 后正确 clone 到共享工作区
- [ ] Agent 加入频道后自动获得共享工作区访问权
- [ ] 频道内命令自动使用共享工作区路径
- [ ] solo task isolate 创建 worktree + 完成合并流程通过
- [ ] worktree 完成后正确清理，不残留
- [ ] 解绑频道后共享工作区正确删除
- [ ] 5 个并发 worktree 限制生效
- [ ] 冲突检测在 2 Agent 同文件场景下正确触发

### 30 天观察指标

| 指标 | 目标 | 数据来源 |
|------|------|----------|
| 绑定项目的频道占比 | >= 30% | 频道配置 |
| worktree 隔离使用次数 | >= 10 / 周 | task isolation 日志 |
| 共享工作区 Git 操作成功率 | >= 95% | Git 操作日志 |
| worktree 清理成功率 | >= 98% | 清理日志 |
| 因冲突导致的 Agent 阻塞次数 | < 5 / 周 | 用户反馈 + 日志 |

---

## 10. 迭代规划

### Sprint W-1（Week 1-2）：频道共享工作区

- [ ] 实现频道项目绑定数据模型（channel config 扩展）
- [ ] 实现 `solo channel bind-project` CLI
- [ ] 实现 git clone 到共享工作区路径
- [ ] 实现 Agent 加入频道时的共享工作区访问注册
- [ ] 改造 Agent shell executor 支持频道上下文动态工作目录
- [ ] 实现频道内共享 Git 操作（pull/commit/push）

### Sprint W-2（Week 2-3）：Git Worktree 任务隔离

- [ ] 实现 `solo task isolate` CLI（创建 worktree）
- [ ] 实现任务完成时 worktree merge + cleanup
- [ ] 实现任务取消时 worktree discard + cleanup
- [ ] 实现冲突检测（多 Agent 同文件）
- [ ] 实现并发 worktree 限制
- [ ] 实现孤 worktree 定时清理

### Sprint W-3（Week 3）：前端 + 集成测试

- [ ] 频道设置页新增项目绑定 section
- [ ] Agent 任务上下文指示器
- [ ] 频道详情页工作区状态卡片
- [ ] 端到端测试 + 手动验收
