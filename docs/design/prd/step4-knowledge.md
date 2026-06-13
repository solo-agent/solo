# PRD — Step 4: Knowledge（Tier 2 结构化知识库）

> 版本: v1.0 | 日期: 2026-06-13 | 状态: Draft
> 前置依赖: [Step 1](step1-foundation.md) · [Step 2](step2-coordination.md) · [Step 3](step3-workspace.md)
> 关联文档: [Agent 协作路线图](../agent-collaboration-roadmap.md)

---

## 1. 产品背景与目标

### 1.1 现状（Step 3 完成后）

Agent 们在频道内协作开发，共享代码、委托任务、隔离工作区。频道共享记忆（CHANNEL.md + decisions.md）作为纯文本元数据已经积累了技术决策和频道上下文。但存在两个明显瓶颈：

1. **检索靠 grep**：decisions.md 是追加式文本，不能按语义检索。Agent 想找"之前我们怎么处理 JWT 过期的？"只能线性扫描。
2. **知识无法跨频道复用**：频道 A 的决策（"用 Redis 做 session store"）频道 B 完全看不到，即使两个频道共享技术栈。
3. **非 Agent 的知识流失**：Agent 离开频道后，它在 decisions.md 中的知识还在，但新 Agent 需要手动翻看历史才能获取上下文。

### 1.2 目标

在纯文本共享记忆之上，建立**结构化、可语义检索的 Tier 2 知识库**：

1. **pgvector 向量存储**：Agent 将重要决策和经验写入知识库，其他 Agent 通过自然语言语义检索
2. **跨频道知识发现**：Agent 可以跨频道搜索知识（权限允许范围内），避免重复踩坑
3. **知识沉淀闭环**：文本记忆（decisions.md）提供原始素材，知识库提供结构化索引
4. **Agent 主动学习**：新 Agent 加入频道时，自动检索该频道的 Top N 条相关知识注入 System Prompt

### 1.3 Step 4 完成后的协作体验

```
频道 #auth-service 中:

1. Backend Agent 解决了 JWT refresh token 轮换问题
   -> solo knowledge add --title "JWT Refresh Token 轮换方案" --content "使用双 token 机制..."
   -> 系统自动生成 embedding 存入 pgvector

2. 一周后，新 Agent @security-bot 加入频道
   -> System Prompt 自动注入频道 Top 5 相关知识
   -> @security-bot 无需翻 decisions.md 即获得上下文

3. 频道 #payment-service 的 Agent 遇到同样的 JWT 问题
   -> solo knowledge search "JWT refresh token 轮换"
   -> 系统返回 #auth-service 的知识条目（如果权限允许跨频道搜索）
   -> Agent 直接复用方案，不再踩坑

4. Product Manager 查询最近所有频道的技术决策
   -> solo knowledge list --channel '#auth-service' --since '2026-06-01'
   -> 结构化展示，支持按 Agent/时间/频道筛选
```

---

## 2. 用户故事地图

### 2.1 知识写入

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| K1 | P0 | 作为 Agent，我希望将重要决策或经验写入知识库，以便后续可被检索和复用 | M |
| K2 | P1 | 作为 Agent，当我在 decisions.md 中追加决策时，系统自动提示是否同步到知识库，以便减少手动操作 | M |
| K3 | P2 | 作为 Agent，我希望在完成任务时自动提取摘要写入知识库，以便关键经验不遗漏 | L |

### 2.2 知识检索

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| K4 | P0 | 作为 Agent，我希望用自然语言搜索知识库，以便获得语义相关而非仅关键词匹配的结果 | M |
| K5 | P0 | 作为 Agent，我希望能筛选搜索范围（频道/Agent/时间），以便精准定位所需知识 | S |
| K6 | P1 | 作为新 Agent 加入频道时，系统自动检索该频道的 Top N 知识注入 System Prompt，以便快速理解频道上下文 | M |
| K7 | P1 | 作为 Agent，跨频道搜索相关知识（需权限），以便复用其他团队的经验 | M |

### 2.3 知识管理

| ID | 优先级 | 故事 | 预估 |
|----|--------|------|------|
| K8 | P1 | 作为管理员，我可以编辑或标记过时知识条目，以便知识库保持高质量 | S |
| K9 | P2 | 作为管理员，我可以设置知识条目的可见范围（频道内 / 全局），以便保护敏感信息 | S |
| K10 | P2 | 作为 Agent，过时知识在检索结果中降低权重，以便优先获取最新信息 | M |

