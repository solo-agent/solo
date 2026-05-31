# Solo 文档中心

Solo 项目的全部产品、架构、设计文档，按版本组织。

## 目录结构

```
docs/
├── v0/                          # MVP 时期
│   ├── PRD.md                   # 产品需求文档 v1.1
│   ├── ARCHITECTURE.md          # 技术架构文档 v1.2
│   └── TASKS.md                 # 任务分解与迭代规划
├── v1.1/                        # 基础加固
│   ├── TASKS-v1.1.md            # v1.1 任务分解
│   ├── architecture-review-v2.md # 架构评审 v2 (三方案对比修正)
│   └── qa/
│       └── rc-acceptance-report.md # 发布候选验收报告
├── v1.2/                        # Agent 能力升级
│   ├── TASKS-v1.2.md            # v1.2 任务分解
│   ├── task-system-spec.md      # 任务系统设计文档（Slack-style）
│   ├── v2-optimization-plan.md  # 三方案对比优化方案
│   └── qa/
│       ├── task-system-test-cases.md  # 任务系统测试用例
│       └── task-system-test-report.md # 任务系统测试报告
├── v1.3/                        # 协作与体验补齐（规划中）
│   └── PRD-v1.3.md              # v1.3 产品需求文档
├── design/                      # 设计文档（跨版本）
│   ├── frontend-redesign-v2.md  # Neubrutalism 设计系统 v3.0
│   └── product-roadmap-v2.md    # 产品路线图 v2.0
├── research/                    # 竞品调研（跨版本参考）
│   ├── slock-architecture.md    # Slock.ai 技术架构分析
│   ├── slock-task-design.md     # Slock 任务系统设计分析
│   └── multica-architecture.md  # Multica 技术架构分析
├── screenshots/                 # 功能截图
├── STATUS.md                    # 功能状态矩阵
└── README.md                    # 本文档
```

## 快速导航

| 想了解什么 | 看哪个文档 |
|-----------|-----------|
| 当前功能完成状态 | [STATUS.md](STATUS.md) |
| 产品路线图和未来规划 | [design/product-roadmap-v2.md](design/product-roadmap-v2.md) |
| MVP 时期的产品设计 | [v0/PRD.md](v0/PRD.md) |
| 技术架构全貌 | [v0/ARCHITECTURE.md](v0/ARCHITECTURE.md) |
| 前端设计规范和组件 | [design/frontend-redesign-v2.md](design/frontend-redesign-v2.md) |
| 任务系统如何设计 | [v1.2/task-system-spec.md](v1.2/task-system-spec.md) |
| 竞品 (slock.ai) 分析 | [research/slock-architecture.md](research/slock-architecture.md) |
| 竞品 (multica) 分析 | [research/multica-architecture.md](research/multica-architecture.md) |
| v1.3 迭代计划 | [v1.3/PRD-v1.3.md](v1.3/PRD-v1.3.md) |

## 版本历史

| 版本 | 日期 | 主题 | 关键变更 |
|------|------|------|---------|
| v0 (MVP) | 2026-05-11 | 核心闭环 | Auth + Channels + Messages + Threads + Agent 自动回复 + DM |
| v1.1 | 2026-05-12 | 基础加固 | Agent workspace 隔离 / memory / local 流式修复 / Neubrutalism UI / 消息编辑删除 |
| v1.2 | 2026-05-15 | Agent 能力升级 | 11 Backend / 任务系统 Phase 1 (Kanban + Claim + asTask) / Agent Tools / 交互模式 |
| v1.3 | 规划中 | 协作与体验补齐 | Bug 修复 / DM 任务 / Agent 自动认领 / 消息搜索 / 文件上传 / 电脑管理 |
| v2.0 | 规划中 | 企业能力 | 多工作区 / Billing / 团队管理 |

## 贡献指南

- 新增版本在 `docs/` 下创建 `vX.Y/` 目录
- 跨版本设计文档放入 `design/`
- 竞品调研放入 `research/`
- QA 测试文档放入对应版本子目录的 `qa/` 下
- 更新 `STATUS.md` 保持功能状态最新
