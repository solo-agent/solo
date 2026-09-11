# Solo 长期团队实施前设计与阶段验证记录

这些是分阶段形成的设计和历史检查记录，保留当时的状态。当前实现与验证结论以 [实现说明](./agent-team-compounding-gap-and-rollout.md) 为准；旧阶段的“待实现”或通过记录不代表最终状态。

## 最终回归发现：持久会话消息读取授权

组队第二轮证明 `message read` 使用首轮环境凭据直接访问 API，首轮 Run 结束后服务端正确拒绝该凭据。修复沿用已有 `message_read` daemon action，CLI 在解析目标后传递分页、Thread 和 DM 选项；daemon 在当前唯一执行中的 Agent／Channel／Thinking 范围获取 Run 凭据，构造固定消息 API 路径，继续由原服务端校验权限。无新增领域对象、数据库迁移、前端状态或凭据持久化。代理不可用时，受管 Runtime 不降级使用旧 token；无显式 daemon 配置的独立 CLI 保留原直接访问行为。原 API、输出 JSON、分页和普通／Thinking／Thread 路径保持兼容。单测校验路径构建，真实组队及主线旧凭据用例校验 Runtime→CLI→daemon→API→数据库与页面结果。测试脚本遇错退出，不允许自行查找其他凭据或管理服务。

## 完整目标补齐审计（进行中）

2026-09-09 重新对照文章、三个指定任务及当前代码。上一轮证明了交付主路径能运行，尚不足以证明完整目标完成。当前本地 master 为 a46f7d2，origin/master 与工作树基线同为 9b9de97；主线兼容还需要覆盖现有产品 E2E，不能只凭新增用例通过。

| 要求 | 当前证据与待完成事项 |
| --- | --- |
| 身份、执行位置、私有 Memory、消息与 Run 分离 | 复用主线；需将既有 E2E 纳入回归记录 |
| Thinking 分叉、Checkpoint、Return、Session 换代与故障恢复 | 已有实现；需验证本次版本和调度改动没有干扰它们 |
| 注意力、订阅、跨会话待办、单 Agent 领取、优先级 | 注意力和跨频道消息已有实测；需要审核跨来源领取与离线恢复，不能以 daemon 串行锁代替全部调度语义 |
| 新鲜度补读后的修改、确认保留、放弃 | 三种决定均已实现；精确游标与理由、CLI 与页面丢弃、义务保留已通过真实 E2E |
| 消息认领、正式任务与交接 | 已补短 ID 原子认领、并发去重和来源约束；真实 Runtime→实时卡片→人工验收 E2E 通过 |
| requirements、Handoff、代码隔离与三类 Gate | 已实现；继续核对与主线提交、Artifact、Session Continuity 的所有消费入口 |
| Automation、父子 Loop | 根任务验收与不重叠已有实测；原 Task 的条件等待、时间到期、依赖验收、外部确认与持久恢复已实现，完整 E2E 1.0m 通过 |
| Agent PR、相同任务／预算／模型下 Selection、团队检查 | 迁移 73 和前后端已实现固定任务集、同模型／额度的成对试验、真实接收检查及接受／拒绝／观察；实际 E2E 通过（1.8 分钟），包含 make 重启后继续 |
| Prompt／Skill 改进进入版本，Team 发布与回退 | Prompt／模型／参数与显式 Skill 文件包已进入不可变版本，UI／真实 Runtime／数据库 E2E 通过；配对远程传输 E2E 已通过（1.9 分钟） |
| 依据工具权限、提及／认领／委派和交付反馈组队 | 现有 Lucy 仍仅套模板，需要复用已有成员、职责选择依据与关系调整入口 |
| 长期复利的可检查记录 | 原始耗时／Token 已有；需补合格交付成本、失败样本、人工介入与返工原因的对照记录；不能宣称收益已成立 |
| 跨 owner 团队 | 本机真实双 owner 通过；保持 Computer、配置和 Memory 权限隔离，补兼容回归 |
| Review、单测、实际 E2E、完整文档 | 逐项核对并记录发现与修复；在全部缺口完成前保持目标进行中 |

### 补读后决定保留或放弃：实施前设计

复用现有消息事务、Run 游标、held draft 和 Run event。发送请求新增 `keep_after_seq` 与 `freshness_reason`：保留旧稿必须明确引用本 Run 最后已展示的序号并说明原因，服务端仍先查新消息；有新消息先补读，旧确认不能放行。确认记录随可见消息 metadata 持久保存。旧客户端不传字段时继续拦截未经确认的原样旧稿；普通修改稿保持原行为，不改 Task 和 Session 状态。

放弃使用 owner／Agent 自身可调用的 `POST /agents/{id}/drafts/discard`，包含 Run ID、draft SHA256 和原因。服务端锁定 Run，核对待办中读到的草稿摘要，删除草稿并在同一事务追加决定事件。重复相同请求返回已处理；其他草稿、其他 Agent 或不同原因不共享幂等结果。放弃只结束这份草稿，不自动完成 Task、work mark 或 Run 的可见交付义务。

CLI 经现有 daemon 当前 Run 代理传递确认字段，补读输出显示序号与正确用法；`work discard --file` 承接放弃。前端待办复用当前刷新和错误状态，在草稿下提供原因输入与放弃动作。无须新表或迁移；失败事务回滚，页面重载重新读取权威状态。验证覆盖旧确认后又有新纠正、无原因保留失败、确认保留成功、草稿变化冲突、重复放弃、越权及实际 Runtime→CLI→daemon→API→PostgreSQL→页面链路。


### 消息原子认领：实施前设计

当前 `Claim` 只查既有 Task，CLI 给普通消息短 ID 时无法创建；`ConvertMessageToTask` 的查询与创建不在同一事务。改为先在频道内解析精确 Task ID／序号或唯一消息前缀，再由服务事务锁住原消息及现有 Task，完成查找、创建和认领。已有 Task 的优先认领窗口保持生效；短 ID 有歧义直接报冲突，不猜测。普通 Convert 与带 message_id 的 Create 同样遵循唯一来源约束，不能从其他入口绕过。

复用 Task、Thread、既有通知与页面刷新，不新增编排实体。数据库给非空 message_id 加唯一约束；安装前检查历史重复，发现重复时报告 ID 并停止该迁移，保留全部记录，不自动删改旧任务。根任务和子任务的 message_id 都检查原消息属于当前频道且不是 Thinking 协议消息。GetTask 的 UUID／序号／消息引用共用同一查询结果，避免不同查找入口丢失父子任务字段。并发失败回滚整项认领，调用方重试获得当前负责人；成功响应继续返回 task number 和源消息 Thread。