---

## 3. 功能详述

### 3.1 数据模型

```sql
CREATE TABLE knowledge (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    author_agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE SET NULL,
    title           TEXT NOT NULL,
    content         TEXT NOT NULL,
    tags            TEXT[] DEFAULT '{}',
    visibility      VARCHAR(20) NOT NULL DEFAULT 'channel',  -- channel | global
    status          VARCHAR(20) NOT NULL DEFAULT 'active',    -- active | deprecated | archived
    embedding       vector(1536),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_knowledge_channel ON knowledge(channel_id);
CREATE INDEX idx_knowledge_author ON knowledge(author_agent_id);
CREATE INDEX idx_knowledge_embedding ON knowledge USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
```

#### 字段说明

| 字段 | 说明 |
|------|------|
| `channel_id` | 知识所属频道（源频道） |
| `author_agent_id` | 知识作者（Agent） |
| `title` | 知识标题，摘要性质，用于列表展示和搜索匹配 |
| `content` | 知识正文，支持 Markdown，embedding 从此字段生成 |
| `tags` | 标签数组，辅助筛选（如 `["auth", "jwt", "security"]`） |
| `visibility` | `channel` = 仅本频道可搜索；`global` = 所有频道可搜索 |
| `status` | `active` = 有效；`deprecated` = 过时；`archived` = 归档 |
| `embedding` | 1536 维向量，从 content 生成（使用 text-embedding-3-small 或等效模型） |

### 3.2 Knowledge CLI

```bash
# 写入知识
solo knowledge add --title "JWT Refresh Token 轮换方案" \
  --content "使用双 token 机制：access token (15min) + refresh token (7d)。轮换时旧 refresh token 加入黑名单。" \
  --channel '#auth-service' \
  --tags "auth,jwt,security" \
  --visibility channel

# 语义搜索
solo knowledge search "JWT token 过期怎么处理"
  # 返回 Top 5 语义相关结果，按相似度排序

# 筛选搜索
solo knowledge search "redis 缓存" \
  --channel '#backend' \
  --author @alice \
  --since 2026-06-01 \
  --limit 10

# 列表查看
solo knowledge list --channel '#auth-service' --status active --limit 20
solo knowledge list --author @backend-bot --since "7 days ago"

# 查看详情
solo knowledge get <knowledge-id>
  # 输出完整 title, content, tags, 相似知识推荐

# 更新
solo knowledge update <knowledge-id> --content "更新内容..."
solo knowledge update <knowledge-id> --status deprecated --reason "已使用新方案替代"

# 删除
solo knowledge delete <knowledge-id>
```

### 3.3 语义搜索机制

```
搜索流程:
1. 用户输入查询文本
2. 系统调用 embedding 模型将查询转为 1536 维向量
3. pgvector 计算余弦相似度:
   SELECT *, 1 - (embedding <=> $query_embedding) AS similarity
   FROM knowledge
   WHERE (channel_id = $channel_id OR visibility = 'global')
     AND status = 'active'
   ORDER BY embedding <=> $query_embedding
   LIMIT $limit;
4. 返回结果附带相似度分数

ranking 加权规则:
  - active 条目: 权重 1.0
  - deprecated 条目: 权重 0.5（降低但不排除）
  - 同一频道的条目: 权重 x1.2（频道内优先）
  - 更新时间 7 天内: 权重 x1.1（新鲜度加分）
```

### 3.4 自动知识提取

#### Trigger 1: decisions.md 追加

```
GIVEN Agent 在 decisions.md 中追加了决策记录
WHEN 系统检测到 decisions.md 变化
THEN 系统向 Agent 发送低优先级提示:
  "检测到你在 #auth-service 中新增了决策，要写入知识库吗？
   [Y] 写入  [N] 忽略  [S] 稍后提醒"
  
选择 Y 时:
  -> 自动填充 title（从决策首行提取）
  -> 自动填充 content（完整决策记录）
  -> 自动填充 tags（从频道名和内容中抽取关键词）
  -> Agent 确认后写入
```

#### Trigger 2: 任务完成

```
GIVEN Agent 完成任务
WHEN 任务描述或关联 thread 中存在关键内容
THEN 系统建议:
  "Task #5 已完成。是否将关键经验写入知识库？"
  -> Agent 可编辑后保存或跳过
```

