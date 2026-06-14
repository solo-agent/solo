# Agent 协作功能 — 验收测试文档

> 分支: feat/collab-step1-foundation | 2026-06-14

---

## 前置条件

```bash
make rebuild
# 确认三个服务都在运行
curl http://localhost:3000   # 前端
curl http://localhost:8081/health  # Daemon
```

---

## 一、Agent 关系管理（Step 1）

### 1.1 创建关系
1. 打开 http://localhost:3000/relationships/manage
2. 点击 "Create Relationship" 按钮
3. 选择 From Agent / To Agent
4. 选择关系类型 `reports_to`
5. 点击 Create
- [ ] 列表出现新关系，类型 badge 为蓝色

### 1.2 频道级关系
1. 创建 `delegates_to` 关系
2. 不选 Channel，点击 Create
- [ ] 报错提示 "requires a channel_id"
3. 选择一个 Channel，点击 Create
- [ ] 创建成功

### 1.3 过滤器
1. 在 filter 下拉中选择 `collaborates_with`
- [ ] 列表只显示该类型的关系

### 1.4 删除关系
1. 点击某条关系的删除按钮
2. 确认弹窗中点击确认
- [ ] 关系从列表消失

### 1.5 关系图谱（ReactFlow）
1. 打开 http://localhost:3000/relationships
- [ ] 看到 Agent 节点和关系连线
- [ ] 4 种关系线型不同（实线蓝=reports_to, 实线紫=delegates_to, 虚线绿=collaborates_with, 双线红=escalates_to）
- [ ] 可拖拽节点
- [ ] 可滚轮缩放
- [ ] 从节点 handle 拖到另一个节点 → 弹出类型选择器

### 1.6 API 直接验证
```bash
# 创建关系
curl -X POST http://localhost:8080/api/v1/agent-relationships \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"from_agent_id":"<id1>","to_agent_id":"<id2>","rel_type":"reports_to"}'
# 预期 201

# 查询关系
curl http://localhost:8080/api/v1/agent-relationships \
  -H "Authorization: Bearer <token>"
# 预期 200，返回数组
```
- [ ] 创建返回 201
- [ ] 查询返回 200

---

## 二、任务依赖（Step 1-2）

### 2.1 查看依赖状态
1. 打开任意 Channel 的 Task Board
2. 如果存在被阻塞的 task，观察
- [ ] 卡片显示 "Blocked — N dependencies" 红色 badge
- [ ] 卡片半透明（opacity-60）

### 2.2 添加依赖（UI）
1. 点击一个 task card，打开 ThreadPanel
- [ ] 卡片下方出现 "Dependencies" 区域
- [ ] 显示 "Blocked by" 和 "Blocks" 列表
2. 点击 "Add" 按钮
3. 搜索另一个 task
4. 点击 "Blocks this" 按钮
- [ ] 依赖添加成功，"Blocked by" 列表更新

### 2.3 删除依赖（UI）
1. 在 Dependencies 区域点击某个依赖项的 X 按钮
- [ ] 依赖被移除

### 2.4 CLI 验证
```bash
# 阻塞任务
solo task block -n <N> --on <M> -c <channel_id>
# 预期: 返回依赖信息 JSON

# 查看是否被阻塞
solo task blocked -n <N> -c <channel_id>
# 预期: {"blocked": true}
```
- [ ] block 成功
- [ ] blocked 返回 true

---

## 三、Agent 委托（Step 1-2）

### 3.1 CLI 委托流程
```bash
# 创建委托
solo agent delegate --to @<agent-name> --task <N> --msg "请处理这个任务" -c <channel_id>
# 预期: 返回 delegation JSON，status=queued

# 查看收到的委托
solo agent delegate list --status queued
# 预期: 列出待处理的委托

# 接受委托
solo agent delegate accept <delegation-id>
# 预期: status 变为 started

# 完成委托
solo agent delegate complete <delegation-id>
# 预期: status 变为 completed
```
- [ ] 创建返回 queued
- [ ] list 能看到
- [ ] accept 变 started
- [ ] complete 变 completed

### 3.2 拒绝委托
```bash
solo agent delegate reject <delegation-id> --reason "不擅长这个"
# 预期: status=rejected, rejection_reason="不擅长这个"
```
- [ ] 拒绝成功，reason 正确

---

## 四、频道共享记忆（Step 2）

### 4.1 查看/编辑 CHANNEL.md
1. 打开任意 Channel
2. 点击工具栏的 FileText 图标（Channel Memory）
- [ ] 弹出对话框，显示 CHANNEL.md 内容
3. 编辑内容，点击 Save
- [ ] 保存成功

