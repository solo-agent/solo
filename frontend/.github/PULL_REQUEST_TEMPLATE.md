## 概要

<!-- 1-3 行说明这次 PR 改了什么、为什么 -->

## 改动类型

<!-- 勾选适用项 -->

- [ ] 新功能
- [ ] Bug fix
- [ ] 重构（无行为变化）
- [ ] 设计系统 / 样式
- [ ] 文档
- [ ] 测试

## 设计系统检查 (如果是样式相关 PR，**必须**勾选)

- [ ] 已运行 `cd frontend && bash scripts/audit-brutal.sh`，输出 `✅ audit clean`
- [ ] 已运行 `cd frontend && npx tsc --noEmit`，无新增错误
- [ ] 如果引入了新组件，已加入 `components/ui/` 并提供 Storybook / 类型导出
- [ ] 没有引入 `rounded-{md,lg,sm,xl,2xl,3xl}`
- [ ] 没有引入 Tailwind 默认色阶（`bg-green-500` 之类）
- [ ] 没有引入 `backdrop-blur`、`shadow-{md,lg,2xl}` 等"软"效果
- [ ] 如果触碰了按钮：使用了 `Button` 组件的现有 variant；如需新 variant 已在
      PR 描述中说明

## E2E 测试 (如果改了 app/ 下任何页面)

- [ ] `cd frontend && npx playwright test e2e/brutal-consistency.spec.ts` 通过
- [ ] 受影响的页面已附 screenshot 到 `.audit-shots/`

## 关联

<!-- 链接到相关 issue / Linear ticket / fe1 / fe2 worktree -->

Closes #
Related: feat/fe1-..., feat/fe2-...

## 截图 (如适用)

<!-- 拖入截图到对话，UI 变化请务必附图 -->