### 3.5 System Prompt 注入（新 Agent 加入频道）

```
Agent 加入频道时，System Prompt 注入格式:

[Channel Knowledge: #auth-service]
以下是从本频道知识库中检索到的 Top 5 相关知识:

1. JWT Refresh Token 轮换方案 (updated 2026-06-10)
   双 token 机制，access 15min + refresh 7d...

2. OAuth2 第三方登录集成 (updated 2026-06-05)
   支持 Google / GitHub / 飞书 三种 OAuth provider...

...
```

### 3.6 前端 — 知识库页面

#### 知识库列表 / 搜索

```
┌──────────────────────────────────────────────────────────┐
│ 知识库              [🔍 搜索知识...            ]  [频道 ▾]│
├──────────────────────────────────────────────────────────┤
│                                                          │
│  ┌── 筛选 ──────────────────────────────────────────┐   │
│  │ 频道: [全部 ▾] 作者: [全部 ▾]                     │   │
│  │ 标签: [auth] [jwt] [security] [+添加标签]         │   │
│  │ 状态: ☑ active  ☐ deprecated  ☐ archived        │   │
│  │ 时间: [最近 7 天 ▾]                               │   │
│  └────────────────────────────────────────────────────┘   │
│                                                          │
│  ┌────────────────────────────────────────────────────┐  │
│  │ 📄 JWT Refresh Token 轮换方案             92% 匹配 │  │
│  │ #auth-service · @backend-bot · 3 天前              │  │
│  │ 标签: auth jwt security                            │  │
│  │ 使用双 token 机制：access token (15min) + refre... │  │
│  └────────────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────┐  │
│  │ 📄 OAuth2 第三方登录集成                  87% 匹配 │  │
│  │ #auth-service · @backend-bot · 5 天前              │  │
│  │ 标签: auth oauth integration                       │  │
│  │ 支持 Google / GitHub / 飞书 三种 OAuth provider... │  │
│  └────────────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────┐  │
│  │ 📄 API Rate Limit 实现方案                78% 匹配 │  │
│  │ #backend · @alice · 1 周前                         │  │
│  │ 标签: api rate-limit middleware                    │  │
│  │ 使用 token bucket 算法，Redis 存储计数器...        │  │
│  └────────────────────────────────────────────────────┘  │
│                                                          │
│  共 23 条结果           [上一页] [1] [2] [3] [下一页]   │
└──────────────────────────────────────────────────────────┘
```

#### 知识详情页

```
┌──────────────────────────────────────────────────────────┐
│ < 返回知识库                                             │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  JWT Refresh Token 轮换方案                              │
│  ─────────────────────────────────────────────           │
│  频道: #auth-service    作者: @backend-bot               │
│  标签: auth jwt security                                 │
│  可见性: 频道内          状态: 🟢 active                 │
│  创建: 2026-06-10 14:30  更新: 2026-06-12 09:15          │
│  ─────────────────────────────────────────────           │
│                                                          │
│  使用双 token 机制：                                     │
│  - access token: 15 分钟有效期，承载用户身份              │
│  - refresh token: 7 天有效期，用于获取新 access token    │
│                                                          │
│  轮换策略:                                               │
│  1. 客户端用 refresh token 请求新 access token           │
│  2. 服务端下发新 access token + 新 refresh token         │
│  3. 旧 refresh token 加入 Redis 黑名单                   │
│  ...                                                     │
│                                                          │
│  ─────────────────────────────────────────────           │
│  相似知识:                                               │
│  📄 API Rate Limit 实现方案 (78%)                        │
│  📄 Session 管理最佳实践 (65%)                           │
│                                                          │
│  [编辑] [标记为过时] [删除]                              │
└──────────────────────────────────────────────────────────┘
```

---

## 4. 验收标准

### 4.1 知识写入

**K1 — 写入知识条目**

```
GIVEN Agent @backend-bot 在频道 #auth-service 中
WHEN 执行
  solo knowledge add --title "JWT 轮换方案" \
    --content "使用双 token 机制..." \
    --channel '#auth-service' \
    --tags "auth,jwt"
THEN 知识条目创建成功
  AND embedding 自动生成
  AND 返回知识 ID
  AND 可通过 solo knowledge get <id> 查询完整内容
```

**K2 — decisions.md 自动提示**

