// ============================================================================
// TeamsAgentItem — one row in the Teams left column's Agents section.
// Selection style matches channel-list.tsx: bg-brutal-primary + black border
// + shadow-brutal-sm (the variable named "pink" is actually #FFD23F yellow).
// No DM button here — the detail header has a Message button for that.
// ============================================================================

'use client';

import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { cn } from '@/lib/utils';
import type { Agent } from '@/lib/types';

interface TeamsAgentItemProps {
  agent: Agent;
  isSelected: boolean;
  onSelect: (agentId: string) => void;
}

export function TeamsAgentItem({ agent, isSelected, onSelect }: TeamsAgentItemProps) {
  return (
    <div
      onClick={() => onSelect(agent.id)}
      className={cn(
        'flex cursor-pointer items-center gap-2 px-3 py-1.5 text-sm transition-all',
        isSelected
          ? 'bg-brutal-primary text-black border-2 border-black shadow-brutal-sm'
          : 'border-2 border-transparent hover:border-black',
      )}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onSelect(agent.id);
        }
      }}
      aria-label={`查看 ${agent.name} 详情`}
      aria-current={isSelected ? 'true' : undefined}
    >
      <PixelAvatar agentId={agent.id} avatarUrl={agent.avatar_url} size="sm" />
      <span className="flex-1 truncate">{agent.name}</span>
    </div>
  );
}
