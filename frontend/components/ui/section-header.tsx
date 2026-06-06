// STUB-FE2: fe1 will override with final API. Delete this comment when fe1 merges.
// ============================================================================
// SectionHeader — collapsible section title (chevron + uppercase label + count)
// Matches the pattern used in Sidebar / TeamsLeftColumn / ComputersLeftColumn.
// - Pressing the header toggles expanded
// - Optional count badge on the right
// ============================================================================

import type { ReactNode } from 'react';
import { ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';

interface SectionHeaderProps {
  label: string;
  expanded: boolean;
  onToggle: () => void;
  count?: number;
  /** Render-only mode: no toggle behavior, no button semantics */
  renderOnly?: boolean;
  children?: ReactNode;
  className?: string;
}

const BASE =
  'flex w-full items-center gap-1.5 px-3 py-2 text-left text-xs font-bold uppercase tracking-wider font-heading text-muted-foreground border-2 border-transparent hover:border-black transition-all';

export function SectionHeader({
  label,
  expanded,
  onToggle,
  count,
  renderOnly = false,
  className,
}: SectionHeaderProps) {
  if (renderOnly) {
    return (
      <h3 className={cn(BASE, 'cursor-default', className)}>
        <span>{label}</span>
        {count !== undefined && (
          <span className="ml-auto text-xs tabular-nums opacity-50">{count}</span>
        )}
      </h3>
    );
  }
  return (
    <button
      type="button"
      onClick={onToggle}
      className={cn(BASE, className)}
      aria-expanded={expanded}
      aria-label={`展开或折叠 ${label}`}
    >
      <ChevronDown
        aria-hidden="true"
        className={cn('h-3 w-3 transition-transform', expanded ? 'rotate-0' : '-rotate-90')}
      />
      <span>{label}</span>
      {count !== undefined && (
        <span className="ml-auto text-xs tabular-nums opacity-50">{count}</span>
      )}
    </button>
  );
}
