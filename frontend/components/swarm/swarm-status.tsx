// ============================================================================
// SwarmStatus — swarm progress display for complex parent tasks
// - Shows swarm progress bar for subtask completion
// - Lists subtasks with status indicators
// - DAG dependency info (blocked / blocking)
// ============================================================================

'use client';

import { useState, useEffect, useCallback } from 'react';
import {
  Network, CheckCircle2, Loader2, Circle, Lock,
  ChevronRight, Clock,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import { Button } from '@/components/ui/button';
import { BrutalAlert } from '@/components/ui/brutal-alert';
import { apiClient } from '@/lib/api-client';
import type { SwarmStatus as SwarmStatusType, Task } from '@/lib/types';

interface SwarmStatusProps {
  /** Parent task ID to show swarm progress for */
  taskId: string;
  /** Channel ID for API context */
  channelId: string;
  /** Called when a subtask is clicked */
  onSubtaskClick?: (taskId: string) => void;
  /** Compact mode */
  compact?: boolean;
}

const STATUS_ICON: Record<string, React.ReactNode> = {
  done: <CheckCircle2 className="h-4 w-4 text-brutal-success" />,
  closed: <CheckCircle2 className="h-4 w-4 text-brutal-muted" />,
  in_progress: <Loader2 className="h-4 w-4 text-brutal-info animate-spin" />,
  in_review: <Clock className="h-4 w-4 text-brutal-violet" />,
  todo: <Circle className="h-4 w-4 text-muted-foreground" />,
};

export function SwarmStatusPanel({ taskId, channelId, onSubtaskClick, compact }: SwarmStatusProps) {
  const [swarm, setSwarm] = useState<SwarmStatusType | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchSwarm = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const data = await apiClient.get<SwarmStatusType>(
        `/api/v1/tasks/${taskId}/swarm-status`,
        { channel_id: channelId },
      );
      setSwarm(data);
    } catch {
      setError(t('swarmLoadError'));
    } finally {
      setIsLoading(false);
    }
  }, [taskId, channelId]);

  useEffect(() => {
    fetchSwarm();
  }, [fetchSwarm]);

  // Auto-refresh every 30s
  useEffect(() => {
    const interval = setInterval(fetchSwarm, 30000);
    return () => clearInterval(interval);
  }, [fetchSwarm]);

  const progress = swarm && swarm.total_subtasks > 0
    ? Math.round((swarm.completed_subtasks / swarm.total_subtasks) * 100)
    : 0;

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 p-4">
        <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        <span className="font-mono text-xs text-muted-foreground">{t('loading')}</span>
      </div>
    );
  }

  if (error) {
    return (
      <BrutalAlert variant="error" className="text-xs m-4">
        {error}
        <button type="button" onClick={fetchSwarm} className="ml-2 underline font-bold">
          {t('retry')}
        </button>
      </BrutalAlert>
    );
  }

  if (!swarm || swarm.total_subtasks === 0) {
    return (
      <div className="p-4 text-center">
        <Network className="mx-auto h-6 w-6 text-muted-foreground mb-1" />
        <p className="font-mono text-xs text-muted-foreground">
          {t('swarmNoSwarm')}
        </p>
      </div>
    );
  }

  return (
    <div className={cn('space-y-3', compact && 'space-y-2')}>
      {/* Header + progress */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <h3 className="font-heading text-sm font-bold uppercase tracking-wider text-foreground">
            <Network className="inline h-4 w-4 mr-1.5 -mt-0.5" />
            {t('swarmStatusTitle')}
          </h3>
          <span className="font-mono text-xs font-bold text-muted-foreground">
            {t('swarmProgress', { done: swarm.completed_subtasks, total: swarm.total_subtasks })}
          </span>
        </div>

        {/* Progress bar — brutal style */}
        <div className="h-4 border-2 border-black bg-brutal-cream relative overflow-hidden">
          <div
            className="h-full bg-brutal-success transition-all duration-300"
            style={{ width: `${progress}%` }}
          />
          {/* Tick marks at 25% intervals */}
          {[25, 50, 75].map((pct) => (
            <div
              key={pct}
              className="absolute top-0 bottom-0 w-px bg-black opacity-30"
              style={{ left: `${pct}%` }}
            />
          ))}
          <span className="absolute inset-0 flex items-center justify-center font-heading text-[10px] font-bold text-black">
            {progress}%
          </span>
        </div>
      </div>

      {/* Subtask list */}
      <div className={cn('space-y-1.5', compact && 'max-h-[50vh] overflow-y-auto')}>
        {swarm.subtasks.map((sub) => (
          <div
            key={sub.task_id}
            role="button"
            tabIndex={0}
            onClick={() => onSubtaskClick?.(sub.task_id)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onSubtaskClick?.(sub.task_id);
              }
            }}
            className={cn(
              'flex items-center gap-2 border-2 border-black bg-white p-2 transition-all cursor-pointer',
              'hover:-translate-y-px hover:shadow-brutal-sm',
              'active:translate-x-0.5 active:translate-y-0.5 active:shadow-none',
              sub.is_blocked && 'opacity-50',
              sub.status === 'in_progress' && 'border-l-4 border-l-brutal-info',
              sub.status === 'done' && 'border-l-4 border-l-brutal-success',
            )}
          >
            {/* Status icon */}
            <span className="flex-shrink-0">
              {STATUS_ICON[sub.status] || <Circle className="h-4 w-4 text-muted-foreground" />}
            </span>

            {/* Task info */}
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-1.5">
                <span className="font-mono text-xs font-bold text-muted-foreground">
                  #{sub.task_number}
                </span>
                <span className="font-body text-xs text-foreground truncate">
                  {sub.title}
                </span>
              </div>
              <div className="flex items-center gap-2 mt-0.5">
                {sub.claimer_name ? (
                  <span className="font-mono text-[10px] text-muted-foreground">
                    {sub.claimer_name}
                  </span>
                ) : !sub.is_blocked && (
                  <span className="font-heading text-[10px] font-bold text-brutal-warning">
                    {t('swarmPending')}
                  </span>
                )}
                {sub.is_blocked && sub.blocking_task_numbers && sub.blocking_task_numbers.length > 0 && (
                  <span className="inline-flex items-center gap-0.5 font-heading text-[10px] font-bold text-brutal-danger">
                    <Lock className="h-2.5 w-2.5" />
                    {t('swarmBlockedBy', { n: sub.blocking_task_numbers.map((x) => `#${x}`).join(', ') })}
                  </span>
                )}
              </div>
            </div>

            {/* Navigate arrow */}
            <ChevronRight className="h-3 w-3 text-muted-foreground flex-shrink-0" />
          </div>
        ))}
      </div>

      {/* Legend */}
      {!compact && (
        <div className="flex items-center gap-3 pt-1 border-t-2 border-black">
          <span className="flex items-center gap-1 font-mono text-[10px] text-muted-foreground">
            <CheckCircle2 className="h-3 w-3 text-brutal-success" />
            {t('swarmLegendDone')}
          </span>
          <span className="flex items-center gap-1 font-mono text-[10px] text-muted-foreground">
            <Loader2 className="h-3 w-3 text-brutal-info" />
            {t('swarmLegendInProgress')}
          </span>
          <span className="flex items-center gap-1 font-mono text-[10px] text-muted-foreground">
            <Lock className="h-3 w-3 text-brutal-danger" />
            {t('swarmLegendBlocked')}
          </span>
          <span className="flex items-center gap-1 font-mono text-[10px] text-muted-foreground">
            <Circle className="h-3 w-3 text-muted-foreground" />
            {t('swarmLegendPending')}
          </span>
        </div>
      )}
    </div>
  );
}
