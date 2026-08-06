'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Background,
  BackgroundVariant,
  Controls,
  ReactFlow,
  type Edge,
  type ReactFlowInstance,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { AlertTriangle, CornerUpLeft, GitMerge, Loader2, Plus, RefreshCw, X } from 'lucide-react';
import {
  ThinkingActivityContext,
  ThinkingNodeCard,
  type ThinkingFlowNode,
} from './thinking-node';
import type { ActivityCardPlacement } from '@/components/relationships/relationship-activity-card';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogCloseButton,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { cn } from '@/lib/utils';
import { useTeamAgentActivity } from '@/lib/hooks/use-team-agent-activity';
import { agentRunShowsHalo } from '@/lib/agent-activity';
import { t } from '@/lib/i18n';
import { motionDuration } from '@/lib/motion';
import type {
  CreateThinkingSynthesisInput,
  ThinkingNode,
  ThinkingSpace,
  ThinkingSynthesis,
} from '@/lib/types';

const nodeTypes = { thinkingNode: ThinkingNodeCard };
const NODE_WIDTH = 156;
const NODE_HEIGHT = 148;
const RING_GAP = 190;
const NODE_GAP = 28;
const OVERVIEW_NODE_LIMIT = 10;

function outwardActivityPlacement(angle: number, isRoot: boolean): ActivityCardPlacement {
  if (isRoot) return 'top';
  const horizontal = Math.cos(angle);
  const vertical = Math.sin(angle);
  if (Math.abs(horizontal) > Math.abs(vertical)) return horizontal > 0 ? 'right' : 'left';
  return vertical > 0 ? 'bottom' : 'top';
}

function layoutSpace(
  space: ThinkingSpace,
  selectedNodeId: string | null,
  synthesisSelectionMode: boolean,
  selectedSynthesisNodeIds: ReadonlySet<string>,
) {
  const children = new Map<string, ThinkingNode[]>();
  for (const node of space.nodes) {
    if (!node.parent_id) continue;
    children.set(node.parent_id, [...(children.get(node.parent_id) ?? []), node]);
  }
  children.forEach((nodes) => nodes.sort((a, b) => a.sort_order - b.sort_order));

  const weights = new Map<string, number>();
  const subtreeWeight = (nodeId: string): number => {
    const cached = weights.get(nodeId);
    if (cached !== undefined) return cached;
    const nodeChildren = children.get(nodeId) ?? [];
    const value = Math.max(1, nodeChildren.reduce((total, child) => total + subtreeWeight(child.id), 0));
    weights.set(nodeId, value);
    return value;
  };

  const angles = new Map<string, number>();
  const placeInSector = (node: ThinkingNode, start: number, end: number) => {
    angles.set(node.id, (start + end) / 2);
    const nodeChildren = children.get(node.id) ?? [];
    const total = nodeChildren.reduce((sum, child) => sum + subtreeWeight(child.id), 0);
    let cursor = start;
    for (const child of nodeChildren) {
      const span = (end - start) * (subtreeWeight(child.id) / total);
      placeInSector(child, cursor, cursor + span);
      cursor += span;
    }
  };

  const roots = space.nodes.filter((node) => !node.parent_id);
  for (const root of roots) {
    const rootChildren = children.get(root.id) ?? [];
    const total = rootChildren.reduce((sum, child) => sum + subtreeWeight(child.id), 0);
    const arc = rootChildren.length > 1 ? Math.PI * 2 : Math.PI;
    let cursor = rootChildren.length > 1 ? -Math.PI / 2 : 0;
    for (const child of rootChildren) {
      const span = arc * (subtreeWeight(child.id) / total);
      placeInSector(child, cursor, cursor + span);
      cursor += span;
    }
  }

  const maxDepth = space.nodes.reduce((max, node) => Math.max(max, node.depth), 0);
  const radii = new Map<number, number>([[0, 0]]);
  for (let depth = 1; depth <= maxDepth; depth++) {
    const levelAngles = space.nodes
      .filter((node) => node.depth === depth)
      .map((node) => angles.get(node.id))
      .filter((angle): angle is number => angle !== undefined)
      .map((angle) => ((angle % (Math.PI * 2)) + Math.PI * 2) % (Math.PI * 2))
      .sort((a, b) => a - b);
    let requiredRadius = 0;
    if (levelAngles.length > 1) {
      let smallestGap = Math.PI * 2;
      for (let index = 0; index < levelAngles.length; index++) {
        const current = levelAngles[index];
        const next = index === levelAngles.length - 1 ? levelAngles[0] + Math.PI * 2 : levelAngles[index + 1];
        smallestGap = Math.min(smallestGap, next - current);
      }
      requiredRadius = (NODE_WIDTH + NODE_GAP) / (2 * Math.sin(smallestGap / 2));
    }
    radii.set(depth, Math.max((radii.get(depth - 1) ?? 0) + RING_GAP, requiredRadius));
  }

  const nodes: ThinkingFlowNode[] = space.nodes.map((node) => {
    const angle = angles.get(node.id) ?? 0;
    const radius = radii.get(node.depth) ?? 0;
    return {
      id: node.id,
      type: 'thinkingNode',
      data: {
        node,
        activityPlacement: outwardActivityPlacement(angle, !node.parent_id),
        synthesisSelectionMode,
        selectedForSynthesis: selectedSynthesisNodeIds.has(node.id),
        synthesisSelectable: Boolean(node.returned_handoff || node.checkpoint_handoff),
      },
      position: node.parent_id
        ? { x: Math.cos(angle) * radius - NODE_WIDTH / 2, y: Math.sin(angle) * radius - NODE_HEIGHT / 2 }
        : { x: -NODE_WIDTH / 2, y: -NODE_HEIGHT / 2 },
      selected: node.id === selectedNodeId,
    };
  });
  const edges: Edge[] = space.nodes.flatMap((node) => node.parent_id ? [{
    id: `${node.parent_id}-${node.id}`,
    source: node.parent_id,
    target: node.id,
    type: 'straight',
    style: { stroke: 'var(--skin-subtle-text)', strokeWidth: 2 },
  }] : []);
  return { nodes, edges };
}

