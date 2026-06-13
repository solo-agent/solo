# Step 4 — Knowledge: Tier 2 结构化知识库

> 依赖 Step 2.4（频道共享记忆落地）| 预估工期: 3-4 weeks
> 新增: `knowledge` 表 (pgvector)、语义检索 API、Agent 知识写入/读取

---

## 1. 基础设施 — pgvector

### 1.1 扩展安装

```sql
CREATE EXTENSION IF NOT EXISTS vector;
```

### 1.2 迁移 — knowledge 表

**文件**: `migrations/000031_create_knowledge.up.sql`

```sql
-- 先安装 pgvector 扩展（手动执行或放在迁移开头）
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE knowledge (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    author_agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    content         TEXT NOT NULL,
    tags            TEXT[] NOT NULL DEFAULT '{}',
    embedding       vector(1536),
    source          VARCHAR(50) NOT NULL DEFAULT 'manual',  -- manual | auto | import
    source_ref      TEXT,          -- 如 "decisions.md#line=15" 或 task_id
    view_count      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 全文搜索
CREATE INDEX idx_knowledge_fts ON knowledge USING GIN (to_tsvector('simple', title || ' ' || content));

-- 按频道和时间排序
CREATE INDEX idx_knowledge_channel_time ON knowledge(channel_id, created_at DESC);

-- 按标签过滤
CREATE INDEX idx_knowledge_tags ON knowledge USING GIN (tags);

-- 向量索引 (IVFFlat — 适合 10K-100K 行)
-- 初始阶段用精确搜索（行数少），超过 1000 行后创建向量索引
-- CREATE INDEX idx_knowledge_embedding ON knowledge USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
```

**文件**: `migrations/000031_create_knowledge.down.sql`

```sql
DROP TABLE IF EXISTS knowledge;
```

---

## 2. 数据模型

### 2.1 Go struct

```go
type KnowledgeEntry struct {
    ID            string    `json:"id"`
    ChannelID     string    `json:"channel_id"`
    AuthorAgentID string    `json:"author_agent_id"`
    AuthorName    string    `json:"author_name,omitempty"`
    Title         string    `json:"title"`
    Content       string    `json:"content"`
    Tags          []string  `json:"tags"`
    Source        string    `json:"source"`
    SourceRef     string    `json:"source_ref,omitempty"`
    ViewCount     int       `json:"view_count"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
    Similarity    float64   `json:"similarity,omitempty"` // 仅搜索时返回
}
```

### 2.2 知识来源

| Source | 含义 | 触发方式 |
|--------|------|---------|
| `manual` | Agent 显式写入 | `solo knowledge add` |
| `auto` | 系统自动提取 | decisions.md 解析器 |
| `import` | 从频道记忆导入 | `solo knowledge import` |

---

## 3. Embedding 生成

### 3.1 架构

```
Agent 写入知识
  → Server INSERT knowledge (embedding = NULL)
  → 异步 goroutine 调用 LLM Embedding API
    → UPDATE knowledge SET embedding = $1
```

### 3.2 Embedding Service

**文件**: `internal/server/service/embedding.go`

```go
type EmbeddingService struct {
    httpClient *http.Client
    apiKey     string
    baseURL    string
    model      string
}

func NewEmbeddingService() *EmbeddingService {
    return &EmbeddingService{
        httpClient: &http.Client{Timeout: 30 * time.Second},
        apiKey:     os.Getenv("OPENAI_API_KEY"),
        baseURL:    os.Getenv("EMBEDDING_BASE_URL"), // 默认 https://api.openai.com/v1
        model:      os.Getenv("EMBEDDING_MODEL"),     // 默认 text-embedding-3-small
    }
}