覆盖服务、API、daemon、CLI 和页面；验证并发转换仅产生一个 Task、并发认领仅一个 owner 成功、同 owner 重试、短前缀歧义和跨频道隔离；真实 Runtime 必须能从普通消息短 ID 创建并认领工作，刷新后的页面与数据库一致。

### 条件等待与原任务恢复：实施设计

借鉴 LoopX `todos/resume_condition.py` 与 `deferred_resume.py` 的“条件可判定、原责任保留、条件未变不唤醒”语义，使用 Solo 原 Task、Run、Session 和远程投递，不引入第二套调度系统。

- 领域与所有权：`task_waits` 追加保存等待记录，一个 Task 同时最多一个有效等待；创建者、负责人或负责人 owner 可登记。Task 保持 `in_progress`，等待有独立状态，保留原负责人。请求带 Task version 和幂等键；只支持可执行的条件：同频道 Task 验收完成、指定时间到达、明确外部信号。外部信号由任务创建者或负责人 owner 根据证据确认，不把普通聊天当作条件满足。
- 生命周期与数据流：登记条件、当前交接和继续动作后，当前 Run 可结束。条件未满足不创建 Run、不消耗模型预算。原有后台责任扫描器检查条件，锁 Task 与等待记录，再取得同 Agent 工作锁；忙碌时让该 Agent 继续现有工作，其他成员不受影响。就绪后在同一事务内创建并关联原 Task 的 Run、保存条件证据与远程 dispatch payload、将等待标为 resumed，事务提交后交由现有执行流程运行。
- 恢复与失败：Server 重启后重新读取数据库；离线 Computer 等待重连，记录有界错误并延迟重试，不新增 Task。已入队 Run 按现有远程交付、watchdog、预算和 Session 恢复机制处理。目标、负责人、任务版本或成员授权变化，使旧等待取消；重新登记须依据新版本。依赖形成环时拒绝登记，依赖被关闭时保留等待及明确阻塞原因供用户处理。
- API 与前端：Task 的 `GET /waits` 返回历史、状态和原因；`POST /wait` 登记；`POST /resolve-wait` 以精确 wait_id 取消或确认外部信号。全局与频道 API 共用处理器，CLI 共用当前 Run 凭据代理。任务卡片提供“等待与继续”入口，刷新时读取真实记录，允许登记、取消与确认，不把等待伪装成任务完成。Session 连续上下文包含条件、交接与继续动作，避免处理其他消息时误做等待中的任务。
- 持久化与兼容：迁移只新增表、索引及任务更新后的失效触发器，不修改旧 Task 状态枚举、旧任务或 Automation。旧交付与验收接口保持原语义；任何路径把 Task 改为非执行状态都会关闭其未消费等待。依赖仅允许同频道，复用已有权限。已有父子任务仍按原验收约束完成。
- 验证：真实 PostgreSQL 检查幂等、权限、循环依赖、版本失效、就绪领取与无重复 Run；真实 E2E 由实际 Runtime 登记等待，页面验证等待、执行独立工作、确认外部条件、原 Task 自动恢复、人工验收并核对持久化结果。


### 等待链路审阅记录

- 等待记录不修改 Task 状态枚举；给 Task 列表和详情增加 `waiting` 投影，Agent 待办也显示等待数量和原 Task 链接。各 Task 读取入口都用真实 PostgreSQL 检查了这一字段。
- 除显式等待边之外，把“父任务须等未结束子任务”纳入环检测，拒绝子任务等待父任务完成。
- 恢复事务重新锁定已完成的依赖 Task，保存其精确版本和提交 ID，避免并发重开造成过期判断。
- 在生成新 Session 的连续上下文之前先消费等待记录，避免恢复 Run 同时收到“继续”和“仍需等待”两条冲突指令；整个事务失败会一起回滚。
- 原有失败重试和常规 Task 派发会跳过未满足条件的 Task。已恢复任务若随后进入已有重试流程，保留条件证据、交接和继续动作，不回到最初的等待登记。
- 与普通 Task 更新共用行锁；目标、负责人、状态改变会经数据库触发器取消旧等待。撤销 Agent 的现有流程会释放其 Task 责任，从而同步取消等待。
- 审核保持优先于等待恢复；等待派发仍使用同 Agent 工作锁，并避开活跃 Run。忙碌确认场景在实际 E2E 中覆盖超过一次扫描周期。

当前新增等待代码的全量 Go 回归、TypeScript 和修改文件 ESLint 均通过；实际等待恢复 E2E 通过（1.0m）。本机 Turbopack 生产构建遇到内部端口绑定权限错误，正在使用 Next.js 官方提供的 Webpack 构建选项验证生产产物。上述结果不替代完整文章范围中的其他待完成项。

### Skill 进入 Revision：下一阶段实施设计

在现有 Revision 配置中增加显式选择的 Skill 文件包。采用文件内容快照，可以在候选的独立目录与后续 Computer 中重建同一份能力；不把某台机器的可变路径当作已固定版本，也不复制整个 Agent 目录。

- 模型与所有权：Skill 包包含名称、相对文件路径、内容、可执行标志和内容摘要；根目录必须有可解析的 SKILL.md。只接受显式提交的文件，拒绝路径穿越、重复路径、符号链接和 Agent 私有 Memory／凭据目录。文件数和总大小有明确上限。包内容并入 Revision 的不可变配置摘要；空 Skill 列表沿用旧配置摘要，避免给所有旧 Run 制造无意义的版本变化。
- 数据与 API：Agent 的当前 Skill 配置由 owner 编辑，候选通过现有 Revision API 登记，正式发布继续走评测和选择。前端在能力版本表单中选择本地 Skill 目录，展示文件清单后创建候选；也支持 Agent 通过当前 Run CLI 提交显式文件包作为提案。候选创建复制所选包，不复制原 Agent 的 Memory、聊天或 custom_env。请求、数据库和远程 Run payload 使用同一校验后的结构。
- 下发与 Session：Server 从 Run 固定引用的 Revision 读取包；daemon 在模型启动前验证摘要，写入专用快照目录，并通过现有 provider Skill 目录接入。只管理有明确归属标记的目录，保留其他本地 Skill、项目文件和 Memory。版本变化沿既有 Revision／Team 判断开启新 Session。新协议能力与配置收据记录实际准备完成的 Skill 摘要；老 daemon 遇到带 Skill 快照的 Run 明确要求升级。
- 生命周期与恢复：缺文件、摘要不匹配、路径冲突或写入失败都使配置准备失败，不把残缺 Skill 当成成功运行。写入采用临时目录后原子替换；重试从数据库内容重建完整快照。旧 Revision 和旧团队版本保留其 Skill 内容，回退只影响后续 Run。运行中修改工作文件不改变已保存的版本历史。
- 兼容与迁移：迁移只给 Agent 增加默认空列表，并扩展 solo_agent_config；不回填猜测的历史 Skill。旧配置、未采用版本包的机器 Skill 与原有 Runtime 接入继续可用。Team Lockfile 仍保存成员版本引用，不向协作方泄露完整私人配置；只有获得相应配置权限的 owner／执行 Agent 能读取包内容。
- 验证：真实临时文件验证摘要、可执行文件、遍历拒绝、失败不覆盖、原有文件保留、版本切换与回退；真实 PostgreSQL 验证版本不可改和旧配置摘要兼容；实际 Runtime E2E 执行包内脚本，比较候选与回退的可见输出、数据库 Revision／Run 引用和配置收据，并检查私有 Memory 未被复制。

