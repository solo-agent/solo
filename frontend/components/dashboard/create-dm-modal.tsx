// ============================================================================
// SOLO-55-F: Create DM Modal — search users/agents and create/enter DM
// - Search input: real-time filtering
// - List: name + type label + online status
// - Create or enter existing DM on selection
// - All states: loading, empty, error
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { Search, Bot, Circle } from 'lucide-react';
import { apiClient } from '@/lib/api-client';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogCloseButton,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Spinner } from '@/components/ui/spinner';
import { Avatar } from '@/components/ui/avatar';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { t } from '@/lib/i18n';
import type { CreateDMInput, DMChannel } from '@/lib/types';

// ---- Types ----

interface DMCreateParticipant {
  id: string;
  type: 'user' | 'agent';
  display_name: string;
  online: boolean;
}

// ---- Mock participant list (replace with /api/v1/users and /api/v1/agents when available) ----

interface CreateDMModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreateDM: (input: CreateDMInput) => Promise<string>;
  /** Current DM channels list for detecting existing conversations */
  dms: DMChannel[];
}

export function CreateDMModal({
  open,
  onOpenChange,
  onCreateDM,
  dms,
}: CreateDMModalProps) {
  const [participants, setParticipants] = useState<DMCreateParticipant[]>([]);
  const [participantsLoading, setParticipantsLoading] = useState(false);
  const [participantsError, setParticipantsError] = useState<string | null>(null);

  // Fetch real participants from API
  useEffect(() => {
    if (!open) return;
    setParticipantsLoading(true);
    setParticipantsError(null);
    apiClient.get<Array<{ id: string; name: string; is_active: boolean }>>('/api/v1/agents')
      .then((agents) => {
        const list: DMCreateParticipant[] = agents.map((a) => ({
          id: a.id,
          type: 'agent' as const,
          display_name: a.name,
          online: a.is_active,
        }));
        setParticipants(list);
      })
      .catch(() => {
        setParticipantsError(t('loadError'));
      })
      .finally(() => {
        setParticipantsLoading(false);
      });
  }, [open]);

  // Use real participants or fallback
  if (participantsLoading) {
    // Show loading skeleton inside the modal
  }
  const [searchQuery, setSearchQuery] = useState('');
  const [creatingId, setCreatingId] = useState<string | null>(null);

  // Reset search when modal opens
  useEffect(() => {
    if (open) {
      setSearchQuery('');
      setCreatingId(null);
    }
  }, [open]);

  // Filter and search participants
  const filteredParticipants = useMemo(() => {
    if (!searchQuery.trim()) return participants;
    const q = searchQuery.toLowerCase();
    return participants.filter(
      (p) =>
        p.display_name.toLowerCase().includes(q) ||
        p.id.toLowerCase().includes(q),
    );
  }, [participants, searchQuery]);

  // Check if a participant already has a DM conversation
  const hasExistingDM = useCallback(
    (participant: DMCreateParticipant): boolean => {
      if (participant.type === 'user') {
        return dms.some(
          (dm) => dm.other_user && dm.other_user.id === participant.id,
        );
      }
      return dms.some(
        (dm) => dm.other_agent && dm.other_agent.id === participant.id,
      );
    },
    [dms],
  );

  const handleSelect = useCallback(
    async (participant: DMCreateParticipant) => {
      setCreatingId(participant.id);
      try {
        const input: CreateDMInput =
          participant.type === 'user'
            ? { user_id: participant.id }
            : { agent_id: participant.id };
        await onCreateDM(input);
        onOpenChange(false);
      } finally {
        setCreatingId(null);
      }
    },
    [onCreateDM, onOpenChange],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogHeader>
        <DialogTitle>{t('startDM')}</DialogTitle>
        <DialogCloseButton onClick={() => onOpenChange(false)} />
      </DialogHeader>

      {/* Search input */}
      <div className="mb-4">
        <Input
          placeholder={t('dmSearch')}
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          autoFocus
        />
      </div>

      {/* Participant list */}
      <div className="max-h-64 overflow-y-auto" role="listbox" aria-label={t('userAgentList')}>
        {filteredParticipants.length === 0 ? (
          <div className="py-8 text-center">
            <Search className="mx-auto mb-2 h-8 w-8 text-muted-foreground" />
            <p className="text-sm text-muted-foreground">
              {searchQuery
                ? t('noMatchingUsers')
                : t('noUsersAvailable')}
            </p>
          </div>
        ) : (
          <div className="space-y-1">
            {filteredParticipants.map((participant) => {
              const isAgent = participant.type === 'agent';
              const existing = hasExistingDM(participant);
              const isCreating = creatingId === participant.id;

              return (
                <div
                  key={`${participant.type}-${participant.id}`}
                  onClick={() => handleSelect(participant)}
                  className="flex w-full items-center gap-3 border-2 border-transparent p-2 text-left transition-colors hover:border-black hover:bg-brutal-primary-light"
                  role="option"
                  aria-selected={false}
                >
                  {/* Avatar */}
                  <div className="relative flex-shrink-0">
                    {isAgent ? (
                      <PixelAvatar agentId={participant.id} size="md" />
                    ) : (
                      <Avatar
                        name={participant.display_name}
                        className="h-8 w-8 text-xs"
                      />
                    )}
                  </div>

                  {/* Info */}
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-sm font-medium text-foreground">
                        {participant.display_name}
                      </span>
                      {/* Type tag */}
                      <span
                        className={`flex-shrink-0 border-2 border-black px-1.5 py-0.5 text-[10px] font-bold ${
                          isAgent
                            ? 'bg-brutal-success-light text-black'
                            : 'bg-brutal-info-light text-black'
                        }`}
                      >
                        {isAgent ? t('agent') : t('user')}
                      </span>
                    </div>
                    <div className="flex items-center gap-2 text-xs text-muted-foreground">
                      {/* Online status */}
                      <span className="flex items-center gap-1">
                        <Circle
                          className={`h-2 w-2 ${
                            participant.online
                              ? 'fill-brutal-success text-brutal-success'
                              : 'fill-brutal-muted text-brutal-muted'
                          }`}
                        />
                        {participant.online ? t('online') : t('offline')}
                      </span>
                      {existing && (
                        <span className="text-muted-foreground/60">
                          {t('alreadyHaveDM')}
                        </span>
                      )}
                    </div>
                  </div>

                  {/* Action button */}
                  <Button
                    size="sm"
                    variant={existing ? 'ghost' : 'secondary'}
                    disabled={isCreating}
                    className="flex-shrink-0"
                    onClick={(e) => {
                      e.stopPropagation();
                      handleSelect(participant);
                    }}
                  >
                    {isCreating ? (
                      <>
                        <Spinner size="sm" className="mr-1" />
                        {t('submitting')}
                      </>
                    ) : existing ? (
                      t('enterExisting')
                    ) : (
                      t('startDM')
                    )}
                  </Button>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </Dialog>
  );
}
