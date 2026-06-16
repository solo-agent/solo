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
import { Loader2, Plus, Trash2, LayoutGrid, Undo2, Redo2, Download, Layers } from 'lucide-react';
import { NavBar } from '@/components/ui/navbar';
import { RelationshipNode } from '@/components/relationships/relationship-node';
import { RelationshipEdge } from '@/components/relationships/relationship-edge';
import { CreateRelationshipModal } from '@/components/relationships/create-relationship-modal';
import { RelationshipDetailPanel } from '@/components/relationships/relationship-detail-panel';
import { Dialog, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { apiClient, ApiError } from '@/lib/api-client';
import { useAuth } from '@/lib/auth-context';
import { useWebSocket } from '@/lib/ws-context';
import { useChannels } from '@/lib/hooks/use-channels';
import { t } from '@/lib/i18n';
import { listTemplates, applyTemplate, type Template } from '@/lib/templates-api';
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
  const { user } = useAuth();
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

  // ---- Template state ----
  const [showTemplateModal, setShowTemplateModal] = useState(false);
  const [templates, setTemplates] = useState<Template[]>([]);
  const [templatesLoading, setTemplatesLoading] = useState(false);
  const [applyingTemplate, setApplyingTemplate] = useState<string | null>(null);
  const [templateError, setTemplateError] = useState<string | null>(null);

  const loadTemplates = useCallback(async () => {
    setTemplatesLoading(true);
    try {
      setTemplates(await listTemplates());
    } catch { /* noop */ }
    finally { setTemplatesLoading(false); }
  }, []);

  const handleApplyTemplate = useCallback(async (templateId: string) => {
    setApplyingTemplate(templateId);
    setTemplateError(null);
    try {
      if (!user?.id) { setTemplateError('Not authenticated'); return; }
      await applyTemplate(templateId, user.id);
      setShowTemplateModal(false);
      loadData();
    } catch (err) {
      setTemplateError(err instanceof Error ? err.message : 'Failed to apply template');
    } finally {
      setApplyingTemplate(null);
    }
  }, [loadData, user]);

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

  const initialNodes: Node[] = useMemo(() => {
    const saved = loadPositions();
    return agents.map((a, i) => ({
      id: a.id,
      type: 'agentNode',
      position: saved[a.id] || {
        x: (i % 4) * 220 + 100,
        y: Math.floor(i / 4) * 160 + 80,
      },
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
      const next = prev.map((n, i) => ({
        ...n,
        position: {
          x: (i % 4) * 220 + 100,
          y: Math.floor(i / 4) * 160 + 80,
        },
      }));
      savePositions(next);
      return next;
    });
  }, [pushUndo, setNodes, savePositions]);

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

          {/* Templates */}
          <button
            type="button"
            onClick={() => { setShowTemplateModal(true); loadTemplates(); }}
            className="flex items-center gap-1.5 h-8 px-3 border-2 border-black bg-white hover:bg-brutal-accent-light font-heading text-xs font-bold uppercase tracking-wider"
          >
            <Layers className="h-3.5 w-3.5" />
            Templates
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

          {/* Template browser modal */}
          <Dialog open={showTemplateModal} onOpenChange={setShowTemplateModal} width="lg">
            <DialogHeader>
              <DialogTitle className="font-heading text-base font-black uppercase tracking-wider">
                Team Templates
              </DialogTitle>
            </DialogHeader>
            <div className="space-y-4 max-h-[60vh] overflow-y-auto">
              {templatesLoading ? (
                <div className="flex items-center justify-center py-8">
                  <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                </div>
              ) : templates.length === 0 ? (
                <p className="font-mono text-sm text-muted-foreground text-center py-4">No templates available.</p>
              ) : (
                <>
                  {/* Group by category */}
                  {(() => {
                    const cats = [...new Set(templates.map((t) => t.category))];
                    return cats.map((cat) => (
                      <div key={cat}>
                        <h3 className="font-heading text-xs font-bold uppercase tracking-wider text-muted-foreground mb-2 border-b-2 border-black pb-1">
                          {cat}
                        </h3>
                        <div className="space-y-2">
                          {templates.filter((t) => t.category === cat).map((tmpl) => (
                            <div
                              key={tmpl.id}
                              className="flex items-start gap-3 p-3 border-2 border-black bg-white"
                            >
                              <span className="text-2xl flex-shrink-0">{tmpl.icon}</span>
                              <div className="flex-1 min-w-0">
                                <div className="font-heading text-sm font-bold text-black">{tmpl.name}</div>
                                <p className="font-sans text-xs text-muted-foreground mt-0.5">{tmpl.description}</p>
                              </div>
                              <button
                                type="button"
                                onClick={() => handleApplyTemplate(tmpl.id)}
                                disabled={applyingTemplate === tmpl.id}
                                className="flex-shrink-0 px-3 py-1.5 border-2 border-black bg-brutal-success text-black font-heading text-[10px] font-bold uppercase tracking-wider hover:bg-brutal-success-light disabled:opacity-50"
                              >
                                {applyingTemplate === tmpl.id ? (
                                  <Loader2 className="h-3 w-3 animate-spin" />
                                ) : (
                                  'Apply'
                                )}
                              </button>
                            </div>
                          ))}
                        </div>
                      </div>
                    ));
                  })()}
                </>
              )}
              {templateError && (
                <p className="font-mono text-xs text-brutal-danger">{templateError}</p>
              )}
            </div>
          </Dialog>

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
