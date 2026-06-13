// ============================================================================
// RelationshipEdge — custom ReactFlow edge for 4 relationship types
// - reports_to: solid blue, arrow
// - delegates_to: solid purple, arrow
// - collaborates_with: dashed green, bidirectional (no arrow)
// - escalates_to: double red line, arrow, pulse dot
// - Click edge → show detail panel for editing/deleting
// ============================================================================

import { BaseEdge, EdgeLabelRenderer, getBezierPath, type EdgeProps } from '@xyflow/react';
import type { RelationshipType } from '@/lib/types';

interface RelationshipEdgeData {
  relType: RelationshipType;
  channelName?: string;
}

const EDGE_COLORS: Record<RelationshipType, { stroke: string; label: string }> = {
  reports_to:       { stroke: '#4A90D9', label: 'Reports To' },
  delegates_to:     { stroke: '#7B6CF6', label: 'Delegates To' },
  collaborates_with: { stroke: '#10B981', label: 'Collaborates' },
  escalates_to:     { stroke: '#EF4444', label: 'Escalates To' },
};

export function RelationshipEdge({
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
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition,
  });

  const edgeData = data as RelationshipEdgeData | undefined;
  const relType = edgeData?.relType || 'collaborates_with';
  const colors = EDGE_COLORS[relType] || EDGE_COLORS.collaborates_with;
  const isEscalation = relType === 'escalates_to';
  const isCollaboration = relType === 'collaborates_with';

  // Compute a parallel offset path for escalates_to double line
  let edgePath2 = '';
  if (isEscalation) {
    // Offset: shift perpendicular by 3px
    const dx = targetX - sourceX;
    const dy = targetY - sourceY;
    const len = Math.sqrt(dx * dx + dy * dy) || 1;
    const offset = 4;
    const nx = -dy / len * offset;
    const ny = dx / len * offset;
    [edgePath2] = getBezierPath({
      sourceX: sourceX + nx, sourceY: sourceY + ny,
      targetX: targetX + nx, targetY: targetY + ny,
      sourcePosition, targetPosition,
    });
  }

  return (
    <>
      {/* Main edge path */}
      <BaseEdge
        id={id}
        path={edgePath}
        style={{
          stroke: colors.stroke,
          strokeWidth: isEscalation ? 2.5 : 2,
          strokeDasharray: isCollaboration ? '8,4' : 'none',
        }}
        markerEnd={isCollaboration ? undefined : markerEnd}
      />

      {/* Escalation double line */}
      {isEscalation && edgePath2 && (
        <BaseEdge
          id={`${id}-parallel`}
          path={edgePath2}
          style={{
            stroke: colors.stroke,
            strokeWidth: 2.5,
            strokeDasharray: 'none',
          }}
          markerEnd={markerEnd}
        />
      )}

      {/* Selected glow */}
      {selected && (
        <BaseEdge
          id={`${id}-glow`}
          path={edgePath}
          style={{
            stroke: colors.stroke,
            strokeWidth: isEscalation ? 6 : 4,
            strokeDasharray: isCollaboration ? '8,4' : 'none',
            opacity: 0.3,
          }}
        />
      )}

      {/* Edge label */}
      <EdgeLabelRenderer>
        <div
          style={{
            position: 'absolute',
            transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
            pointerEvents: 'all',
          }}
          className="nodrag nopan"
        >
          <span className={[
            'inline-block px-2 py-0.5 border-2 border-black',
            'font-heading text-[9px] font-bold uppercase tracking-wider',
            selected
              ? 'bg-brutal-accent text-black'
              : 'bg-white text-muted-foreground',
          ].join(' ')}>
            {relType.replace(/_/g, ' ')}
            {edgeData?.channelName ? ` · #${edgeData.channelName}` : ''}
          </span>
        </div>
      </EdgeLabelRenderer>
    </>
  );
}
