// ============================================================================
// TeamsHumanItem — one row in the Teams left column's Humans section.
// No DM button (you can't DM yourself). Same selection style as the agent row.
// ============================================================================

'use client';

import { cn } from '@/lib/utils';

interface TeamsHumanItemProps {
  user: { id: string; display_name: string; avatar_url?: string | null };
  isSelected: boolean;
  onSelect: (userId: string) => void;
}

export function TeamsHumanItem({ user, isSelected, onSelect }: TeamsHumanItemProps) {
  return (
    <div
      onClick={() => onSelect(user.id)}
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
          onSelect(user.id);
        }
      }}
      aria-label={`查看 ${user.display_name} 详情`}
      aria-current={isSelected ? 'true' : undefined}
    >
      <div className="flex h-7 w-7 flex-shrink-0 items-center justify-center border-2 border-black shadow-brutal-sm bg-primary text-xs font-bold text-primary-foreground font-heading">
        {user.display_name.charAt(0).toUpperCase()}
      </div>
      <span className="flex-1 truncate">{user.display_name}</span>
    </div>
  );
}
