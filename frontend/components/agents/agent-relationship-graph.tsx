// ============================================================================
// AgentRelationshipGraph — pure SVG relationship graph (Step 2)
// - Zero new dependencies (~150 lines)
// - Fetch from GET /api/v1/agent-relationships
// - Tree layout (BFS from root nodes)
// - 4 edge types with distinct line styles
// - Pan/zoom via SVG viewBox transform
// - Click node → navigate to agent detail
// ============================================================================

'use client';

import { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { useRouter } from 'next/navigation';
import { Loader2, Maximize2 } from 'lucide-react';
import { cn } from '@/lib/utils';
import { apiClient } from '@/lib/api-client';
import { t } from '@/lib/i18n';
import type { AgentRelationship, RelationshipType } from '@/lib/types';

// ---- Layout constants ----

const NODE_RADIUS = 32;
const LAYER_GAP = 120;
const NODE_GAP = 200;
const INITIAL_VB = { x: -100, y: -100, w: 1200, h: 800 };

// ---- Edge style config ----

interface EdgeStyle {
  stroke: string;
  strokeWidth: number;
  dasharray: string;
}

const EDGE_STYLES: Record<RelationshipType, EdgeStyle> = {
  reports_to:       { stroke: '#4A90D9', strokeWidth: 2.5, dasharray: '' },
  delegates_to:     { stroke: '#7B6CF6', strokeWidth: 2,   dasharray: '' },
  collaborates_with: { stroke: '#10B981', strokeWidth: 2,   dasharray: '8,4' },
  escalates_to:     { stroke: '#EF4444', strokeWidth: 3,   dasharray: '' },
};

// ---- Layout engine: BFS from root nodes ----

interface LayoutNode {
  agent_id: string;
  agent_name: string;
  is_active: boolean;
  x: number;
  y: number;
}

interface LayoutEdge {
  from: string;
  to: string;
  type: RelationshipType;
}

function layoutGraph(relationships: AgentRelationship[]): { nodes: LayoutNode[]; edges: LayoutEdge[] } {
  // Collect unique agents
  const agentMap = new Map<string, { name: string; active: boolean }>();
  for (const r of relationships) {
    if (r.from_agent_id && !agentMap.has(r.from_agent_id)) {
      agentMap.set(r.from_agent_id, { name: r.from_agent_name || r.from_agent_id.slice(0, 8), active: r.from_agent_active ?? false });
    }
    if (r.to_agent_id && !agentMap.has(r.to_agent_id)) {
      agentMap.set(r.to_agent_id, { name: r.to_agent_name || r.to_agent_id.slice(0, 8), active: r.to_agent_active ?? false });
    }
  }

  // Build adjacency for parent→child (reports_to edges)
  const children = new Map<string, string[]>();
  const parentOf = new Map<string, string>();
  const edges: LayoutEdge[] = [];

  for (const r of relationships) {
    edges.push({ from: r.from_agent_id, to: r.to_agent_id, type: r.rel_type });
    if (r.rel_type === 'reports_to') {
      if (!children.has(r.to_agent_id)) children.set(r.to_agent_id, []);
      children.get(r.to_agent_id)!.push(r.from_agent_id);
      parentOf.set(r.from_agent_id, r.to_agent_id);
    }
  }

  // Root nodes: agents that don't report_to anyone
  const allAgentIds = Array.from(agentMap.keys());
  const rootIds = allAgentIds.filter((id) => !parentOf.has(id));

  // BFS layout
  const visited = new Set<string>();
  const nodes: LayoutNode[] = [];
  const queue: { id: string; depth: number }[] = [];
  const depthCounts: number[] = [];

  function enqueue(id: string, depth: number) {
    if (visited.has(id)) return;
    visited.add(id);
    queue.push({ id, depth });
    while (depthCounts.length <= depth) depthCounts.push(0);
    const x = depthCounts[depth] * NODE_GAP + 160;
    depthCounts[depth]++;
    const info = agentMap.get(id)!;
    nodes.push({ agent_id: id, agent_name: info.name, is_active: info.active, x, y: 120 + depth * LAYER_GAP });
  }

  for (const id of rootIds) {
    enqueue(id, 0);
  }

  let head = 0;
  while (head < queue.length) {
    const { id, depth } = queue[head++];
    for (const child of children.get(id) || []) {
      enqueue(child, depth + 1);
    }
  }

  // Enqueue any remaining orphan nodes
  for (const id of allAgentIds) {
    if (!visited.has(id)) {
      enqueue(id, depthCounts.length > 0 ? depthCounts.length - 1 : 0);
    }
  }

  // Fix: non-reports_to edges need coordinate lookup
  return { nodes, edges };
}

// ---- Component ----

export function AgentRelationshipGraph() {
  const router = useRouter();
  const svgRef = useRef<SVGSVGElement>(null);

  const [relationships, setRelationships] = useState<AgentRelationship[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Pan/zoom state
  const [viewBox, setViewBox] = useState(INITIAL_VB);
  const [drag, setDrag] = useState<{ sx: number; sy: number; vx: number; vy: number } | null>(null);
  const [hoveredNode, setHoveredNode] = useState<string | null>(null);

  // Fetch data
  useEffect(() => {
    let cancelled = false;
    async function load() {
      setIsLoading(true);
      setError(null);
      try {
        const res = await apiClient.get<AgentRelationship[]>('/api/v1/agent-relationships');
        if (!cancelled) setRelationships(Array.isArray(res) ? res : []);
      } catch (err) {
        if (!cancelled) setError(t('relationshipGraphLoading'));
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    }
    load();
    return () => { cancelled = true; };
  }, []);

  const { nodes, edges } = useMemo(() => layoutGraph(relationships), [relationships]);

  // Pan handlers
  const onMouseDown = useCallback((e: React.MouseEvent) => {
    if ((e.target as SVGElement).closest('[data-node]')) return;
    setDrag({ sx: e.clientX, sy: e.clientY, vx: viewBox.x, vy: viewBox.y });
  }, [viewBox]);

  const onMouseMove = useCallback((e: React.MouseEvent) => {
    if (!drag) return;
    const dx = e.clientX - drag.sx;
    const dy = e.clientY - drag.sy;
    setViewBox((vb) => ({ ...vb, x: drag.vx - dx * (vb.w / 1200), y: drag.vy - dy * (vb.h / 800) }));
  }, [drag]);

  const onMouseUp = useCallback(() => setDrag(null), []);

  const onWheel = useCallback((e: React.WheelEvent) => {
    e.preventDefault();
    const scale = e.deltaY > 0 ? 1.15 : 0.87;
    setViewBox((vb) => ({
      x: vb.x + vb.w * (1 - scale) * 0.5,
      y: vb.y + vb.h * (1 - scale) * 0.5,
      w: Math.max(200, Math.min(4000, vb.w * scale)),
      h: Math.max(150, Math.min(3000, vb.h * scale)),
    }));
  }, []);

  const resetView = useCallback(() => {
    setViewBox(INITIAL_VB);
  }, []);

  // Loading state
  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64 text-muted-foreground gap-2">
        <Loader2 className="h-5 w-5 animate-spin" />
        <span className="font-heading text-sm uppercase tracking-wider">{t('relationshipGraphLoading')}</span>
      </div>
    );
  }

  // Error state
  if (error) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="font-mono text-sm text-brutal-danger">{error}</p>
      </div>
    );
  }

  // Empty state
  if (nodes.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-3 py-16">
        <div className="w-16 h-16 border-4 border-black bg-brutal-cream flex items-center justify-center">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <circle cx="12" cy="5" r="2" />
            <path d="M12 12v7M8 19l4-7 4 7" />
          </svg>
        </div>
        <p className="font-heading text-sm text-muted-foreground max-w-xs text-center">
          {t('relationshipGraphEmpty')}
        </p>
      </div>
    );
  }

  // Branch the layout to handle multiple root branches
  const branchColors = ['#4A90D9', '#7B6CF6', '#10B981', '#EF4444', '#FFD93D', '#FF6B6B'];

  return (
    <div className="relative w-full overflow-hidden border-4 border-black bg-brutal-cream">
      {/* Toolbar */}
      <div className="absolute top-3 right-3 z-10 flex items-center gap-1">
        <button
          type="button"
          onClick={resetView}
          className="flex h-8 w-8 items-center justify-center border-2 border-black bg-white hover:bg-brutal-primary shadow-brutal-sm"
          title="Reset view"
          aria-label="Reset view"
        >
          <Maximize2 className="h-3.5 w-3.5" />
        </button>
      </div>

      {/* SVG canvas */}
      <svg
        ref={svgRef}
        viewBox={`${viewBox.x} ${viewBox.y} ${viewBox.w} ${viewBox.h}`}
        className="w-full h-[500px] cursor-grab select-none"
        style={{ background: '#FFFDF5' }}
        onMouseDown={onMouseDown}
        onMouseMove={onMouseMove}
        onMouseUp={onMouseUp}
        onMouseLeave={onMouseUp}
        onWheel={onWheel}
      >
        {/* Grid pattern */}
        <defs>
          <pattern id="grid" width="40" height="40" patternUnits="userSpaceOnUse">
            <path d="M 40 0 L 0 0 0 40" fill="none" stroke="rgba(0,0,0,0.06)" strokeWidth="0.5" />
          </pattern>
          {['reports_to', 'delegates_to', 'escalates_to'].map((t) => (
            <marker key={t} id={`arrow-${t}`} viewBox="0 0 10 10" refX="8" refY="5" markerWidth="8" markerHeight="8" orient="auto-start-reverse">
              <path d="M 0 0 L 10 5 L 0 10 z" fill={EDGE_STYLES[t as RelationshipType].stroke} />
            </marker>
          ))}
        </defs>
        <rect width="100%" height="100%" fill="url(#grid)" />

        {/* Edges */}
        {edges.map((e, i) => {
          const fromNode = nodes.find((n) => n.agent_id === e.from);
          const toNode = nodes.find((n) => n.agent_id === e.to);
          if (!fromNode || !toNode) return null;
          const style = EDGE_STYLES[e.type] || EDGE_STYLES.collaborates_with;
          const midX = (fromNode.x + toNode.x) / 2;
          const midY = (fromNode.y + toNode.y) / 2;

          // For escalates_to: draw double line effect
          if (e.type === 'escalates_to') {
            const dx = toNode.x - fromNode.x;
            const dy = toNode.y - fromNode.y;
            const len = Math.sqrt(dx * dx + dy * dy) || 1;
            const nx = -dy / len * 3;
            const ny = dx / len * 3;
            return (
              <g key={i}>
                <line
                  x1={fromNode.x + nx} y1={fromNode.y + ny}
                  x2={toNode.x + nx} y2={toNode.y + ny}
                  stroke={style.stroke} strokeWidth={style.strokeWidth}
                  markerEnd={`url(#arrow-escalates_to)`}
                />
                <line
                  x1={fromNode.x - nx} y1={fromNode.y - ny}
                  x2={toNode.x - nx} y2={toNode.y - ny}
                  stroke={style.stroke} strokeWidth={style.strokeWidth}
                />
                {/* Pulsing dot on double line */}
                <circle cx={midX} cy={midY} r="3" fill={style.stroke} className="animate-pulse" />
              </g>
            );
          }

          return (
            <g key={i}>
              <line
                x1={fromNode.x} y1={fromNode.y}
                x2={toNode.x} y2={toNode.y}
                stroke={style.stroke}
                strokeWidth={style.strokeWidth}
                strokeDasharray={style.dasharray || undefined}
                markerEnd={e.type !== 'collaborates_with' ? `url(#arrow-${e.type})` : undefined}
              />
              {/* Collaborates_with: bidirectional dots at both ends */}
              {e.type === 'collaborates_with' && (
                <>
                  <circle cx={toNode.x - 6} cy={toNode.y} r="3" fill={style.stroke} />
                  <circle cx={fromNode.x + 6} cy={fromNode.y} r="3" fill={style.stroke} />
                </>
              )}
            </g>
          );
        })}

        {/* Nodes */}
        {nodes.map((n) => {
          const isHovered = hoveredNode === n.agent_id;
          return (
            <g
              key={n.agent_id}
              data-node
              className="cursor-pointer"
              onClick={() => router.push(`/workspace?agent=${n.agent_id}`)}
              onMouseEnter={() => setHoveredNode(n.agent_id)}
              onMouseLeave={() => setHoveredNode(null)}
            >
              {/* Status ring */}
              <circle
                cx={n.x} cy={n.y} r={NODE_RADIUS + 4}
                fill="none"
                stroke={n.is_active ? '#10B981' : '#c0b9b1'}
                strokeWidth={isHovered ? 4 : 2}
              />
              {/* Node circle */}
              <circle
                cx={n.x} cy={n.y} r={NODE_RADIUS}
                fill={isHovered ? '#000' : '#1E293B'}
                stroke="#000"
                strokeWidth={2}
              />
              {/* Initial letter */}
              <text
                x={n.x} y={n.y + 7}
                textAnchor="middle"
                fill="white"
                fontSize={16}
                fontWeight={700}
                fontFamily="Space Grotesk, sans-serif"
              >
                {n.agent_name[0]?.toUpperCase() || '?'}
              </text>
              {/* Agent name label below */}
              <text
                x={n.x} y={n.y + NODE_RADIUS + 20}
                textAnchor="middle"
                fill="#000"
                fontSize={12}
                fontWeight={700}
                fontFamily="Space Grotesk, sans-serif"
              >
                {n.agent_name}
              </text>
              {/* Status dot */}
              <circle
                cx={n.x + NODE_RADIUS - 8}
                cy={n.y - NODE_RADIUS + 8}
                r={6}
                fill={n.is_active ? '#10B981' : '#c0b9b1'}
                stroke="#000"
                strokeWidth={1.5}
              />
            </g>
          );
        })}
      </svg>

      {/* Legend */}
      <div className="absolute bottom-3 left-3 z-10 flex flex-wrap gap-3 bg-white border-2 border-black px-3 py-1.5">
        <span className="font-heading text-[10px] font-bold uppercase tracking-wider">{t('relationshipGraphLegend')}:</span>
        {(['reports_to', 'delegates_to', 'collaborates_with', 'escalates_to'] as RelationshipType[]).map((type) => {
          const style = EDGE_STYLES[type];
          return (
            <span key={type} className="flex items-center gap-1.5 font-mono text-[10px]">
              <svg width="24" height="8" className="flex-shrink-0">
                <line x1="0" y1="4" x2="24" y2="4"
                  stroke={style.stroke}
                  strokeWidth={style.strokeWidth}
                  strokeDasharray={style.dasharray || undefined}
                />
              </svg>
              {t(type === 'reports_to' ? 'reportsTo' : type === 'delegates_to' ? 'delegatesTo' : type === 'collaborates_with' ? 'collaboratesWith' : 'escalatesTo')}
            </span>
          );
        })}
      </div>

      {/* Pan hint */}
      <div className="absolute top-3 left-3 z-10">
        <span className="font-mono text-[10px] text-muted-foreground bg-white/80 px-1.5 py-0.5">
          {t('relationshipGraphPanHint')}
        </span>
      </div>
    </div>
  );
}
