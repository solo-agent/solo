// ============================================================================
// RelationshipWorkspace — graph editor embedded in Teams
// - ReactFlow-based drag-and-drop relationship graph
// - Create/delete relationships by connecting agent nodes
// - 4 edge types with distinct visuals
// - WebSocket sync for real-time collaboration
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useMemo, useRef, type ReactNode } from 'react';
import { useRouter } from 'next/navigation';
import {
  ReactFlow,
  Background,
  Controls,
  useNodesState,
  useEdgesState,
  type Connection,
  type Edge,
  type Node,
  type NodeMouseHandler,
  type ReactFlowInstance,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import dagre from 'dagre';
import { ArrowLeft, Loader2, Plus, LayoutGrid, Undo2, Redo2, Layers } from 'lucide-react';
import { AppFrame } from '@/components/layout/app-frame';
import { Button } from '@/components/ui/button';
import { RelationshipNode, type AgentNodeTask } from '@/components/relationships/relationship-node';
import { RelationshipEdge } from '@/components/relationships/relationship-edge';
import { CreateRelationshipModal } from '@/components/relationships/create-relationship-modal';
import { RelationshipDetailPanel } from '@/components/relationships/relationship-detail-panel';
import { AgentForm, type AgentFormValues } from '@/components/agents/agent-form';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogCloseButton,
} from '@/components/ui/dialog';
import { Select } from '@/components/ui/select';
import { apiClient, ApiError } from '@/lib/api-client';
import { useWebSocket } from '@/lib/ws-context';
import { useToast } from '@/components/ui/toast';
import { t } from '@/lib/i18n';
import type { Agent, AgentBackendDetectItem, AgentDetailTarget, AgentRelationship, RelationshipType } from '@/lib/types';
import { useAgents } from '@/lib/hooks/use-agents';
import { useTeamAgentActivity } from '@/lib/hooks/use-team-agent-activity';
import { listTemplates, applyTemplate, type Template } from '@/lib/templates-api';
import { useCliDetection } from '@/lib/hooks/use-cli-detection';

// ---- Node/Edge types ----

const NODE_TYPES = { agentNode: RelationshipNode };
const EDGE_TYPES = { relationship: RelationshipEdge };

function isEditableTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName);
}

function relationshipTypeLabel(type: RelationshipType | string) {
  return type === 'assigns_to' ? t('assignsTo') : t('collaboratesWith');
}

// ---- Helpers ----

interface UndoEntry {
  nodes: Node[];
  edges: Edge[];
}

type GraphAgent = AgentDetailTarget & { is_active?: boolean };
type ChannelTeam = {
  agents: Array<{ id: string; name: string; status?: string }>;
};
type AgentRunListItem = { id: string };

// ---- Component ----

interface RelationshipWorkspaceProps {
  title?: string;
  embedded?: boolean;
  channelFilterId?: string;
  channelTeam?: ChannelTeam | null;
  agentTasks?: Record<string, AgentNodeTask | undefined>;
  onOpenTask?: (taskId: string) => void;
  onOpenTaskArtifact?: (taskId: string) => void;
  onChannelTeamRefresh?: () => void;
  onDetailOpen?: (detail: { relationship: AgentRelationship | null; agent: GraphAgent | null }) => void;
  onDetailClose?: () => void;
  embeddedActions?: ReactNode;
}

