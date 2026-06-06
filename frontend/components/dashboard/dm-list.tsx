// ============================================================================
// SOLO-54-F: DM List — displays DM conversations in the sidebar
// - Loading: skeleton screen
// - Empty: "还没有私信"
// - List: avatar + name + last message preview (truncated 50 chars)
// - Unread: bold + blue dot indicator
// - Selected: highlighted
// - Sorted by last_reply_at descending
// ============================================================================

'use client';

import { useEffect, useMemo, useState } from 'react';
import { MessageSquare, Plus, X, ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { Skeleton } from '@/components/ui/skeleton';
import type { DMChannel } from '@/lib/types';

interface DMListProps {
  dms: DMChannel[];
  isLoading: boolean;
  selectedDmId: string | null;
  onSelectDM: (dmId: string) => void;
  onCreateDM: () => void;
  isExpanded: boolean;
  onToggleExpand: () => void;
}

// ---- Helpers ----

function getDisplayName(dm: DMChannel): string {
  if (dm.other_user) return dm.other_user.display_name;
  if (dm.other_agent) return dm.other_agent.name;
  return '未知用户';
}

function isAgentDM(dm: DMChannel): boolean {
  return !!dm.other_agent;
}

function isAgentDeleted(dm: DMChannel): boolean {
  return isAgentDM(dm) && dm.other_agent?.is_active === false;
}

/** Truncate text to maxLen characters */
function truncate(text: string, maxLen: number): string {
  if (text.length <= maxLen) return text;
  return text.slice(0, maxLen) + '...';
}

// ---- Loading skeleton ----

function DMListSkeleton() {
  return (
    <div className="space-y-1">
      {[1, 2, 3].map((i) => (
        <div key={i} className="flex items-center gap-2 px-2 py-1.5">
          <Skeleton className="h-6 w-6 rounded-none" />
          <div className="flex-1 space-y-1">
            <Skeleton className={`h-3 ${i === 1 ? 'w-16' : i === 2 ? 'w-20' : 'w-14'}`} />
            <Skeleton className={`h-2 w-${i === 1 ? '24' : i === 2 ? '20' : '16'}`} />
          </div>
        </div>
      ))}
    </div>
  );
}

// ---- Empty state ----

function DMListEmpty({ onCreateDM }: { onCreateDM: () => void }) {
  return (
    <div className="space-y-2 px-2 py-3 text-center">
      <p className="text-sm text-sidebar-muted-foreground">还没有私信</p>
      <button
        onClick={onCreateDM}
        className="inline-flex items-center gap-1 rounded-md bg-sidebar-accent px-3 py-1 text-sm font-medium text-sidebar-accent-foreground hover:bg-sidebar-accent/80 transition-colors"
      >
        <Plus className="h-3.5 w-3.5" />
        发起私信
      </button>
    </div>
  );
}

// ---- DM item ----

function DMItem({
  dm,
  isSelected,
  onSelect,
  onClose,
}: {
  dm: DMChannel;
  isSelected: boolean;
  onSelect: () => void;
  onClose: () => void;
}) {
  const name = getDisplayName(dm);
  const isAgent = isAgentDM(dm);
  const deleted = isAgentDeleted(dm);
  const hasUnread = dm.unread_count > 0;
  const lastMessageText = dm.last_message?.content ?? null;

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onSelect();
        }
      }}
      className={cn(
        'group flex cursor-pointer items-center gap-2 px-2 py-1.5 text-sm transition-all',
        isSelected
          ? 'bg-brutal-pink text-black border-2 border-black shadow-brutal-sm'
          : 'text-black hover:bg-brutal-pink/60 border-2 border-transparent',
      )}
      aria-current={isSelected ? 'true' : undefined}
    >
      <PixelAvatar
        agentId={dm.other_user?.id || dm.other_agent?.id || dm.id}
        size="sm"
      />

      {/* Name + preview */}
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <span
            className={cn(
              'truncate font-body',
              hasUnread && 'font-semibold text-sidebar-accent-foreground',
            )}
          >
            {name}
          </span>
          {deleted && (
            <span className="badge-brutal bg-brutal-stone text-black flex-shrink-0">
              DELETED
            </span>
          )}
          {!deleted && isAgent && (
            <span className="badge-brutal bg-brutal-pink text-black text-[10px]">
              Agent
            </span>
          )}
          {/* Unread dot */}
          {hasUnread && (
            <span className="h-2 w-2 flex-shrink-0 bg-brutal-pink" />
          )}
        </div>
        {lastMessageText && (
          <p
            className={cn(
              'truncate text-xs font-body',
              hasUnread
                ? 'font-medium text-sidebar-accent-foreground/80'
                : 'text-black/50',
            )}
          >
            {truncate(lastMessageText, 25)}
          </p>
        )}
      </div>

      {/* Close button — visible on hover */}
      <button
        onClick={(e) => {
          e.stopPropagation();
          onClose();
        }}
        className="hidden group-hover:flex items-center justify-center rounded-none p-1 hover:bg-brutal-pink-light transition-colors flex-shrink-0"
        aria-label="关闭私信"
      >
        <X className="h-4 w-4" />
      </button>
    </div>
  );
}

