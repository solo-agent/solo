# Step 5 — Visualization: 拖拽关系编辑器

> 依赖 Step 1-2 | 启动条件: Agent 数量 10+ 且 Step 2.5 只读图谱验证有价值
> 预估工期: 2-3 weeks | 新增依赖: `@xyflow/react`

---

## 1. 触发条件与风险评估

### 1.1 启动前检查

| 条件 | 验证方式 |
|------|---------|
| Agent 数量 >= 10 | `SELECT COUNT(*) FROM agents WHERE is_active = true` |
| 关系数据 >= 20 条 | `SELECT COUNT(*) FROM agent_relationships` |
| Step 2.5 SVG 图谱有使用 | 前端埋点或日志分析 |
| 用户反馈"需要拖拽编辑" | 至少 2 个独立反馈 |

### 1.2 不做拖拽编辑的备选

如果 Step 2.5 的只读 SVG 图谱已满足需求，本 Step 可降级为:
- 增强 SVG 图谱的交互（tooltip、筛选、搜索）
- 不引入 ReactFlow 依赖

---

## 2. 前端组件架构

### 2.1 依赖

```bash
cd frontend && npm install @xyflow/react
```

### 2.2 组件树

```
app/relationships/
├── page.tsx                          # 入口页面
└── components/
    ├── RelationshipEditor.tsx        # ReactFlow 容器
    ├── RelationshipNode.tsx          # 自定义节点 (Agent 头像 + 名称 + 状态)
    ├── RelationshipEdge.tsx          # 自定义边 (4 种视觉区分)
    ├── RelationshipToolbar.tsx       # 工具栏 (添加关系、筛选类型、导出)
    ├── CreateRelationshipModal.tsx   # 创建关系弹窗
    └── RelationshipDetailPanel.tsx   # 点击边的详情面板
```

### 2.3 视觉规范 — 4 种关系

```
reports_to:         实线 + vertical  →  stroke="#4A90D9" (蓝)
delegates_to:       实线 + horizontal → stroke="#7B6CF6" (紫)
collaborates_with:  虚线 + bidirectional → stroke="#10B981" (绿)
escalates_to:       双实线 + 红色 → stroke="#EF4444" (红) + animate="pulse"
```

ReactFlow 自定义边实现:

```tsx
// components/relationships/RelationshipEdge.tsx
import { BaseEdge, EdgeLabelRenderer, getBezierPath, type EdgeProps } from '@xyflow/react';

const EDGE_STYLES: Record<RelationshipType, { stroke: string; dash: string; width: number }> = {
  reports_to:       { stroke: '#4A90D9', dash: '', width: 2 },
  delegates_to:     { stroke: '#7B6CF6', dash: '', width: 1.5 },
  collaborates_with: { stroke: '#10B981', dash: '6,4', width: 1.5 },
  escalates_to:     { stroke: '#EF4444', dash: '', width: 3 },
};

export function RelationshipEdge({ id, sourceX, sourceY, targetX, targetY, data, markerEnd }: EdgeProps) {
  const [edgePath, labelX, labelY] = getBezierPath({ sourceX, sourceY, targetX, targetY });
  const style = EDGE_STYLES[data.relType as RelationshipType] || EDGE_STYLES.collaborates_with;

  return (
    <>
      <BaseEdge id={id} path={edgePath} style={{ stroke: style.stroke, strokeWidth: style.width, strokeDasharray: style.dash }} />
      <EdgeLabelRenderer>
        <div style={{ position: 'absolute', transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`, pointerEvents: 'all' }}
             className="text-xs px-2 py-0.5 rounded-full bg-gray-800 text-gray-300">
          {data.relType.replace('_', ' ')}
        </div>
      </EdgeLabelRenderer>
    </>
  );
}
```

自定义节点:

```tsx
// components/relationships/RelationshipNode.tsx
import { Handle, Position } from '@xyflow/react';

export function RelationshipNode({ data }: { data: { agent: Agent } }) {
  const { agent } = data;
  return (
    <div className="px-4 py-3 rounded-lg border-2 border-gray-700 bg-gray-900 min-w-[140px] cursor-pointer">
      <Handle type="target" position={Position.Top} />
      <div className="flex items-center gap-2">
        <div className="w-8 h-8 rounded-full bg-blue-600 flex items-center justify-center text-white text-xs font-bold">
          {agent.name[0]}
        </div>
        <div>
          <div className="text-sm font-semibold text-gray-100">{agent.name}</div>
          <div className="text-xs text-gray-400">{agent.is_active ? 'online' : 'offline'}</div>
        </div>
      </div>
      <Handle type="source" position={Position.Bottom} />
    </div>
  );
}
```

---

## 3. 图谱编辑器核心功能

### 3.1 ReactFlow 容器

```tsx
// page.tsx
'use client';
import { useCallback, useMemo } from 'react';
import { ReactFlow, Background, Controls, MiniMap, addEdge, useNodesState, useEdgesState, Connection } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { RelationshipNode } from './components/RelationshipNode';
import { RelationshipEdge } from './components/RelationshipEdge';
import { useRelationships } from '@/lib/hooks/use-relationships';

const nodeTypes = { agentNode: RelationshipNode };
const edgeTypes = { relationship: RelationshipEdge };

