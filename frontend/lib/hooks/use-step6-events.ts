// ============================================================================
// useStep6Events — WebSocket event handlers for Step 6 (Swarm + Wake)
// (T6.4.3) Listens for:
//   - reminder_fired    → toast notification
//   - task_escalated    → toast notification
//   - task_unclaimed_auto → auto-refresh task board (via callback)
//   - swarm_decomposed  → toast: "Task decomposed into N subtasks"
//   - swarm_all_done    → auto-refresh swarm status panel (via callback)
// ============================================================================

'use client';

import { useEffect, useRef } from 'react';
import { useWebSocket } from '@/lib/ws-context';
import { useToast } from '@/components/ui/toast';
import { t } from '@/lib/i18n';

interface UseStep6EventsOptions {
  /** Called when task_unclaimed_auto fires — pass a refetch function */
  onTaskBoardRefresh?: () => void;
  /** Called when swarm_all_done fires — pass a refetch function */
  onSwarmRefresh?: (parentTaskId: string) => void;
}

/**
 * Registers WebSocket listeners for Step 6 real-time events.
 * Must be used inside WSProvider and ToastProvider.
 *
 * Usage:
 *   useStep6Events({
 *     onTaskBoardRefresh: refetchTasks,
 *     onSwarmRefresh: (taskId) => { if (taskId === currentTaskId) refetchSwarm(); },
 *   });
 */
export function useStep6Events({
  onTaskBoardRefresh,
  onSwarmRefresh,
}: UseStep6EventsOptions = {}): void {
  const { onEvent } = useWebSocket();
  const { showToast } = useToast();

  // Keep callbacks in refs to avoid effect re-subscriptions
  const taskBoardRefreshRef = useRef(onTaskBoardRefresh);
  taskBoardRefreshRef.current = onTaskBoardRefresh;

  const swarmRefreshRef = useRef(onSwarmRefresh);
  swarmRefreshRef.current = onSwarmRefresh;

  useEffect(() => {
    const unsub = onEvent((event) => {
      switch (event.type) {
        case 'reminder_fired': {
          showToast(
            t('reminderFiredToast', { message: event.message }),
            'info',
          );
          break;
        }

        case 'task_escalated': {
          const level = event.level === 'red' ? 'RED' : 'YELLOW';
          const agentName = event.escalated_to_name || event.escalated_to_agent_id.slice(0, 8);
          showToast(
            t('taskEscalatedToast', {
              n: event.task_number ?? '?',
              agent: agentName,
              level,
            }),
            'error',
          );
          // Also refresh the task board when an escalation happens
          taskBoardRefreshRef.current?.();
          break;
        }

        case 'task_unclaimed_auto': {
          showToast(
            t('taskUnclaimedToast', {
              n: event.task_number ?? '?',
              reason: event.reason,
            }),
            'info',
          );
          // Auto-refresh task board so the unclaimed task shows up
          taskBoardRefreshRef.current?.();
          break;
        }

        case 'swarm_decomposed': {
          showToast(
            t('swarmDecomposedToast', {
              parent: event.parent_task_number ?? '?',
              n: event.subtask_count,
            }),
            'success',
          );
          break;
        }

        case 'swarm_all_done': {
          showToast(
            t('swarmAllDoneToast', {
              parent: event.parent_task_number ?? '?',
              n: event.completed_count,
            }),
            'success',
          );
          swarmRefreshRef.current?.(event.parent_task_id);
          break;
        }
      }
    });
    return unsub;
  }, [onEvent, showToast]);
}
