// ============================================================================
// SwarmDAG — SVG-based DAG visualization for swarm subtask dependencies
// (Step 6 — T6.3.9)
// - Parent task rendered at the top as a header node
// - Subtask nodes rendered below with color-coded status outlines
// - Dependency arrows between nodes (subtask depends on other subtask)
// - Pure SVG, no external dependencies
// ============================================================================

'use client';

import { useMemo } from 'react';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import type { SwarmStatus, SwarmSubtaskStatus, TaskStatus } from '@/lib/types';

interface SwarmDAGProps {
  /** Swarm status data from API */
  swarm: SwarmStatus;
  /** Called when a subtask node is clicked */
  onSubtaskClick?: (taskId: string) => void;
  /** Optional CSS class */
  className?: string;
}

// ---- Layout constants ----
const NODE_WIDTH = 180;
const NODE_HEIGHT = 52;
const HEADER_HEIGHT = 60;
const GAP_X = 40;
const GAP_Y = 80;
const PADDING = 24;
const ARROW_SIZE = 8;
const CORNER_RADIUS = 4;

// ---- Status color config ----

const STATUS_COLORS: Record<TaskStatus, { stroke: string; fill: string; text: string }> = {
  todo: { stroke: '#666666', fill: '#ffffff', text: '#000000' },
  in_progress: { stroke: '#74B9FF', fill: '#E0F0FF', text: '#000000' },
  in_review: { stroke: '#bbafe6', fill: '#f0ecff', text: '#000000' },
  done: { stroke: '#88D498', fill: '#E0F5E0', text: '#000000' },
  closed: { stroke: '#c0b9b1', fill: '#ece6df', text: '#000000' },
};

const STATUS_LABELS: Record<TaskStatus, string> = {
  todo: 'TODO',
  in_progress: 'DOING',
  in_review: 'REVIEW',
  done: 'DONE',
  closed: 'CLOSED',
};

function truncate(str: string, max: number): string {
  if (str.length <= max) return str;
  return str.slice(0, max - 1) + '…';
}

