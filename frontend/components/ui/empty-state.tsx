// STUB-FE2: fe1 will override with final API. Delete this comment when fe1 merges.
// ============================================================================
// EmptyState — brutalist empty state with bordered icon, title, copy, action
// - Icon: bordered colored square (size: sm/md/lg)
// - Optional action button on the bottom
// ============================================================================

import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';

type IconSize = 'sm' | 'md' | 'lg';

interface EmptyStateProps {
  icon: ReactNode;
  title: string;
  description?: string;
  action?: ReactNode;
  /** Background color of the icon square; default bg-brutal-pink */
  iconBgClass?: string;
  iconSize?: IconSize;
  className?: string;
}

const ICON_SIZE: Record<IconSize, string> = {
  sm: 'h-10 w-10',
  md: 'h-14 w-14',
  lg: 'h-16 w-16',
};

const ICON_INNER: Record<IconSize, string> = {
  sm: 'h-5 w-5',
  md: 'h-7 w-7',
  lg: 'h-8 w-8',
};

export function EmptyState({
  icon,
  title,
  description,
  action,
  iconBgClass = 'bg-brutal-pink',
  iconSize = 'md',
  className,
}: EmptyStateProps) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center border-2 border-dashed border-black py-20 text-center',
        className,
      )}
    >
      <div
        className={cn(
          'mb-4 flex items-center justify-center border-2 border-black shadow-brutal-sm',
          ICON_SIZE[iconSize],
          iconBgClass,
        )}
      >
        <div className={cn('text-white', ICON_INNER[iconSize])}>{icon}</div>
      </div>
      <h2 className="font-heading text-lg font-bold text-foreground">{title}</h2>
      {description && (
        <p className="mt-2 max-w-sm font-body text-sm text-muted-foreground">
          {description}
        </p>
      )}
      {action && <div className="mt-6">{action}</div>}
    </div>
  );
}
