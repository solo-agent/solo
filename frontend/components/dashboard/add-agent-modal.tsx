// ============================================================================
// AddAgentModal — brutalist dialog to add an Agent to the current channel
// - card-brutal dialog wrapper (via Dialog component)
// - input-brutal search bar
// - Agent list with status indicators and add buttons
// ============================================================================

'use client';

import { useState, useEffect, useCallback } from 'react';
import { Bot, Circle, AlertCircle, RefreshCw } from 'lucide-react';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { useAgents } from '@/lib/hooks/use-agents';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogCloseButton,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { t } from '@/lib/i18n';
import type { Agent } from '@/lib/types';

interface AddAgentModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Already added agent IDs (to filter out) */
  existingAgentIds: string[];
  onAdd: (agentId: string, agentName: string) => Promise<void>;
}

export function AddAgentModal({
  open,
  onOpenChange,
  existingAgentIds,
  onAdd,
}: AddAgentModalProps) {
  const { agents, isLoading, error: agentsError, refetch } = useAgents();
  const [searchQuery, setSearchQuery] = useState('');
  const [addingId, setAddingId] = useState<string | null>(null);

  // Reset search when modal opens
  useEffect(() => {
    if (open) {
      setSearchQuery('');
      setAddingId(null);
    }
  }, [open]);

  const availableAgents = agents.filter(
    (a) => !existingAgentIds.includes(a.id),
  );

  const filteredAgents = searchQuery
    ? availableAgents.filter(
        (a) =>
          a.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
          (a.description &&
            a.description.toLowerCase().includes(searchQuery.toLowerCase())),
      )
    : availableAgents;

  const handleAdd = useCallback(
    async (agent: Agent) => {
      setAddingId(agent.id);
      try {
        await onAdd(agent.id, agent.name);
        onOpenChange(false);
      } finally {
        setAddingId(null);
      }
    },
    [onAdd, onOpenChange],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogHeader>
        <DialogTitle>{t('addAgentToChannel')}</DialogTitle>
        <DialogCloseButton onClick={() => onOpenChange(false)} />
      </DialogHeader>

      {/* Search */}
      <div className="mb-4">
        <input
          type="text"
          placeholder={t('searchAgent')}
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="input-brutal"
          autoFocus
        />
      </div>

      {/* Error state */}
      {agentsError && (
        <div className="mb-4 flex items-center gap-2 border-2 border-brutal-danger bg-brutal-danger-light/30 px-3 py-2">
          <AlertCircle className="h-4 w-4 flex-shrink-0 text-brutal-danger" />
          <span className="font-mono text-xs flex-1 text-brutal-danger">
            {agentsError}
          </span>
          <button
            type="button"
            onClick={refetch}
            className="btn-brutal btn-brutal-sm flex-shrink-0"
          >
            <RefreshCw className="mr-1 h-3 w-3" />
            {t('retry')}
          </button>
        </div>
      )}

      {/* Agent list */}
      <div className="max-h-64 overflow-y-auto">
        {isLoading ? (
          <div className="space-y-2">
            {[1, 2, 3].map((i) => (
              <div key={i} className="flex items-center gap-3 border-2 border-black p-2 shadow-brutal-sm">
                <Skeleton className="h-8 w-8 rounded-none" />
                <div className="flex-1 space-y-1">
                  <Skeleton className="h-4 w-24 rounded-none" />
                  <Skeleton className="h-3 w-32 rounded-none" />
                </div>
              </div>
            ))}
          </div>
        ) : filteredAgents.length === 0 ? (
          <div className="py-8 text-center">
            <div className="mx-auto mb-2 flex h-10 w-10 items-center justify-center border-2 border-black bg-white shadow-brutal-sm">
              <Bot className="h-5 w-5 text-muted-foreground" />
            </div>
            <p className="font-body text-sm text-muted-foreground">
              {searchQuery ? t('noMatchingAgents') : t('noAgentsAvailable')}
            </p>
            {!searchQuery && (
              <p className="mt-1 font-mono text-[11px] text-muted-foreground">
                {t('allAgentsInChannel')}
              </p>
            )}
          </div>
        ) : (
          <div className="space-y-1">
            {filteredAgents.map((agent) => (
              <button
                key={agent.id}
                onClick={() => handleAdd(agent)}
                disabled={addingId === agent.id}
                className="flex w-full items-center gap-3 border-2 border-black bg-white p-2 text-left shadow-brutal-sm transition-all hover:-translate-x-px hover:-translate-y-px hover:shadow-brutal-lg disabled:opacity-50"
              >
                <PixelAvatar agentId={agent.id} size="sm" />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate font-heading text-sm font-bold text-foreground">
                      {agent.name}
                    </span>
                    <Circle
                      className={`h-2 w-2 flex-shrink-0 ${
                        agent.is_active
                          ? 'fill-brutal-success text-brutal-success'
                          : 'fill-brutal-muted text-brutal-muted'
                      }`}
                    />
                  </div>
                  {agent.description && (
                    <p className="truncate font-mono text-[11px] text-muted-foreground mt-0.5">
                      {agent.description}
                    </p>
                  )}
                </div>
                <span className="btn-brutal btn-brutal-sm flex-shrink-0">
                  {addingId === agent.id ? t('adding') : t('add')}
                </span>
              </button>
            ))}
          </div>
        )}
      </div>
    </Dialog>
  );
}