```
GIVEN Agent 在频道 #auth-service 的 decisions.md 中追加了决策
WHEN 系统检测到文件变化
THEN Agent 收到提示 "检测到新决策，要写入知识库吗？[Y/N/S]"
WHEN Agent 回复 Y
THEN 系统预填充 title/content/tags 并展示给 Agent 确认
  AND Agent 确认后写入 knowledge 表
```

**K3 — 任务完成自动提取**

```
GIVEN Task #5 完成，任务描述为 "实现 JWT refresh token 轮换"
  AND 关联 thread 中有详细讨论
WHEN 任务状态变为 completed
THEN Agent 收到建议 "是否将 Task #5 的关键经验写入知识库？"
  AND 建议包含预填充的摘要（从任务描述和 thread 提取）
```

### 4.2 知识检索

**K4 — 语义搜索**

```
GIVEN knowledge 表中有关于 JWT 和 OAuth 的条目
WHEN Agent 执行 solo knowledge search "token 过期怎么处理"
THEN 返回 Top 5 结果，按语义相似度排序
  AND 结果中包含 JWT 相关条目（即使内容中没有"过期处理"这个精确短语）
  AND 每条结果附带相似度分数
```

**K5 — 筛选搜索**

```
GIVEN knowledge 表中有多个频道的条目
WHEN Agent 执行 solo knowledge search "缓存" --channel '#backend' --author @alice
THEN 只返回 #backend 频道中 @alice 创建的与"缓存"语义相关的条目
```

**K6 — 新 Agent System Prompt 注入**

```
GIVEN 频道 #auth-service 知识库中有 12 条 active 知识
WHEN 新 Agent @security-bot 加入频道 #auth-service
THEN System Prompt 中注入该频道的 Top 5 知识（按更新时间 + 语义与频道上下文匹配度排序）
  AND 每条注入的知识包含 title、摘要（前 200 字）、更新时间
```

**K7 — 跨频道搜索**

```
GIVEN 频道 #auth-service 有一条 visibility=global 的知识"JWT 轮换方案"
  AND 频道 #payment-service 有一条 visibility=channel 的知识"支付回调安全"
WHEN @payment-bot 在 #payment-service 中搜索 "JWT"
THEN 返回 "JWT 轮换方案"（因为 visibility=global）
  AND 不返回其他频道的 channel 级私有知识
```

### 4.3 知识管理

**K8 — 标记过时**

```
GIVEN 知识条目 #K1 状态为 active
WHEN 管理员执行 solo knowledge update K1 --status deprecated --reason "已使用新方案替代"
THEN 条目状态变为 deprecated
  AND reason 保存到 updated_at 的变更记录中
  AND 搜索时该条目权重降为 0.5
```

**K9 — 可见性设置**

```
GIVEN 知识条目 #K1 visibility=channel
WHEN 频道 #payment-service 的 Agent 搜索
THEN 不返回 #K1（因为 visibility 限定为频道内）
WHEN 管理员修改 visibility 为 global
THEN 其他频道 Agent 搜索时可返回 #K1
```

**K10 — 过时知识权重降低**

```
GIVEN 搜索查询匹配到 3 条 active 和 2 条 deprecated 条目
WHEN 排序结果
THEN active 条目排在前面，deprecated 条目排在后面
  AND deprecated 条目标注"已过时"标记
```

---

## 5. 非功能需求

### 5.1 性能

| 指标 | 目标 |
|------|------|
| Embedding 生成 | < 2s / 条目（调用外部 API） |
| 语义搜索（1000 条知识） | < 200ms（pgvector + ivfflat 索引） |
| System Prompt 注入（Top 5） | < 100ms |
| 知识写入（含 embedding） | < 3s |
| pgvector 索引构建（首次 1000 条） | < 5 分钟 |

### 5.2 扩展性

- pgvector ivfflat 索引支持 10 万条知识无需重建
- embedding 生成支持批量处理（重建索引场景）
- 支持切换 embedding 模型（维度变更时需重建索引）

### 5.3 安全性

- 知识写入校验 Agent 是该频道成员
- 跨频道搜索只返回 visibility=global 的条目
- 知识删除需作者本人或管理员权限
- 知识内容不包含敏感信息过滤器（PII / API key 检测，给出 warning）

---

## 6. 范围边界

### 本次做

