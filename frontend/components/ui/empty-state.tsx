// ============================================================================
// EmptyState — placeholder for empty lists / pages (v3.2 Phase 2)
// - Two visual variants:
//     "plain"  → solid 4px black border + 12px shadow (default hero-style)
//     "dashed" → dashed border (for "no results" panels / drop targets)
// - Optional sticker rotation (-2 to 2 deg) for the "hand-placed" feel.
// - Optional decorative texture (halftone/grid) behind the content.
// - Optional action button (uses Button variant="primary" by default).
// ============================================================================

import * as React from "react";
import { cn } from "@/lib/utils";
import { Button } from "./button";

export type EmptyStateVariant = "plain" | "dashed";
export type EmptyStatePattern = "none" | "halftone" | "grid";

export interface EmptyStateProps {
  icon?: React.ReactNode;
  title: string;
  description?: string;
  actionLabel?: string;
  onAction?: () => void;
  variant?: EmptyStateVariant;
  /** Sticker rotation in degrees. Default 0 (axis-aligned). */
  rotation?: number;
  /** Decorative background pattern. Default "none". */
  pattern?: EmptyStatePattern;
  className?: string;
}

export function EmptyState({
  icon,
  title,
  description,
  actionLabel,
  onAction,
  variant = "plain",
  rotation = 0,
  pattern = "none",
  className,
}: EmptyStateProps) {
  // v3.2 (Phase 2): pattern sits behind the content via an absolute
  // positioned div. Pointer-events-none so it never intercepts clicks
  // on the action button.
  const patternClass =
    pattern === "halftone"
      ? "bg-halftone"
      : pattern === "grid"
        ? "bg-grid"
        : null;

  return (
    <div
      style={rotation !== 0 ? { transform: `rotate(${rotation}deg)` } : undefined}
      className={cn(
        "relative flex flex-col items-center justify-center gap-3 px-6 py-10 text-center",
        variant === "plain"
          ? "border-brutal-4 bg-white shadow-brutal-2xl"
          : "border-2 border-dashed border-brutal-muted bg-transparent",
        className,
      )}
    >
      {patternClass && (
        <div
          className={cn("absolute inset-0 pointer-events-none opacity-60", patternClass)}
          aria-hidden
        />
      )}
      <div className="relative flex flex-col items-center gap-3">
        {icon != null && (
          <div className="flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-primary-light text-brutal-black">
            {icon}
          </div>
        )}
        <h3 className="font-heading text-base font-bold text-foreground">
          {title}
        </h3>
        {description && (
          <p className="max-w-sm font-body text-sm text-muted-foreground">
            {description}
          </p>
        )}
        {actionLabel && onAction && (
          <Button variant="primary" onClick={onAction} className="mt-2">
            {actionLabel}
          </Button>
        )}
      </div>
    </div>
  );
}