export function SwarmDAG({ swarm, onSubtaskClick, className }: SwarmDAGProps) {
  if (!swarm || swarm.total_subtasks === 0) {
    return (
      <div className={cn('flex flex-col items-center justify-center py-10 text-center', className)}>
        <svg width="48" height="48" viewBox="0 0 48 48" className="mb-3 text-muted-foreground">
          <rect x="2" y="2" width="44" height="44" fill="none" stroke="currentColor" strokeWidth="2" />
          <line x1="16" y1="16" x2="32" y2="32" stroke="currentColor" strokeWidth="2" />
          <line x1="32" y1="16" x2="16" y2="32" stroke="currentColor" strokeWidth="2" />
        </svg>
        <p className="font-heading text-sm font-bold text-muted-foreground">
          {t('swarmDagNoData')}
        </p>
      </div>
    );
  }

  const subtasks = swarm.subtasks;

  // Compute layout: parent at top center, subtasks in a grid/row below
  const { totalWidth, totalHeight, layout } = useMemo(() => {
    const cols = Math.min(subtasks.length, Math.max(2, Math.ceil(Math.sqrt(subtasks.length))));
    const rows = Math.ceil(subtasks.length / cols);

    const gridWidth = cols * NODE_WIDTH + (cols - 1) * GAP_X;
    const gridHeight = rows * NODE_HEIGHT + (rows - 1) * GAP_Y;

    const headerY = PADDING;
    const gridStartY = headerY + HEADER_HEIGHT + GAP_Y;
    const totalW = Math.max(NODE_WIDTH + 80, gridWidth) + PADDING * 2;
    const totalH = gridStartY + gridHeight + PADDING;

    const centerX = totalW / 2;

    // Position header (parent node)
    const layoutData = {
      header: { x: centerX - NODE_WIDTH / 2, y: headerY, taskNumber: swarm.parent_task_number, title: swarm.parent_title },
      nodes: subtasks.map((sub, i) => {
        const col = i % cols;
        const row = Math.floor(i / cols);
        const gridCenterX = centerX - gridWidth / 2;
        const x = gridCenterX + col * (NODE_WIDTH + GAP_X);
        const y = gridStartY + row * (NODE_HEIGHT + GAP_Y);
        return { x, y, subtask: sub };
      }),
      centerX,
      headerBottomY: headerY + HEADER_HEIGHT,
    };

    return { totalWidth: totalW, totalHeight: totalH, layout: layoutData };
  }, [subtasks]);

  // Build dependency edge set
  const edges = useMemo(() => {
    const result: { fromIdx: number; toIdx: number; fromTaskNumber: number }[] = [];
    for (let i = 0; i < subtasks.length; i++) {
      const sub = subtasks[i];
      if (sub.is_blocked && sub.blocking_task_numbers) {
        for (const blockerNum of sub.blocking_task_numbers) {
          const blockerIdx = subtasks.findIndex((s) => s.task_number === blockerNum);
          if (blockerIdx !== -1) {
            result.push({ fromIdx: blockerIdx, toIdx: i, fromTaskNumber: blockerNum });
          }
        }
      }
    }
    return result;
  }, [subtasks]);

  // Draw an arrow path from point A to point B
  const arrowPath = (x1: number, y1: number, x2: number, y2: number): string => {
    const midX = (x1 + x2) / 2;
    const startY = y1 + NODE_HEIGHT;
    const endY = y2;

    // Route: down from source -> horizontal -> down to target
    if (Math.abs(x1 - x2) < 20) {
      // Nearly aligned — straight line down
      return `M ${x1 + NODE_WIDTH / 2} ${startY} L ${x2 + NODE_WIDTH / 2} ${endY}`;
    }

    const path = [
      `M ${x1 + NODE_WIDTH / 2} ${startY}`,
      `L ${x1 + NODE_WIDTH / 2} ${startY + GAP_Y / 2}`,
      `L ${x2 + NODE_WIDTH / 2} ${startY + GAP_Y / 2}`,
      `L ${x2 + NODE_WIDTH / 2} ${endY}`,
    ].join(' ');

    return path;
  };

  return (
    <div className={cn('overflow-auto', className)}>
      <svg
        width={totalWidth}
        height={totalHeight}
        viewBox={`0 0 ${totalWidth} ${totalHeight}`}
        className="block"
        aria-label={t('swarmDagTitle')}
      >
        {/* Background grid pattern */}
        <defs>
          <pattern id="dag-grid" width="20" height="20" patternUnits="userSpaceOnUse">
            <circle cx="2" cy="2" r="0.8" fill="rgba(0,0,0,0.08)" />
          </pattern>
        </defs>
        <rect x="0" y="0" width={totalWidth} height={totalHeight} fill="url(#dag-grid)" />

        {/* Arrowhead marker */}
        <defs>
          <marker id="arrowhead" markerWidth={ARROW_SIZE} markerHeight={ARROW_SIZE} refX={ARROW_SIZE / 2} refY={ARROW_SIZE} orient="auto">
            <polygon points={`0,0 ${ARROW_SIZE},0 ${ARROW_SIZE / 2},${ARROW_SIZE}`} fill="#000" />
          </marker>
        </defs>

        {/* Parent-to-subtask connector lines (dashed) */}
        {layout.nodes.map((node, i) => (
          <line
            key={`parent-conn-${i}`}
            x1={layout.centerX}
            y1={layout.headerBottomY}
            x2={node.x + NODE_WIDTH / 2}
            y2={node.y}
            stroke="#000"
            strokeWidth="1.5"
            strokeDasharray="4 3"
            opacity="0.4"
          />
        ))}

        {/* Dependency edges */}
        {edges.map((edge, i) => {
          const fromNode = layout.nodes[edge.fromIdx];
          const toNode = layout.nodes[edge.toIdx];
          const path = arrowPath(fromNode.x, fromNode.y, toNode.x, toNode.y);
          return (
            <g key={`edge-${i}`}>
              <path
                d={path}
                stroke="#000"
                strokeWidth="2"
                fill="none"
                markerEnd="url(#arrowhead)"
              />
              {/* Tooltip label */}
              <title>{t('swarmDagBlocked', { n: edge.fromTaskNumber })}</title>
            </g>
          );
        })}

        {/* Parent header node */}
        <g>
          <rect
            x={layout.header.x}
            y={layout.header.y}
            width={NODE_WIDTH}
            height={HEADER_HEIGHT}
            rx={CORNER_RADIUS}
            fill="#FFD23F"
            stroke="#000"
            strokeWidth="3"
          />
          <text
            x={layout.header.x + 12}
            y={layout.header.y + 22}
            fontFamily="'Space Grotesk', sans-serif"
            fontSize="10"
            fontWeight="900"
            fill="#000"
            textAnchor="start"
          >
            {layout.header.taskNumber ? `#${layout.header.taskNumber}` : t('swarmParentTask')}
          </text>
          <text
            x={layout.header.x + 12}
            y={layout.header.y + 42}
            fontFamily="'Space Grotesk', sans-serif"
            fontSize="12"
            fontWeight="700"
            fill="#000"
            textAnchor="start"
          >
            {truncate(layout.header.title, 20)}
          </text>
        </g>

        {/* Subtask nodes */}
        {layout.nodes.map((node, i) => {
          const sub = node.subtask;
          const colors = STATUS_COLORS[sub.status] || STATUS_COLORS.todo;
          const isBlocked = sub.is_blocked;
          const blockedBy = sub.blocking_task_numbers;

          return (
            <g
              key={sub.task_id}
              className="cursor-pointer"
              onClick={() => onSubtaskClick?.(sub.task_id)}
              role="button"
              aria-label={`Task #${sub.task_number}: ${sub.title} (${STATUS_LABELS[sub.status]})`}
            >
              {/* Node background */}
              <rect
                x={node.x}
                y={node.y}
                width={NODE_WIDTH}
                height={NODE_HEIGHT}
                rx={CORNER_RADIUS}
                fill={isBlocked ? '#f5f0e8' : colors.fill}
                stroke={colors.stroke}
                strokeWidth="3"
                filter={isBlocked ? undefined : undefined}
                opacity={isBlocked ? 0.7 : 1}
              />
              {/* Status left edge indicator */}
              <rect
                x={node.x}
                y={node.y + CORNER_RADIUS}
                width="5"
                height={NODE_HEIGHT - CORNER_RADIUS * 2}
                fill={colors.stroke}
                rx="1"
              />

              {/* Task number */}
              <text
                x={node.x + 14}
                y={node.y + 18}
                fontFamily="'Space Mono', monospace"
                fontSize="11"
                fontWeight="700"
                fill={colors.text}
                textAnchor="start"
              >
                #{sub.task_number}
              </text>

              {/* Title */}
              <text
                x={node.x + 14}
                y={node.y + 36}
                fontFamily="'Inter', sans-serif"
                fontSize="11"
                fontWeight="600"
                fill={colors.text}
                textAnchor="start"
              >
                {truncate(sub.title, 18)}
              </text>

              {/* Status badge (top-right) */}
              <rect
                x={node.x + NODE_WIDTH - 48}
                y={node.y - 1}
                width="48"
                height="16"
                rx="2"
                fill={colors.stroke}
                stroke={colors.stroke}
                strokeWidth="1"
              />
              <text
                x={node.x + NODE_WIDTH - 24}
                y={node.y + 11}
                fontFamily="'Space Grotesk', sans-serif"
                fontSize="8"
                fontWeight="900"
                fill={colors.stroke === '#666666' || colors.stroke === '#c0b9b1' ? '#000' : '#000'}
                textAnchor="middle"
              >
                {STATUS_LABELS[sub.status]}
              </text>

              {/* Claimer name */}
              {sub.claimer_name && (
                <text
                  x={node.x + NODE_WIDTH - 10}
                  y={node.y + NODE_HEIGHT - 8}
                  fontFamily="'Space Mono', monospace"
                  fontSize="9"
                  fill={isBlocked ? '#999' : '#666'}
                  textAnchor="end"
                >
                  {truncate(sub.claimer_name, 14)}
                </text>
              )}

              {/* Blocked badge */}
              {isBlocked && blockedBy && blockedBy.length > 0 && (
                <g>
                  <rect
                    x={node.x + 10}
                    y={node.y + NODE_HEIGHT - 18}
                    width={Math.min(NODE_WIDTH - 20, blockedBy.length * 22 + 16)}
                    height="14"
                    rx="2"
                    fill="#ffe0dc"
                    stroke="#f97264"
                    strokeWidth="1.5"
                  />
                  <text
                    x={node.x + 16}
                    y={node.y + NODE_HEIGHT - 7}
                    fontFamily="'Space Grotesk', sans-serif"
                    fontSize="8"
                    fontWeight="900"
                    fill="#f97264"
                    textAnchor="start"
                  >
                    {t('swarmBlockedBy', { n: blockedBy.slice(0, 3).map((n) => `#${n}`).join(', ') })}
                  </text>
                </g>
              )}
            </g>
          );
        })}

        {/* Legend */}
        <g transform={`translate(${PADDING}, ${totalHeight - 60})`}>
          {(['done', 'in_progress', 'todo'] as TaskStatus[]).map((status, i) => {
            const colors = STATUS_COLORS[status];
            return (
              <g key={status} transform={`translate(${i * 80}, 0)`}>
                <rect x="0" y="0" width="10" height="10" fill={colors.fill} stroke={colors.stroke} strokeWidth="2" />
                <text x="14" y="9" fontFamily="'Space Grotesk', sans-serif" fontSize="8" fontWeight="700" fill="#000">
                  {STATUS_LABELS[status]}
                </text>
              </g>
            );
          })}
        </g>
      </svg>
    </div>
  );
}