### 主线 Thinking 画布回归修正设计

真实四成员 Thinking E2E 在手动分支创建后发现：画布仍按“十个节点以内显示全图”取范围，但 `fitView` 强制最小缩放 0.7，较窄分栏无法装下全图，新分支被裁切。录屏已确认这不是服务端创建失败。修正只影响前端视口，不改变节点、父子关系、Session、数据库和 API：用现有 React Flow 的边界/视口计算方法，在全图会低于可读缩放时聚焦所选节点与父节点；仍容不下时聚焦所选节点。原生 ResizeObserver 让分栏尺寸变化也触发同一计算。节点尺寸沿用组件已有固定值，取消的 animation frame 不提前登记为已经完成的聚焦。失败恢复仍由重新渲染和真实拓扑状态驱动，无迁移，旧模式和手动缩放入口保留。验证继续使用原有真实 Thinking E2E 的可见、可点击和持久化断言。

### Skill 文件版本实施与验证

Thinking 的后续回归还覆盖自动分支：新增节点优先进入视口；原生分支选择框使用已有 onSelect、URL 与节点状态，让窄分栏中的屏外节点仍可直接访问。全图无法保持可读尺寸时，测试通过这一真实导航入口选择兄弟分支，再验证节点完整可见、可点击及消息隔离。无新增后端状态或依赖。

### Selection 配对评估：实施前设计

Selection 保存一次不可修改的评估计划：原成员、基线与候选 Revision、暴露问题的工作引用及说明、改动说明、固定任务集、每侧 Token 预算、接收成员 Revision 和交接检查。创建者必须拥有原成员与接收成员，并有权使用同一 Computer；接收成员必须是不同成员。基线和候选的 Provider、模型、custom_args 完全一致，避免把模型差异算成 Prompt／Skill 改进。模型升级可单独作为配置选择处理，不能冒充这一配对评估。

创建计划的事务同时建立基线、候选、接收方的独立成员和频道，以及各样例的普通 Task。只复制 Revision 中的显式能力，沿用真实 Computer，不复制聊天、Memory、custom_env 或运行 Session。每个样例给双方相同输入和逐项验收要求，接收方 Task 固定检查要求。所有 Task 使用原有提交、证据、人工 Gate 与版本锁；任务内容、验收要求、负责人在试验中禁止修改，正常提交和审核仍可进行。评估按样例顺序执行，每侧沿用其独立状态。

后台复用现有 5 秒责任扫描器和 Run／远程队列；Task 与首个 Run 的关联、远程 payload 在事务中一起保存，Server 重启可继续未派发项目。基线与候选分别执行，接收方只在该样例候选产物已经验收后执行，输入保存精确 submission ID、artifact version、handoff 与 evidence；不能只看候选自称兼容。运行前重新检查固定配置和授权。失败保留普通 Run 记录及明确原因，原有失败恢复继续适用。

预算沿用 agent_run_token_usage 账本，在统一 ReserveRunTx 中增加 Selection 成员的终身额度约束，覆盖所有来源的 Run，跨月不清零。并发使用现有预算锁，预留沿用系统每 Run 额度并取剩余额度上限；缺用量的已启动 Run 按预留占用，失败成本不丢弃。额度是新 Run 启动预算，模型单次执行仍可能超支，照实记账并阻止后续运行；不能将其描述为 provider 原生硬 Token 上限。基线与候选使用相同机制和额度，接收方单独计量。普通 Agent 的月预算不变。

Owner 在已有版本面板填写计划，查看逐样例 Task 链接、实际接受／退回、Run 用量与错误，记录接受、拒绝或继续观察及理由。决定追加保存；接受须双方样例已有终态，候选和接收方全部验收，相关 Run 已结束且用量已确认，没有配置或任务偏移。拒绝和观察不自动发布。发布新候选必须有有效接受决定，并核对当前 live 仍是被比较的基线；已发布版本仍可显式回滚。决定记录引用精确结果，后续重开任务不会保留旧接受资格。

迁移新增 Selection、试验成员与样例关联、追加决定记录，不改既有 Task 状态或旧 Run；先前已发布版本保留回滚能力。API 仅 owner 可读写，Agent 可提交候选但不能代替 owner 作 Selection 决定。前端重载读取数据库权威状态，错误允许按原计划重试，不重复建副本。验证覆盖计划幂等、权限、条件一致性、预算与并发、不可变边界、失败成本、接受约束和发布基线漂移；实际 E2E 验证双方真实运行、接收方消费确切输出、页面审核与决定、发布和数据库记录。

Selection 审阅补充：未按合同提交的完成 Run 也计入失败恢复，最多尝试三次后关闭该样例并保留原因；按固定样例顺序继续。启动预算失败须回滚 Run 插入的 savepoint，不能留下没有执行者的 queued Run。试验成员还核对 Computer、空 custom_env 和无外部 Team 固定，防止版本摘要以外的配置编辑污染对照。现有普通 Task 重试扫描跳过 Selection，使用同一责任扫描器里的固定计划派发，避免两条路径同时恢复。Dashboard 的主线单测现在使用真实 Workspace context 隔离样例，避免累积 E2E 消息挤出其词频前 80 项；HTTP 查询仍沿用原有 Workspace 范围，产品语义未改。

消息恢复审阅发现，已删除成员的 DM membership 会按既有产品规则保留，而 DM 里旧的 pending wake 不会随普通频道 membership 删除。统一 advancePendingMessageWake 在没有活跃 Run、且 Agent 已失效时清理其待派发消息和空 wake slot；继续保留原消息、DM、已发生的 Run 和责任记录。清理在已有 Agent 工作锁与事务内完成，启动恢复和周期扫描使用同一路径；无需改 API 或数据库结构。真实 PostgreSQL 检查 DM 消息仍在、无新 Run、无残留派发以及重复恢复幂等。

