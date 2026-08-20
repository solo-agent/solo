// ============================================================================
// EmptyState — placeholder for empty lists / pages (Warm Editorial v3.5)
// - Editorial linework illustration floats on the card surface (no tile bg).
// - Optional pattern (halftone/grid) behind the content, very low opacity.
// - Soft rounded card, no brutalist black border / hard shadow.
// ============================================================================

import * as React from "react";
import Image from "next/image";
import { cn } from "@/lib/utils";
import { Button } from "./button";

export type EmptyStateVariant = "plain" | "dashed";
export type EmptyStatePattern = "none" | "halftone" | "grid";
export type EmptyStateSize = "sm" | "md" | "lg";

export interface EmptyStateProps {
  /** Preferred: editorial linework illustration (1:1, transparent corners). */
  illustration?: { src: string; alt: string };
  /** Legacy: small icon inside a 12x12 box. Use illustration instead. */
  icon?: React.ReactNode;
  title: string;
  description?: string;
  actionLabel?: string;
  onAction?: () => void;
  variant?: EmptyStateVariant;
  /** Illustration size: sm=112px, md=192px, lg=240px. */
  size?: EmptyStateSize;
  /** Decorative background pattern. Default "none". */
  pattern?: EmptyStatePattern;
  /**
   * Custom actions to render inside the card, below the title/description.
   * Use this when you need more than the single `actionLabel` button
   * (e.g. multiple buttons, buttons with icons, or mixed primary/outline).
   * Overrides `actionLabel`/`onAction` when both are provided.
   */
  actions?: React.ReactNode;
  className?: string;
}

const sizeClassMap: Record<EmptyStateSize, string> = {
  sm: "h-24 w-24 sm:h-28 sm:w-28",
  md: "h-44 w-44 sm:h-52 sm:w-52",
  lg: "h-56 w-56 sm:h-60 sm:w-60",
};

export function EmptyState({
  illustration,
  icon,
  title,
  description,
  actionLabel,
  onAction,
  variant = "plain",
  size = "md",
  pattern = "none",
  actions,
  className,
}: EmptyStateProps) {
  // v3.5: linework illustration sits on the card surface directly (no bg tile).
  // Pattern (opt-in) sits behind the content with very low opacity.
  const patternClass =
    pattern === "halftone"
      ? "bg-halftone"
      : pattern === "grid"
        ? "bg-grid"
        : null;

  return (
    <div
      className={cn(
        "relative flex flex-col items-center justify-center gap-4 rounded-2xl bg-skin-surface px-6 py-12 text-center",
        variant === "dashed"
          ? "border border-dashed border-skin-rule"
          : "border border-skin-rule shadow-[var(--archive-shadow-sm)]",
        className,
      )}
    >
      {patternClass && (
        <div
          className={cn("absolute inset-0 pointer-events-none opacity-30", patternClass)}
          aria-hidden
        />
      )}

      <div className="relative flex flex-col items-center gap-4">
        {illustration && (
          <div className={cn("relative", sizeClassMap[size])}>
            <Image
              src={illustration.src}
              alt={illustration.alt}
              fill
              sizes="(max-width: 640px) 112px, 240px"
              className="object-contain"
              priority={false}
              unoptimized
            />
          </div>
        )}
        {illustration == null && icon != null && (
          <div className="flex h-12 w-12 items-center justify-center rounded-lg bg-skin-primary-light text-skin-ink">
            {icon}
          </div>
        )}
        <h3 className="font-heading text-base font-bold text-skin-ink">
          {title}
        </h3>
        {description && (
          <p className="max-w-sm font-body text-sm leading-6 text-skin-subtle-text">
            {description}
          </p>
        )}
        {actions ? (
          <div className="mt-2 flex flex-wrap items-center justify-center gap-2">
            {actions}
          </div>
        ) : actionLabel && onAction ? (
          <Button variant="primary" onClick={onAction} className="mt-2">
            {actionLabel}
          </Button>
        ) : null}
      </div>
    </div>
  );
}