// ---- Constants ----

const CLOSED_DM_STORAGE_KEY = 'solo-closed-dm-ids';

function loadClosedDmIds(): Set<string> {
  if (typeof window === 'undefined') return new Set();
  try {
    const stored = localStorage.getItem(CLOSED_DM_STORAGE_KEY);
    if (stored) return new Set(JSON.parse(stored));
  } catch { /* ignore corrupt data */ }
  return new Set();
}

function saveClosedDmIds(ids: Set<string>) {
  try {
    localStorage.setItem(CLOSED_DM_STORAGE_KEY, JSON.stringify([...ids]));
  } catch { /* ignore quota errors */ }
}

// ---- Main component ----

export function DMList({
  dms,
  isLoading,
  selectedDmId,
  onSelectDM,
  onCreateDM,
  isExpanded,
  onToggleExpand,
}: DMListProps) {
  const [closedDmIds, setClosedDmIds] = useState<Set<string>>(loadClosedDmIds);

  // Re-sync when createOrGetDM clears a DM from the closed list
  useEffect(() => {
    const handler = () => setClosedDmIds(loadClosedDmIds());
    window.addEventListener('dm-closed-changed', handler);
    return () => window.removeEventListener('dm-closed-changed', handler);
  }, []);

  // Sort by last_reply_at descending (most recent first), exclude closed DMs
  const sortedDMs = useMemo(() => {
    return [...dms]
      .filter((dm) => !closedDmIds.has(dm.id))
      .sort((a, b) => {
        const aTime = a.last_reply_at ? new Date(a.last_reply_at).getTime() : 0;
        const bTime = b.last_reply_at ? new Date(b.last_reply_at).getTime() : 0;
        return bTime - aTime;
      });
  }, [dms, closedDmIds]);

  return (
    <div>
      {/* Section header */}
      <div className="mb-2 flex items-center justify-between border-b-2 border-sidebar-border">
        <button
          type="button"
          onClick={onToggleExpand}
          className="flex flex-1 items-center gap-1.5 px-2 py-2 text-left text-xs font-bold uppercase tracking-wider text-sidebar-muted-foreground font-heading hover:bg-brutal-pink/40"
          aria-label="展开或折叠 直接消息"
          aria-expanded={isExpanded}
        >
          <ChevronDown
            className={cn(
              'h-3 w-3 transition-transform',
              isExpanded ? 'rotate-0' : '-rotate-90',
            )}
          />
          直接消息
        </button>
        <button
          onClick={onCreateDM}
          className="mr-2 flex h-5 w-5 items-center justify-center text-sidebar-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground transition-colors"
          aria-label="发起私信"
        >
          <Plus className="h-3.5 w-3.5" />
        </button>
      </div>

      {/* Content */}
      {isExpanded && (
        isLoading ? (
          <DMListSkeleton />
        ) : sortedDMs.length === 0 ? (
          <DMListEmpty onCreateDM={onCreateDM} />
        ) : (
          <div className="space-y-0.5">
            {sortedDMs.map((dm) => (
              <DMItem
                key={dm.id}
                dm={dm}
                isSelected={dm.id === selectedDmId}
                onSelect={() => onSelectDM(dm.id)}
                onClose={() => setClosedDmIds((prev) => {
                  const next = new Set(prev).add(dm.id);
                  saveClosedDmIds(next);
                  return next;
                })}
              />
            ))}
          </div>
        )
      )}
    </div>
  );
}