### Selection 实施与实测记录

新增迁移 73、owner 的 `GET/POST /agents/{id}/selections` 与 `POST /agents/{id}/selections/{selectionId}/decide`。能力版本面板可填写最多十个样例、选择接收成员、设定每侧终身 Token 额度，保存问题和改动说明。基线、候选和接收方各用一个独立成员与频道；样例是已有 Task，带真实根消息和 Thread。逐样例复用原交付与验收弹窗，结果记录的 Task／Run／Submission 可直接追溯。新候选发布必须有对当前 live 基线有效的接受记录，单边评测已经不能放行发布；之前正式发布过的版本仍可恢复。

`/tmp/solo-selection-reviewed-full-go.log` 全量 Go 通过，包含真实 PostgreSQL。`/tmp/solo-selection-dm-unit.log` 补充验证删除成员的 DM 派发清理；Selection 用例覆盖幂等、计划／决定不可改、固定输入、拒绝非评估消息启动、并发预算预留、跨月失败成本、接受条件、发布、回退和重开后失效。TypeScript、改动文件 ESLint 与 `git diff --check` 通过。

`/tmp/solo-selection-final-e2e.log` 实际 E2E **1 passed，1.8 分钟**。同一输入 `[21,21]`、相同 Claude 模型与每侧预先固定的 200 万 Token 额度下，基线真实脚本返回 40，候选真实脚本返回 42；两侧保存实际证据。基线审核后使用 `make rebuild` 重启整个测试栈，再审核候选，由接收副本执行 Python 解析候选精确 E1 内容并检查结果为 42。最后在页面接受 Selection、发布候选，核对正式配置、追加决定、三份已验收的 Run 用量，以及 receiver.input_submission_id 指向当前候选提交。此样例证明闭环与记录有效，不代表通用能力或成本改善。

保留失败记录：首轮 E2E 因测试误以为审核后弹窗自动关闭而失败；后续一轮候选重复改写已提交请求，服务端拒绝覆盖原证据，且实际消耗超过该轮预先设定的 50 万 Token，因此不能接受。该轮停止测试但未改写结果；执行说明随后补充“脚本已提交成功则结束 Run”，新一轮在执行前固定双方 200 万额度。预算为 Run 启动额度，单次超支会真实记录，不能追溯调高旧轮额度使其通过。

当前旧的 Skill 与 Task-contract E2E 仍需迁移到强制 Selection 发布条件，不能把之前单边发布测试的通过记录当作本阶段回归通过。Thinking 最新重跑遇到 Claude 连续十次 529，仍需完整重跑；自动组队反馈、统一跨来源 Inbox 和完整长期指标尚在实施。

已加入迁移 72：Agent 默认空 Skill 列表，非空列表并入不可变 Revision；空列表不改变旧配置摘要。文件包采用 JSON base64 内容，限制总计 20 个 Skill、500 个文件、4 MiB，逐项验证相对路径、大小写冲突、文件／目录冲突、SKILL.md 名称与说明、可执行权限及摘要。拒绝隐藏目录、Memory、凭据文件与依赖目录；这是明确的文件边界校验，不是任意内容的隐私识别器。

前端在「能力版本与评测」选择本地目录并逐文件标明执行权限。候选从当前 owner 配置加载已有包，明确移除才删除；评测副本只复制所选配置和包。`solo work revisions` 与 `solo work propose-revision --file` 让实际 Run 可以读取版本并登记提案，发布仍受 owner 与评测规则约束。普通成员列表不传整包，owner 详情、版本记录与授权执行 Run 可以读取内容。

Daemon 使用标准库 `os.Root` 约束落盘，先写新快照、再原子切换 current 指针；provider 的已有 Skill 搜索目录只链接明确属于版本管理的 Skill，名称冲突时停止并要求重命名，不覆盖机器原有目录。后续 Run 从数据库文件包重建快照，恢复旧版也恢复脚本与资源；模型 Session 根据 Revision／Team／Skill 摘要变化重建。运行配置收据包含实际准备的 Skill 摘要。远程队列在入队和领取时都检查 `skill_bundle_v1` 与 `run_snapshot_v1`，包括 Computer 在离线期间降级的情况。纯 API Completion provider 没有本地文件执行能力，明确拒绝文件包；本地 CLI backend 和代码 Gate 使用真实文件准备路径。

本轮证据：`/tmp/solo-skills-full-go.log` 全量 Go 通过（含 PostgreSQL 的版本发布与团队固定测试）；`/tmp/solo-skills-unit.log` 包校验、真实脚本、二进制资源、provider 切换、回滚、非托管目录冲突与路径越界检查通过；TypeScript 与改动文件 ESLint 通过。`/tmp/solo-skills-e2e-final.log` 真实 Skill E2E **1 passed，1.4 分钟**，核对 UI、文件和数据库：上传新资源 → 独立副本运行脚本并通过 CLI 提案 → 人工验收 → 发布 → 团队固定新版 → 恢复 live 旧版但固定团队仍运行新版 → 解除固定后运行旧文件。原成员 Memory 保留、评测副本不继承；两个版本的 Run 收据摘要不同。配对远程传输 E2E `/tmp/solo-skills-remote-e2e.log` 另通过（1.9 分钟），核对真实 Computer enrollment、远程领取 Run、执行 attempt／accepted_at 和落库收据。

主线消息 E2E `/tmp/solo-mainline3-agent-delivery.log` **5 passed，4.1 分钟**，覆盖可见结果、Session 复用、过期 provider Token、模板查询、Coordinator 路由及 make 重启续接。旧测试没有指定 Workspace 时会落到公共空间，已让测试显式建立／使用私有 Workspace，而非改变公共空间的唤醒规则。生产 Webpack 构建 `/tmp/solo-wait-webpack-build.log` 已通过；默认 Turbopack 在本机因内部子进程绑定端口权限失败，不能写为默认构建通过。

### 长期交付指标：实施前设计

以已有 Task 为计量单位，不另建项目或实验执行框架。通过已有 Run–Task 关联收集全生命周期执行、审核、失败与返工成本；Submission 与当前 accepted Review 决定是否合格。时间窗口按 Task 创建时间选择样本，样本内累计全部后续成本，避免把窗口外失败排除。Run 同时关联多个 Task 时明确显示共享数量，禁止直接把它用于无重复成本的组间比较。

