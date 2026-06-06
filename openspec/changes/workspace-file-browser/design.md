# Design: Workspace File Browser

## Layout Architecture

采用经典双栏布局（对标 VS Code / GitHub 代码视图）：

```
┌─────────────────────────────────────────────────────┐
│  Breadcrumb:  agent-name / workspace / src / App.tsx │
├──────────────┬──────────────────────────────────────┤
│ File Tree    │  File Preview                        │
│ (260px)      │                                      │
│              │  ┌────────────────────────────────┐  │
│ ├── src/     │  │  syntax-highlighted code       │  │
│ │  ├── App   │  │  (Shiki)                       │  │
│ │  └── main  │  │                                │  │
│ ├── docs/    │  │  or MD rendered (ReactMarkdown │  │
│ │  └── API   │  │   + GFM + Mermaid + highlight) │  │
│ └── README   │  └────────────────────────────────┘  │
│              │                                      │
│ resizable ◄──►                                      │
└──────────────┴──────────────────────────────────────┘
```

- **左面板**：可缩放文件树（默认 260px，可拖拽调整）
- **右面板**：文件预览区（MD 渲染 / 代码高亮 / 二进制提示）
- **顶部**：面包屑导航

## Route Design

```
/workspace                     → 空状态页（引导选择 agent）
/workspace?agent=<agentId>     → 按 agent 显示 workspace 文件树
```

入口：在侧边栏导航添加 "Workspace" 菜单项。

## Component Tree

```
WorkspacePage
├── AgentSelector (agent 选择下拉/无 agent 时引导)
├── WorkspaceLayout (flex row, resizable)
│   ├── FileTreePanel (260px, 可缩放)
│   │   ├── FileTreeHeader ("Files" + 操作按钮)
│   │   └── FileTree
│   │       └── FileTreeNode (递归组件, react-arborist 或自研)
│   └── FilePreviewPanel
│       ├── BreadcrumbBar (文件路径导航)
│       ├── FilePreviewToolbar (文件操作: 编辑/下载/复制路径)
│       └── FilePreviewContent
│           ├── MarkdownPreview (.md 文件 → ReactMarkdown)
│           ├── CodePreview (代码文件 → Shiki 语法高亮)
│           ├── ImagePreview (图片文件)
│           └── BinaryPlaceholder (二进制文件)
```

## Code Highlighting — Shiki

选择 **Shiki** 而非 Monaco Editor：

| 维度 | Shiki | Monaco |
|------|-------|--------|
| Bundle | ~80KB (core) / ~600KB (WASM) | ~1.5-2MB |
| 用途 | 只读代码预览 | 完整 IDE 编辑器 |
| 语言支持 | 223+ | 与 VS Code 一致 |
| 主题 | VS Code 主题兼容 | 完整主题系统 |
| 服务端渲染 | 支持 | 不支持 |

使用 `@shikijs/rehype` 集成到 Markdown 渲染管道中，代码块自动高亮。

## Data Flow — 修复 Server-Daemon 断层

### 当前架构（问题）

```
Frontend → Server → Server 本地文件系统
                    ↑ 读取的是 Server 磁盘，不是 Daemon 磁盘
Daemon → Daemon 本地文件系统（agent workspace 实际在这里）
```

### 目标架构

```
Frontend → Server → Daemon Proxy → Daemon 本地文件系统
```

### 新增 Daemon 端点

在 Daemon 端新增 workspace 代理端点：

```
GET  /internal/daemon/workspace/list?path=&agent_id=
     → 返回文件树 JSON（复用现有 workspaceNode 结构）

GET  /internal/daemon/workspace/read?path=&agent_id=
     → 返回文件内容（plain text）

POST /internal/daemon/workspace/write?path=&agent_id=
     → 写入文件内容

DELETE /internal/daemon/workspace/delete?path=&agent_id=
     → 删除文件/目录

POST /internal/daemon/workspace/mkdir?path=&agent_id=
     → 创建目录

POST /internal/daemon/workspace/rename?path=&agent_id=&new_name=
     → 重命名
```

### Server 端改动

- `AgentHandler.Workspace()` 方法改为通过 `DaemonManager` 向对应 Daemon 发送 HTTP 请求
- 根据 agentID 查找 agent 所在的 daemon，获取 daemon URL
- 将 workspace API 请求代理到 daemon 的 `/internal/daemon/workspace/*` 端点
- 新方法：`DaemonManager.ProxyWorkspaceRequest()`

### 向后兼容

- 如果 daemon URL 指向 localhost（同机部署），行为不变
- 如果 daemon 不支持 workspace 端点（旧版本），返回明确错误提示

## File Operations

一期实现基础 CRUD：

| 操作 | 前端交互 | API |
|------|---------|-----|
| 创建文件 | FileTree header "+" 按钮 → 弹出输入框 | `POST /.../write` |
| 创建目录 | FileTree header "New Folder" 按钮 | `POST /.../mkdir` |
| 重命名 | 右键菜单 → Rename → inline 编辑 | `POST /.../rename` |
| 删除 | 右键菜单 → Delete → 确认弹窗 | `DELETE /.../delete` |

## Tech Decisions

| 决策 | 选择 | 理由 |
|------|------|------|
| 代码高亮 | Shiki (WASM) | 轻量，223+ 语言，VS Code 主题兼容 |
| 文件树组件 | 自研递归组件（基于现有 FileTreeNode） | 当前实现已可用，避免引入 react-arborist 依赖 |
| 面板缩放 | CSS `resize` 或自研拖拽分隔条 | 简单实现，无需额外依赖 |
| Markdown 渲染 | react-markdown + remarkGfm + rehypeShiki | 已有 react-markdown，增加 Shiki 插件即可 |
| Daemon 通信 | HTTP REST（非 WebSocket） | 文件浏览为按需操作，不需要实时推送 |

## Open Questions

1. `react-arborist` 是否需要引入？（当前自研 FileTreeNode 在 < 100 文件场景足够）
2. 是否需要文件修改后自动刷新树？（Polling vs WebSocket vs 手动刷新）
3. 面包屑导航是否需要可点击跳转？
