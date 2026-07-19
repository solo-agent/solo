'use client';

import { createContext, useContext, type CSSProperties } from 'react';
import { Handle, Position, type Node, type NodeProps } from '@xyflow/react';
import { Bot, BrainCircuit, Check, GitBranch, Loader2, MessageSquare } from 'lucide-react';
import {
  RelationshipActivityCard,
  type ActivityCardPlacement,
} from '@/components/relationships/relationship-activity-card';
import { agentRunShowsHalo, agentRunStatusColor } from '@/lib/agent-activity';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import type { LiveAgentState } from '@/lib/hooks/use-team-agent-activity';
import type { ThinkingNode } from '@/lib/types';

export type ThinkingNodeData = {
  node: ThinkingNode;
  activityPlacement?: ActivityCardPlacement;
};
export type ThinkingFlowNode = Node<ThinkingNodeData, 'thinkingNode'>;

const EMPTY_ACTIVITY = new Map<string, LiveAgentState>();
export const ThinkingActivityContext = createContext<ReadonlyMap<string, LiveAgentState>>(EMPTY_ACTIVITY);

export function ThinkingNodeCard({ data, selected }: NodeProps<ThinkingFlowNode>) {
  const { node } = data;
  const liveActivity = useContext(ThinkingActivityContext).get(node.id);
  const isRoot = !node.parent_id;
  const isTeam = node.source === 'team';
  const NodeIcon = isRoot ? BrainCircuit : isTeam ? Bot : GitBranch;
  const orbCenter = isRoot ? 48 : isTeam ? 40 : 32;
  const runStatus = liveActivity?.currentRun?.status;
  const statusColor = agentRunStatusColor(runStatus);
  return (
    <div
      className="relative flex h-[148px] w-[156px] flex-col items-center text-center"
      data-thinking-node-id={node.id}
      data-thinking-node-kind={isRoot ? 'root' : isTeam ? 'team' : 'branch'}
      data-agent-run-status={runStatus}
      aria-current={selected ? 'true' : undefined}
      title={node.returned_handoff || node.checkpoint_handoff || node.inherited_handoff || t('thinkingNewBranch')}
    >
      {node.parent_id && <Handle type="target" position={Position.Top} style={{ top: orbCenter }} className="!h-1 !w-1 !border-0 !bg-transparent" />}
      <div
        className={cn(
          'thinking-node-orb relative flex shrink-0 items-center justify-center border-2 border-black bg-white shadow-brutal-sm transition-[transform,box-shadow,background-color]',
          'rounded-[48%_52%_46%_54%/54%_46%_52%_48%]',
          isRoot ? 'h-24 w-24' : isTeam ? 'h-20 w-20' : 'h-16 w-16',
          agentRunShowsHalo(runStatus) && 'team-agent-active-halo',
          selected && 'thinking-node-selected -translate-x-0.5 -translate-y-0.5 bg-brutal-primary shadow-brutal-md ring-2 ring-black ring-offset-2 ring-offset-brutal-cream',
        )}
        style={{ '--team-agent-status-color': statusColor } as CSSProperties}
      >
        <RelationshipActivityCard
          activity={liveActivity}
          placement={data.activityPlacement}
        />
        <span className={cn('thinking-node-icon flex items-center justify-center rounded-full', isRoot ? 'h-12 w-12' : 'h-10 w-10')}>
          <NodeIcon className={cn(isRoot ? 'h-8 w-8' : 'h-6 w-6')} />
        </span>
        {node.returned_at && (
          <span className="absolute -right-1 -top-1 flex h-5 w-5 items-center justify-center rounded-full border-2 border-black bg-brutal-success text-black">
            <Check className="h-3 w-3" />
          </span>
        )}
        {node.returning_at && !node.returned_at && (
          <span className="absolute -right-1 -top-1 flex h-5 w-5 items-center justify-center rounded-full border-2 border-black bg-brutal-info-light text-black">
            <Loader2 className="h-3 w-3 animate-spin" />
          </span>
        )}
        {node.fork_handoff_pending && !node.returning_at && !node.returned_at && (
          <span className="absolute -right-1 -top-1 flex h-5 w-5 items-center justify-center rounded-full border-2 border-black bg-brutal-info-light text-black">
            <Loader2 className="h-3 w-3 animate-spin" />
          </span>
        )}
      </div>
      <p className={cn('mt-2 w-full truncate font-heading text-sm font-black', selected && 'text-foreground')}>
        {node.title}
      </p>
      <div className="mt-0.5 flex max-w-full items-center gap-1.5 font-mono text-[9px] text-muted-foreground">
        {node.agent_name && <span className="max-w-20 truncate">{node.agent_name}</span>}
        <span className="flex items-center gap-0.5"><MessageSquare className="h-2.5 w-2.5" />{node.message_count}</span>
        <span className="uppercase">{node.source}</span>
      </div>
      <Handle type="source" position={Position.Top} style={{ top: orbCenter }} className="!h-1 !w-1 !border-0 !bg-transparent" />
    </div>
  );
}