export function RelationshipWorkspace({
  title = t('relationshipEditor'),
  embedded = false,
  channelFilterId,
  channelTeam,
  agentTasks,
  onOpenTask,
  onOpenTaskArtifact,
  onChannelTeamRefresh,
  onDetailOpen,
  onDetailClose,
  embeddedActions,
}: RelationshipWorkspaceProps = {}) {
  const router = useRouter();
  const { agents, isLoading: agentsLoading, refetch: refetchAgents, createAgent } = useAgents();
  const { liveByAgent, getLatestRunId } = useTeamAgentActivity();
  const { showToast } = useToast();
  const { results: detection, isLoading: detectionLoading } = useCliDetection();
  const [relationships, setRelationships] = useState<AgentRelationship[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showChoiceDialog, setShowChoiceDialog] = useState(false);
  const [showCreateAgentModal, setShowCreateAgentModal] = useState(false);
  const [isCreatingAgent, setIsCreatingAgent] = useState(false);
  const [showTemplateModal, setShowTemplateModal] = useState(false);
  const [templates, setTemplates] = useState<Template[]>([]);
  const [templatesLoading, setTemplatesLoading] = useState(false);
  const [applyingTemplate, setApplyingTemplate] = useState<string | null>(null);
  const [templateError, setTemplateError] = useState<string | null>(null);
  const [selectedModelProvider, setSelectedModelProvider] = useState('');
  const [preselectedFrom, setPreselectedFrom] = useState<string | null>(null);
  const [preselectedTo, setPreselectedTo] = useState<string | null>(null);
  const [selectedEdge, setSelectedEdge] = useState<Edge | null>(null);
  const [detailRel, setDetailRel] = useState<AgentRelationship | null>(null);
  const [detailAgent, setDetailAgent] = useState<GraphAgent | null>(null);
  const [detailPanelWidth, setDetailPanelWidth] = useState(400);
  const [undoStack, setUndoStack] = useState<UndoEntry[]>([]);
  const [redoStack, setRedoStack] = useState<UndoEntry[]>([]);
  const edgeToRelationshipMap = useRef<Map<string, AgentRelationship>>(new Map());
  const flowRef = useRef<ReactFlowInstance | null>(null);
  const detailPanelOpen = !!detailRel || !!detailAgent;
  const activeChannelFilterId = channelFilterId ?? '';

  const fitGraph = useCallback(() => {
    requestAnimationFrame(() => {
      flowRef.current?.fitView({ padding: 0.25, maxZoom: 0.85, duration: 250 });
    });
  }, []);

  const handleOpenLatestRun = useCallback(async (agentId: string) => {
    const runId = getLatestRunId(agentId);
    if (runId) {
      router.push(`/observability/live?run_id=${encodeURIComponent(runId)}`);
      return;
    }
    const runs = await apiClient.get<AgentRunListItem[]>(`/api/v1/agents/${agentId}/runs`).catch(() => []);
    const fallbackRunId = Array.isArray(runs) ? runs[0]?.id : undefined;
    if (fallbackRunId) {
      router.push(`/observability/live?run_id=${encodeURIComponent(fallbackRunId)}`);
      return;
    }
    showToast(t('observabilityNoRecord'), 'info');
  }, [getLatestRunId, router, showToast]);

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
  const posStorageKey = activeChannelFilterId ? `${POS_STORAGE_KEY}:${activeChannelFilterId}` : POS_STORAGE_KEY;
  const previousPosStorageKeyRef = useRef(posStorageKey);

  const loadPositions = useCallback((): Record<string, { x: number; y: number }> => {
    try {
      const raw = localStorage.getItem(posStorageKey);
      return raw ? JSON.parse(raw) : {};
    } catch { return {}; }
  }, [posStorageKey]);

  const savePositions = useCallback((nodes: Node[]) => {
    const pos: Record<string, { x: number; y: number }> = {};
    for (const n of nodes) {
      pos[n.id] = n.position;
    }
    try { localStorage.setItem(posStorageKey, JSON.stringify(pos)); } catch { /* noop */ }
  }, [posStorageKey]);

  // ---- Build initial nodes/edges ----

  const activeChannelTeam = channelTeam ?? null;

  const visibleAgents = useMemo<GraphAgent[]>(() => {
    if (!activeChannelFilterId) return agents;
    if (!activeChannelTeam) return [];
    const agentsById = new Map(agents.map((agent) => [agent.id, agent]));
    return activeChannelTeam.agents.map((teamAgent) => {
      const agent = agentsById.get(teamAgent.id);
      return {
        id: teamAgent.id,
        name: agent?.name ?? teamAgent.name,
        avatar_url: agent?.avatar_url ?? null,
        is_active: agent?.is_active ?? (teamAgent.status === 'active' || teamAgent.status === 'online'),
      };
    });
  }, [activeChannelFilterId, activeChannelTeam, agents]);

  const visibleRelationships = useMemo(() => {
    if (!activeChannelFilterId) return relationships;
    if (!activeChannelTeam) return [];
    const visibleIds = new Set(activeChannelTeam.agents.map((agent) => agent.id));
    return relationships.filter((relationship) => (
      visibleIds.has(relationship.from_agent_id) && visibleIds.has(relationship.to_agent_id)
    ));
  }, [activeChannelFilterId, activeChannelTeam, relationships]);

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

    return visibleAgents.map((a, i) => ({
      id: a.id,
      type: 'agentNode',
      position: saved[a.id] || findFreePos(i),
      data: {
        agentId: a.id,
        agentName: a.name,
        isActive: a.is_active,
        task: agentTasks?.[a.id],
        onOpenRun: handleOpenLatestRun,
        onOpenTask,
        onOpenTaskArtifact,
      },
    }));
  }, [agentTasks, handleOpenLatestRun, onOpenTask, onOpenTaskArtifact, visibleAgents, loadPositions]);

  const initialEdges: Edge[] = useMemo(() => {
    const map = new Map<string, AgentRelationship>();
    const edges = visibleRelationships.map((r) => {
      map.set(r.id, r);
      const isCollab = r.rel_type === 'collaborates_with';
      return {
        id: r.id,
        source: r.from_agent_id,
        target: r.to_agent_id,
        type: 'relationship',
        // Collaboration is horizontal; assignment is vertical and starts below
        // any mounted task card.
        ...(isCollab
          ? { sourceHandle: 'right', targetHandle: 'left' }
          : { sourceHandle: 'bottom', targetHandle: 'top' }),
        data: {
          relType: r.rel_type,
          channelName: r.channel_name,
        },
      };
    });
    edgeToRelationshipMap.current = map;
    return edges;
  }, [visibleRelationships]);

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  // Sync when data reloads — keep existing node positions, only add new / remove deleted.
  useEffect(() => {
    const keepExistingPositions = previousPosStorageKeyRef.current === posStorageKey;
    previousPosStorageKeyRef.current = posStorageKey;
    setNodes((prev) => {
      const existingPos = keepExistingPositions ? new Map(prev.map((n) => [n.id, n.position])) : new Map();
      return initialNodes.map((n) => ({
        ...n,
        position: existingPos.get(n.id) || n.position,
      }));
    });
    setEdges(initialEdges);
    fitGraph();
  }, [initialNodes, initialEdges, posStorageKey, setNodes, setEdges, fitGraph]);

  useEffect(() => {
    setNodes((prev) => prev.map((node) => ({
      ...node,
      data: {
        ...node.data,
        liveActivity: liveByAgent.get(node.id),
        task: agentTasks?.[node.id],
        onOpenRun: handleOpenLatestRun,
        onOpenTask,
        onOpenTaskArtifact,
      },
    })));
  }, [agentTasks, handleOpenLatestRun, liveByAgent, onOpenTask, onOpenTaskArtifact, setNodes]);

  // Save positions when nodes change (drag via ReactFlow onNodesChange)
  const saveTimeoutRef = useRef<ReturnType<typeof setTimeout>>(null);
  useEffect(() => {
    if (saveTimeoutRef.current) clearTimeout(saveTimeoutRef.current);
    saveTimeoutRef.current = setTimeout(() => savePositions(nodes), 500);
    return () => { if (saveTimeoutRef.current) clearTimeout(saveTimeoutRef.current); };
  }, [nodes, savePositions]);

  // ---- WebSocket sync ----

  const { onEvent } = useWebSocket();
  const visibleAgentIdsRef = useRef(new Set<string>());

  useEffect(() => {
    visibleAgentIdsRef.current = new Set(visibleAgents.map((agent) => agent.id));
  }, [visibleAgents]);

  useEffect(() => {
    const unsub = onEvent((event) => {
      if (event.type === 'relationship_created') {
        setEdges((prev) => {
          if (prev.find((e) => e.id === event.id)) return prev;
          if (activeChannelFilterId) {
            const ids = visibleAgentIdsRef.current;
            if (!ids.has(event.from_agent_id) || !ids.has(event.to_agent_id)) return prev;
          }
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
            ...(isCollab
              ? { sourceHandle: 'right', targetHandle: 'left' }
              : { sourceHandle: 'bottom', targetHandle: 'top' }),
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
        setEdges((prev) => {
          const toDrop = new Set<string>();
          for (const e of prev) {
            if (e.source === event.agent_id || e.target === event.agent_id) {
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
            ),
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
  }, [activeChannelFilterId, onEvent, refetchAgents, setEdges, setNodes]);

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

  const refreshChannelFilter = useCallback(() => {
    if (!activeChannelFilterId) return;
    onChannelTeamRefresh?.();
  }, [activeChannelFilterId, onChannelTeamRefresh]);

  const handleRelationshipCreated = useCallback(() => {
    loadData();
    refreshChannelFilter();
  }, [loadData, refreshChannelFilter]);

  const handleCreateAgent = useCallback(async (values: AgentFormValues) => {
    if (isCreatingAgent) return;
    setIsCreatingAgent(true);
    try {
      await createAgent(values);
      await refetchAgents();
      setShowCreateAgentModal(false);
      showToast(t('teamsAgentCreated'), 'success');
    } catch {
      showToast(t('teamsAgentCreateError'), 'error');
    } finally {
      setIsCreatingAgent(false);
    }
  }, [createAgent, isCreatingAgent, refetchAgents, showToast]);

  const loadTemplates = useCallback(async () => {
    setTemplatesLoading(true);
    setTemplateError(null);
    try {
      setTemplates(await listTemplates());
    } catch (err) {
      setTemplateError(err instanceof Error ? err.message : t('relationshipTemplateLoadError'));
    } finally {
      setTemplatesLoading(false);
    }
  }, []);

  const handleOpenTemplates = useCallback(() => {
    setShowChoiceDialog(false);
    setShowTemplateModal(true);
    void loadTemplates();
    if (!selectedModelProvider) {
      const available = (Object.values(detection) as AgentBackendDetectItem[]).find((rt) => rt.available);
      if (available) setSelectedModelProvider(available.type);
    }
  }, [detection, loadTemplates, selectedModelProvider]);

  const handleApplyTemplate = useCallback(async (templateID: string) => {
    if (!selectedModelProvider) {
      setTemplateError(t('relationshipRuntimeRequiredError'));
      return;
    }
    setApplyingTemplate(templateID);
    setTemplateError(null);
    try {
      await applyTemplate(templateID, selectedModelProvider);
      await refetchAgents();
      await loadData();
      refreshChannelFilter();
      setShowTemplateModal(false);
      showToast(t('relationshipTemplateApplied'), 'success');
    } catch (err) {
      setTemplateError(err instanceof Error ? err.message : t('relationshipTemplateApplyError'));
    } finally {
      setApplyingTemplate(null);
    }
  }, [loadData, refetchAgents, refreshChannelFilter, selectedModelProvider, showToast]);

  // ---- Edge click → show detail panel ----

  const agentNameMap = useMemo(() => {
    const m = new Map<string, { name: string; isActive: boolean }>();
    for (const a of visibleAgents) m.set(a.id, { name: a.name, isActive: a.is_active ?? false });
    return m;
  }, [visibleAgents]);

  const onEdgeClick = useCallback((_event: React.MouseEvent, edge: Edge) => {
    setSelectedEdge(edge);
    setEdges((prev) => prev.map((e) => ({ ...e, selected: e.id === edge.id })));
    const rel = edgeToRelationshipMap.current.get(edge.id);
    if (rel) {
      const fromInfo = agentNameMap.get(rel.from_agent_id);
      const toInfo = agentNameMap.get(rel.to_agent_id);
      const detail = {
        ...rel,
        from_agent_name: fromInfo?.name,
        from_agent_active: fromInfo?.isActive,
        to_agent_name: toInfo?.name,
        to_agent_active: toInfo?.isActive,
      };
      if (onDetailOpen) {
        onDetailOpen({ relationship: detail, agent: null });
        setDetailRel(null);
      } else {
        setDetailRel(detail);
      }
      setDetailAgent(null);
    }
  }, [agentNameMap, onDetailOpen, setEdges]);

  const onNodeClick: NodeMouseHandler = useCallback((_event, node) => {
    setSelectedEdge(null);
    setEdges((prev) => prev.map((e) => ({ ...e, selected: false })));
    const agent = visibleAgents.find((a) => a.id === node.id);
    if (agent) {
      if (onDetailOpen) {
        onDetailOpen({ relationship: null, agent });
        setDetailAgent(null);
      } else {
        setDetailAgent(agent);
      }
      setDetailRel(null);
    }
  }, [onDetailOpen, setEdges, visibleAgents]);

  const closeDetailPanel = useCallback(() => {
    setDetailRel(null);
    setDetailAgent(null);
    setSelectedEdge(null);
    setEdges((prev) => prev.map((e) => ({ ...e, selected: false })));
    onDetailClose?.();
  }, [onDetailClose, setEdges]);

  const handleDetailUpdate = useCallback(() => {
    loadData();
    refreshChannelFilter();
  }, [loadData, refreshChannelFilter]);

  const handleDetailDelete = useCallback((id: string) => {
    setEdges((prev) => prev.filter((e) => e.id !== id));
    edgeToRelationshipMap.current.delete(id);
    setSelectedEdge(null);
    refreshChannelFilter();
  }, [refreshChannelFilter, setEdges]);

  const handleAgentDeleted = useCallback(() => {
    void refetchAgents();
    loadData();
    refreshChannelFilter();
  }, [loadData, refetchAgents, refreshChannelFilter]);

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
    setEdges((prev) => prev.map((e) => ({ ...e, selected: false })));
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
        if (isEditableTarget(e.target)) return;
        e.preventDefault();
        if (e.shiftKey) redo(); else undo();
      }
      if (e.key === 'Escape') {
        setSelectedEdge(null);
        setEdges((prev) => prev.map((edge) => ({ ...edge, selected: false })));
        setShowCreateModal(false);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [selectedEdge, deleteSelectedEdge, undo, redo, setEdges]);

  // ---- Auto layout ----
  // Dagre-based layered layout, TB direction.
  // - assigns_to: directional edges that define the rank hierarchy
  //   (parent on top, child below).
  // - collaborates_with: same-rank constraint, implemented via a compound
  //   graph. Every collab pair gets wrapped in a shared parent cluster
  //   with rank: 'same', which forces dagre to keep them on one row.
  // - assigns_to wins when both relationships exist on the same pair.

  const autoLayout = useCallback((recordUndo = true) => {
    if (recordUndo) pushUndo();
    try { localStorage.removeItem(posStorageKey); } catch { /* noop */ }
    setNodes((prev) => {
      const nodeSize = (node: Node) => {
        const hasTask = !!(node.data as { task?: unknown }).task;
        return hasTask ? { width: 280, height: 220 } : { width: 180, height: 100 };
      };
      const g = new dagre.graphlib.Graph({ compound: true });
      g.setGraph({ rankdir: 'TB', nodesep: 100, ranksep: 140, marginx: 80, marginy: 80 });
      g.setDefaultEdgeLabel(() => ({}));

      for (const n of prev) {
        g.setNode(n.id, nodeSize(n));
      }

      const pairKey = (a: string, b: string) => a < b ? `${a}|${b}` : `${b}|${a}`;
      const assignsPairs = new Set<string>();
      for (const e of edges) {
        if (!g.hasNode(e.source) || !g.hasNode(e.target)) continue;
        if (e.data?.relType === 'assigns_to') {
          assignsPairs.add(pairKey(e.source, e.target));
        }
      }
      const connectedIds = new Set<string>();
      for (const e of edges) {
        if (!g.hasNode(e.source) || !g.hasNode(e.target)) continue;
        connectedIds.add(e.source);
        connectedIds.add(e.target);
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

      const laidOut = prev.map((n) => {
        const pos = g.node(n.id);
        if (!pos) return n;
        const size = nodeSize(n);
        return {
          ...n,
          position: { x: pos.x - size.width / 2, y: pos.y - size.height / 2 },
        };
      });
      const connectedBottom = Math.max(
        0,
        ...laidOut
          .filter((n) => connectedIds.has(n.id))
          .map((n) => n.position.y + nodeSize(n).height),
      );
      let isolatedIndex = 0;
      const next = laidOut.map((n) => {
        if (connectedIds.has(n.id)) return n;
        const i = isolatedIndex++;
        return {
          ...n,
          position: {
            x: (i % 4) * 300,
            y: connectedIds.size ? connectedBottom + 160 + Math.floor(i / 4) * 240 : Math.floor(i / 4) * 240,
          },
        };
      });
      savePositions(next);
      return next;
    });
    fitGraph();
  }, [pushUndo, posStorageKey, setNodes, savePositions, fitGraph, edges]);

  const taskLayoutKey = nodes
    .map((n) => {
      const task = (n.data as { task?: AgentNodeTask }).task;
      return task ? `${n.id}:${task.id}` : '';
    })
    .filter(Boolean)
    .sort()
    .join(',');
  const autoLayoutKey = `${posStorageKey}:${nodes.map((n) => n.id).sort().join(',')}:${edges.map((e) => e.id).sort().join(',')}:${taskLayoutKey}`;
  const autoLayoutKeyRef = useRef('');
  const taskLayoutKeyRef = useRef<string | null>(null);
  useEffect(() => {
    if (nodes.length === 0 || autoLayoutKeyRef.current === autoLayoutKey) return;
    const previousTaskLayoutKey = taskLayoutKeyRef.current;
    const taskLayoutChanged = previousTaskLayoutKey !== null && previousTaskLayoutKey !== taskLayoutKey;
    const hasTaskCards = taskLayoutKey.length > 0;
    taskLayoutKeyRef.current = taskLayoutKey;
    autoLayoutKeyRef.current = autoLayoutKey;
    if (Object.keys(loadPositions()).length > 0 && !hasTaskCards && !taskLayoutChanged) return;
    autoLayout(false);
  }, [autoLayout, autoLayoutKey, loadPositions, nodes.length, taskLayoutKey]);

  // ---- Loading ----

  const loading = isLoading || agentsLoading;
  const workspaceError = error;

  if (loading) {
    const content = (
      <div className="flex flex-1 items-center justify-center gap-2">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        <span className="font-heading text-sm uppercase tracking-wider text-muted-foreground">
          {t('relationshipEditorLoading')}
        </span>
      </div>
    );
    return embedded ? content : <AppFrame>{content}</AppFrame>;
  }

  if (workspaceError) {
    const content = (
      <div className="flex flex-1 flex-col items-center justify-center gap-4">
        <p className="font-mono text-sm text-brutal-danger">{workspaceError}</p>
        <button
          type="button"
          onClick={() => {
            void loadData();
          }}
          className="btn-brutal px-4 py-2"
        >
          {t('retry')}
        </button>
      </div>
    );
    return embedded ? content : <AppFrame>{content}</AppFrame>;
  }

  // ---- Render ----

  const content = (
    <div className={`${embedded ? 'h-full ' : ''}flex min-w-0 flex-1 overflow-hidden`}>
      {/* Main editor area */}
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        {/* Toolbar */}
        {!embedded && (
          <div className="sidebar-collapse-offset flex items-center gap-2 h-14 px-4 border-b-2 border-black bg-brutal-cream">
            <h1 className="font-heading text-lg font-bold uppercase tracking-wider mr-auto">
              {title}
            </h1>

            {/* Undo/Redo */}
            <Button
              type="button"
              onClick={undo}
              disabled={undoStack.length === 0}
              variant="outline"
              size="sm"
              className="gap-1 px-2"
              title={t('relationshipEditorUndo')}
              aria-label={t('relationshipEditorUndo')}
            >
              <Undo2 className="h-3.5 w-3.5" />
            </Button>
            <Button
              type="button"
              onClick={redo}
              disabled={redoStack.length === 0}
              variant="outline"
              size="sm"
              className="gap-1 px-2"
              title={t('relationshipEditorRedo')}
              aria-label={t('relationshipEditorRedo')}
            >
              <Redo2 className="h-3.5 w-3.5" />
            </Button>

            <div className="w-px h-6 bg-black/20" />

            {/* Auto layout */}
            <Button
              type="button"
              onClick={() => autoLayout()}
              variant="outline"
              size="sm"
              className="gap-1.5 uppercase tracking-wider"
            >
              <LayoutGrid className="h-3.5 w-3.5" />
              {t('relationshipEditorAutoLayout')}
            </Button>
            <Button
              type="button"
              onClick={() => setShowChoiceDialog(true)}
              variant="success"
              size="sm"
              className="gap-1.5 uppercase tracking-wider"
            >
              <Plus className="h-3.5 w-3.5" />
              {t('relationshipAddAgent')}
            </Button>
          </div>
        )}
        {embedded && (
          <div className="flex h-12 flex-shrink-0 items-center justify-end gap-2 bg-brutal-cream px-3">
            {embeddedActions}
            <Button
              type="button"
              onClick={() => autoLayout()}
              variant="outline"
              size="sm"
              className="gap-1.5 uppercase tracking-wider"
              title={t('relationshipEditorAutoLayout')}
              aria-label={t('relationshipEditorAutoLayout')}
            >
              <LayoutGrid className="h-3.5 w-3.5" />
              {t('relationshipEditorAutoLayout')}
            </Button>
          </div>
        )}

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
            onInit={(instance) => {
              flowRef.current = instance;
              fitGraph();
            }}
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
          </ReactFlow>

          {/* Create relationship modal (T5.2.4) */}
          <CreateRelationshipModal
            open={showCreateModal}
            onOpenChange={handleCreateModalClose}
            onCreated={handleRelationshipCreated}
            preselectedFrom={preselectedFrom ?? undefined}
            preselectedTo={preselectedTo ?? undefined}
            agents={visibleAgents}
          />

          <Dialog open={showChoiceDialog} onOpenChange={setShowChoiceDialog} width="sm">
            <DialogHeader>
              <DialogTitle>{t('relationshipCreateAgent')}</DialogTitle>
              <DialogCloseButton onClick={() => setShowChoiceDialog(false)} />
            </DialogHeader>
            <div className="space-y-3">
              <button
                type="button"
                onClick={() => {
                  setShowChoiceDialog(false);
                  setShowCreateAgentModal(true);
                }}
                className="w-full flex items-center gap-3 p-4 border-2 border-black bg-white hover:bg-brutal-primary-light text-left"
              >
                <Plus className="h-5 w-5 flex-shrink-0" />
                <div>
                  <div className="font-heading text-sm font-bold">{t('relationshipSingleAgent')}</div>
                  <p className="font-sans text-xs text-muted-foreground mt-0.5">{t('relationshipSingleAgentDesc')}</p>
                </div>
              </button>
              <button
                type="button"
                onClick={handleOpenTemplates}
                className="w-full flex items-center gap-3 p-4 border-2 border-black bg-white hover:bg-brutal-accent-light text-left"
              >
                <Layers className="h-5 w-5 flex-shrink-0" />
                <div>
                  <div className="font-heading text-sm font-bold">{t('relationshipFromTemplate')}</div>
                  <p className="font-sans text-xs text-muted-foreground mt-0.5">{t('relationshipFromTemplateDesc')}</p>
                </div>
              </button>
            </div>
          </Dialog>

          <Dialog
            open={showCreateAgentModal}
            onOpenChange={(open) => {
              if (!open) setShowCreateAgentModal(false);
            }}
            width="lg"
          >
            <DialogHeader>
              <DialogTitle>{t('teamsCreateAgent')}</DialogTitle>
              <DialogCloseButton onClick={() => setShowCreateAgentModal(false)} />
            </DialogHeader>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                setShowCreateAgentModal(false);
                setShowChoiceDialog(true);
              }}
              className="mb-4 gap-1.5"
            >
              <ArrowLeft className="h-3.5 w-3.5" />
              {t('back')}
            </Button>
            <AgentForm
              onSubmit={handleCreateAgent}
              isSubmitting={isCreatingAgent}
              submitLabel={t('teamsCreateAgent')}
            />
          </Dialog>

          <Dialog open={showTemplateModal} onOpenChange={setShowTemplateModal} width="lg">
            <DialogHeader>
              <DialogTitle>{t('relationshipCreateFromTemplate')}</DialogTitle>
              <DialogCloseButton onClick={() => setShowTemplateModal(false)} />
            </DialogHeader>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                setShowTemplateModal(false);
                setShowChoiceDialog(true);
              }}
              className="mb-4 gap-1.5"
            >
              <ArrowLeft className="h-3.5 w-3.5" />
              {t('back')}
            </Button>
            <div className="space-y-4 max-h-[60vh] overflow-y-auto">
              <div>
                <label className="block font-heading text-xs font-bold uppercase tracking-wider mb-1.5">
                  {t('relationshipRuntimeRequired')}
                </label>
                {detectionLoading ? (
                  <p className="font-mono text-xs text-muted-foreground">{t('relationshipDetectingRuntimes')}</p>
                ) : (
                  <Select
                    value={selectedModelProvider}
                    onChange={setSelectedModelProvider}
                    options={(Object.values(detection) as AgentBackendDetectItem[]).map((rt) => ({
                      value: rt.type,
                      label: `${rt.available ? '●' : '○'} ${rt.display_name}${rt.version ? ` (${rt.version})` : ''}`,
                      disabled: !rt.available,
                    }))}
                    placeholder={t('relationshipSelectRuntime')}
                    size="md"
                    className="w-full"
                  />
                )}
              </div>

              {templatesLoading ? (
                <div className="flex items-center justify-center py-8">
                  <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                </div>
              ) : templates.length === 0 ? (
                <p className="font-mono text-sm text-muted-foreground text-center py-4">{t('relationshipNoTemplates')}</p>
              ) : (
                [...new Set(templates.map((tmpl) => tmpl.category))].map((category) => (
                  <div key={category}>
                    <h3 className="font-heading text-xs font-bold uppercase tracking-wider text-muted-foreground mb-2 border-b-2 border-black pb-1">
                      {category}
                    </h3>
                    <div className="space-y-2">
                      {templates.filter((tmpl) => tmpl.category === category).map((tmpl) => (
                        <div key={tmpl.id} className="flex items-start gap-3 p-3 border-2 border-black bg-white">
                          <span className="text-2xl flex-shrink-0">{tmpl.icon}</span>
                          <div className="flex-1 min-w-0">
                            <div className="flex items-center gap-1.5">
                              <span className="font-heading text-sm font-bold text-black">{tmpl.name}</span>
                              <span className="inline-flex items-center justify-center h-5 min-w-[1.25rem] px-1 border-2 border-black bg-brutal-cream font-mono text-[10px] font-bold text-black">
                                {tmpl.member_count}
                              </span>
                            </div>
                            <p className="font-sans text-xs text-muted-foreground mt-0.5">{tmpl.description}</p>
                          </div>
                          <Button
                            type="button"
                            onClick={() => handleApplyTemplate(tmpl.id)}
                            disabled={applyingTemplate === tmpl.id}
                            variant="success"
                            size="sm"
                            className="flex-shrink-0"
                          >
                            {applyingTemplate === tmpl.id ? <Loader2 className="h-3 w-3 animate-spin" /> : t('relationshipApplyTemplate')}
                          </Button>
                        </div>
                      ))}
                    </div>
                  </div>
                ))
              )}
              {templateError && (
                <p className="font-mono text-xs text-brutal-danger">{templateError}</p>
              )}
            </div>
          </Dialog>

          {/* Empty state overlay */}
          {visibleAgents.length === 0 && (
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
            {t('relationshipLegend')}:
          </span>
          {[
            { type: 'assigns_to', color: '#4A90D9', dash: '' },
            { type: 'collaborates_with', color: '#10B981', dash: '8,4' },
          ].map(({ type, color, dash }) => (
            <span key={type} className="flex items-center gap-1.5 font-mono text-[10px]">
              <svg width="24" height="8"><line x1="0" y1="4" x2="24" y2="4" stroke={color} strokeWidth={2} strokeDasharray={dash || undefined} /></svg>
              {relationshipTypeLabel(type)}
            </span>
          ))}
        </div>
      </div>
      <div
        className="flex-shrink-0 bg-brutal-cream overflow-hidden relative transition-[width] duration-100 ease-linear border-l-2 border-transparent"
        style={{ width: detailPanelOpen ? detailPanelWidth : 0, borderLeftColor: detailPanelOpen ? '#000' : 'transparent' }}
      >
        {detailPanelOpen && (
          <div
            className="absolute left-0 top-0 bottom-0 w-1.5 cursor-col-resize hover:bg-brutal-primary/50 transition-colors z-10"
            onMouseDown={(e) => {
              e.preventDefault();
              const startX = e.clientX;
              const startWidth = detailPanelWidth;
              const onMove = (ev: MouseEvent) => {
                const newWidth = Math.max(280, Math.min(800, startWidth + startX - ev.clientX));
                setDetailPanelWidth(newWidth);
              };
              const onUp = () => {
                document.removeEventListener('mousemove', onMove);
                document.removeEventListener('mouseup', onUp);
              };
              document.addEventListener('mousemove', onMove);
              document.addEventListener('mouseup', onUp);
            }}
          />
        )}
        {detailPanelOpen && (
          <RelationshipDetailPanel
            key={detailAgent ? `agent-${detailAgent.id}` : `relationship-${detailRel?.id}`}
            relationship={detailRel}
            agent={detailAgent}
            onClose={closeDetailPanel}
            onUpdate={handleDetailUpdate}
            onDelete={handleDetailDelete}
            onAgentDeleted={handleAgentDeleted}
            embedded
          />
        )}
      </div>
      </div>
  );

  return embedded ? content : <AppFrame>{content}</AppFrame>;
}