新增仅供真实用户追加的 Task observation：人工投入分钟、重复工作、重复解释、额外交接、退回原因分类、恢复后首个动作是否正确，以及明确的任务类型／难度对照组。每项必须有说明、幂等键与记录人；退回分类引用该 Task 的真实 rejected Review，恢复判断引用该 Task 的真实恢复 Run。分类与恢复判断同一对象只记录一次，避免重复计数。对照组仅任务创建者或当前负责人 owner 可登记，保留更改历史。人工分钟只表示记录人的实际投入，不从聊天时间差或 Agent 推断生成。记录不修改 Task 状态、验收结论、产物或 Selection 条件。

预算账本增加运行时预算快照，复用 ReserveRunTx 已获取的预算锁，将当时启用的用户／成员月额度与 Selection 终身额度写入同一事务。旧记录不猜测回填，显示未知。模型来自 Run 固定 Revision；不会以成员当前配置覆盖历史。没有可核实货币价格时使用实际 Token、执行秒数、从创建到当前验收的历时和人工分钟，不生成美元估价。未知用量保留占用并单独计数。

API 复用 Task 权限和全局／频道处理器：GET metrics 返回指标及 observation 历史；POST observations 只允许活跃用户且仍是频道成员。洞察页复用现有 Dashboard，增加按显式任务组、实际模型集合和预算快照集合分组的交付样本；保留失败／未验收任务和成本，展示合格数、返工及原因重复数、人工记录覆盖率、共享／未知样本数。只有分组完整、模型和预算已知、用量完整、无共享 Run 的样本才进入可比汇总，并同时展示排除数量，避免幸存者偏差。

前端在原有交付弹窗读取真实统计并填写观察，保存成功后重读；失败保留输入可按原幂等键重试。后台没有新增扫描器，数据始终从事实表读取，Server／daemon 重启无需重建指标。恢复耗时单独取等待消费到第一个真实工具事件的间隔；没有事件时保持未知，首个动作正确性依赖人工记录。迁移只新增观察表、索引、预算快照列和只读查询，向下迁移不修改已有业务记录。验证用真实 PostgreSQL 覆盖权限、幂等、引用归属、未知数据、失败成本、共享成本和分组；实际 E2E 在已真实运行并验收的 Task 上填写人工观察，检查页面、API 和数据库一致，再核对洞察页汇总。

指标权限审阅：同频道可以查看交付产物和归属成本，但成员 owner 的个人／月预算政策仍是私有配置。Task 与 Dashboard 的同一查询出口将其他 owner 的预算快照显示为未知，不泄露额度，也不把这些样本算成条件完整。恢复正确性必须已经存在该 Run 的真实工具事件才能记录；“首个工具动作”是可测恢复指标，不直接等同于有效工作，需结合人工正确性判断。

### 指标实施与首次实测

迁移 74 提供只读 task_delivery_measurements 视图、追加 task_observations 和 Run 预算快照。观察接口复用 Task 权限与版本交付面板，洞察页展示完整分母、失败成本、用量未知／共享 Run、人工记录覆盖、退回类别重复及恢复正确性。对照按显式任务组、运行历史中的模型集合及预算集合拆开；同组可比 Token 总数包含未验收和失败任务，除以合格交付数量。未知或共享样本明确列为排除，不能得出总体已改善的结论。相同预算集合表示执行条件集合一致，不声称各次预算剩余额度或执行顺序一致。

`/tmp/solo-metrics-unit.log` 真实 PostgreSQL 检查通过，覆盖人工分钟不能由 Agent 填写、幂等、记录不可改、退回引用与只分类一次、非成员无权读取、跨年失败 Run 成本保留、共享 Run 不重复计数和未知样本不进入可比汇总。`/tmp/solo-metrics-e2e.log` 实际 Selection 与指标 E2E **1 passed，1.1 分钟**：真实固定脚本和接收检查之后，在 UI 记录人工投入 2.5 分钟及任务组，核对 Task、Dashboard、API 和 PostgreSQL 的合格数量与用量。脚本 EXIT trap 曾因切换到 frontend 目录导致 make stop 找不到根目录 Makefile；随后已从根目录执行 make stop 并验证隔离端口关闭，后续 trap 固定仓库根目录。此为测试清理脚本错误，未改写实际通过结果。

Skill 复核补上 OpenClaw 的原生 skills 根目录，与既有 provider 共用同一受控文件写入／切换流程，并增加实际脚本执行及跨 provider 残留清理检查。此处修复只补齐已有 provider 的目录覆盖，不引入新的运行方式。

迁移回退审阅补充：72 的回退必须先移除 live Skill 配置、结束仍使用 Skill 包的 Run；历史 Revision 文件仍保留。73 的回退必须先结束试验 Run、停用试验副本，避免移除隔离与终身预算规则后副本变成普通自动执行成员。74 回退会删除新增观察与预算快照，需先导出这些新功能记录；不修改已有 Task、Review、Run 和用量账本其他字段。以下回退验证只操作专门的新建测试库，不回退已完成 E2E 的数据库。

指标与目录修正后的 `/tmp/solo-metrics-full-go.log` 全量 Go 通过（service 6.087s，pkg/agent 16.241s）；`/tmp/solo-migrations-74-fresh.log` 全新测试库完整升级通过，`/tmp/solo-migrations-72-74-down.log` 与 `/tmp/solo-migrations-72-74-reup.log` 验证 72–74 回退、再升级，三条版本登记恢复。临时库已删除，实际 E2E 记录库保留。预算隐私边界的修正晚于首次指标 E2E，须在后续真实回归再次验证。

### 自动组队与协作反馈：实施前设计

保留手工模板新建行为。Lucy 的现有 Form 从用户原消息和官方模板开始，继续使用同一幂等来源、建队租约与事务；自动模式默认先查可复用成员，也允许计划明确要求新成员。计划可给每个模板职责补充成员 ID、必需 Skill 名称、Provider／模型要求与选择依据，不允许重复职责、模板之外的职责或一个成员同时充当多个独立职责。没有覆盖工具要求的成员时返回具体缺口，不创建一个名义上具备能力的空成员。

成员资格由服务端重新核对：活跃普通 Agent、当前用户所有、目标 Workspace 和新频道允许其 owner 参与、可用且未撤销的 Computer 访问权限、实际配置包含所需 Skill、Provider 可由该 Computer 执行、既有预算允许启动。评估副本与 Lucy 不进入复用池。首版自动复用只从调用用户拥有的成员中选择；其他 owner 的成员继续使用已有显式频道加入与双方协议流程，不能由 Lucy 自动扩大授权。能力相关性来自确切模板职责的历史绑定（既有 formation result）或同模板的未变职责配置；不把同名和关键词相似当作能力证明。