interface ThinkingWorkspaceProps {
  space: ThinkingSpace | null;
  selectedNodeId: string | null;
  isLoading: boolean;
  error: string | null;
  onSelect: (nodeId: string) => void;
  onCreateChild: (parentId: string, title: string) => Promise<ThinkingNode>;
  onRetryForkHandoff: (nodeId: string) => Promise<ThinkingNode>;
  onReturnNode: (nodeId: string) => Promise<ThinkingNode>;
  onCreateSynthesis: (input: CreateThinkingSynthesisInput) => Promise<ThinkingSynthesis>;
}

export function ThinkingWorkspace({
  space,
  selectedNodeId,
  isLoading,
  error,
  onSelect,
  onCreateChild,
  onRetryForkHandoff,
  onReturnNode,
  onCreateSynthesis,
}: ThinkingWorkspaceProps) {
  const [showSplit, setShowSplit] = useState(false);
  const [title, setTitle] = useState('');
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [flow, setFlow] = useState<ReactFlowInstance<ThinkingFlowNode, Edge> | null>(null);
  const [synthesisSelectionMode, setSynthesisSelectionMode] = useState(false);
  const [selectedSynthesisNodeIds, setSelectedSynthesisNodeIds] = useState<Set<string>>(() => new Set());
  const [showSynthesis, setShowSynthesis] = useState(false);
  const [synthesisTitle, setSynthesisTitle] = useState('');
  const [synthesisObjective, setSynthesisObjective] = useState('');
  const [hardConstraints, setHardConstraints] = useState('');
  const [notice, setNotice] = useState<string | null>(null);
  const { liveByThinkingNode } = useTeamAgentActivity();
  const previousNodeIdsRef = useRef<Set<string>>(new Set());
  const lastFocusKeyRef = useRef('');
  const graph = useMemo(
    () => space
      ? layoutSpace(space, selectedNodeId, synthesisSelectionMode, selectedSynthesisNodeIds)
      : { nodes: [], edges: [] },
    [selectedNodeId, selectedSynthesisNodeIds, space, synthesisSelectionMode],
  );
  const selectedNode = space?.nodes.find((node) => node.id === selectedNodeId) ?? null;
  const selectedNodeRunActive = agentRunShowsHalo(liveByThinkingNode.get(selectedNodeId ?? '')?.currentRun?.status);
  const parentRunActive = agentRunShowsHalo(liveByThinkingNode.get(selectedNode?.parent_id ?? '')?.currentRun?.status);
  const hasActiveChildren = Boolean(selectedNode && space?.nodes.some(
    (node) => node.parent_id === selectedNode.id && !node.returned_at,
  ));
  const rootHasTeamChildren = Boolean(selectedNode && !selectedNode.parent_id
    && space?.nodes.some((node) => node.parent_id === selectedNode.id && node.source === 'team'));
  const splitDisabled = !selectedNode || selectedNode.fork_handoff_pending || Boolean(selectedNode.returning_at || selectedNode.returned_at) || rootHasTeamChildren || busy;
  const returnDisabled = !selectedNode?.parent_id
    || selectedNode.message_count === 0
    || selectedNode.fork_handoff_pending
    || Boolean(selectedNode.returning_at || selectedNode.returned_at)
    || hasActiveChildren
    || selectedNodeRunActive
    || busy;
  const returnHint = selectedNode?.returned_at
    ? t('thinkingReturned')
    : selectedNode?.returning_at
      ? t('thinkingReturningHint')
      : hasActiveChildren
        ? t('thinkingReturnChildrenFirst')
        : t('thinkingReturnHint');
  const topologyKey = graph.nodes.map((node) => `${node.id}:${node.data.node.parent_id ?? ''}`).join('|');
  const selectedSynthesisNodes = useMemo(
    () => space?.nodes.filter((node) => selectedSynthesisNodeIds.has(node.id)) ?? [],
    [selectedSynthesisNodeIds, space?.nodes],
  );
  const hasStaleSynthesisSources = selectedSynthesisNodes.some(
    (node) => !node.returned_handoff && node.checkpoint_status === 'stale',
  );

  useEffect(() => {
    if (!flow || graph.nodes.length === 0) return;
    const focusKey = `${topologyKey}:${selectedNodeId ?? ''}`;
    if (focusKey === lastFocusKeyRef.current) return;

    const previousIds = previousNodeIdsRef.current;
    const added = previousIds.size === 0 ? [] : graph.nodes.filter((node) => !previousIds.has(node.id));
    const focusIds = new Set<string>();
    const showOverview = graph.nodes.length <= OVERVIEW_NODE_LIMIT;
    if (showOverview) {
      graph.nodes.forEach((node) => focusIds.add(node.id));
    } else if (added.length > 0) {
      for (const node of added) {
        focusIds.add(node.id);
        if (node.data.node.parent_id) focusIds.add(node.data.node.parent_id);
      }
    } else if (selectedNodeId) {
      focusIds.add(selectedNodeId);
      const selected = graph.nodes.find((node) => node.id === selectedNodeId);
      if (selected?.data.node.parent_id) {
        focusIds.add(selected.data.node.parent_id);
        graph.nodes.forEach((node) => {
          if (node.data.node.parent_id === selected.data.node.parent_id) focusIds.add(node.id);
        });
      }
      graph.nodes.forEach((node) => {
        if (node.data.node.parent_id === selectedNodeId) focusIds.add(node.id);
      });
    }
    if (focusIds.size === 0) focusIds.add(graph.nodes[0].id);

    previousNodeIdsRef.current = new Set(graph.nodes.map((node) => node.id));
    lastFocusKeyRef.current = focusKey;
    const frame = requestAnimationFrame(() => {
      void flow.fitView({
        nodes: graph.nodes.filter((node) => focusIds.has(node.id)),
        padding: showOverview ? 0.2 : 0.65,
        minZoom: 0.7,
        maxZoom: 1,
        duration: motionDuration(420),
      });
    });
    return () => cancelAnimationFrame(frame);
  }, [flow, graph.nodes, selectedNodeId, topologyKey]);

  useEffect(() => {
    setShowSplit(false);
    setTitle('');
    setActionError(null);
  }, [selectedNodeId]);

  useEffect(() => {
    if (!space) return;
    const availableIds = new Set(space.nodes.map((node) => node.id));
    setSelectedSynthesisNodeIds((current) => {
      const next = new Set([...current].filter((id) => availableIds.has(id)));
      return next.size === current.size ? current : next;
    });
  }, [space]);

  const exitSynthesisSelection = () => {
    setSynthesisSelectionMode(false);
    setSelectedSynthesisNodeIds(new Set());
    setShowSynthesis(false);
    setSynthesisTitle('');
    setSynthesisObjective('');
    setHardConstraints('');
    setActionError(null);
  };

  const toggleSynthesisNode = (node: ThinkingNode) => {
    if (!node.returned_handoff && !node.checkpoint_handoff) {
      setActionError(t('thinkingSynthesisSourceMissing'));
      return;
    }
    if (!selectedSynthesisNodeIds.has(node.id) && selectedSynthesisNodeIds.size >= 8) {
      setActionError(t('thinkingSynthesisSourceLimit'));
      return;
    }
    setActionError(null);
    setSelectedSynthesisNodeIds((current) => {
      const next = new Set(current);
      if (next.has(node.id)) {
        next.delete(node.id);
      } else {
        next.add(node.id);
      }
      return next;
    });
  };

  const createSynthesisDraft = async () => {
    if (busy || selectedSynthesisNodeIds.size < 2 || !synthesisObjective.trim()) return;
    setBusy(true);
    setActionError(null);
    try {
      const lines = hardConstraints.split('\n').map((line) => line.trim()).filter(Boolean);
      const synthesis = await onCreateSynthesis({
        title: synthesisTitle.trim() || undefined,
        objective: synthesisObjective.trim(),
        node_ids: [...selectedSynthesisNodeIds],
        constraints: {
          must_preserve: [],
          must_exclude: [],
          hard_constraints: lines,
          preferences: [],
          open_questions: [],
        },
        mode: 'single_agent',
      });
      setNotice(t('thinkingSynthesisCreated', { name: synthesis.title }));
      exitSynthesisSelection();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : t('thinkingSynthesisCreateError'));
    } finally {
      setBusy(false);
    }
  };

  const createBranch = async () => {
    if (!selectedNode || !title.trim() || busy) return;
    setBusy(true);
    setActionError(null);
    try {
      const node = await onCreateChild(selectedNode.id, title.trim());
      onSelect(node.id);
      setTitle('');
      setShowSplit(false);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : t('thinkingSpaceError'));
    } finally {
      setBusy(false);
    }
  };

  const returnNode = async () => {
    if (!selectedNode || busy) return;
    setBusy(true);
    setActionError(null);
    try {
      await onReturnNode(selectedNode.id);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : t('thinkingSpaceError'));
    } finally {
      setBusy(false);
    }
  };

  const retryForkHandoff = async () => {
    if (!selectedNode || busy || parentRunActive) return;
    setBusy(true);
    setActionError(null);
    try {
      await onRetryForkHandoff(selectedNode.id);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : t('thinkingSpaceError'));
    } finally {
      setBusy(false);
    }
  };

  if (isLoading && !space) {
    return <div className="flex flex-1 items-center justify-center"><Loader2 className="h-5 w-5 animate-spin" aria-label={t('thinkingSpaceLoading')} /></div>;
  }
  if ((error || actionError) && !space) {
    return <div className="m-4 border-2 border-black bg-brutal-danger-light p-3 font-mono text-xs">{error || actionError}</div>;
  }
  if (!space || space.nodes.length === 0) {
    return <div className="flex flex-1 items-center justify-center font-mono text-xs text-muted-foreground">{t('thinkingEmpty')}</div>;
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="border-b-2 border-black bg-brutal-cream px-3 py-2">
        <div className="flex items-center justify-between gap-2">
          <div className="min-w-0">
            <p className="font-mono text-[9px] font-bold uppercase tracking-widest text-muted-foreground">{t('thinkingCurrentBranch')}</p>
            <p className="truncate font-heading text-sm font-black">
              {synthesisSelectionMode
                ? t('thinkingSynthesisSelectedCount', { n: selectedSynthesisNodeIds.size })
                : selectedNode?.title ?? t('thinkingMode')}
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {synthesisSelectionMode ? (
              <>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="bg-white"
                  onClick={exitSynthesisSelection}
                  disabled={busy}
                >
                  <X className="h-3.5 w-3.5" /> {t('cancel')}
                </Button>
                <Button
                  type="button"
                  variant="success"
                  size="sm"
                  onClick={() => setShowSynthesis(true)}
                  disabled={selectedSynthesisNodeIds.size < 2 || busy}
                >
                  <GitMerge className="h-3.5 w-3.5" /> {t('thinkingSynthesisCreateDraft')}
                </Button>
              </>
            ) : (
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="bg-white"
                onClick={() => {
                  setNotice(null);
                  setActionError(null);
                  setSynthesisSelectionMode(true);
                }}
                disabled={busy}
              >
                <GitMerge className="h-3.5 w-3.5" /> {t('thinkingSynthesisCombine')}
              </Button>
            )}
            {!synthesisSelectionMode && (
              <>
            {selectedNode?.fork_handoff_pending && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="bg-white"
                onClick={() => { void retryForkHandoff(); }}
                disabled={parentRunActive || busy}
                title={parentRunActive ? t('thinkingForkPreparingHint') : t('thinkingForkRetryHint')}
              >
                {parentRunActive ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
                {parentRunActive ? t('thinkingForkPreparing') : t('thinkingForkRetry')}
              </Button>
            )}
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="bg-white"
              onClick={() => setShowSplit(true)}
              disabled={splitDisabled}
              title={selectedNode?.returned_at ? t('thinkingReturnedNoSplit') : undefined}
            >
              <Plus className="h-3.5 w-3.5" /> {t('thinkingSplit')}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="bg-white"
              onClick={() => { void returnNode(); }}
              disabled={returnDisabled}
              title={returnHint}
            >
              {selectedNode?.returning_at ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <CornerUpLeft className="h-3.5 w-3.5" />}
              {selectedNode?.returning_at ? t('thinkingReturning') : selectedNode?.returned_at ? t('thinkingReturned') : t('thinkingReturn')}
            </Button>
              </>
            )}
          </div>
        </div>
        {notice && <p className="mt-2 font-mono text-[10px] font-bold text-brutal-success">{notice}</p>}
        {synthesisSelectionMode && hasStaleSynthesisSources && (
          <p className="mt-2 flex items-center gap-1 font-mono text-[10px] font-bold text-black">
            <AlertTriangle className="h-3 w-3" /> {t('thinkingSynthesisStaleWarning')}
          </p>
        )}
        {(error || actionError) && <p className="mt-2 font-mono text-[10px] text-brutal-danger">{actionError || error}</p>}
      </div>
      <div className={cn('min-h-0 flex-1 bg-brutal-cream', busy && 'cursor-progress')}>
        <ThinkingActivityContext.Provider value={liveByThinkingNode}>
          <ReactFlow<ThinkingFlowNode>
            className="thinking-flow"
            nodes={graph.nodes}
            edges={graph.edges}
            nodeTypes={nodeTypes}
            onNodeClick={(_, node) => {
              if (synthesisSelectionMode) {
                toggleSynthesisNode(node.data.node);
              } else {
                onSelect(node.id);
              }
            }}
            nodesDraggable={false}
            nodesConnectable={false}
            elementsSelectable
            minZoom={0.25}
            maxZoom={1.5}
            onInit={setFlow}
            proOptions={{ hideAttribution: true }}
          >
            <Background variant={BackgroundVariant.Dots} gap={18} size={1.5} color="#b8b1a3" />
            <Controls
              showInteractive={false}
              className="flow-controls"
              style={{ border: '2px solid var(--color-brutal-black)', boxShadow: '3px 3px 0 var(--color-brutal-shadow)' }}
            />
          </ReactFlow>
        </ThinkingActivityContext.Provider>
      </div>
      <Dialog open={showSplit} onOpenChange={(open) => { if (!busy) setShowSplit(open); }} width="md">
        <DialogHeader>
          <DialogTitle>{t('thinkingSplitDialogTitle')}</DialogTitle>
          <DialogCloseButton onClick={() => setShowSplit(false)} />
        </DialogHeader>
        <DialogDescription>{t('thinkingSplitDescription', { name: selectedNode?.title ?? '' })}</DialogDescription>
        <form className="mt-5 space-y-4" onSubmit={(event) => { event.preventDefault(); void createBranch(); }}>
          <div className="space-y-2">
            <Label htmlFor="thinking-branch-title">{t('thinkingSplitTitleLabel')}</Label>
            <Input
              id="thinking-branch-title"
              autoFocus
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              maxLength={100}
              placeholder={t('thinkingSplitTitle')}
              disabled={busy}
            />
          </div>
          {actionError && <p className="font-mono text-xs text-brutal-danger" role="alert">{actionError}</p>}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setShowSplit(false)} disabled={busy}>{t('cancel')}</Button>
            <Button type="submit" variant="success" disabled={!title.trim() || busy}>
              {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : t('create')}
            </Button>
          </DialogFooter>
        </form>
      </Dialog>
      <Dialog open={showSynthesis} onOpenChange={(open) => { if (!busy) setShowSynthesis(open); }} width="lg">
        <DialogHeader>
          <DialogTitle>{t('thinkingSynthesisDialogTitle')}</DialogTitle>
          <DialogCloseButton onClick={() => setShowSynthesis(false)} />
        </DialogHeader>
        <DialogDescription>{t('thinkingSynthesisDialogDescription')}</DialogDescription>
        <form className="mt-5 space-y-4" onSubmit={(event) => { event.preventDefault(); void createSynthesisDraft(); }}>
          <div className="border-2 border-black bg-brutal-muted p-3">
            <p className="font-mono text-[10px] font-bold uppercase tracking-widest">{t('thinkingSynthesisSources')}</p>
            <ul className="mt-2 space-y-1 font-body text-sm">
              {selectedSynthesisNodes.map((node) => (
                <li key={node.id} className="flex items-center justify-between gap-3">
                  <span className="truncate">{node.title}</span>
                  <span className="shrink-0 font-mono text-[9px] font-bold uppercase">
                    {node.returned_handoff ? t('thinkingCurrentStateFinal') : node.checkpoint_status === 'stale' ? t('thinkingCurrentStateStale') : t('thinkingCurrentStateFresh')}
                  </span>
                </li>
              ))}
            </ul>
          </div>
          <div className="space-y-2">
            <Label htmlFor="thinking-synthesis-title">{t('thinkingSynthesisTitleLabel')}</Label>
            <Input
              id="thinking-synthesis-title"
              value={synthesisTitle}
              onChange={(event) => setSynthesisTitle(event.target.value)}
              maxLength={150}
              placeholder={t('thinkingSynthesisTitlePlaceholder')}
              disabled={busy}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="thinking-synthesis-objective">{t('thinkingSynthesisObjectiveLabel')}</Label>
            <Textarea
              id="thinking-synthesis-objective"
              autoFocus
              value={synthesisObjective}
              onChange={(event) => setSynthesisObjective(event.target.value)}
              maxLength={4000}
              placeholder={t('thinkingSynthesisObjectivePlaceholder')}
              disabled={busy}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="thinking-synthesis-constraints">{t('thinkingSynthesisConstraintsLabel')}</Label>
            <Textarea
              id="thinking-synthesis-constraints"
              value={hardConstraints}
              onChange={(event) => setHardConstraints(event.target.value)}
              placeholder={t('thinkingSynthesisConstraintsPlaceholder')}
              disabled={busy}
            />
            <p className="font-mono text-[9px] text-muted-foreground">{t('thinkingSynthesisConstraintsHint')}</p>
          </div>
          {hasStaleSynthesisSources && (
            <p className="flex items-center gap-1 border-2 border-black bg-brutal-warning-light p-2 font-mono text-[10px] font-bold">
              <AlertTriangle className="h-3.5 w-3.5" /> {t('thinkingSynthesisStaleWarning')}
            </p>
          )}
          {actionError && <p className="font-mono text-xs text-brutal-danger" role="alert">{actionError}</p>}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setShowSynthesis(false)} disabled={busy}>{t('cancel')}</Button>
            <Button type="submit" variant="success" disabled={!synthesisObjective.trim() || selectedSynthesisNodeIds.size < 2 || busy}>
              {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : t('thinkingSynthesisCreateDraft')}
            </Button>
          </DialogFooter>
        </form>
      </Dialog>
    </div>
  );
}
