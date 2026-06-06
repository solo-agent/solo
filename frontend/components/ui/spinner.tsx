// STUB-FE2: fe1 will override with final API. Delete this comment when fe1 merges.
// ============================================================================
// Spinner — brutalist loading indicator (hard square + black border, no blur)
// Size variants: sm (h-4 w-4), default (h-6 w-6), lg (h-8 w-8)
// ============================================================================

import { Loader2 } from 'lucide-react';
import { cn } from '@/lib/utils';

type SpinnerSize = 'sm' | 'default' | 'lg';

interface SpinnerProps {
  size?: SpinnerSize;
  className?: string;
  /** Border thickness in tailwind units; default 4 for brutalist weight */
  borderWidth?: 2 | 4;
  /** When true, render a square (CSS border) spinner; when false, render lucide Loader2 */
  square?: boolean;
}

const SIZE_CLASS: Record<SpinnerSize, string> = {
  sm: 'h-4 w-4',
  default: 'h-6 w-6',
  lg: 'h-8 w-8',
};

export function Spinner({
  size = 'default',
  className,
  borderWidth = 4,
  square = true,
}: SpinnerProps) {
  if (square) {
    return (
      <div
        role="status"
        aria-label="加载中"
        className={cn(
          'inline-block animate-spin rounded-none border-black border-t-transparent',
          borderWidth === 2 ? 'border-2' : 'border-4',
          SIZE_CLASS[size],
          className,
        )}
      />
    );
  }
  return (
    <Loader2
      role="status"
      aria-label="加载中"
      className={cn('animate-spin text-muted-foreground', SIZE_CLASS[size], className)}
    />
  );
}
