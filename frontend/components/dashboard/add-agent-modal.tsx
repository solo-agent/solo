'use client';

import { useCallback, useEffect, useState } from 'react';
import { AlertCircle, Bot, Link2 } from 'lucide-react';
import { AgentForm, type AgentFormValues } from '@/components/agents/agent-form';
import {
  Dialog,
  DialogCloseButton,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useAgents } from '@/lib/hooks/use-agents';
import { t } from '@/lib/i18n';
import { apiClient } from '@/lib/api-client';
import { Button } from '@/components/ui/button';

interface WorkspaceAgent {
  id: string;
  name: string;
  description: string;
  owner_id: string;
  home_channel_id: string;
  avatar_url?: string;
}

interface AddAgentModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channelId: string;
  onChanged?: () => Promise<void> | void;
}

/**
 * Creates a fresh Agent whose home is the current Channel.
 *
 * Existing Agents are intentionally not offered here: visible Agents are
 * Channel-scoped and cannot be moved or reused across Channels.
 */
export function AddAgentModal({
  open,
  onOpenChange,
  channelId,
  onChanged,
}: AddAgentModalProps) {
  const { createAgent } = useAgents(channelId);
  const [isCreating, setIsCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [formKey, setFormKey] = useState(0);
  const [workspaceAgents, setWorkspaceAgents] = useState<WorkspaceAgent[]>([]);

  useEffect(() => {
    if (!open) return;
    apiClient.get<WorkspaceAgent[]>('/api/v1/agents')
      .then((items) => setWorkspaceAgents(items.filter((item) => item.home_channel_id !== channelId)))
      .catch(() => setWorkspaceAgents([]));
  }, [open, channelId]);

  const handleOpenChange = useCallback((next: boolean) => {
    setError(null);
    if (!next) setFormKey((key) => key + 1);
    onOpenChange(next);
  }, [onOpenChange]);

  const handleCreate = useCallback(async (values: AgentFormValues) => {
    setIsCreating(true);
    setError(null);
    try {
      await createAgent(values);
      await onChanged?.();
      handleOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('teamsAgentCreateError'));
    } finally {
      setIsCreating(false);
    }
  }, [createAgent, handleOpenChange, onChanged]);

  return (
    <Dialog
      open={open}
      width="lg"
      onOpenChange={handleOpenChange}
    >
      <DialogHeader>
        <DialogTitle>{t('teamsCreateAgent')}</DialogTitle>
        <DialogCloseButton onClick={() => handleOpenChange(false)} />
      </DialogHeader>

      {workspaceAgents.length > 0 && (
        <section className="mb-5">
          <h3 className="mb-2 flex items-center gap-2 font-heading text-sm font-black uppercase"><Link2 className="h-4 w-4" /> {t('agentAddConnectExisting')}</h3>
          <div className="max-h-40 space-y-2 overflow-y-auto pr-1">
            {workspaceAgents.map((agent) => (
              <div key={agent.id} className="flex items-center gap-3 border-2 border-black bg-white px-3 py-2">
                <div className="flex h-8 w-8 items-center justify-center border-2 border-black bg-[#DBEAFE]"><Bot className="h-4 w-4" /></div>
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-bold">{agent.name}</div>
                  <div className="truncate font-body text-xs text-black/50">{agent.description || t('agentAddWorkspaceAgent')}</div>
                </div>
                <Button size="sm" variant="outline" onClick={async () => {
                  setIsCreating(true);
                  setError(null);
                  try {
                    await apiClient.post(`/api/v1/channels/${channelId}/members`, { member_type: 'agent', member_id: agent.id });
                    await onChanged?.();
                    handleOpenChange(false);
                  } catch (err) {
                    setError(err instanceof Error ? err.message : t('agentAddConnectError'));
                  } finally {
                    setIsCreating(false);
                  }
                }} disabled={isCreating}>{t('agentAddConnect')}</Button>
              </div>
            ))}
          </div>
        </section>
      )}

      {workspaceAgents.length > 0 && <h3 className="mb-2 font-heading text-sm font-black uppercase">{t('agentAddCreateNew')}</h3>}

      {error && (
        <div className="mb-4 flex items-center gap-2 border-2 border-brutal-danger bg-brutal-danger-light/30 px-3 py-2">
          <AlertCircle className="h-4 w-4 flex-shrink-0 text-brutal-danger" />
          <span className="flex-1 font-body text-xs text-brutal-danger">{error}</span>
        </div>
      )}

      <AgentForm
        key={formKey}
        onSubmit={handleCreate}
        onCancel={() => handleOpenChange(false)}
        isSubmitting={isCreating}
        submitLabel={t('teamsCreateAgent')}
      />
    </Dialog>
  );
}
