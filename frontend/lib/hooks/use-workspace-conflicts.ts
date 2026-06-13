// ============================================================================
// useWorkspaceConflicts — listens for workspace_conflict WS events (Step 3)
// Shows toast notification when another agent is editing a file in the same
// worktree. Must be used inside WSProvider and ToastProvider.
// ============================================================================

'use client';

import { useEffect } from 'react';
import { useWebSocket } from '@/lib/ws-context';
import { useToast } from '@/components/ui/toast';
import { t } from '@/lib/i18n';

/**
 * Listens for `workspace_conflict` WebSocket events and shows a toast
 * when another agent is editing a file in the same isolated worktree.
 *
 * Usage: call in a component that lives within a channel context
 * (e.g. channel-view or task-board). No-op if WS is disconnected.
 */
export function useWorkspaceConflicts(): void {
  const { onEvent } = useWebSocket();
  const { showToast } = useToast();

  useEffect(() => {
    const unsub = onEvent((event) => {
      if (event.type !== 'workspace_conflict') return;

      const agentName = event.agent_name || event.agent_id.slice(0, 8);
      const fileName = event.file_path.split('/').pop() || event.file_path;
      showToast(
        t('taskWorktreeConflict', { agent: agentName, file: fileName }),
        'info',
      );
    });
    return unsub;
  }, [onEvent, showToast]);
}
