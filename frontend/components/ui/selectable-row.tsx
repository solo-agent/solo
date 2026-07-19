// ============================================================================
// SelectableRow — list row with a "selected" state
// - Renders a <button> by default; pass `as="li"` semantics via children/wrappers
//   when used inside <ul>/<ol> (still keyboard accessible).
// - Selected: yellow fill + black border + small hard shadow.
// - Unselected: transparent until hover, then a 2px black border outline.
// ============================================================================

import * as React from "react";
import { cn } from "@/lib/utils";

export interface SelectableRowProps
  extends Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, "onClick"> {
  selected?: boolean;
  onClick?: (e: React.MouseEvent<HTMLButtonElement>) => void;
  /** Optional leading content (icon, avatar). */
  leading?: React.ReactNode;
  /** Optional trailing content (badge, count). */
  trailing?: React.ReactNode;
}

export function selectableRowClass(selected = false, className?: string) {
  return cn(
    "selectable-row group flex cursor-pointer items-center gap-2 border-2 px-3 py-1.5 text-sm transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brutal-info focus-visible:ring-offset-2",
    selected
      ? "border-black bg-brutal-primary text-black shadow-brutal-sm"
      : "border-transparent text-black hover:border-black",
    className,
  );
}

export function selectableRowIconClass(className?: string) {
  return cn(
    "selectable-row-icon flex h-7 w-7 flex-shrink-0 items-center justify-center border-2 border-black shadow-brutal-sm",
    className,
  );
}

export function SelectableRow({
  selected = false,
  onClick,
  leading,
  trailing,
  className,
  children,
  type = "button",
  ...props
}: SelectableRowProps) {
  return (
    <button
      type={type}
      onClick={onClick}
      aria-current={selected ? "true" : undefined}
      className={selectableRowClass(selected, cn("w-full text-left", className))}
      {...props}
    >
      {leading != null && <span className="flex-shrink-0">{leading}</span>}
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {trailing != null && <span className="flex-shrink-0">{trailing}</span>}
    </button>
  );
}
