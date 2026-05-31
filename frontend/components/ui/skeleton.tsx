// ============================================================================
// Skeleton — pulsing loading placeholder (zero-radius for neubrutalism)
// ============================================================================

import { cn } from '@/lib/utils';

interface SkeletonProps {
  className?: string;
}

export function Skeleton({ className }: SkeletonProps) {
  return <div className={cn('animate-pulse bg-muted', className)} />;
}
