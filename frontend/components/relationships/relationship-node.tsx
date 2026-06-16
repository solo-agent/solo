// ============================================================================
// RelationshipNode — custom ReactFlow node for agent relationship editor
// - Shows agent pixel avatar, name, online status
// - 4 handles (top/bottom/left/right) for connection
// - Click handled by parent via onNodeClick
// - Neubrutalist styling: 4px border, hard shadow, no radius
// ============================================================================

import { memo } from 'react';
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { PixelAvatar } from '@/components/ui/pixel-avatar';

export interface AgentNodeData {
  agentId: string;
  agentName: string;
  isActive?: boolean;
}

function RelationshipNodeComponent({ data }: NodeProps) {
  const agentData = data as unknown as AgentNodeData;
  const isActive = agentData.isActive ?? false;

  return (
    <>
      <Handle type="target" position={Position.Top} className="!w-3 !h-3 !border-2 !border-black !bg-white" />
      <Handle type="source" position={Position.Bottom} className="!w-3 !h-3 !border-2 !border-black !bg-white" />
      <Handle type="target" position={Position.Left} className="!w-3 !h-3 !border-2 !border-black !bg-white" />
      <Handle type="source" position={Position.Right} className="!w-3 !h-3 !border-2 !border-black !bg-white" />

      <div
        className="px-4 py-3 border-4 border-black bg-white shadow-brutal min-w-[140px] cursor-pointer hover:-translate-y-0.5 hover:shadow-brutal-lg transition-transform duration-100"
      >
        <div className="flex items-center gap-2.5">
          <PixelAvatar agentId={agentData.agentId} size="sm" />
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