合格候选按对应职责的真实交付与退回情况选择，再参考已知的每次合格交付 Token 成本及模型／预算要求；样本数与未知项一起保存在选择依据，不宣称一个分数代表通用能力。无历史时保留模板职责和调用者已验证的执行配置。固定选择不会改写被复用 Agent 的 Prompt、Skill、Memory、home Channel 或私有环境；只加入新频道，在新频道建立独立 Session 和职责关系。

使用现有 messages.mentioned_agent_ids 统计明确提及，保留发起者和 Task 关联；不回扫文本猜测旧称呼。新增小型责任事件表，记录真实创建时指派和 claimTaskTx 首次认领，普通 Claim 与消息转任务共用同一路径；重试不重复记录。旧未保存的认领过程保持未知。交付／退回与交接补充来自当前 Review、Submission 和 Task observation。Lucy 候选查询以及组队结果提供这些证据，实际成员反馈可通过已有 work 入口读取，再用已有关系提案／owner 审核更新输入、输出与回滚约定。不会根据提及热度绕过工具资格或验收证据。

模板关系创建始终显式写目标 channel_id。涉及复用成员时，在同一事务写入该 owner 已授权的确切关系提案及批准记录，再写边；遵守已有数据库 scope／cycle 检查，不借 home Channel 默认值绕过双方协议。手动修改和跨 owner 调整继续走现有提案与撤回。结果保存每个职责的 reused、新旧成员 ID、依据、相关样本和配置条件；前端在已有建队结果卡显示选择理由，重新加载仍读取落库结果。

CLI Form 的原 JSON 通道保持兼容，补充只读候选查询并使用当前 Run 凭据代理。没有新增常驻服务或新调度器。Server 故障仍由原 formation 租约恢复；事务失败不会留下半个频道或半套关系。运行中成员占用、Computer 离线及预算变化在后续原 Run 入队阶段再次验证。迁移只新增责任事件和必要索引，不推断历史，也不改变旧 Task 状态。验证覆盖权限、历史资格、Skill／Provider 缺口、预算、复用身份和 Memory 保留、不同频道关系 scope、幂等、无效计划回滚、显式新建兼容；实际 E2E 从 Lucy 真实 CLI 查询、建队、复用后 Task 执行到验收，核对 UI 和 PostgreSQL。

### 自动组队当前实施记录

迁移 75 为新发生的指派与认领保存责任事件；初始指派由 Task INSERT 触发器记录，第一次认领在共享 claimTaskTx 内与版本更新同事务记录，普通认领和消息转任务均覆盖。现有消息的明确提及数组直接复用。没有回填猜测的历史事件。

Lucy 新增 `solo team candidates -c <Lucy频道> -m <owner原消息> --template <id>`，通过真实 Run 凭据代理调用只读候选查询。查询与 Form 事务使用同一资格判断：owner、精确模板职责历史、配置中明确 Skill 包、Computer 授权／后端可用性、实际预算检查。排序先用可见合格／退回计数（固定加一平滑），再在任务组、模型和预算完全相同的单一可比组内参考成本，最后参考明确提及；原始计数与排除原因保留。成本汇总复用长期指标，包含相同组内关闭而未验收的失败 Task，缺组或混合条件时不提供成本排名。

Form 默认复用，`reuse_existing:false` 保留显式新建。可指定成员职责、Skill／模型约束和关系交接内容；配置冲突或预算缺口失败会回滚新频道。复用只增加频道 membership，不移动 home Channel、不复制身份或 Memory。已有 owner 在用户授权的建队事务里批准确切的频道关系，之后仍可通过合作约定撤回。新频道的公开建队消息移除旧工作的详细反馈，owner 的源请求结果与 formation 记录保留依据。前端已有建队卡片增加“成员选择依据”。原有手动模板入口沿用新建模式。

`/tmp/solo-formation-provision-unit.log` 实际 PostgreSQL 与真实文件生成检查通过：首次建队、再次复用相同 ID、home Channel 不变、频道关系与批准记录、RELATIONSHIPS.md 写入、幂等、公开消息不含旧工作详细证据、明确新建保留行为。`/tmp/solo-formation-full-go.log` 全量 Go 通过；TypeScript 与改动文件 ESLint 通过。实际组队反馈 E2E 已启动，结果尚待确认，不能提前写为通过。

待审阅：共享成员的 RELATIONSHIPS.md 是可变文件，GenerateForAgent 会生成该成员所有频道的关系；Run 启动另有固定的频道关系快照。需继续核对生成时的执行互斥和运行中文件边界，确保建队／改约定不会覆盖正在使用的精确快照。统一跨来源 Inbox 与调度审计尚未完成。Thinking 最近几轮仍在真实初始化中遇到 Claude 529；顺序初始化已修正“Run 尚未入队就认为空闲”的测试竞态，但完整 Thinking 回归仍未通过。

成本审阅补充：代码 Gate 运行刻意不写 LLM 预算账本，属于实际进程检查，Token 为已知 0；指标视图将其单独标记为 code_gate/process，计入执行历时，不当作模型用量缺失，也不使用 Reviewer 配置中的模型冒充执行过推理。普通 Run 的模型字段目前来自固定 Revision 的配置名称；Provider 使用别名时，它不等同于已解析的精确模型发布版本，后续需要从真实使用记录验证这一比较条件。

首次组队 E2E 已验证实际 Lucy CLI 的模板查询、候选查询、Form 与幂等重放，页面建队结果、真实任务执行、Memory 写入和人工验收；随后测试额外等待已被 Task 看板列移动卸载的弹窗关闭按钮，第二次组队尚未执行。已中止该轮并按 make 停止测试栈，保留其 Task／Run／验收记录；后续移除这一多余 UI 假设并给原生动作设置 30 秒超时。该轮不记为完整 E2E 通过。

### 关系文件执行边界：修正前设计

关系事实仍由数据库与 Team lockfile 持有，Server 不再写成员运行中的工作目录。原建队完成步骤改为校验可渲染的关系快照，兼容原结果字段与重试；daemon 在已有 acquireAgentTurn 之后写本次 Run 的精确频道关系，Session 配置摘要和执行收据继续校验同一内容。成员复用、关系增删和建队重放不会覆盖旧 Run 文件。此修正不增加缓存或迁移；重启后仍由持久 Run payload 重建，旧运行记录不改写。测试使用真实数据库和目录，确保建队／改关系不触及模拟已有的真实文件，再通过真实建队 E2E 验证运行时仍收到协议。

