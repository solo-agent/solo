# 代码 Gate 固定样例

这是本手册单独创建的测试仓库，不是 Solo 产品仓库。不含真实业务文件或凭据。

仓库绝对路径（复制到表单）：

```text
/Users/langgengxin/.codex/worktrees/a8d8/solo/tmp/pm-acceptance-code-20260909
```

基准 Git commit（复制到表单）：

```text
cfebd12c6dd4f66383512ac3c22acd63ee75edb9
```

验收命令 JSON：

```json
[{"requirement_id":"R1","command":["python3","check.py"]}]
```

故意错误基线：double(n) 返回 n*2+1。负责人仅在 Task worktree 修复为 n*2。禁止修改 check.py。测试真实验证 -2、0、3 三个输入，通过时输出 PM_CODE_GATE_PASS。

PM 不需要运行 Git 命令；把主手册 PM-10 的完整指令发到测试任务线程。第一次可以交付基准 commit 的真实错误状态，Gate 应退回；随后提交修复后的新 commit。

此目录已准备，算法负向与正向检查仅为离线样例检查，不代表真实 Gate E2E 已执行。若目录被清理，先请维护者重新准备并更新本说明，不能凭空填写 commit。
