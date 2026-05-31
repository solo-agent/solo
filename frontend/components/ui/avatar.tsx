// ============================================================================
// Avatar — displays user's initials as fallback avatar
// ============================================================================

import { cn } from '@/lib/utils';

interface AvatarProps {
  name: string;
  className?: string;
}

export function Avatar({ name, className }: AvatarProps) {
  const initials = (name || '')
    .split(' ')
    .map((n) => n[0])
    .join('')
    .toUpperCase()
    .substring(0, 2)
    .slice(0, 2);

  return (
    <div
      className={cn(
        'flex h-8 w-8 flex-shrink-0 items-center justify-center border-2 border-black shadow-brutal-sm bg-primary text-xs font-bold text-primary-foreground font-heading',
        className,
      )}
      aria-label={name}
    >
      {initials || '?'}
    </div>
  );
}
