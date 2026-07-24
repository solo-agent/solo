'use client';

import { useCallback, useState } from 'react';
import { AlertCircle } from 'lucide-react';
import { AgentForm, type AgentFormValues } from '@/components/agents/agent-form';
import {
  Dialog,
  DialogCloseButton,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useAgents } from '@/lib/hooks/use-agents';
import { t } from '@/lib/i18n';

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

      <p className="mb-4 border-2 border-black bg-brutal-primary-light px-3 py-2 font-body text-xs">
        {t('channelAgentScopeNotice')}
      </p>

      {error && (
        <div className="mb-4 flex items-center gap-2 border-2 border-brutal-danger bg-brutal-danger-light/30 px-3 py-2">
          <AlertCircle className="h-4 w-4 flex-shrink-0 text-brutal-danger" />
          <span className="flex-1 font-mono text-xs text-brutal-danger">{error}</span>
        </div>
      )}

      <AgentForm
        key={formKey}
        onSubmit={handleCreate}
        isSubmitting={isCreating}
        submitLabel={t('teamsCreateAgent')}
      />
    </Dialog>
  );
}