### 统一 Inbox：实施前架构

保留消息合并、审核、等待恢复与 Selection 的原始事实表，新增一张只保存尚未领取的普通 Task／Thinking／Artifact／入场工作的持久表，并用只读视图汇总所有可执行来源。不是复制 Task 或 Session：待办只引用原频道、Task、Thinking 节点及触发记录，领取后指向原 agent_runs；现有待跟进标记和未满足等待仍单独显示，不因已读或领取而自动解决。

所有生产派发在同一 Agent 的 PostgreSQL 事务锁下检查忙碌状态和汇总视图的首项；同级按最早待处理时间和稳定标识排序。直接提及／DM／审核／明确 Task 与 Thinking 请求为高优先级，普通订阅和入场问候在后。当前 Run 的人工纠正沿现有消息新鲜度路径进入，不新建并发 Run。所有领取仍使用原 Run、预算锁、固定 Revision、Session、Computer 投递与 daemon 执行锁。新待办创建时不预订 Token，领取时再检查当前权限、预算和运行配置。

普通待办不保存凭据、custom_env、Skill 文件或固定配置。领取时加载当前授权配置、最新 Thinking Handoff 与有效 Session；Run 创建、Task 关联和已配对 Computer payload 在同一事务提交，已领取后的故障由原 Run 恢复处理。未领取失败保留具体原因和退避时间，服务重启由原 5 秒扫描器继续；已撤销成员、已删除频道或过期 Task 责任不得继续派发。队列状态转换在数据库持久化，前端与 CLI 的 work 同时显示一致的排序、来源、等待与错误。

原手动消息语义、同 Thread 合并、当前 Run 不抢占、Thinking 原分支与 Return 所有权继续保留。迁移增表／只读视图和索引；回退须先处理未领取工作，不能静默丢弃。验证需要真实数据库的跨来源顺序、并发领取、预算失败回滚、权限撤销及重启恢复；实际 E2E 同一 Agent 依次执行 Task、DM／提及和审核，检查后端开始／结束区间、原生待办 UI、最终回复及数据库持久状态。

### Inbox 当前实现与验证记录

迁移 76 新增 agent_pending_work；agent_inbox 与 agent_inbox_heads 从原消息、审核、条件恢复、Selection 和新普通待办投影出相同顺序。原生产 Task、Thinking、Artifact 与问候入口先持久入队，已有 5 秒扫描器领取；没有追加一个常驻服务或进程内工作队列。领取时重读当前 Task 和 Thinking Handoff、权限、配置与预算，和 Run／Task 关联／配对 Computer payload 同事务提交。失败保留退避与原因。前端原 Agent 面板与 `solo work list` 显示同一队列；复用中的关系文件现由 daemon 独占落盘。近期实际 Review 与协作观察也加入同一 work 入口。

`/tmp/solo-formation-final-e2e.log`：真实组队闭环 1 passed（8.6 分钟），验证 CLI 查询与 Form、验收后再次复用相同成员、保留 Memory、跨频道独立 Session 及数据库事实。此通过早于关系写入边界和 Inbox 改动，不能替代后续回归。

`/tmp/solo-inbox-serial-full-go.log`：全量 Go 通过，service 6.597s、pkg/agent 16.114s。包含跨来源排序、8 路并发只有一个领取者、无 Computer 时持久退避、新 Service 实例恢复、移除成员后的取消、Code Gate 零 Token 与执行历时。前两轮失败分别来自旧提示断言及测试调用会复用同频道的 fixture，并发现额外异步领取与扫描器重复；已移除额外异步入口、交由既有扫描器统一推进。类型检查与新 Inbox E2E 文件 ESLint 通过。

Thinking `/tmp/solo-thinking-inbox-e2e.log` 本轮已通过真实初始化、原 Session 延续、旧 CLI 节点路由、分叉交接和父节点 Return 限制，随后停在画布外子节点的鼠标定位。该轮中止并 make stop，不记作完整通过；已改用原生 Current branch 选择框并给 UI 动作设置 30 秒上限。完整 Thinking 回归仍须重跑。

后续审阅补充：明确 checkpoint／return／artifact 请求各自保留独立待办，避免相同协议文本使后一次有效操作被旧记录吞掉；取消尚未执行的 Return 会释放原节点返回锁。静音与消息领取共享事务锁，清理未领取自动消息；恢复或旧路由结果再次入队时检查已持久化的静音政策，不改变 Task、已执行 Run 或消息原文。上述补充晚于 serial-full-go，须单独复核与再跑完整回归。

### Agent 协作提案：实施前边界设计

原 agent_relationship_proposals.proposed_by 外键只引用用户，虽然服务允许频道内 Agent 发起，实际 INSERT 会失败。保留该用户归属与既有 owner 审批字段，新增可空 proposed_by_agent_id 记录代该 owner 发起的实际 Agent；旧提案保持原样，不伪造发起历史。Agent 只能提出待审提案，不能用自己的身份批准；只能撤回自己仍待审的提案，已生效协议继续由原 owner 权限控制。Agent 硬删除后保留原 owner 与提案记录、清空失效 Agent 引用；常规停用继续保留完整身份。

CLI 的 team agreements／propose-agreement 通过既有 Run 代理读取／提交到原频道 API，权限、scope 和相关 owner 审批均复用原服务。前端原合作约定显示实际 Agent 发起者，审批成功后再影响后续 Run 关系快照。Server／daemon 重启不改变待审状态。迁移回退前应先处理 Agent 发起的待审提案并导出新增发起身份记录；原用户提案与批准记录不更改。验证覆盖 Agent 提出、禁止自我批准、待审撤回、双方 owner 批准和真实 CLI→页面审批→关系数据库记录。

### 问候去重的兼容性修正

普通问候仍沿统一待办派发，但其去重身份需要包含 channel_members.joined_at。同一次加入的重复触发保留一个待办；退出后重新加入属于新的成员生命周期，应能再次问候。仅在问候入队时读取现有成员记录，不增加表、API 或前端状态；成员权限撤销仍由原领取检查取消。真实 PostgreSQL 检查覆盖重复触发与重新加入，现有频道问候 E2E 覆盖实际运行。

### Codex 登录 shell 与注入 CLI：修正前设计

