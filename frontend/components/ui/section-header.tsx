// ============================================================================
// SectionHeader — collapsible section header (sidebar/list pattern)
// - Renders an <h*> heading with an optional count badge and chevron toggle.
// - When `expanded` is provided, the chevron rotates and the button announces
//   `aria-expanded`; when omitted, renders as a static header.
// ============================================================================

import * as React from "react";
import { ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";

export interface SectionHeaderProps {
  label: string;
  /** Optional numeric count rendered as a mono uppercase badge. */
  count?: number;
  /** Controlled expanded state. Omit to render a static header. */
  expanded?: boolean;
  onToggle?: () => void;
  /** Heading level; defaults to h3 (sidebar pattern). */
  as?: "h1" | "h2" | "h3" | "h4" | "h5" | "h6";
  className?: string;
  /** Optional trailing action slot (e.g. "+" button). */
  trailing?: React.ReactNode;
}

export function SectionHeader({
  label,
  count,
  expanded,
  onToggle,
  as: Heading = "h3",
  className,
  trailing,
}: SectionHeaderProps) {
  const isInteractive = onToggle !== undefined;
  const content = (
    <>
      {isInteractive && (
        <ChevronDown
          className={cn(
            "h-3 w-3 transition-transform",
            expanded ? "rotate-0" : "-rotate-90",
          )}
        />
      )}
      <span className="font-heading text-[10px] font-bold uppercase tracking-widest text-foreground">
        {label}
      </span>
      {count !== undefined && (
        <span className="font-mono text-[10px] tabular-nums text-muted-foreground">
          {count}
        </span>
      )}
      {trailing && <span className="ml-auto">{trailing}</span>}
    </>
  );

  if (isInteractive) {
    return (
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={!!expanded}
        className={cn(
          "flex w-full items-center gap-1.5 px-3 py-1 text-left",
          "hover:bg-black/5 focus-visible:outline-none",
          className,
        )}
      >
        {content}
      </button>
    );
  }

  return (
    <Heading
      className={cn(
        "flex items-center gap-1.5 px-3 py-1",
        className,
      )}
    >
      {content}
    </Heading>
  );
}