### 4.2 查看 decisions.md
1. 在 Channel Memory 对话框中
- [ ] 有 "Decisions" 标签或区域，列出决策记录

### 4.3 CLI 验证
```bash
solo channel memory -c <channel_id>
# 预期: 显示 CHANNEL.md 内容
```
- [ ] 能读到共享记忆

---

## 五、频道工作区绑定（Step 3）

### 5.1 绑定项目
1. 打开任意 Channel
2. 点击工具栏的 GitBranch 图标（Channel Binding）
- [ ] 弹出绑定对话框
3. 输入 repo URL，点击 Bind
- [ ] 显示 "Binding successful" + repo 信息

### 5.2 查看文件树
1. 绑定成功后，展开 Workspace Files 区域
- [ ] 显示文件树，可展开目录
- [ ] 点击文件可查看内容

### 5.3 解绑
1. 点击 Unbind 按钮
- [ ] 绑定信息清除

---

## 六、知识库（Step 4）

### 6.1 创建知识条目
1. 在 Channel 中点击 BookOpen 图标（Knowledge）
2. 点击 "+" 创建新条目
3. 填写 Title 和 Content，选择 Channel
4. 点击 Create
- [ ] 创建成功，条目出现在列表中

### 6.2 搜索知识
1. 在搜索框中输入关键词
- [ ] 返回相关结果
2. 不选 Channel，直接搜索
- [ ] 跨频道搜索正常工作

### 6.3 Tag 过滤
1. 点击某个 tag chip
- [ ] 列表只显示包含该 tag 的条目
2. 点击 "Clear filter"
- [ ] 恢复显示全部

### 6.4 查看/编辑详情
1. 点击某条知识条目
- [ ] 弹出详情对话框，显示完整内容
2. 点击 Edit 按钮
- [ ] 进入编辑模式，可修改标题/内容/tag
3. 保存修改
- [ ] 更新成功

---

## 七、提醒系统（Step 6）

### 7.1 创建提醒
1. 在 Channel 中点击 Bell 图标（Reminders）
2. 点击 Create Reminder
3. 填写标题，选择时间，设置重复（可选）
4. 点击 Create
- [ ] 提醒出现在列表中
- [ ] 日期输入框显示英文标签（不出现日语/中文）

---

## 八、看门狗（Step 6）

### 8.1 查看看门狗状态
1. 在 Channel 中点击 Shield 图标（Watchdog）
- [ ] 显示 "0 Overdue / All on track"（无逾期任务时）
- [ ] 或有逾期任务列表（如有）

---

## 九、蜂群任务（Step 6）

### 9.1 创建蜂群任务
```bash
solo task create --title "重构用户系统" --swarm -c <channel_id>
# 预期: 创建成功，自动分解为子任务
```

### 9.2 查看蜂群状态
1. 在 Task Board 中点击蜂群任务
2. 在 ThreadPanel 中点击 "Swarm" badge
- [ ] 弹出 Swarm 状态面板，显示子任务列表和进度条

---

## 十、API 健康检查

```bash
# 所有协作 API 应响应（200 或 401）
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/v1/agent-relationships
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/v1/task-dependencies
curl -s -o /dev/null -w "%{http_code}" "http://localhost:8080/api/v1/agent-delegations/incoming?agent_id=test"
curl -s -o /dev/null -w "%{http_code}" "http://localhost:8080/api/v1/knowledge/search?q=test"
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/v1/reminders
```
- [ ] 所有 API 返回非 404

---

## 验收标准汇总

| # | 场景 | 关键验证点 |
|---|------|-----------|
| 1 | Agent 关系 CRUD | 创建/过滤/删除正常，4 种类型区分 |
| 2 | ReactFlow 图谱 | 节点可见、连线可拖拽创建、缩放 |
| 3 | 任务依赖 UI | 能添加/删除依赖，blocked 卡片灰显 |
| 4 | Agent 委托 CLI | create→accept→complete 全流程 |
| 5 | 频道共享记忆 | 查看/编辑 CHANNEL.md |
| 6 | 频道工作区 | 绑定 repo、查看文件树 |
| 7 | 知识库 | 创建/搜索/编辑/tag 过滤/跨频道搜索 |
| 8 | 提醒 | 创建提醒、英文日期标签 |
| 9 | 看门狗 | 面板可用 |
| 10 | API | 所有端点非 404 |
