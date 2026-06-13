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
  addEdge,
  useNodesState,
  useEdgesState,
  type Connection,
  type Edge,
  type Node,
  type NodeMouseHandler,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { Loader2, Plus, Trash2, LayoutGrid, Undo2, Redo2, Download } from 'lucide-react';
import { NavBar } from '@/components/ui/navbar';
import { RelationshipNode } from '@/components/relationships/relationship-node';
import { RelationshipEdge } from '@/components/relationships/relationship-edge';
import { TypeSelector } from '@/components/relationships/type-selector';
import { RelationshipDetailPanel } from '@/components/relationships/relationship-detail-panel';
import { apiClient, ApiError } from '@/lib/api-client';
import { useWebSocket } from '@/lib/ws-context';
import { useChannels } from '@/lib/hooks/use-channels';
import { t } from '@/lib/i18n';
import type { Agent, AgentRelationship, RelationshipType, CreateRelationshipInput } from '@/lib/types';
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
  const { agents, isLoading: agentsLoading } = useAgents();
  const { channels } = useChannels();
  const [relationships, setRelationships] = useState<AgentRelationship[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showSelector, setShowSelector] = useState<{
    sourceId: string;
    targetId: string;
    sourceName: string;
    targetName: string;
  } | null>(null);
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

  // ---- Build initial nodes/edges ----

  const initialNodes: Node[] = useMemo(() => {
    return agents.map((a, i) => ({
      id: a.id,
      type: 'agentNode',
      position: {
        x: (i % 4) * 220 + 100,
        y: Math.floor(i / 4) * 160 + 80,
      },
      data: {
        agentId: a.id,
        agentName: a.name,
        isActive: a.is_active,
      },
    }));
  }, [agents]);

  const initialEdges: Edge[] = useMemo(() => {
    const map = new Map<string, AgentRelationship>();
    const edges = relationships.map((r) => {
      map.set(r.id, r);
      return {
        id: r.id,
        source: r.from_agent_id,
        target: r.to_agent_id,
        type: 'relationship',
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

  // Sync when data reloads
  useEffect(() => {
    setNodes(initialNodes);
    setEdges(initialEdges);
  }, [initialNodes, initialEdges, setNodes, setEdges]);

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
          return [...prev, {
            id: event.id,
            source: event.from_agent_id,
            target: event.to_agent_id,
            type: 'relationship',
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

  // ---- Connect (create relationship) ----

  const onConnect = useCallback((connection: Connection) => {
    if (!connection.source || !connection.target) return;

    const sourceAgent = agents.find((a) => a.id === connection.source);
    const targetAgent = agents.find((a) => a.id === connection.target);

    setShowSelector({
      sourceId: connection.source,
      targetId: connection.target,
      sourceName: sourceAgent?.name || connection.source.slice(0, 8),
      targetName: targetAgent?.name || connection.target.slice(0, 8),
    });
  }, [agents]);

  const handleTypeSelect = useCallback(async (relType: RelationshipType) => {
    if (!showSelector) return;
    pushUndo();

    const input: CreateRelationshipInput = {
      from_agent_id: showSelector.sourceId,
      to_agent_id: showSelector.targetId,
      rel_type: relType,
    };

    try {
      const created = await apiClient.post<AgentRelationship>('/api/v1/agent-relationships', input);
      setEdges((prev) => {
        if (prev.find((e) => e.id === created.id)) return prev;
        return [...prev, {
          id: created.id,
          source: created.from_agent_id,
          target: created.to_agent_id,
          type: 'relationship',
          data: { relType, channelName: created.channel_name },
        }];
      });
    } catch (err) {
      console.error('Failed to create relationship:', err);
    }
    setShowSelector(null);
  }, [showSelector, pushUndo, setEdges]);

  // ---- Edge click → show detail panel ----

  const onEdgeClick = useCallback((_event: React.MouseEvent, edge: Edge) => {
    setSelectedEdge(edge);
    const rel = edgeToRelationshipMap.current.get(edge.id);
    if (rel) {
      setDetailRel(rel);
      setDetailAgent(null);
    }
  }, []);

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
        setShowSelector(null);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [selectedEdge, deleteSelectedEdge, undo, redo]);

  // ---- Export ----

  const exportPNG = useCallback(() => {
    const svg = document.querySelector('.react-flow__renderer svg') as SVGSVGElement;
    if (!svg) return;
    const serializer = new XMLSerializer();
    const source = serializer.serializeToString(svg);
    const blob = new Blob(['<?xml version="1.0" standalone="no"?>\r\n' + source], { type: 'image/svg+xml' });
    const link = document.createElement('a');
    link.href = URL.createObjectURL(blob);
    link.download = 'relationship-graph.svg';
    link.click();
  }, []);

  // ---- Auto layout ----

  const autoLayout = useCallback(() => {
    pushUndo();
    setNodes((prev) => {
      return prev.map((n, i) => ({
        ...n,
        position: {
          x: (i % 4) * 220 + 100,
          y: Math.floor(i / 4) * 160 + 80,
        },
      }));
    });
  }, [pushUndo, setNodes]);

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

          {/* Export */}
          <button
            type="button"
            onClick={exportPNG}
            className="flex items-center gap-1.5 h-8 px-3 border-2 border-black bg-white hover:bg-brutal-primary font-heading text-xs font-bold uppercase tracking-wider"
          >
            <Download className="h-3.5 w-3.5" />
            SVG
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
            nodeTypes={NODE_TYPES}
            edgeTypes={EDGE_TYPES}
            fitView
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

          {/* Type selector popup */}
          {showSelector && (
            <div className="absolute inset-0 z-50 flex items-center justify-center bg-black/30">
              <TypeSelector
                fromName={showSelector.sourceName}
                toName={showSelector.targetName}
                onSelect={handleTypeSelect}
                onCancel={() => setShowSelector(null)}
              />
            </div>
          )}

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
            { type: 'reports_to', color: '#4A90D9', dash: '' },
            { type: 'delegates_to', color: '#7B6CF6', dash: '' },
            { type: 'collaborates_with', color: '#10B981', dash: '8,4' },
            { type: 'escalates_to', color: '#EF4444', dash: '' },
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