// GenerateEmbedding 调用 OpenAI-compatible embedding API。
func (s *EmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
    reqBody := map[string]interface{}{
        "model": s.model,
        "input": text,
    }
    body, _ := json.Marshal(reqBody)
    req, _ := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/embeddings", bytes.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+s.apiKey)
    req.Header.Set("Content-Type", "application/json")

    resp, err := s.httpClient.Do(req)
    if err != nil { return nil, err }
    defer resp.Body.Close()

    var result struct {
        Data []struct {
            Embedding []float32 `json:"embedding"`
        } `json:"data"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    if len(result.Data) == 0 {
        return nil, errors.New("no embedding returned")
    }
    return result.Data[0].Embedding, nil
}
```

### 3.3 异步 embedding 队列

在 `KnowledgeService.Create` 中:

```go
func (s *KnowledgeService) Create(ctx context.Context, channelID, authorAgentID string, req CreateKnowledgeRequest) (*KnowledgeEntry, error) {
    // INSERT with embedding = NULL
    // ...

    // 异步生成 embedding
    go func() {
        embedding, err := s.embedSvc.GenerateEmbedding(context.Background(), req.Title+"\n"+req.Content)
        if err != nil {
            slog.Warn("embedding generation failed", "knowledge_id", id, "error", err)
            return
        }
        // 使用 pgvector 的 vector 类型
        vecStr := vectorToString(embedding)
        s.pool.Exec(context.Background(),
            `UPDATE knowledge SET embedding = $1 WHERE id = $2`, vecStr, id)
    }()

    return entry, nil
}

func vectorToString(v []float32) string {
    parts := make([]string, len(v))
    for i, f := range v {
        parts[i] = fmt.Sprintf("%f", f)
    }
    return "[" + strings.Join(parts, ",") + "]"
}
```

---

## 4. API

### 4.1 REST Endpoints

| Method | Path | 说明 |
|--------|------|------|
| POST   | `/api/v1/knowledge` | 创建知识条目 |
| GET    | `/api/v1/knowledge/search` | 搜索（全文 + 语义） |
| GET    | `/api/v1/knowledge/{id}` | 获取条目详情 |
| PATCH  | `/api/v1/knowledge/{id}` | 更新条目 |
| DELETE | `/api/v1/knowledge/{id}` | 删除条目 |
| GET    | `/api/v1/knowledge` | 列表（按频道/tag 过滤） |

#### POST /api/v1/knowledge

```json
// Request
{
    "channel_id": "uuid",
    "title": "Why we chose PostgreSQL over MongoDB",
    "content": "PostgreSQL gives us ACID transactions, pgvector for semantic search...",
    "tags": ["database", "decision", "architecture"],
    "source": "manual"
}

// Response 201
{
    "id": "uuid",
    "channel_id": "uuid",
    "author_agent_id": "uuid",
    "author_name": "Alice",
    "title": "Why we chose PostgreSQL over MongoDB",
    "content": "...",
    "tags": ["database", "decision", "architecture"],
    "source": "manual",
    "created_at": "2026-06-13T10:00:00Z"
}
```

#### GET /api/v1/knowledge/search?q=...&channel_id=...&top_k=5

支持三种搜索模式:

1. **纯语义搜索** (有 embedding 且 q 非空)
2. **全文搜索** (FTS，fallback)
3. **混合搜索** (embedding + FTS，推荐)

```json
// Response 200
{
    "results": [
        {
            "id": "uuid",
            "title": "...",
            "content_preview": "PostgreSQL gives us...",
            "similarity": 0.85,
            "match_type": "semantic"
        }
    ],
    "total": 3
}
```

#### 混合搜索 SQL

```sql
-- 在 embedding 存在时使用语义搜索
SELECT k.id, k.title, k.content, k.tags, 1 - (k.embedding <=> $1::vector) AS similarity
FROM knowledge k
WHERE k.channel_id = $2
  AND k.embedding IS NOT NULL
  AND 1 - (k.embedding <=> $1::vector) > 0.7  -- 相似度阈值
ORDER BY k.embedding <=> $1::vector
LIMIT $3;

-- 全文搜索 fallback (同时进行)
SELECT k.id, k.title, k.content, k.tags,
       ts_rank(to_tsvector('simple', k.title || ' ' || k.content), plainto_tsquery('simple', $1)) AS rank
FROM knowledge k
WHERE k.channel_id = $2
  AND to_tsvector('simple', k.title || ' ' || k.content) @@ plainto_tsquery('simple', $1)
ORDER BY rank DESC
LIMIT $3;
```

### 4.2 Service

**文件**: `internal/server/service/knowledge.go`

```go
type KnowledgeService struct {
    pool     *pgxpool.Pool
    embedSvc *EmbeddingService
}

func NewKnowledgeService(pool *pgxpool.Pool, embedSvc *EmbeddingService) *KnowledgeService {
    return &KnowledgeService{pool: pool, embedSvc: embedSvc}
}

type CreateKnowledgeRequest struct {
    ChannelID string   `json:"channel_id"`
    Title     string   `json:"title"`
    Content   string   `json:"content"`
    Tags      []string `json:"tags,omitempty"`
    Source    string   `json:"source,omitempty"`
}

func (s *KnowledgeService) Create(ctx context.Context, authorAgentID string, req CreateKnowledgeRequest) (*KnowledgeEntry, error) { /* ... */ }

func (s *KnowledgeService) Search(ctx context.Context, channelID, query string, topK int) ([]KnowledgeEntry, error) {
    // 1. 尝试生成 query embedding
    // 2. 语义搜索
    // 3. 全文搜索做补充
    // 4. 合并去重，按相似度排序
}

func (s *KnowledgeService) Get(ctx context.Context, id string) (*KnowledgeEntry, error) { /* ... */ }
func (s *KnowledgeService) Update(ctx context.Context, id, agentID string, req UpdateKnowledgeRequest) (*KnowledgeEntry, error) { /* ... */ }
func (s *KnowledgeService) Delete(ctx context.Context, id, agentID string) error { /* ... */ }
```

---

## 5. CLI

```bash
# 写入知识
solo knowledge add -c <channel_id> --title "决策: 选择 Postgres" \
    --content "我们选择 PostgreSQL 因为..." \
    --tags "decision,database"

# 搜索知识（Agent 在需要决策参考时使用）
solo knowledge search -c <channel_id> --q "为什么选了 PostgreSQL" [--top 5]

# 获取条目
solo knowledge get <id>

# 从 decisions.md 批量导入
solo knowledge import -c <channel_id> --source decisions.md
```

CLI 实现:

```go
case "knowledge":
    handleKnowledge(args[1:], baseURL, token)
```

---

## 6. Daemon 集成 — Agent 检索知识

Agent 在频道内处理任务时，daemon 可自动注入相关知识:

```go
func (h *daemonHandler) injectRelevantKnowledge(ctx context.Context, agentID, channelID, taskDescription string) string {
    // 任务描述 → embedding → 语义搜索
    embedSvc := NewEmbeddingService()
    embedding, err := embedSvc.GenerateEmbedding(ctx, taskDescription)
    if err != nil {
        return ""
    }

    rows, err := h.pool.Query(ctx, `
        SELECT title, content, 1 - (embedding <=> $1::vector) AS sim
        FROM knowledge
        WHERE channel_id = $2 AND embedding IS NOT NULL
        ORDER BY embedding <=> $1::vector
        LIMIT 3
    `, vectorToString(embedding), channelID)
    if err != nil { return "" }
    defer rows.Close()

    var sb strings.Builder
    sb.WriteString("\n## Relevant Knowledge from Channel\n\n")
    for rows.Next() {
        var title, content string
        var sim float64
        rows.Scan(&title, &content, &sim)
        sb.WriteString(fmt.Sprintf("### %s (relevance: %.0f%%)\n%s\n\n", title, sim*100, content))
    }
    return sb.String()
}
```

---

## 7. 从 decisions.md 自动导入

Step 2.4 的频道共享记忆中 `decisions.md` 包含追加式的决策记录。周期性任务可将它们结构化导入 `knowledge` 表:

```
decisions.md 格式:
---
## 2026-06-13: 选择 PostgreSQL 作为主数据库

理由: ACID, pgvector, JSONB...
替代方案: MongoDB, MySQL
影响: ...
---

解析器提取:
  - title: "选择 PostgreSQL 作为主数据库"
  - tags: ["decision", "database"]
  - source: "auto"
  - source_ref: "decisions.md#L15"
```

```go
func (s *KnowledgeService) ImportFromDecisions(ctx context.Context, channelID string) (int, error) {
    path := filepath.Join(channelMemoryRoot(), channelID, "memory", "decisions.md")
    data, err := os.ReadFile(path)
    if err != nil { return 0, err }

    // 解析 --- 区块
    entries := parseDecisionBlocks(string(data))
    count := 0
    for _, entry := range entries {
        _, err := s.Create(ctx, channelID, "system", CreateKnowledgeRequest{
            ChannelID: channelID,
            Title:     entry.Title,
            Content:   entry.Content,
            Tags:      []string{"decision"},
            Source:    "auto",
            SourceRef: entry.SourceRef,
        })
        if err == nil { count++ }
    }
    return count, nil
}
```

---

## 8. 前端 — 知识库面板

**组件位置**: `frontend/components/knowledge/knowledge-panel.tsx`

功能:
- 知识条目列表（按频道、tag 过滤）
- 搜索框（输入关键词 → API 搜索）
- 条目卡片（标题 + 内容预览 + tags + 相似度）
- 点击展开全文

---

## 9. WebSocket 事件

| 事件 | Payload |
|------|---------|
| `knowledge_created` | `{ id, title, channel_id, author }` |
| `knowledge_updated` | `{ id, title, channel_id }` |

---

## 10. 迁移清单

| 迁移 | 内容 |
|------|------|
| `000031` | knowledge 表 + pgvector 扩展 |

---

## 11. 环境变量

```
EMBEDDING_BASE_URL=https://api.openai.com/v1   # Embedding API base
EMBEDDING_MODEL=text-embedding-3-small         # Embedding model
EMBEDDING_API_KEY=<key>                        # API key (回退 OPENAI_API_KEY)
```

---

## 12. 初始行数与索引策略

- 预估首月 100-500 行/频道
- 低于 1000 行: 不需要 IVFFlat 索引，精确搜索即可
- 超过 1000 行: 在 `goose` 或手动脚本中创建 IVFFlat 索引
