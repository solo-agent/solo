// ============================================================================
// EmptyState — placeholder for empty lists / pages
// - Two visual variants:
//     "plain"  → solid 2px black border (default placeholders)
//     "dashed" → dashed border (use when the area itself isn't filled,
//                 e.g. drop targets, "no results" search panel)
// - Optional action button (uses Button variant="primary" by default).
// ============================================================================

import * as React from "react";
import { cn } from "@/lib/utils";
import { Button } from "./button";

export type EmptyStateVariant = "plain" | "dashed";

export interface EmptyStateProps {
  icon?: React.ReactNode;
  title: string;
  description?: string;
  actionLabel?: string;
  onAction?: () => void;
  variant?: EmptyStateVariant;
  className?: string;
}

export function EmptyState({
  icon,
  title,
  description,
  actionLabel,
  onAction,
  variant = "plain",
  className,
}: EmptyStateProps) {
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center gap-3 px-6 py-10 text-center",
        variant === "plain"
          ? "border-2 border-black bg-white shadow-brutal-sm"
          : "border-2 border-dashed border-brutal-muted bg-transparent",
        className,
      )}
    >
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
  );
}
