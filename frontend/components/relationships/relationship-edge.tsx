// ============================================================================
// RelationshipEdge — custom ReactFlow edge for 2 relationship types
// - assigns_to: solid line with arrow
// - collaborates_with: dashed line, no arrow (bidirectional)
// - Uses smoothstep path for cleaner routing
// ============================================================================

import { memo, type CSSProperties } from 'react';
import { BaseEdge, EdgeLabelRenderer, getSmoothStepPath, Position, type EdgeProps } from '@xyflow/react';
import { GitFork, Handshake } from 'lucide-react';
import type { RelationshipType } from '@/lib/types';

interface RelationshipEdgeData {
  relType: RelationshipType;
  channelName?: string;
  stroke?: string;
  ariaLabel?: string;
  onSelect?: (event: React.MouseEvent<HTMLButtonElement>) => void;
}

const EDGE_STYLES: Record<RelationshipType, { stroke: string }> = {
  assigns_to:        { stroke: 'var(--skin-accent)' },
  collaborates_with: { stroke: 'var(--skin-success)' },
};

function collaborationDetour(
  sourceX: number,
  sourceY: number,
  targetX: number,
  targetY: number,
  sourcePosition: Position,
  targetPosition: Position,
): [string, number, number] | null {
  // ponytail: long orthogonal detour; replace with obstacle routing only if free-form graphs need it.
  if (
    sourcePosition === Position.Right
    && targetPosition === Position.Left
    && Math.abs(targetX - sourceX) > 180
  ) {
    const direction = Math.sign(targetX - sourceX) || 1;
    const detourY = Math.max(sourceY, targetY) + 52;
    return [
      `M${sourceX} ${sourceY} C${sourceX + direction * 16} ${sourceY},${sourceX + direction * 16} ${detourY},${sourceX + direction * 32} ${detourY} L${targetX - direction * 32} ${detourY} C${targetX - direction * 16} ${detourY},${targetX - direction * 16} ${targetY},${targetX} ${targetY}`,
      sourceX + direction * 48,
      detourY,
    ];
  }
  if (
    sourcePosition === Position.Bottom
    && targetPosition === Position.Top
    && Math.abs(targetY - sourceY) > 180
  ) {
    const direction = Math.sign(targetY - sourceY) || 1;
    const detourX = Math.max(sourceX, targetX) + 52;
    return [
      `M${sourceX} ${sourceY} C${sourceX} ${sourceY + direction * 16},${detourX} ${sourceY + direction * 16},${detourX} ${sourceY + direction * 32} L${detourX} ${targetY - direction * 32} C${detourX} ${targetY - direction * 16},${targetX} ${targetY - direction * 16},${targetX} ${targetY}`,
      detourX,
      sourceY + direction * 48,
    ];
  }
  return null;
}

function RelationshipEdgeComponent({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  data,
  markerEnd,
  selected,
}: EdgeProps) {
  const smoothPath = getSmoothStepPath({
    sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition,
    borderRadius: 8,
  });

  const edgeData = data as RelationshipEdgeData | undefined;
  const relType = edgeData?.relType || 'collaborates_with';
  const style = EDGE_STYLES[relType] || EDGE_STYLES.collaborates_with;
  const stroke = edgeData?.stroke || style.stroke;
  const isCollaboration = relType === 'collaborates_with';
  const [edgePath, pathLabelX, pathLabelY] = (
    isCollaboration
      ? collaborationDetour(sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition)
      : null
  ) ?? smoothPath;
  const [labelX, labelY] = !isCollaboration && targetPosition === Position.Top
    ? [targetX, targetY - 28]
    : [pathLabelX, pathLabelY];

  return (
    <>
      {/* Selected color halo sits behind the main path. */}
      {selected && (
        <BaseEdge
          id={`${id}-glow`}
          path={edgePath}
          style={{
            stroke,
            strokeWidth: 8,
            strokeDasharray: isCollaboration ? '8,4' : 'none',
            opacity: 0.2,
            pointerEvents: 'none',
          }}
        />
      )}

      {/* Main edge path */}
      <BaseEdge
        id={id}
        path={edgePath}
        style={{
          stroke,
          strokeWidth: selected ? 4 : (isCollaboration ? 2 : 2.5),
          strokeDasharray: isCollaboration ? '8,4' : 'none',
          cursor: 'pointer',
        }}
        interactionWidth={12}
        markerEnd={isCollaboration ? undefined : markerEnd}
      />

      {/* Language-neutral relationship affordance with its own unambiguous hit target. */}
      <EdgeLabelRenderer>
        <div
          style={{
            position: 'absolute',
            transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
            pointerEvents: 'none',
            zIndex: selected ? 20 : 10,
          }}
          className="nodrag nopan"
        >
          <button
            type="button"
            className="relationship-edge-hit-target nodrag nopan inline-flex h-8 w-8 items-center justify-center rounded-full focus-visible:outline-none"
            style={{ pointerEvents: 'auto' }}
            aria-label={edgeData?.ariaLabel}
            aria-pressed={selected}
            data-relationship-edge-id={id}
            onPointerDown={(event) => event.stopPropagation()}
            onClick={(event) => {
              event.stopPropagation();
              edgeData?.onSelect?.(event);
            }}
          >
            <span className={[
              'relationship-edge-label inline-flex h-5 w-5 items-center justify-center rounded-full border border-[var(--skin-rule)] bg-[var(--skin-surface)]',
              selected
                ? 'relationship-edge-label-selected'
                : '',
            ].join(' ')}
              style={{
                '--relationship-edge-color': stroke,
                color: selected ? 'var(--skin-surface)' : stroke,
                background: selected ? stroke : undefined,
                borderColor: selected ? stroke : undefined,
              } as CSSProperties}
            >
              {isCollaboration
                ? <Handshake className="h-3 w-3" />
                : <GitFork className="h-3 w-3" />}
            </span>
          </button>
        </div>
      </EdgeLabelRenderer>
    </>
  );
}

export const RelationshipEdge = memo(RelationshipEdgeComponent);