- knowledge 表 + pgvector 索引
- 知识 CRUD CLI（add/search/get/list/update/delete）
- 语义搜索（自然语言 -> embedding -> pgvector 余弦相似度）
- 筛选搜索（频道/作者/标签/时间/状态）
- 新 Agent 加入频道时自动注入 Top N 知识到 System Prompt
- 跨频道搜索（基于 visibility）
- 知识过时标记和权重降低
- decisions.md 追加 -> 提示写入知识库
- 前端知识库列表/搜索/详情页

### 本次不做

- 知识版本历史（暂时只有 updated_at）
- 知识审核工作流（Agent 写入直接生效）
- 知识自动合并/去重（两个 Agent 写相同主题时需手动合并）
- 知识图谱/关系可视化
- 知识到外部 Wiki 的同步
- Agent 主动学习（自动从对话中提取知识）——触发仍需 Agent 确认

---

## 7. 依赖与风险

### 依赖

| 依赖项 | 说明 | 风险等级 |
|--------|------|----------|
| pgvector 扩展 | PostgreSQL 需安装 pgvector 扩展 | 中 — 需运维配合 |
| Embedding 模型 API | OpenAI text-embedding-3-small 或等效 | 中 — 需 API key 和网络 |
| Step 2.4 共享记忆 | decisions.md 是知识库的原始素材来源 | 低 — 已实现 |

### 风险

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Embedding API 不可用 | 知识写入和搜索全部中断 | 写入降级为无 embedding（仅关键词搜索）；搜索失败时回退到 pg_trgm 模糊匹配 |
| pgvector 索引性能衰减 | 搜索变慢 | 定期 REINDEX；监控查询耗时 |
| 知识条目质量低（Agent 乱写） | 搜索噪声大 | status=active/deprecated 区分；管理员可标记过时 |
| Embedding 模型切换导致维度变化 | 历史 embedding 失效 | 提供 `solo knowledge reindex` 批量重建 embedding |
| 跨频道搜索的权限边界模糊 | 敏感知识泄露 | 严格执行 visibility 检查；频道私有知识不出频道 |

---

## 8. 成功指标

### 上线验证

- [ ] knowledge 表创建 + pgvector 索引构建成功
- [ ] solo knowledge add 写入并生成 embedding
- [ ] solo knowledge search 返回语义相关结果（相似度下降合理）
- [ ] 筛选搜索（频道/作者/标签/时间）正确
- [ ] 新 Agent 加入频道时 System Prompt 注入 Top 5 知识
- [ ] 跨频道搜索只返回 global 条目
- [ ] decisions.md 追加 -> 提示写入流程通过

### 30 天观察指标

| 指标 | 目标 | 数据来源 |
|------|------|----------|
| 知识库条目数 | >= 50 | knowledge 表 |
| 有至少 1 条知识的频道占比 | >= 30% | knowledge 表 |
| 语义搜索日均调用次数 | >= 20 | 搜索日志 |
| 搜索点击率（点击了某条结果） | >= 40% | 前端埋点 |
| decisions.md -> 知识库的转化率 | >= 20% | 提示:写入 比例 |
| 跨频道搜索占比 | >= 10% | 搜索日志 |

---

## 9. 迭代规划

### Sprint K-1（Week 1-2）：基础设施 + 写入

- [ ] 安装 pgvector 扩展
- [ ] 创建 knowledge 表 + ivfflat 索引
- [ ] 集成 embedding 生成 API
- [ ] 实现 solo knowledge add/list/get/update/delete
- [ ] 实现 decisions.md 追加 -> 提示写入

### Sprint K-2（Week 2-3）：搜索 + System Prompt

- [ ] 实现语义搜索（pgvector 余弦相似度）
- [ ] 实现筛选搜索（频道/作者/标签/时间/状态）
- [ ] 实现搜索 ranking 加权规则
- [ ] 实现新 Agent 加入频道时 System Prompt 注入 Top N
- [ ] 实现跨频道搜索（基于 visibility）
- [ ] 实现过时知识权重降低

### Sprint K-3（Week 3-4）：前端 + 集成测试

- [ ] 知识库列表/搜索页
- [ ] 知识详情页（含相似知识推荐）
- [ ] 知识写入编辑器
- [ ] embedding API 降级方案（回退到关键词搜索）
- [ ] solo knowledge reindex 批量重建命令
- [ ] 端到端测试 + 手动验收
