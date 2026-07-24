// ============================================================================
// MemberList — channel member panel showing users and agents separately
// ============================================================================

'use client';

import { useState } from 'react';
import { Users, Bot, Circle, Plus, User as UserIcon, X } from 'lucide-react';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { UserAvatar } from '@/components/ui/user-avatar';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogCloseButton,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { t } from '@/lib/i18n';
import type { AgentDetailTarget, ChannelMember } from '@/lib/types';

interface MemberListProps {
  users: ChannelMember[];
  agents: ChannelMember[];
  isLoading: boolean;
  onAddAgent?: () => void;
  onRemoveAgent?: (memberId: string) => Promise<void>;
  onAgentClick?: (agent: AgentDetailTarget) => void;
  showHeader?: boolean;
  canAddAgent?: boolean;
}

function MemberItem({
  member,
  onRemove,
  onAgentClick,
}: {
  member: ChannelMember;
  onRemove?: (id: string) => Promise<void>;
  onAgentClick?: (agent: AgentDetailTarget) => void;
}) {
  const isAgent = member.member_type === 'agent';
  const [confirming, setConfirming] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const statusColor = {
    online: 'fill-brutal-success text-brutal-success',
    offline: 'fill-brutal-muted text-brutal-muted',
    thinking: 'fill-brutal-accent text-brutal-accent',
    typing: 'fill-brutal-info text-brutal-info',
  }[member.status] || 'fill-brutal-muted text-brutal-muted';

  const statusLabel = {
    online: t('online'),
    offline: t('offline'),
    thinking: t('thinking'),
    typing: t('typing'),
  }[member.status] || t('offline');

  const handleRemove = async () => {
    if (!onRemove) return;
    setIsDeleting(true);
    setDeleteError(null);
    try {
      await onRemove(member.member_id);
      setConfirming(false);
    } catch {
      setDeleteError(t('agentDeleteError'));
    } finally {
      setIsDeleting(false);
    }
  };

  return (
    <div className="group flex items-center gap-3 border-2 border-transparent bg-white p-2 transition-[background-color,border-color,box-shadow] hover:border-black hover:bg-brutal-primary-light hover:shadow-brutal-sm">
      {/* Icon / Avatar */}
      {isAgent ? (
        <PixelAvatar
          agentId={member.member_id}
          avatarUrl={member.avatar_url}
          size="md"
          onClick={onAgentClick ? () => onAgentClick?.({
            id: member.member_id,
            name: member.display_name,
            is_active: member.status !== 'offline',
          }) : undefined}
          ariaLabel={t('viewAgentDetail', { name: member.display_name })}
        />
      ) : (
        <UserAvatar
          userId={member.member_id}
          name={member.display_name}
          avatarUrl={member.avatar_url}
          size="md"
        />
      )}

      {/* Info */}
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate font-heading text-sm font-bold text-foreground">
            {member.display_name}
          </span>
          <span className="flex-shrink-0 border-2 border-black bg-brutal-primary px-1.5 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider text-black">
            {isAgent ? t('agent') : t('user')}
          </span>
        </div>
        <div className="mt-0.5 flex items-center gap-1 font-mono text-[11px] text-muted-foreground">
          <Circle className={`h-2 w-2 flex-shrink-0 ${statusColor}`} />
          {statusLabel}
        </div>
      </div>

      {/* Remove */}
      <div className="flex flex-shrink-0 items-center">
        {isAgent && (
          onRemove && (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                setDeleteError(null);
                setConfirming(true);
              }}
              className="flex-shrink-0 border-2 border-black bg-white px-1.5 py-0.5 opacity-100 shadow-brutal-sm transition-[background-color,color,opacity] hover:bg-brutal-danger hover:text-black md:opacity-0 md:group-hover:opacity-100 md:group-focus-within:opacity-100"
              aria-label={`${t('agentDeleteButton')}: ${member.display_name}`}
              title={t('agentDeleteButton')}
            >
              <X className="h-3 w-3" />
            </button>
          )
        )}
      </div>

      <Dialog
        open={confirming}
        onOpenChange={(next) => {
          if (!isDeleting) setConfirming(next);
        }}
      >
        <DialogHeader>
          <DialogTitle>{t('agentDeleteTitle')}</DialogTitle>
          <DialogCloseButton onClick={() => !isDeleting && setConfirming(false)} />
        </DialogHeader>
        <DialogDescription>
          {t('agentDeleteDesc', { name: member.display_name })}
        </DialogDescription>
        {deleteError && (
          <p className="border-2 border-black bg-brutal-danger-light p-2 font-mono text-xs text-brutal-danger" role="alert">
            {deleteError}
          </p>
        )}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => setConfirming(false)}
            disabled={isDeleting}
          >
            {t('cancel')}
          </Button>
          <Button
            type="button"
            variant="danger"
            onClick={handleRemove}
            disabled={isDeleting}
          >
            {isDeleting ? t('deleting') : t('agentDeleteButton')}
          </Button>
        </DialogFooter>
      </Dialog>
    </div>
  );
}

function MemberListSkeleton() {
  return (
    <div className="space-y-2 px-2">
      {[1, 2, 3, 4].map((i) => (
        <div key={i} className="flex items-center gap-2">
          <Skeleton className="h-7 w-7 rounded-none" />
          <Skeleton className={`h-4 ${i % 2 === 0 ? 'w-20' : 'w-16'}`} />
        </div>
      ))}
    </div>
  );
}

export function MemberList({ users, agents, isLoading, onAddAgent, onRemoveAgent, onAgentClick, showHeader = true, canAddAgent = true }: MemberListProps) {
  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Section header */}
      {showHeader && (
      <div className="flex items-center justify-between border-b-2 border-black px-4 py-3">
        <div className="flex items-center gap-2 text-sm font-medium text-foreground">
          <Users className="h-4 w-4" />
          <span>{t('members')}</span>
          <span className="text-xs text-muted-foreground">
            {users.length + agents.length}
          </span>
        </div>
        {canAddAgent && onAddAgent && (
          <Button
            type="button"
            onClick={onAddAgent}
            variant="success"
            size="icon"
            className="h-7 w-7"
            aria-label={t('addAgentToChannel')}
          >
            <Plus className="h-3.5 w-3.5" />
          </Button>
        )}
      </div>
      )}

      {/* Member list */}
      <div className="flex-1 overflow-y-auto py-2">
        {isLoading ? (
          <MemberListSkeleton />
        ) : (
          <div className="space-y-3">
            {/* Users section */}
            {users.length > 0 && (
              <div>
                <div className="mb-1 flex items-center gap-1.5 px-2">
                  <UserIcon className="h-3 w-3 text-muted-foreground" />
                  <span className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">
                    {t('membersUsers', { n: users.length })}
                  </span>
                </div>
                {users.map((member) => (
                  <MemberItem key={`user-${member.member_id}`} member={member} />
                ))}
              </div>
            )}

            {/* Agents section */}
            {agents.length > 0 && (
              <div>
                <div className="mb-1 flex items-center gap-1.5 px-2">
                  <Bot className="h-3 w-3 text-muted-foreground" />
                  <span className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">
                    {t('agent')} ({agents.length})
                  </span>
                </div>
                {agents.map((member) => (
                  <MemberItem
                    key={`agent-${member.member_id}`}
                    member={member}
                    onRemove={onRemoveAgent}
                    onAgentClick={onAgentClick}
                  />
                ))}
              </div>
            )}

            {/* Empty state */}
            {users.length === 0 && agents.length === 0 && (
              <div className="px-4 py-6 text-center text-sm text-muted-foreground">
                {t('noMembersYet')}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
