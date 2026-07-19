// ============================================================================
// RelationshipNode — custom ReactFlow node for agent relationship editor
// - Shows agent pixel avatar, name, online status
// - 4 handles (top/bottom/left/right) for connection
// - Click handled by parent via onNodeClick
// - Neubrutalist styling: 4px border, hard shadow, no radius
// ============================================================================

import { memo, type CSSProperties } from 'react';
import { Handle, Position, type NodeProps } from '@xyflow/react';
import { Activity } from 'lucide-react';
import { RelationshipActivityCard } from '@/components/relationships/relationship-activity-card';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { agentRunShowsDots, agentRunShowsHalo, agentRunStatusColor, agentRunStatusText } from '@/lib/agent-activity';
import { t } from '@/lib/i18n';
import type { LiveAgentState } from '@/lib/hooks/use-team-agent-activity';
import type { TaskStatus } from '@/lib/types';

export interface AgentNodeTask {
  id: string;
  taskNumber?: number;
  title: string;
  status: TaskStatus;
  artifactStatus?: 'none' | 'pending' | 'available';
}

export interface AgentNodeData {
  agentId: string;
  agentName: string;
  isActive?: boolean;
  liveActivity?: LiveAgentState;
  task?: AgentNodeTask;
  onOpenRun?: (agentId: string) => void;
  onOpenTask?: (taskId: string) => void;
  onOpenTaskArtifact?: (taskId: string) => void;
}

const TASK_STATUS_BORDER: Partial<Record<TaskStatus, string>> = {
  in_progress: 'var(--color-brutal-info)',
  in_review: 'var(--color-brutal-violet)',
};

function RunningDots() {
  return (
    <span className="ml-1.5 inline-flex items-end gap-0.5 align-middle" aria-hidden="true">
      {[0, 150, 300].map((delay) => (
        <span
          key={delay}
          className="team-agent-running-dot h-1 w-1 border border-black"
          style={{
            animationDelay: `${delay}ms`,
            backgroundColor: 'var(--team-agent-status-color)',
          }}
        />
      ))}
    </span>
  );
}

function taskStatusText(status: TaskStatus) {
  return status === 'in_review' ? t('statusInReview') : t('statusInProgress');
}

function AgentTaskMiniCard({
  task,
  onOpenTask,
  onOpenTaskArtifact,
}: {
  task: AgentNodeTask;
  onOpenTask?: (taskId: string) => void;
  onOpenTaskArtifact?: (taskId: string) => void;
}) {
  const borderColor = TASK_STATUS_BORDER[task.status] ?? 'var(--color-brutal-info)';
  const taskLabel = task.taskNumber ? `#${task.taskNumber}` : 'TASK';
  const artifactLabel = task.artifactStatus === 'available'
    ? 'ARTIFACT'
    : task.artifactStatus === 'pending'
      ? 'PENDING'
      : null;
  const isArtifactAvailable = task.artifactStatus === 'available';
  const artifactClassName = [
    'mt-2 inline-flex border-2 border-black bg-brutal-primary px-1.5 py-0.5 font-mono text-[9px] font-bold uppercase text-black',
    isArtifactAvailable
      ? 'animate-bounce-slow cursor-pointer shadow-brutal-sm transition-all duration-100 hover:-translate-x-0.5 hover:-translate-y-0.5 hover:shadow-brutal active:translate-x-0.5 active:translate-y-0.5 active:shadow-none'
      : '',
  ].join(' ');
  const openTask = () => onOpenTask?.(task.id);

  return (
    <div className="nodrag nopan mt-3 flex flex-col items-center">
      <div className="relationship-task-connector h-3 border-l-2 border-black" />
      <div
        role="button"
        tabIndex={0}
        aria-label={`Open ${taskLabel} ${task.title}`}
        className="relationship-task-card w-[240px] cursor-pointer border-4 bg-white p-3 text-left shadow-brutal-sm transition-transform hover:-translate-y-0.5 hover:shadow-brutal"
        style={{ borderColor, '--relationship-task-status-color': borderColor } as CSSProperties}
        onClick={(event) => {
          event.stopPropagation();
          openTask();
        }}
        onPointerDown={(event) => event.stopPropagation()}
        onKeyDown={(event) => {
          if (event.key !== 'Enter' && event.key !== ' ') return;
          event.preventDefault();
          event.stopPropagation();
          openTask();
        }}
      >
        <div className="mb-1 flex items-center justify-between gap-2">
          <span className="font-mono text-[10px] font-bold text-muted-foreground">{taskLabel}</span>
          <span
            className="relationship-task-status border-2 border-black px-1.5 py-0.5 font-mono text-[9px] font-bold uppercase text-black"
            style={{ backgroundColor: borderColor }}
          >
            {taskStatusText(task.status)}
          </span>
        </div>
        <div className="line-clamp-2 font-heading text-[12px] font-black leading-snug text-black">
          {task.title}
        </div>
        {artifactLabel && (isArtifactAvailable ? (
          <button
            type="button"
            className={artifactClassName}
            onClick={(event) => {
              event.stopPropagation();
              onOpenTaskArtifact?.(task.id);
            }}
            onPointerDown={(event) => event.stopPropagation()}
            onKeyDown={(event) => event.stopPropagation()}
            aria-label={`Open artifact for ${taskLabel} ${task.title}`}
          >
            {artifactLabel}
          </button>
        ) : (
          <div className={artifactClassName}>{artifactLabel}</div>
        ))}
      </div>
    </div>
  );
}