真实 E2E 的 Codex exec_command 默认登录 shell 重新加载机器 PATH，选中了旧全局 solo；daemon 已注入的新 CLI 在 Agent workspace，未被正确使用。Codex 使用原生 allow_login_shell=false，可使省略 login 的工具默认普通 shell，并拒绝 login=true；该字段来自 [官方配置文档](https://learn.chatgpt.com/docs/config-file/config-reference)。仅当 ExecuteOptions.Env 带 Solo Agent 身份时提供此默认值，保留现有 ExtraArgs／CustomArgs 显式覆盖顺序，其他 provider 和独立 SDK 调用不变。沿用统一 buildCodexArgs 覆盖持久与单次执行，不改全局 CLI、用户配置、权限、数据库或前端。新启动／重启的 Session 生效；旧运行不改写。单测验证作用条件和覆盖顺序，真实 Codex E2E 验证注入 CLI 的新任务命令与持久 Session。

### 退出订阅与待办领取：修正前设计

已有路由会检查 followed=false，但旧路由结果重入队和既有消息待办领取只检查 nothing，可能在退出 Thread 后继续启动普通消息。沿用 agent-work 事务锁：退出订阅和清理本 Thread 的非显式待办一起提交；直接领取、恢复入队和扫描领取共享当前策略／订阅检查。明确提及、DM 责任保留，nothing 仍阻止所有自动消息；Task 和已开始 Run 不改变。原 API／CLI 入口与响应不变，不增加表或扫描器。真实数据库覆盖退出后清理、旧路由重试不能恢复普通唤醒、明确提及仍可入队及回复恢复订阅。

### Thinking 的跨后端进程验收

原 E2E 用 Claude 专属进程结束日志作断言，不能直接用于 Codex。保留实际关闭方法，为 Codex 记录 provider Session ID 与进程是否已回收；测试按实际 provider 匹配，并要求 Codex reaped=true。只增加可观察记录，不改 Session 生命周期、权限或数据库。Codex 的模型连接重试较慢，完整 Thinking 场景使用更长的测试总期限，单个 UI 动作仍限制 30 秒，不能将超时当作成功。

### 排队期间取消认领与 Artifact 兼容：修正前设计

普通 Task 入队时记录当时的负责人。领取时若原来已有负责人而现在被取消，旧指派待办应取消；原本未认领的广播仍可由现有候选阅读并认领。其他内容修改继续使用领取时的权威 Task，不回放旧目标。责任字段复用内部待办 payload，仅属调度元数据，不改变公开 API 或表；旧 payload 无此字段时保留兼容行为。

Artifact 是原任务的展示请求，不等于认领任务。领取时按已有 findArtifactLeader（活跃负责人优先，否则活跃创建者）重选并核对同一 Agent，保留由 Agent 创建、人工认领的任务的展示路径，且不因条件等待或任务结束误取消渲染。普通 Task 的状态／负责人／等待约束不弱化。真实数据库分别验证取消认领后不启动、人工负责 Task 的合法 Artifact 请求仍留在离线待办中；原 Run 和授权边界不变。

### 跨 Workspace 对照试验：修正前设计

Selection 的所有权仍属于原 Agent owner，三方 Revision、预算和决定保持原模型；试验频道与 Task 归属于发起请求的已授权 Workspace。创建时复用请求 scope，并在服务层验证 owner 的 Workspace 成员身份与原 Agent 的参与关系；无请求 scope 的内部调用继续使用 home Workspace。原 Agent 身份、home、Memory 与已存在的 Session 不迁移。幂等重放必须匹配原试验 Workspace，列表按当前 scope 过滤，防止页面拿到无法打开的其他空间 Task。复用试验频道的 workspace_id，不新增表或公开参数；已有试验保留原位置，故障与重启继续由原持久试验调度恢复。前端继续使用现有 Workspace 选择与 Task 页面，不引入第二份客户端状态。真实 PostgreSQL 检查空间、授权和幂等边界；Skill E2E 将旧成员带入另一个 Workspace 后完成实际三方运行、页面验收、发布与回退。

### Revision 后端兼容：修正前设计

审阅发现候选版本的服务端与页面各自列了旧后端清单，遗漏主线已注册的 OpenClaw、Hermes、Kimi 等，导致这些成员无法创建候选。版本 API 改为查询原 GlobalRegistry，页面复用已有 useBackendMeta；原 API 型 openai／anthropic 继续兼容。所有权、配置摘要、发布、Session 与失败恢复不改变，也不增加注册表、依赖或迁移。真实数据库逐一创建已注册后端的候选并拒绝未知后端；Skill E2E 从真实接口核对页面可选项，再验证实际已安装后端的完整流程。
# 2026-09-09：原库历史数据兼容补充设计

原库停在 63：三个历史 Task 共用一条来源消息，150 条旧合作关系的 from Agent 没有 home channel。旧记录合法存续，不删 Task、不清空 message_id、不推定关系所属频道。

- 领域与持久化：只给历史重复 Task 增加内部 `legacy_message_source` 标记，原 ID、编号、内容、负责人、状态与来源不变。普通 Task 保持消息唯一索引；触发器禁止新 Task 借用旧重复来源或自行开启兼容标记，变更旧来源后自动退出兼容。消息引用查到多个 Task 返回现有 409 歧义错误，编号／UUID 操作不变。
- 合作关系：允许升级前留下的 NULL channel_id 存续；既有列表本就排除没有 home 的关系，继续保留该可见性。新插入或变更频道仍必须通过所属频道、成员与双方 owner 授权校验，原版 34 明确保留全局 Agent 的运行行为，因此通过只读 `agent_relationship_scopes` 视图将旧关系投影到双方同 owner、仍共同参与的频道；不写回归属。派发、Thinking、关系文档、团队 Lockfile 和环检测复用该视图；显式频道约定优先。移除成员后旧投影立即消失，跨 owner 不继承全局约定。
- 数据流与前端：沿用消息锁、Task 服务、现有错误响应与任务卡；不新增产品前端入口。Server、daemon、CLI 和 Runtime 仍走现有路径，修复只改变升级兼容与歧义拒绝行为。
- 部署与恢复：修正尚未发布的 68、70 迁移，使 63 能升级；新增 78 修补已到 77 的环境。78 的回退保留兼容修复，直到回退所属的 70／68 功能迁移；这些 down 不删除旧业务记录。原库先备份，在恢复出的独立副本上升级、逐列验证旧数据及账号密码摘要未变，再做真实 PostgreSQL 并发与权限检查、实际页面和 Runtime 回归。副本扫描器不得触发原用户任务；实际 E2E 使用独立测试库与 Computer。
- 切换：验证通过后停止旧工作树与测试工作树的 make 托管服务，刷新原库备份，再用当前工作树 `make rebuild` 连接原 `solo` 数据库和 3000／8080／8081，沿用原 Computer 的正常本地配置文件路径，不提取其他进程或凭据文件里的秘密。账号密码不变，登录会话可能需要重新登录。
