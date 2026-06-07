# Tasks: Workspace File Browser

## Phase 1: Daemon 端 Workspace 代理端点

- [x] **1.1** 在 Daemon 路由中注册 `/internal/daemon/workspace/*` 端点组
  - 路径：`cmd/daemon/main.go`
- [x] **1.2** 实现 `GET /list?agent_id=&path=` — 返回 agent workspace 文件树 JSON
  - 路径：`cmd/daemon/handler.go`，复用 `WorkspaceManager.WorkspacePath()`
  - 路径安全校验（防 directory traversal）
  - 隐藏文件过滤
  - 最大深度 20，文件大小上限 1MB
- [x] **1.3** 实现 `GET /read?agent_id=&path=` — 返回单个文件内容
  - 二进制文件检测（null byte），返回 `[binary file]`
  - 文件大小上限 1MB

## Phase 2: Server 端代理转发

- [x] **2.1** 在 `DaemonManager` 中新增 `ProxyWorkspace()` 方法，将 workspace 请求转发到对应 daemon
  - 路径：`internal/server/service/daemon.go`
- [x] **2.2** 修改 `AgentHandler.Workspace()` 方法，通过 DaemonManager 代理而非读取本地文件系统
  - 路径：`internal/server/handler/agent.go`
- [x] **2.3** 向后兼容：daemon 不支持 workspace 端点时，回退到 Server 本地文件系统读取（现有逻辑）

## Phase 3: 前端独立 Workspace 页面

- [x] **3.1** 创建 `/workspace` 路由页面
  - 路径：`frontend/app/workspace/page.tsx`
  - 支持 `?agent=<id>` URL 参数
- [x] **3.2** 在侧边栏导航添加 "Workspace" 入口
  - 路径：`frontend/components/layout/` (sidebar/导航组件)
- [x] **3.3** 实现 AgentSelector 组件（顶部下拉框选择 agent workspace）
- [x] **3.4** 实现空状态页：无 agent 选中 → 引导选择；workspace 为空 → "Agent workspace 尚无文件"
- [x] **3.5** 将 FileTreeNode / FilePreview 从 `agent-workspace-tab.tsx` 抽离为独立组件
  - 新路径：`frontend/components/workspace/file-tree.tsx`, `file-preview.tsx`
- [x] **3.6** AgentDetailPanel 的 Workspace tab 复用抽离后的组件，添加 "在新页面打开" 按钮

## Phase 4: Shiki 语法高亮

- [x] **4.1** 安装 `shiki` 依赖
- [x] **4.2** 实现 `CodePreview` 组件，使用 Shiki 进行语法高亮
  - 基于文件扩展名自动检测语言（223+ 语言支持）
  - 主题：dark-plus（与项目 dark theme 一致）
  - 行号显示
  - 大文件优化：>500 行切分懒渲染
- [x] **4.3** 在 MarkdownPreview 中集成 `@shikijs/rehype`，MD 代码块统一高亮

## Phase 5: 面板交互优化

- [x] **5.1** 可拖拽分隔条（resize 左右面板宽度，默认 260px）
- [x] **5.2** 面包屑导航（每段可点击，触发 navigate/load）
- [x] **5.3** 文件树状态持久化（展开/折叠状态存 localStorage）
- [x] **5.4** 文件列表排序（目录优先，字母排序）
- [x] **5.5** 手动刷新按钮（不自动 polling）
