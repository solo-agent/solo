// ============================================================================
// TeamsAgentItem — one row in the Teams left column's Agents section.
// - Selection style matches channel-list.tsx: bg-brutal-pink + black border
//   + shadow-brutal-sm (the variable named "pink" is actually #FFD23F yellow).
// - "DM" button on the right triggers a navigation to /dashboard?dm=<id>.
// ============================================================================

'use client';

import { useCallback, useState } from 'react';
import { useRouter } from 'next/navigation';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { useDM } from '@/lib/hooks/use-dm';
import { useToast } from '@/components/ui/toast';
import { cn } from '@/lib/utils';
import type { Agent } from '@/lib/types';

interface TeamsAgentItemProps {
  agent: Agent;
  isSelected: boolean;
  onSelect: (agentId: string) => void;
}

export function TeamsAgentItem({ agent, isSelected, onSelect }: TeamsAgentItemProps) {
  const router = useRouter();
  const { createOrGetDM } = useDM();
  const { showToast } = useToast();
  const [isDMLoading, setIsDMLoading] = useState(false);

  const handleClick = useCallback(() => {
    onSelect(agent.id);
  }, [agent.id, onSelect]);

  const handleDM = useCallback(
    async (e: React.MouseEvent) => {
      e.stopPropagation();
      if (isDMLoading) return;
      setIsDMLoading(true);
      try {
        const dm = await createOrGetDM({ agent_id: agent.id });
        router.push(`/dashboard?dm=${dm.id}`);
      } catch {
        showToast('无法发起会话，请稍后再试', 'error');
      } finally {
        setIsDMLoading(false);
      }
    },
    [agent.id, createOrGetDM, isDMLoading, router, showToast],
  );

  return (
    <div
      onClick={handleClick}
      className={cn(
        'flex cursor-pointer items-center gap-2 px-3 py-1.5 text-sm',
        isSelected
          ? 'bg-brutal-pink text-black border-2 border-black shadow-brutal-sm'
          : 'border-2 border-transparent hover:bg-brutal-pink/60',
      )}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          handleClick();
        }
      }}
      aria-label={`查看 ${agent.name} 详情`}
      aria-current={isSelected ? 'true' : undefined}
    >
      <PixelAvatar agentId={agent.id} avatarUrl={agent.avatar_url} size="sm" />
      <span className="flex-1 truncate">{agent.name}</span>
      <button
        type="button"
        onClick={handleDM}
        disabled={isDMLoading}
        className="border-2 border-black bg-white px-1.5 py-0.5 text-[10px] font-bold hover:bg-brutal-pink/60 disabled:opacity-50"
        aria-label={`与 ${agent.name} 发起私信`}
      >
        {isDMLoading ? '...' : 'DM'}
      </button>
    </div>
  );
}
