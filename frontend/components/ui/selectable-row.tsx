// STUB-FE2: fe1 will override with final API. Delete this comment when fe1 merges.
// ============================================================================
// SelectableRow — brutalist selectable list row
// - Click anywhere to select
// - Selected: bg-brutal-pink, border-2 border-black, shadow-brutal-sm
// - Unselected: transparent border, hover:border-black
// - Keyboard accessible: role=button, tabIndex=0, Enter/Space to activate
// ============================================================================

import { forwardRef, type ReactNode, type KeyboardEvent } from 'react';
import { cn } from '@/lib/utils';

interface SelectableRowProps {
  selected?: boolean;
  onSelect: () => void;
  onDelete?: () => void;
  /** Optional delete affordance rendered on hover (right side) */
  children: ReactNode;
  className?: string;
  ariaLabel?: string;
}

export const SelectableRow = forwardRef<HTMLDivElement, SelectableRowProps>(
  function SelectableRow({ selected, onSelect, children, className, ariaLabel }, ref) {
    const handleKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        onSelect();
      }
    };

    return (
      <div
        ref={ref}
        role="button"
        tabIndex={0}
        onClick={onSelect}
        onKeyDown={handleKeyDown}
        aria-current={selected ? 'true' : undefined}
        aria-label={ariaLabel}
        className={cn(
          'group flex cursor-pointer items-center gap-2 px-3 py-1.5 text-sm transition-all',
          selected
            ? 'border-2 border-black bg-brutal-pink text-black shadow-brutal-sm'
            : 'border-2 border-transparent text-black hover:border-black',
          className,
        )}
      >
        {children}
      </div>
    );
  },
);
