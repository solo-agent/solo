// ============================================================================
// Spinner — single canonical loading indicator
// - Uses --color-brutal-primary for the visible arc, transparent top edge.
// - Two sizes only; prefer `sm` inside buttons / inline, `md` for full blocks.
// - Respects prefers-reduced-motion via the global override in
//   globals.brutal.css (animate-spin animation: none).
// ============================================================================

import { cn } from '@/lib/utils';

export type SpinnerSize = 'sm' | 'md';

export interface SpinnerProps {
  size?: SpinnerSize;
  className?: string;
  /** Accessible label; rendered as aria-label and visually hidden. */
  label?: string;
}

const SIZE_CLASSES: Record<SpinnerSize, string> = {
  sm: 'h-3.5 w-3.5 border-2',
  md: 'h-5 w-5 border-[3px]',
};

export function Spinner({ size = 'sm', className, label = '加载中' }: SpinnerProps) {
  return (
    <span
      role="status"
      aria-label={label}
      className={cn(
        'inline-block animate-spin rounded-full',
        'border-brutal-primary border-t-transparent',
        SIZE_CLASSES[size],
        className,
      )}
    />
  );
}
