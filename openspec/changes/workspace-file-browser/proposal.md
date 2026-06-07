# Proposal: Workspace File Browser

## Problem

当前 workspace 功能存在以下问题：

1. **入口隐藏**：文件浏览功能仅存在于 AgentDetailPanel slide-in 的 "Workspace" tab 中，用户难以发现。`/teams` 页面需要先选中某个 agent 才能看到 workspace，没有独立的访问路径。

2. **代码预览无语法高亮**：非 MD 文件使用纯 `<pre>` 标签展示，无语法高亮，代码可读性差。

3. **架构缺陷（Server-Daemon 文件访问断层）**：Server 的 workspace API 读取的是 Server 本地 `~/.solo/agents/<id>/workspace/`，而实际 agent workspace 文件创建在 Daemon 机器上。Server 和 Daemon 在同一台机器时无问题，**分布式部署时 workspace 将为空**。

4. **缺少文件操作**：当前为只读浏览，不支持文件创建、编辑、删除、重命名。

5. **缺少搜索**：无 workspace 内文件搜索能力。

6. **无实时更新**：Agent 执行过程中产生的文件变更不会自动反映到 UI，需手动刷新。

## Goals

- 创建独立的 `/workspace` 路由页面，用户可直达文件浏览
- 代码文件预览支持语法高亮（Shiki）
- Markdown 文件预览体验对标 GitHub（GFM + Mermaid 图表 + 代码高亮）
- 修复 Server-Daemon 文件访问断层，确保分布式场景下可正常浏览文件
- 目录树 + 文件预览的经典双栏布局
- 支持文件基础操作（创建、重命名、删除）

## Non-Goals

- 不实现完整在线 IDE（不需要 Monaco Editor，不需要 IntelliSense）
- 不实现拖拽移动文件（一期不做）
- 不实现 Git 集成
- 不实现多文件 Tab 切换（一期单文件预览）

## Scope

| 模块 | 范围 |
|------|------|
| 前端路由 | 新增 `/workspace` 页面 |
| 前端组件 | 重构 FileTree、FilePreview 为独立组件；新增语法高亮 |
| 后端 API | 新增 Daemon 端 workspace 代理端点；Server 端转发 |
| 文件操作 | 新增创建/重命名/删除 文件/目录 API |
