// ============================================================================
// Relationships Editor Page (Step 5)
// - ReactFlow-based drag-and-drop relationship graph
// - Create/delete relationships by connecting agent nodes
// - 4 edge types with distinct visuals
// - WebSocket sync for real-time collaboration
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  type Connection,
  type Edge,
  type Node,
  type NodeMouseHandler,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import dagre from 'dagre';
import { Loader2, Plus, Trash2, LayoutGrid, Undo2, Redo2 } from 'lucide-react';
import { NavBar } from '@/components/ui/navbar';
import { RelationshipNode } from '@/components/relationships/relationship-node';
import { RelationshipEdge } from '@/components/relationships/relationship-edge';
import { CreateRelationshipModal } from '@/components/relationships/create-relationship-modal';
import { RelationshipDetailPanel } from '@/components/relationships/relationship-detail-panel';
import { apiClient, ApiError } from '@/lib/api-client';
import { useWebSocket } from '@/lib/ws-context';
import { t } from '@/lib/i18n';
import type { Agent, AgentRelationship, RelationshipType } from '@/lib/types';
import { useAgents } from '@/lib/hooks/use-agents';

// ---- Node/Edge types ----

const NODE_TYPES = { agentNode: RelationshipNode };
const EDGE_TYPES = { relationship: RelationshipEdge };

// ---- Helpers ----

interface UndoEntry {
  nodes: Node[];
  edges: Edge[];
}

// ---- Page ----

