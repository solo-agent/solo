# Solo 自动评测

通过真实 Solo Task 比较单成员、团队和候选工作方法，使用独立程序验证产物，并记录交付状态、耗时和用量。评测复用现有前端、API、PostgreSQL、Daemon 和本地 Agent Runtime。

## 运行

需要已有的 `.env`、配对 Computer、本地 Claude/Codex 登录、Docker，以及仓库的前端依赖。服务由仓库根目录的 `make rebuild` 管理，评测入口复用 `scripts/run-local-e2e.sh`。

```bash
# 完整流程：开发对照、候选生成、验收对照、公开题和工作流回归。
python3 evals/pipeline.py --output /tmp/solo-eval-cycle

# 使用另一份产品代码相同、已有服务配置的 checkout。
python3 evals/pipeline.py --stack-root /path/to/configured/solo --output /tmp/solo-eval-cycle

# 小批次对照。
python3 evals/run.py --cases csv-money,relative-path --repetitions 1 --output /tmp/solo-eval-smoke

# 指定 Runtime 和模型。
python3 evals/run.py --provider codex --model <model> --output /tmp/solo-eval-codex

# 校验真实 Docker 判卷器、题库和采用规则；CI 也执行此检查。
python3 -m unittest discover -s evals -p test_evals.py -v

# 从现有数据生成 HTML 报告。
python3 evals/report.py /tmp/solo-eval-smoke/report.json

# 从本地会话记录补充模型归因，不改写原报告。
python3 evals/runtime.py /tmp/solo-eval-smoke/report.json --output /tmp/runtime-audit.json
```

完整流程包含 60 次开发对照、1 次候选生成、60 次验收对照、20 次 HumanEval 子集试验，以及五组工作流回归。当前原创题库为 `solo-skills-v3.json`；题目范围、版本差异和维护规则见 [题库说明](datasets/README.md)。

## 执行与数据

每次试验创建独立评测成员、频道和工作目录。Task 通过现有队列与 Run/Session 派发至本地 Runtime；作者提交实际文件，指定审核者检查不可变提交，独立 Docker 判卷器验证最终产物。通过结果还需核对前端成果和 PostgreSQL 中的 Task、Submission、Run 及用量。

单成员策略由程序验收；团队策略先由独立成员审核，再由程序判卷。候选方法只追加给评测作者，审核成员保持固定。学习材料只取开发集；候选冻结后才能进入验收对照，已见过结果的题目只能用于回归。

评测复用现有 API、权限、版本和幂等规则，不新增产品数据库表或页面。候选采用仅生成评测用方法文件，正式 Agent 配置仍由用户决定。

## 固定规则

- 原创题每个策略默认独立重复 3 次，交替执行策略顺序；公开子集执行 1 次。
- 每项任务默认限时 360 秒，作者和审核者共享时限。全部成员的介绍、审核、失败与返工用量计入总量；每次试验的资格上限为 300 万 Token，属于最终检查，并非账单硬截断。
- 采用候选需要至少 10 个验收题、每题至少 3 次完整配对、用量完整且无逐题退步。按题进行 5,000 次固定种子的配对 bootstrap。
- 质量差异的 95% 区间下界须大于 0，且 Token 不超过基线 1.25 倍；或成功率相同、Token 至少节省 20%，且节省比例的 95% 区间下界大于 0。否则保持基线；任一验收题退步则拒绝。
- 缺失用量记为 `null`。判卷环境故障、未结束试验和清理失败不能作为采用候选的依据。

默认使用 Claude Code Runtime 和请求别名 `sonnet`，可通过 `--provider`、`--model` 统一指定。报告区分请求别名、供应商响应标识和 Codex `turn_context` 中的模型与推理档位；运行配置不等同于供应商内部权重证明。

`evals/run.py --disable-thinking` 仅为该批 Claude 评测成员设置 `MAX_THINKING_TOKENS=0`，不改变普通成员或全局配置，也不自动作用于完整 pipeline。报告记录请求设置和响应块类型，不导出思考正文。`SOLO_E2E_CODEX_BIN` 可为隔离 E2E Daemon 指定 Codex 可执行文件，正常服务恢复时仍使用原配置。

## 报告与恢复

每批输出 `report.json`、`summary.json`、`index.html`、执行日志，以及逐项源码、提交、判卷、Run、事件元数据和截图。Playwright 附件保存在批次自己的 `test-results/`。运行产物和过程笔记保存在 Git 忽略的 `evals/results/`，不提交到公共仓库。

`run.py` 要求空输出目录。`pipeline.py` 可在原目录恢复，只复用冻结版本一致且完整结束的阶段；中断尝试保留，后续尝试使用新编号。修改源码、模型或题库后须使用新目录，不能混合不同条件的成绩。

模型阶段的基础设施故障以非零退出；工作流中一组失败不会阻止其余组执行，但整体仍为失败。清理仅操作评测创建的成员、频道、额外工作区和 Computer 租约，保留数据库历史证据并恢复普通 Computer。服务启停只通过 `make rebuild` / `make stop`。

## 工作流覆盖

| 套件 | 检查范围 |
|---|---|
| task-delivery-contract | 实际文件提交、独立审核、版本验收、返工与用量 |
| agent-selection | 固定 Revision、接收成员消费与采用权限 |
| task-wait-resume | 持久等待、服务重启恢复与同一任务续跑 |
| team-work-compounding | 多拥有者授权、连续纠正与责任记录 |
| thinking-mode | 分支 Session、交接、空闲休眠与恢复 |

## 隔离与范围

判卷器运行于无网络、只读根目录、无宿主挂载且有资源限制的临时容器。真实本地 Agent 保留其既有宿主权限，提示词中的题库访问约束不构成操作系统级隔离；该套件用于受控、非对抗评测。

原创题是受控维护任务。HumanEval 使用固定公开子集和上游原始测试，按完整文件 Task 执行；结果口径与官方补全排行榜不同。上游版本、抽样规则和 MIT 许可见 [来源记录](vendor/human-eval/provenance.json)。
