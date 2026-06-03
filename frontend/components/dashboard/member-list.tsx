// ============================================================================
// MemberList — channel member panel showing users and agents separately
// ============================================================================

'use client';

import { useState } from 'react';
import { Users, Bot, Circle, Plus, User as UserIcon, X } from 'lucide-react';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { Skeleton } from '@/components/ui/skeleton';
import type { ChannelMember } from '@/lib/types';

interface MemberListProps {
  users: ChannelMember[];
  agents: ChannelMember[];
  isLoading: boolean;
  onAddAgent: () => void;
  onRemoveAgent?: (memberId: string) => void;
}

function MemberItem({ member, onRemove }: { member: ChannelMember; onRemove?: (id: string) => void }) {
  const isAgent = member.member_type === 'agent';
  const [confirming, setConfirming] = useState(false);
  const statusColor = {
    online: 'fill-green-500 text-green-500',
    offline: 'fill-gray-300 text-gray-300',
    thinking: 'fill-yellow-500 text-yellow-500',
    typing: 'fill-blue-500 text-blue-500',
  }[member.status] || 'fill-gray-300 text-gray-300';

  const statusLabel = {
    online: '在线',
    offline: '离线',
    thinking: '思考中',
    typing: '输入中',
  }[member.status] || '离线';

  return (
    <div className="group flex items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-accent/50">
      {/* Icon / Avatar */}
      {isAgent ? (
        <PixelAvatar agentId={member.member_id} size="sm" />
      ) : (
        <div className="flex h-7 w-7 flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-stone shadow-brutal-sm">
          <span className="font-heading text-[10px] font-bold text-black">
            {member.display_name?.charAt(0)?.toUpperCase() || '?'}
          </span>
        </div>
      )}

      {/* Name */}
      <span className="min-w-0 flex-1 truncate text-sm text-foreground">
        {member.display_name}
      </span>

      {/* Type badge */}
      {isAgent && (
        <span className="badge-brutal bg-brutal-pink text-black text-[10px]">
          Agent
        </span>
      )}

      {/* Status */}
      <div className="flex-shrink-0" title={statusLabel}>
        <Circle className={`h-2 w-2 ${statusColor}`} />
      </div>

      {/* Remove button (agents only) — brutalist: thick borders, no rounding, bold */}
      {isAgent && onRemove && (
        confirming ? (
          <button
            onClick={(e) => { e.stopPropagation(); onRemove(member.member_id); }}
            className="btn-brutal btn-brutal-sm bg-brutal-red text-black border-2 border-black font-heading text-[10px] font-bold shadow-brutal-sm"
          >
            KICK
          </button>
        ) : (
          <button
            onClick={(e) => { e.stopPropagation(); setConfirming(true); }}
            className="flex-shrink-0 border-2 border-black bg-white px-1.5 py-0.5 opacity-0 group-hover:opacity-100 transition-all shadow-brutal-sm hover:bg-brutal-red hover:text-black"
            title="移除 Agent"
          >
            <X className="h-3 w-3" />
          </button>
        )
      )}
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

export function MemberList({ users, agents, isLoading, onAddAgent, onRemoveAgent }: MemberListProps) {
  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Section header */}
      <div className="flex items-center justify-between border-b-2 border-black px-4 py-3">
        <div className="flex items-center gap-2 text-sm font-medium text-foreground">
          <Users className="h-4 w-4" />
          <span>成员</span>
          <span className="text-xs text-muted-foreground">
            {users.length + agents.length}
          </span>
        </div>
        <button
          onClick={onAddAgent}
          className="flex h-6 w-6 items-center justify-center rounded text-muted-foreground hover:bg-accent hover:text-accent-foreground transition-colors"
          aria-label="添加 Agent 到频道"
        >
          <Plus className="h-3.5 w-3.5" />
        </button>
      </div>

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
                  <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                    用户 ({users.length})
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
                  <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                    Agent ({agents.length})
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
                暂无成员
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
