// ============================================================================
// Tag — inline status/type pill
// - Variants map to the brutal palette:
//     "status"  → primary yellow (active/default state)
//     "type"    → info blue (entity classification)
//     "agent"   → violet (AI/agent context)
//     "deleted" → muted stone (soft / archived)
// - Renders as <span>; pass `as="button"` only via children composition.
// - Note (v3.1 audit): this component overlaps conceptually with the
//   `.badge-brutal` CSS class (used 16x in the codebase vs Tag's 1 use).
//   They are intentionally NOT consolidated: Tag is smaller / uppercase
//   (10px), badge-brutal is the larger agent/system message pill. Future
//   cleanup: pick one as canonical and have the other delegate to it.
// ============================================================================

import * as React from "react";
import { cn } from "@/lib/utils";

export type TagVariant = "status" | "type" | "agent" | "deleted";

export interface TagProps {
  variant?: TagVariant;
  className?: string;
  children: React.ReactNode;
}

const VARIANT_CLASSES: Record<TagVariant, string> = {
  status: "bg-brutal-primary text-black",
  type: "bg-brutal-info text-black",
  agent: "bg-brutal-violet text-black",
  deleted: "bg-brutal-muted text-black",
};

export function Tag({ variant = "status", className, children }: TagProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 px-1.5 py-0.5",
        "font-heading text-[10px] font-bold uppercase tracking-wider",
        "border-2 border-black",
        VARIANT_CLASSES[variant],
        className,
      )}
    >
      {children}
    </span>
  );
}
