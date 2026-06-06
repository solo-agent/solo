// STUB-FE2: fe1 will override with final API. Delete this comment when fe1 merges.
// ============================================================================
// Tag — brutalist small label/chip
// - Variants: pink | yellow | cyan | lime | orange | red | stone
// - Default shape: square (border-2 border-black), uppercase mono text
// - Optional remove button
// ============================================================================

import type { ReactNode } from 'react';
import { X } from 'lucide-react';
import { cn } from '@/lib/utils';

type TagVariant = 'pink' | 'yellow' | 'cyan' | 'lime' | 'orange' | 'red' | 'stone';

interface TagProps {
  variant?: TagVariant;
  children: ReactNode;
  onRemove?: () => void;
  className?: string;
}

const VARIANT_BG: Record<TagVariant, string> = {
  pink: 'bg-brutal-pink',
  yellow: 'bg-brutal-yellow',
  cyan: 'bg-brutal-cyan',
  lime: 'bg-brutal-lime',
  orange: 'bg-brutal-orange',
  red: 'bg-brutal-red',
  stone: 'bg-brutal-stone',
};

export function Tag({ variant = 'pink', children, onRemove, className }: TagProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 border-2 border-black px-1.5 py-0.5 font-mono text-[10px] font-bold uppercase tracking-wider text-black',
        VARIANT_BG[variant],
        className,
      )}
    >
      {children}
      {onRemove && (
        <button
          type="button"
          onClick={onRemove}
          className="ml-0.5 flex h-3 w-3 items-center justify-center hover:bg-black/10"
          aria-label="移除标签"
        >
          <X className="h-2.5 w-2.5" />
        </button>
      )}
    </span>
  );
}
