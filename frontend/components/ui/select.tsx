// STUB-FE2: fe1 will override with final API. Delete this comment when fe1 merges.
// ============================================================================
// Select — brutalist native <select> wrapper
// - Inherits input-brutal styling (sharp corners, thick black border, hard
//   shadow) for visual consistency across all form controls.
// - Native <select> chosen over a custom popover to keep the design canon
//   (no floating panels, no rounding, no blur).
// ============================================================================

import { forwardRef, type SelectHTMLAttributes } from 'react';
import { cn } from '@/lib/utils';

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  error?: boolean;
}

export const Select = forwardRef<HTMLSelectElement, SelectProps>(function Select(
  { className, error, children, ...props },
  ref,
) {
  return (
    <select
      ref={ref}
      className={cn(
        'input-brutal',
        error && 'input-error',
        className,
      )}
      {...props}
    >
      {children}
    </select>
  );
});