function RelationshipNodeComponent({ data, selected }: NodeProps) {
  const agentData = data as unknown as AgentNodeData;
  const isActive = agentData.isActive ?? false;
  const status = agentData.liveActivity?.currentRun?.status;
  const borderColor = agentRunStatusColor(status);
  const statusText = status ? agentRunStatusText(status) : (isActive ? t('online') : t('offline'));
  const showDots = agentRunShowsDots(status);
  const showHalo = agentRunShowsHalo(status);
  const hasTask = !!agentData.task;

  return (
    <div className="relative flex flex-col items-center">
      {hasTask && (
        <Handle id="bottom" type="source" position={Position.Bottom} className="!z-30 !w-3 !h-3 !border-2 !border-black !bg-white" />
      )}
      <div
        className={[
          'relationship-agent-node relative overflow-visible px-4 py-3 border-4 min-w-[140px] cursor-pointer hover:-translate-y-0.5 hover:shadow-brutal-lg active:translate-x-0.5 active:translate-y-0.5 transition-transform duration-100',
          showHalo ? 'team-agent-active-halo' : '',
          selected ? 'bg-brutal-primary shadow-brutal-lg' : 'bg-white shadow-brutal',
        ].join(' ')}
        style={{ borderColor, '--team-agent-status-color': borderColor } as CSSProperties}
      >
        <Handle id="top" type="target" position={Position.Top} className="!w-3 !h-3 !border-2 !border-black !bg-white" />
        {!hasTask && (
          <Handle id="bottom" type="source" position={Position.Bottom} className="!w-3 !h-3 !border-2 !border-black !bg-white" />
        )}
        <Handle id="left" type="target" position={Position.Left} className="!w-3 !h-3 !border-2 !border-black !bg-white" />
        <Handle id="right" type="source" position={Position.Right} className="!w-3 !h-3 !border-2 !border-black !bg-white" />
        <RelationshipActivityCard activity={agentData.liveActivity} />
        {agentData.onOpenRun && (
          <button
            type="button"
            className="nodrag nopan absolute -right-3 -top-3 z-20 flex h-8 w-8 items-center justify-center border-2 border-black bg-brutal-info-light text-black shadow-brutal-sm transition-transform hover:-translate-y-0.5 hover:bg-brutal-info active:translate-x-0.5 active:translate-y-0.5 active:shadow-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brutal-info"
            onClick={(event) => {
              event.stopPropagation();
              agentData.onOpenRun?.(agentData.agentId);
            }}
            onPointerDown={(event) => event.stopPropagation()}
            title={t('observabilityLive')}
            aria-label={`${agentData.agentName} ${t('observabilityLive')}`}
          >
            <Activity className="h-4 w-4" />
          </button>
        )}
        <div className="flex items-center gap-2.5">
          <PixelAvatar agentId={agentData.agentId} size="sm" className="flex-shrink-0" />
          <div className="min-w-0">
            <div className="font-heading text-sm font-bold text-black truncate">
              {agentData.agentName}
            </div>
            <div className="font-mono text-[10px] font-bold uppercase tracking-wider mt-0.5">
              {status ? (
                <span className={status.startsWith('waiting') ? 'text-brutal-warning' : 'text-black'}>
                  {statusText}
                  {showDots && <RunningDots />}
                </span>
              ) : isActive ? (
                <span className="text-brutal-success">{statusText}</span>
              ) : (
                <span className="text-brutal-muted">{statusText}</span>
              )}
            </div>
          </div>
        </div>
      </div>
      {agentData.task && (
        <AgentTaskMiniCard
          task={agentData.task}
          onOpenTask={agentData.onOpenTask}
          onOpenTaskArtifact={agentData.onOpenTaskArtifact}
        />
      )}
    </div>
  );
}

export const RelationshipNode = memo(RelationshipNodeComponent);
