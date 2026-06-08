// ============================================================================
// MemberList — channel member panel showing users and agents separately
// ============================================================================

'use client';

import { useState } from 'react';
import { Users, Bot, Circle, Plus, User as UserIcon, X } from 'lucide-react';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { Skeleton } from '@/components/ui/skeleton';
import { t } from '@/lib/i18n';
import type { ChannelMember } from '@/lib/types';

interface MemberListProps {
  users: ChannelMember[];
  agents: ChannelMember[];
  isLoading: boolean;
  onAddAgent: () => void;
  onRemoveAgent?: (memberId: string) => void;
  showHeader?: boolean;
}

function MemberItem({ member, onRemove }: { member: ChannelMember; onRemove?: (id: string) => void }) {
  const isAgent = member.member_type === 'agent';
  const [confirming, setConfirming] = useState(false);
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

  return (
    <div className="group flex items-center gap-2 border-2 border-transparent px-2 py-1.5 transition-colors hover:border-black hover:bg-brutal-primary-light">
      {/* Icon / Avatar */}
      {isAgent ? (
        <PixelAvatar agentId={member.member_id} size="sm" />
      ) : (
        <div className="flex h-7 w-7 flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-muted shadow-brutal-sm">
          <span className="font-heading text-[10px] font-bold text-black">
            {member.display_name?.charAt(0)?.toUpperCase() || '?'}
          </span>
        </div>
      )}

      {/* Name */}
      <span className="min-w-0 flex-1 truncate text-sm text-foreground">
        {member.display_name}
      </span>

      {/* Badge + remove */}
      <div className="ml-auto flex flex-shrink-0 items-center gap-1">
        {isAgent && (
          <span className="badge-brutal bg-brutal-primary text-black text-[10px]">
            Agent
          </span>
        )}

        {isAgent && onRemove && (
          confirming ? (
            <button
              onClick={(e) => { e.stopPropagation(); onRemove(member.member_id); }}
              className="btn-brutal btn-brutal-sm bg-brutal-danger text-black border-2 border-black font-heading text-[10px] font-bold shadow-brutal-sm"
            >
              KICK
            </button>
          ) : (
            <button
              onClick={(e) => { e.stopPropagation(); setConfirming(true); }}
              className="flex-shrink-0 border-2 border-black bg-white px-1.5 py-0.5 opacity-0 group-hover:opacity-100 transition-all shadow-brutal-sm hover:bg-brutal-danger hover:text-black"
              title={t('removeAgent')}
            >
              <X className="h-3 w-3" />
            </button>
          )
        )}
      </div>

      {/* Status dot — always at rightmost, consistent X regardless of badge */}
      <div className="flex-shrink-0" title={statusLabel} style={{ width: 8 }}>
        <Circle className={`h-2 w-2 ${statusColor}`} />
      </div>
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

export function MemberList({ users, agents, isLoading, onAddAgent, onRemoveAgent, showHeader = true }: MemberListProps) {
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
        <button
          onClick={onAddAgent}
          className="btn-brutal flex h-6 w-6 items-center justify-center border-2 border-black font-heading font-bold"
          aria-label={t('addAgentToChannel')}
        >
          <Plus className="h-3.5 w-3.5" />
        </button>
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
                  <MemberItem key={`agent-${member.member_id}`} member={member} onRemove={onRemoveAgent} />
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
