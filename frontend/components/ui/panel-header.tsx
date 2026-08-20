import { cn } from "@/lib/utils";

export function panelHeaderClass(className?: string) {
  return cn(
    "panel-header flex items-center justify-between border-b border-border bg-brutal-cream px-4 py-3",
    className,
  );
}

export function panelTitleClass(className?: string) {
  return cn("font-heading text-sm font-black uppercase tracking-wider", className);
}
