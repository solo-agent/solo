// ============================================================================
// RelationshipNode — custom ReactFlow node for agent relationship editor
// - Shows agent avatar initial, name, online status
// - 4 handles (top/bottom/left/right) for connection
// - Click → navigate to agent detail
// - Neubrutalist styling: 4px border, hard shadow, no radius
// ============================================================================

import { memo } from 'react';
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { useRouter } from 'next/navigation';

export interface AgentNodeData {
  agentId: string;
  agentName: string;
  isActive?: boolean;
}

function RelationshipNodeComponent({ data }: NodeProps) {
  const router = useRouter();
  const agentData = data as unknown as AgentNodeData;
  const initial = agentData.agentName?.[0]?.toUpperCase() || '?';
  const isActive = agentData.isActive ?? false;

  return (
    <>
      <Handle type="target" position={Position.Top} className="!w-3 !h-3 !border-2 !border-black !bg-white" />
      <Handle type="source" position={Position.Bottom} className="!w-3 !h-3 !border-2 !border-black !bg-white" />
      <Handle type="target" position={Position.Left} className="!w-3 !h-3 !border-2 !border-black !bg-white" />
      <Handle type="source" position={Position.Right} className="!w-3 !h-3 !border-2 !border-black !bg-white" />

      <div
        className="px-4 py-3 border-4 border-black bg-white shadow-brutal min-w-[140px] cursor-pointer hover:-translate-y-0.5 hover:shadow-brutal-lg transition-transform duration-100"
        onClick={(e) => {
          e.stopPropagation();
          router.push(`/workspace?agent=${agentData.agentId}`);
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.stopPropagation();
            router.push(`/workspace?agent=${agentData.agentId}`);
          }
        }}
        role="button"
        tabIndex={0}
      >
        <div className="flex items-center gap-2.5">
          {/* Avatar circle */}
          <div className={[
            'flex-shrink-0 w-9 h-9 border-2 border-black flex items-center justify-center font-heading text-sm font-black',
            isActive ? 'bg-brutal-success text-black' : 'bg-brutal-muted text-black',
          ].join(' ')}>
            {initial}
          </div>
          <div className="min-w-0">
            <div className="font-heading text-sm font-bold text-black truncate">
              {agentData.agentName}
            </div>
            <div className="font-mono text-[10px] font-bold uppercase tracking-wider mt-0.5">
              {isActive ? (
                <span className="text-brutal-success">ONLINE</span>
              ) : (
                <span className="text-brutal-muted">OFFLINE</span>
              )}
            </div>
          </div>
        </div>
      </div>
    </>
  );
}

export const RelationshipNode = memo(RelationshipNodeComponent);