export default function RelationshipEditorPage() {
  const { relationships, createRelationship, deleteRelationship } = useRelationships();

  const initialNodes = useMemo(() => {
    // 从 relationships 去重提取所有涉及的 agent
    const agentMap = new Map<string, Agent>();
    relationships.forEach(r => {
      agentMap.set(r.from_agent_id, { id: r.from_agent_id, name: r.from_name, /* ... */ });
      agentMap.set(r.to_agent_id, { id: r.to_agent_id, name: r.to_name, /* ... */ });
    });
    return Array.from(agentMap.values()).map((agent, i) => ({
      id: agent.id,
      type: 'agentNode',
      position: { x: (i % 4) * 220 + 50, y: Math.floor(i / 4) * 120 + 50 },
      data: { agent },
    }));
  }, [relationships]);

  const initialEdges = useMemo(() => {
    return relationships.map(r => ({
      id: r.id,
      source: r.from_agent_id,
      target: r.to_agent_id,
      type: 'relationship',
      data: { relType: r.rel_type },
    }));
  }, [relationships]);

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  const onConnect = useCallback(async (connection: Connection) => {
    // 弹窗选择关系类型
    const relType = await promptRelationshipType(); // 自定义弹窗
    if (!relType) return;

    // POST /api/v1/agents/{connection.source}/relationships
    await createRelationship({
      from_agent_id: connection.source,
      to_agent_id: connection.target,
      rel_type: relType,
    });

    setEdges((eds) => addEdge({
      ...connection,
      id: `edge-${Date.now()}`,
      type: 'relationship',
      data: { relType },
    }, eds));
  }, [setEdges, createRelationship]);

  const onEdgeClick = useCallback((_event: React.MouseEvent, edge: Edge) => {
    // 点击边 → 显示详情面板 → 可删除关系
    showDetailPanel(edge);
  }, []);

  return (
    <div className="w-full h-screen">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onEdgeClick={onEdgeClick}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        fitView
      >
        <Background />
        <Controls />
        <MiniMap />
      </ReactFlow>
    </div>
  );
}
```

### 3.2 拖拽创建关系流程

```
1. 用户从一个 Agent 节点的 Handle 拖出连接线
2. 释放到另一个 Agent 节点
3. 弹出关系类型选择弹窗:
   ┌──────────────────────────────┐
   │  Create Relationship          │
   │  From: Alice → To: Bob        │
   │                               │
   │  ○ reports_to                 │
   │  ○ delegates_to               │
   │  ○ collaborates_with          │
   │  ○ escalates_to               │
   │  [Channel: #frontend ▼]      │ ← delegates_to / collaborates_with 需要
   │                               │
   │  [Cancel]    [Create]         │
   └──────────────────────────────┘
4. API 调用 → 边出现在图上
```

### 3.3 删除关系

点击边 → 详情面板显示 → 点击删除 → `DELETE /api/v1/agents/{from}/relationships/{id}`

---

## 4. WebSocket 实时同步

### 4.1 事件流

```
Agent A 编辑关系
  → POST/PATCH/DELETE API
    → Server 处理
      → WebSocket broadcast "relationship_changed"
        → 其他在线用户实时更新 ReactFlow 图
```

### 4.2 前端订阅

```typescript
// hooks/use-relationships.ts
function useRelationships() {
  const [relationships, setRelationships] = useState<AgentRelationship[]>([]);

  useEffect(() => {
    // 初始加载
    apiClient.get<AgentRelationship[]>('/api/v1/relationships/graph').then(setRelationships);

    // WebSocket 订阅
    const ws = createWsConnection();
    ws.on('relationship_created', (data) => {
      setRelationships((prev) => [...prev, data]);
    });
    ws.on('relationship_deleted', (data) => {
      setRelationships((prev) => prev.filter(r => r.id !== data.id));
    });
  }, []);

  const createRelationship = async (req: CreateRelationshipRequest) => { /* ... */ };
  const deleteRelationship = async (id: string) => { /* ... */ };

  return { relationships, createRelationship, deleteRelationship };
}
```

### 4.3 WebSocket 事件扩展

在 `internal/realtime/envelope.go` 中新增事件类型常量:

```go
const (
    EventTypeRelationshipCreated = "relationship_created"
    EventTypeRelationshipDeleted = "relationship_deleted"
)
```

在 `RelationshipService.Create/Delete` 中广播:

```go
hub.Broadcast(realtime.Envelope(realtime.EventTypeRelationshipCreated, rel))
```

需要使用 `Broadcaster` 接口的 handler 需注入 hub。

---

## 5. 路由注册

在 `frontend/app/relationships/page.tsx` 以及 server 端无需新路由（关系 CRUD 已在 Step 1 完成）。

前端 NavBar 中新增入口:

```tsx
<NavItem href="/relationships" icon={<GitBranch />} label={t('relationships')} />
```

---

## 6. 性能考量

| 场景 | 策略 |
|------|------|
| Agent 数量 10-30 | ReactFlow 默认渲染，全部加载 |
| Agent 数量 30-100 | 虚拟化（仅渲染可视区节点） + 分页加载 |
| 关系线过多 | 过滤按钮：只显示某类型关系 |
| 拖拽卡顿 | `nodesDraggable` 去抖 + `transform` GPU 加速 |

---

## 7. 不引入的工作

以下功能明确推迟:
- 自动布局算法（dagre/elkjs）— 手动布局先行验证
- 关系历史版本 — 无用户需求信号
- 批量编辑模式 — 单关系编辑先行验证
- 导出为图片 — 浏览器截图已足够