export default function RelationshipsPage() {
  const { agents, isLoading: agentsLoading, refetch: refetchAgents } = useAgents();
  const [relationships, setRelationships] = useState<AgentRelationship[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [preselectedFrom, setPreselectedFrom] = useState<string | null>(null);
  const [preselectedTo, setPreselectedTo] = useState<string | null>(null);
  const [selectedEdge, setSelectedEdge] = useState<Edge | null>(null);
  const [detailRel, setDetailRel] = useState<AgentRelationship | null>(null);
  const [detailAgent, setDetailAgent] = useState<Agent | null>(null);
  const [undoStack, setUndoStack] = useState<UndoEntry[]>([]);
  const [redoStack, setRedoStack] = useState<UndoEntry[]>([]);
  const edgeToRelationshipMap = useRef<Map<string, AgentRelationship>>(new Map());

  // ---- Fetch data ----

  const loadData = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const rels = await apiClient.get<AgentRelationship[]>('/api/v1/agent-relationships');
      setRelationships(Array.isArray(rels) ? rels : []);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('relationshipEditorLoading'));
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => { loadData(); }, [loadData]);

  // ---- Position persistence ----

  const POS_STORAGE_KEY = 'solo-relationship-positions';

  const loadPositions = useCallback((): Record<string, { x: number; y: number }> => {
    try {
      const raw = localStorage.getItem(POS_STORAGE_KEY);
      return raw ? JSON.parse(raw) : {};
    } catch { return {}; }
  }, []);

  const savePositions = useCallback((nodes: Node[]) => {
    const pos: Record<string, { x: number; y: number }> = {};
    for (const n of nodes) {
      pos[n.id] = n.position;
    }
    try { localStorage.setItem(POS_STORAGE_KEY, JSON.stringify(pos)); } catch { /* noop */ }
  }, []);

  // ---- Build initial nodes/edges ----

  // ---- Build initial nodes/edges ----

  const initialNodes: Node[] = useMemo(() => {
    const saved = loadPositions();
    // Build set of occupied positions from saved data (for existing agents).
    const occupied = new Set(
      Object.values(saved).map((p) => `${Math.round(p.x)},${Math.round(p.y)}`),
    );

    const findFreePos = (i: number) => {
      const COLS = 4;
      const CELL_W = 220;
      const CELL_H = 160;
      let attempts = 0;
      while (attempts < 100) {
        const col = attempts < COLS ? attempts % COLS : (i + attempts) % COLS;
        const row = Math.floor((i + attempts) / COLS);
        const x = col * CELL_W + 100;
        const y = row * CELL_H + 80;
        if (!occupied.has(`${x},${y}`)) {
          occupied.add(`${x},${y}`);
          return { x, y };
        }
        attempts++;
      }
      return { x: (i % COLS) * CELL_W + 100, y: Math.floor(i / COLS) * CELL_H + 80 };
    };

    return agents.map((a, i) => ({
      id: a.id,
      type: 'agentNode',
      position: saved[a.id] || findFreePos(i),
      data: {
        agentId: a.id,
        agentName: a.name,
        isActive: a.is_active,
      },
    }));
  }, [agents, loadPositions]);

  const initialEdges: Edge[] = useMemo(() => {
    const map = new Map<string, AgentRelationship>();
    const edges = relationships.map((r) => {
      map.set(r.id, r);
      const isCollab = r.rel_type === 'collaborates_with';
      return {
        id: r.id,
        source: r.from_agent_id,
        target: r.to_agent_id,
        type: 'relationship',
        // Collaboration is bidirectional: pin it to side handles so the line
        // stays horizontal between same-rank peers instead of arcing top/bottom.
        ...(isCollab ? { sourceHandle: 'right', targetHandle: 'left' } : {}),
        data: {
          relType: r.rel_type,
          channelName: r.channel_name,
        },
      };
    });
    edgeToRelationshipMap.current = map;
    return edges;
  }, [relationships]);

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  // Sync when data reloads — keep existing node positions, only add new / remove deleted.
  useEffect(() => {
    setNodes((prev) => {
      const existingPos = new Map(prev.map((n) => [n.id, n.position]));
      return initialNodes.map((n) => ({
        ...n,
        position: existingPos.get(n.id) || n.position,
      }));
    });
    setEdges(initialEdges);
  }, [initialNodes, initialEdges, setNodes, setEdges]);

  // Save positions when nodes change (drag via ReactFlow onNodesChange)
  const saveTimeoutRef = useRef<ReturnType<typeof setTimeout>>(null);
  useEffect(() => {
    if (saveTimeoutRef.current) clearTimeout(saveTimeoutRef.current);
    saveTimeoutRef.current = setTimeout(() => savePositions(nodes), 500);
    return () => { if (saveTimeoutRef.current) clearTimeout(saveTimeoutRef.current); };
  }, [nodes, savePositions]);

  // ---- WebSocket sync ----

  const { onEvent } = useWebSocket();

  useEffect(() => {
    const unsub = onEvent((event) => {
      if (event.type === 'relationship_created') {
        setEdges((prev) => {
          if (prev.find((e) => e.id === event.id)) return prev;
          const newRel: AgentRelationship = {
            id: event.id,
            from_agent_id: event.from_agent_id,
            to_agent_id: event.to_agent_id,
            rel_type: event.rel_type as RelationshipType,
            channel_id: event.channel_id,
          };
          edgeToRelationshipMap.current.set(event.id, newRel);
          const isCollab = event.rel_type === 'collaborates_with';
          return [...prev, {
            id: event.id,
            source: event.from_agent_id,
            target: event.to_agent_id,
            type: 'relationship',
            ...(isCollab ? { sourceHandle: 'right', targetHandle: 'left' } : {}),
            data: { relType: event.rel_type as RelationshipType, channelName: (event as { channel_name?: string }).channel_name },
          }];
        });
      }
      if (event.type === 'relationship_updated') {
        setEdges((prev) => prev.map((e) => {
          if (e.id !== event.id) return e;
          const existing = edgeToRelationshipMap.current.get(event.id);
          if (existing) {
            existing.rel_type = event.rel_type as RelationshipType;
            if (event.channel_id !== undefined) existing.channel_id = event.channel_id;
          }
          return {
            ...e,
            data: { ...e.data, relType: event.rel_type as RelationshipType, channelName: (event as { channel_name?: string }).channel_name },
          };
        }));
        // Update detail panel if showing this relationship
        setDetailRel((prev) => {
          if (prev?.id === event.id) {
            return { ...prev, rel_type: event.rel_type as RelationshipType, channel_id: event.channel_id };
          }
          return prev;
        });
      }
      if (event.type === 'relationship_deleted') {
        setEdges((prev) => prev.filter((e) => e.id !== event.id));
        edgeToRelationshipMap.current.delete(event.id);
        // Close detail panel if showing this relationship
        setDetailRel((prev) => prev?.id === event.id ? null : prev);
      }

      // agent_deleted — server cascaded the agent's relationships and
      // dropped the agent row's active flag. Drop every edge / node that
      // referenced it locally so the graph doesn't show ghost nodes, then
      // refetch agents to keep the agents list in sync.
      if (event.type === 'agent_deleted') {
        const removedIds = new Set(event.deleted_relationship_ids ?? []);
        setEdges((prev) => {
          const toDrop = new Set<string>();
          for (const e of prev) {
            if (e.source === event.agent_id || e.target === event.agent_id) {
              toDrop.add(e.id);
            } else if (removedIds.has(e.id)) {
              toDrop.add(e.id);
            }
          }
          for (const id of toDrop) {
            edgeToRelationshipMap.current.delete(id);
          }
          return prev.filter((e) => !toDrop.has(e.id));
        });
        setNodes((prev) => prev.filter((n) => n.id !== event.agent_id));
        setRelationships((prev) =>
          prev
            .filter((r) =>
              r.from_agent_id !== event.agent_id &&
              r.to_agent_id !== event.agent_id,
            )
            .filter((r) => !removedIds.has(r.id)),
        );
        // Close any detail panel showing data about the deleted agent.
        setDetailRel((prev) =>
          prev && (prev.from_agent_id === event.agent_id || prev.to_agent_id === event.agent_id)
            ? null
            : prev,
        );
        setDetailAgent((prev) => (prev?.id === event.agent_id ? null : prev));
        // useAgents doesn't subscribe to WS; trigger a refetch so the
        // nodes list rebuilt from `agents` matches the server's view.
        void refetchAgents();
      }
    });
    return unsub;
  }, [onEvent, setEdges]);

  // ---- Undo/Redo ----

  const pushUndo = useCallback(() => {
    setUndoStack((prev) => [...prev, { nodes: [...nodes], edges: [...edges] }]);
    setRedoStack([]);
  }, [nodes, edges]);

  const undo = useCallback(() => {
    const entry = undoStack[undoStack.length - 1];
    if (!entry) return;
    setRedoStack((prev) => [...prev, { nodes: [...nodes], edges: [...edges] }]);
    setNodes(entry.nodes);
    setEdges(entry.edges);
    setUndoStack((prev) => prev.slice(0, -1));
  }, [undoStack, nodes, edges, setNodes, setEdges]);

  const redo = useCallback(() => {
    const entry = redoStack[redoStack.length - 1];
    if (!entry) return;
    setUndoStack((prev) => [...prev, { nodes: [...nodes], edges: [...edges] }]);
    setNodes(entry.nodes);
    setEdges(entry.edges);
    setRedoStack((prev) => prev.slice(0, -1));
  }, [redoStack, nodes, edges, setNodes, setEdges]);

  // ---- Connect (create relationship via modal) ----

  const onConnect = useCallback((connection: Connection) => {
    if (!connection.source || !connection.target) return;
    pushUndo();
    setPreselectedFrom(connection.source);
    setPreselectedTo(connection.target);
    setShowCreateModal(true);
  }, [pushUndo]);

  const handleCreateModalClose = useCallback((open: boolean) => {
    setShowCreateModal(open);
    if (!open) {
      setPreselectedFrom(null);
      setPreselectedTo(null);
    }
  }, []);

  const handleRelationshipCreated = useCallback(() => {
    loadData();
  }, [loadData]);

  // ---- Edge click → show detail panel ----

  const agentNameMap = useMemo(() => {
    const m = new Map<string, { name: string; isActive: boolean }>();
    for (const a of agents) m.set(a.id, { name: a.name, isActive: a.is_active });
    return m;
  }, [agents]);

  const onEdgeClick = useCallback((_event: React.MouseEvent, edge: Edge) => {
    setSelectedEdge(edge);
    const rel = edgeToRelationshipMap.current.get(edge.id);
    if (rel) {
      const fromInfo = agentNameMap.get(rel.from_agent_id);
      const toInfo = agentNameMap.get(rel.to_agent_id);
      setDetailRel({
        ...rel,
        from_agent_name: fromInfo?.name,
        from_agent_active: fromInfo?.isActive,
        to_agent_name: toInfo?.name,
        to_agent_active: toInfo?.isActive,
      });
      setDetailAgent(null);
    }
  }, [agentNameMap]);

  const onNodeClick: NodeMouseHandler = useCallback((_event, node) => {
    setSelectedEdge(null);
    const agent = agents.find((a) => a.id === node.id);
    if (agent) {
      setDetailAgent(agent);
      setDetailRel(null);
    }
  }, [agents]);

  const closeDetailPanel = useCallback(() => {
    setDetailRel(null);
    setDetailAgent(null);
    setSelectedEdge(null);
  }, []);

  const handleDetailUpdate = useCallback(() => {
    loadData();
  }, [loadData]);

  const handleDetailDelete = useCallback((id: string) => {
    setEdges((prev) => prev.filter((e) => e.id !== id));
    edgeToRelationshipMap.current.delete(id);
    setSelectedEdge(null);
  }, [setEdges]);

  const deleteSelectedEdge = useCallback(async () => {
    if (!selectedEdge) return;
    pushUndo();
    try {
      await apiClient.delete(`/api/v1/agent-relationships/${selectedEdge.id}`);
      setEdges((prev) => prev.filter((e) => e.id !== selectedEdge.id));
      edgeToRelationshipMap.current.delete(selectedEdge.id);
    } catch (err) {
      console.error('Failed to delete relationship:', err);
    }
    setSelectedEdge(null);
    setDetailRel(null);
  }, [selectedEdge, pushUndo, setEdges]);

  // ---- Keyboard shortcuts ----

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Delete' || e.key === 'Backspace') {
        if (selectedEdge && document.activeElement === document.body) {
          deleteSelectedEdge();
        }
      }
      if ((e.ctrlKey || e.metaKey) && e.key === 'z') {
        e.preventDefault();
        if (e.shiftKey) redo(); else undo();
      }
      if (e.key === 'Escape') {
        setSelectedEdge(null);
        setShowCreateModal(false);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [selectedEdge, deleteSelectedEdge, undo, redo]);

  // ---- Auto layout ----
  // Dagre-based layered layout, TB direction.
  // - assigns_to: directional edges that define the rank hierarchy
  //   (parent on top, child below).
  // - collaborates_with: same-rank constraint, implemented via a compound
  //   graph. Every collab pair gets wrapped in a shared parent cluster
  //   with rank: 'same', which forces dagre to keep them on one row.
  // - assigns_to wins when both relationships exist on the same pair.

  const autoLayout = useCallback(() => {
    pushUndo();
    try { localStorage.removeItem(POS_STORAGE_KEY); } catch { /* noop */ }
    setNodes((prev) => {
      const NODE_W = 180;
      const NODE_H = 100;
      const g = new dagre.graphlib.Graph({ compound: true });
      g.setGraph({ rankdir: 'TB', nodesep: 100, ranksep: 140, marginx: 80, marginy: 80 });
      g.setDefaultEdgeLabel(() => ({}));

      for (const n of prev) {
        g.setNode(n.id, { width: NODE_W, height: NODE_H });
      }

      const pairKey = (a: string, b: string) => a < b ? `${a}|${b}` : `${b}|${a}`;
      const assignsPairs = new Set<string>();
      for (const e of edges) {
        if (!g.hasNode(e.source) || !g.hasNode(e.target)) continue;
        if (e.data?.relType === 'assigns_to') {
          assignsPairs.add(pairKey(e.source, e.target));
        }
      }

      // Union-find over collaborates_with pairs so a chain (A↔B, B↔C)
      // collapses into one same-rank cluster.
      const parent = new Map<string, string>();
      const find = (x: string): string => {
        const p = parent.get(x);
        if (!p || p === x) { parent.set(x, x); return x; }
        const r = find(p); parent.set(x, r); return r;
      };
      const union = (a: string, b: string) => { parent.set(find(a), find(b)); };

      for (const e of edges) {
        if (!g.hasNode(e.source) || !g.hasNode(e.target)) continue;
        if (e.data?.relType !== 'collaborates_with') continue;
        if (assignsPairs.has(pairKey(e.source, e.target))) continue;
        union(e.source, e.target);
      }

      // Build clusters: collab roots with > 1 member become same-rank parents.
      const clusters = new Map<string, string[]>();
      for (const n of prev) {
        const root = find(n.id);
        if (!clusters.has(root)) clusters.set(root, []);
        clusters.get(root)!.push(n.id);
      }
      let clusterIdx = 0;
      for (const [, members] of clusters) {
        if (members.length < 2) continue;
        const clusterId = `__collab_cluster_${clusterIdx++}`;
        g.setNode(clusterId, {});
        for (const m of members) g.setParent(m, clusterId);
      }

      // Add edges. assigns_to defines ranks; collaborates_with is purely
      // visual once the cluster does the same-rank work.
      for (const e of edges) {
        if (!g.hasNode(e.source) || !g.hasNode(e.target)) continue;
        const relType = e.data?.relType;
        if (relType === 'assigns_to') {
          g.setEdge(e.source, e.target, { minlen: 1, weight: 2 });
        }
        // collaborates_with: no dagre edge needed — the cluster handles rank.
      }

      dagre.layout(g);

      const next = prev.map((n) => {
        const pos = g.node(n.id);
        if (!pos) return n;
        return {
          ...n,
          position: { x: pos.x - NODE_W / 2, y: pos.y - NODE_H / 2 },
        };
      });
      savePositions(next);
      return next;
    });
  }, [pushUndo, setNodes, savePositions, edges]);

  // ---- Loading ----

  if (isLoading || agentsLoading) {
    return (
      <div className="flex h-screen">
        <NavBar />
        <div className="flex-1 flex items-center justify-center gap-2">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          <span className="font-heading text-sm uppercase tracking-wider text-muted-foreground">
            {t('relationshipEditorLoading')}
          </span>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex h-screen">
        <NavBar />
        <div className="flex-1 flex flex-col items-center justify-center gap-4">
          <p className="font-mono text-sm text-brutal-danger">{error}</p>
          <button type="button" onClick={loadData} className="btn-brutal px-4 py-2">{t('retry')}</button>
        </div>
      </div>
    );
  }

  // ---- Render ----

  return (
    <div className="flex h-screen">
      <NavBar />

      {/* Main editor area */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {/* Toolbar */}
        <div className="flex items-center gap-2 h-14 px-4 border-b-2 border-black bg-brutal-cream">
          <h1 className="font-heading text-lg font-bold uppercase tracking-wider mr-auto">
            {t('relationshipEditor')}
          </h1>

          {/* Undo/Redo */}
          <button
            type="button"
            onClick={undo}
            disabled={undoStack.length === 0}
            className="flex items-center gap-1 h-8 px-2 border-2 border-black bg-white hover:bg-brutal-primary-light disabled:opacity-30"
            title={t('relationshipEditorUndo')}
          >
            <Undo2 className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={redo}
            disabled={redoStack.length === 0}
            className="flex items-center gap-1 h-8 px-2 border-2 border-black bg-white hover:bg-brutal-primary-light disabled:opacity-30"
            title={t('relationshipEditorRedo')}
          >
            <Redo2 className="h-3.5 w-3.5" />
          </button>

          <div className="w-px h-6 bg-black/20" />

          {/* Auto layout */}
          <button
            type="button"
            onClick={autoLayout}
            className="flex items-center gap-1.5 h-8 px-3 border-2 border-black bg-white hover:bg-brutal-info-light font-heading text-xs font-bold uppercase tracking-wider"
          >
            <LayoutGrid className="h-3.5 w-3.5" />
            {t('relationshipEditorAutoLayout')}
          </button>

          {/* Delete selected */}
          {selectedEdge && (
            <button
              type="button"
              onClick={deleteSelectedEdge}
              className="flex items-center gap-1.5 h-8 px-3 border-2 border-black bg-brutal-danger text-white font-heading text-xs font-bold uppercase tracking-wider hover:bg-red-600"
            >
              <Trash2 className="h-3.5 w-3.5" />
              {t('relationshipEditorDeleteEdge')}
            </button>
          )}
        </div>

        {/* Graph */}
        <div className="flex-1 relative">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            onEdgeClick={onEdgeClick}
            onPaneClick={() => { setSelectedEdge(null); closeDetailPanel(); }}
            onNodeClick={onNodeClick}
            onNodeDragStop={(_event, node) => {
              setNodes((prev) => {
                const next = prev.map((n) => n.id === node.id ? { ...n, position: node.position } : n);
                savePositions(next);
                return next;
              });
            }}
            nodeTypes={NODE_TYPES}
            edgeTypes={EDGE_TYPES}
            fitView
            fitViewOptions={{ padding: 0.25, maxZoom: 0.85 }}
            defaultEdgeOptions={{
              type: 'relationship',
            }}
            deleteKeyCode={null}
          >
            <Background color="rgba(0,0,0,0.08)" gap={20} />
            <Controls
              className="!border-2 !border-black !shadow-brutal-sm"
              position="bottom-right"
            />
            <MiniMap
              className="!border-2 !border-black !shadow-brutal"
              nodeColor={(n) => {
                const data = n.data as { isActive?: boolean } | undefined;
                return data?.isActive ? '#88D498' : '#c0b9b1';
              }}
              position="bottom-left"
            />
          </ReactFlow>

          {/* Create relationship modal (T5.2.4) */}
          <CreateRelationshipModal
            open={showCreateModal}
            onOpenChange={handleCreateModalClose}
            onCreated={handleRelationshipCreated}
            preselectedFrom={preselectedFrom ?? undefined}
            preselectedTo={preselectedTo ?? undefined}
            agents={agents}
          />

          {/* Detail panel */}
          {(detailRel || detailAgent) && (
            <RelationshipDetailPanel
              relationship={detailRel}
              agent={detailAgent}
              onClose={closeDetailPanel}
              onUpdate={handleDetailUpdate}
              onDelete={handleDetailDelete}
            />
          )}

          {/* Empty state overlay */}
          {agents.length === 0 && (
            <div className="absolute inset-0 flex items-center justify-center pointer-events-none z-10">
              <div className="flex flex-col items-center gap-3 p-8 border-4 border-black bg-white shadow-brutal-xl">
                <Plus className="h-10 w-10 text-muted-foreground" />
                <p className="font-heading text-sm text-muted-foreground max-w-xs text-center">
                  {t('relationshipEditorEmpty')}
                </p>
              </div>
            </div>
          )}
        </div>

        {/* Bottom legend */}
        <div className="flex items-center gap-4 h-10 px-4 border-t-2 border-black bg-white">
          <span className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
            Legend:
          </span>
          {[
            { type: 'assigns_to', color: '#4A90D9', dash: '' },
            { type: 'collaborates_with', color: '#10B981', dash: '8,4' },
          ].map(({ type, color, dash }) => (
            <span key={type} className="flex items-center gap-1.5 font-mono text-[10px]">
              <svg width="24" height="8"><line x1="0" y1="4" x2="24" y2="4" stroke={color} strokeWidth={2} strokeDasharray={dash || undefined} /></svg>
              {type.replace(/_/g, ' ')}
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}
