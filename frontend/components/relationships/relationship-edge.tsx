// ============================================================================
// RelationshipEdge — custom ReactFlow edge for 2 relationship types
// - assigns_to: solid line with arrow, double-stroke for emphasis
// - collaborates_with: dashed line, no arrow (bidirectional)
// - Uses smoothstep path for cleaner routing
// ============================================================================

import { memo } from 'react';
import { BaseEdge, EdgeLabelRenderer, getSmoothStepPath, type EdgeProps } from '@xyflow/react';
import { t } from '@/lib/i18n';
import type { RelationshipType } from '@/lib/types';

interface RelationshipEdgeData {
  relType: RelationshipType;
  channelName?: string;
}

const EDGE_STYLES: Record<RelationshipType, { stroke: string }> = {
  assigns_to:        { stroke: 'var(--color-brutal-info)' },
  collaborates_with: { stroke: 'var(--color-brutal-success)' },
};

function relationshipTypeLabel(type: RelationshipType) {
  return type === 'assigns_to' ? t('assignsTo') : t('collaboratesWith');
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
  const [edgePath, labelX, labelY] = getSmoothStepPath({
    sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition,
    borderRadius: 8,
  });

  const edgeData = data as RelationshipEdgeData | undefined;
  const relType = edgeData?.relType || 'collaborates_with';
  const style = EDGE_STYLES[relType] || EDGE_STYLES.collaborates_with;
  const isCollaboration = relType === 'collaborates_with';

  // Offset path for assigns_to parallel line (smoothstep)
  let offsetPath = '';
  if (!isCollaboration) {
    const offset = 5;
    [offsetPath] = getSmoothStepPath({
      sourceX: sourceX + offset, sourceY: sourceY + offset,
      targetX: targetX + offset, targetY: targetY + offset,
      sourcePosition, targetPosition, borderRadius: 8,
    });
  }

  return (
    <>
      {/* Shadow/secondary line for assigns_to */}
      {!isCollaboration && offsetPath && (
        <BaseEdge
          id={`${id}-shadow`}
          path={offsetPath}
          style={{
            stroke: style.stroke,
            strokeWidth: 1.5,
            opacity: 0.35,
            cursor: 'pointer',
          }}
          markerEnd={markerEnd}
        />
      )}

      {/* Main edge path */}
      <BaseEdge
        id={id}
        path={edgePath}
        style={{
          stroke: style.stroke,
          strokeWidth: selected ? 4 : (isCollaboration ? 2 : 2.5),
          strokeDasharray: isCollaboration ? '8,4' : 'none',
          cursor: 'pointer',
        }}
        interactionWidth={24}
        markerEnd={isCollaboration ? undefined : markerEnd}
      />

      {/* Selected glow */}
      {selected && (
        <BaseEdge
          id={`${id}-glow`}
          path={edgePath}
          style={{
            stroke: style.stroke,
            strokeWidth: 4,
            strokeDasharray: isCollaboration ? '8,4' : 'none',
            opacity: 0.25,
            cursor: 'pointer',
          }}
          interactionWidth={24}
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
            'relationship-edge-label inline-block px-2 py-0.5 border-2 border-black',
            'font-heading text-[9px] font-bold uppercase tracking-wider',
            'cursor-pointer hover:-translate-y-0.5 hover:shadow-brutal-lg active:translate-x-0.5 active:translate-y-0.5 transition-transform duration-100',
            selected
              ? 'bg-brutal-primary text-black shadow-brutal-sm'
              : 'bg-white text-muted-foreground',
          ].join(' ')}>
            {relationshipTypeLabel(relType)}
            {edgeData?.channelName ? ` · #${edgeData.channelName}` : ''}
          </span>
        </div>
      </EdgeLabelRenderer>
    </>
  );
}

export const RelationshipEdge = memo(RelationshipEdgeComponent);
