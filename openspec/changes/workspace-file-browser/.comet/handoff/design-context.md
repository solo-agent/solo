# Design Context: workspace-file-browser

## Source: openspec/changes/workspace-file-browser/

### proposal.md Summary
- Problem: workspace frontend empty, entry hidden in agent detail panel tab, no syntax highlighting, Server-Daemon file access gap, no file operations, no search
- Goals: independent /workspace route, Shiki syntax highlighting, fix daemon proxy gap, file CRUD operations
- Non-Goals: no Monaco/online IDE, no drag-drop move, no Git integration, no multi-tab
- Scope: frontend route/page, component refactor, daemon proxy endpoints, file operations

### design.md Summary
- Layout: left file tree (260px) + right preview + breadcrumb
- Route: /workspace?agent=<agentId>
- Code highlighting: Shiki (not Monaco) for lightweight syntax highlighting
- Data flow fix: Frontend → Server → Daemon (HTTP proxy) → Daemon filesystem
- 6 new daemon endpoints: list, read, write, mkdir, delete, rename
- File tree: reuse existing FileTreeNode component
- Markdown: react-markdown + remarkGfm + @shikijs/rehype

### tasks.md Summary
- Phase 1 (7 tasks): Daemon workspace proxy endpoints
- Phase 2 (4 tasks): Server proxy forwarding + backward compat
- Phase 3 (5 tasks): Frontend /workspace page + component extraction
- Phase 4 (3 tasks): Shiki syntax highlighting + MD code block highlight
- Phase 5 (4 tasks): Context menu + file CRUD UI
- Phase 6 (4 tasks): Resizable panels, breadcrumbs, state persistence
