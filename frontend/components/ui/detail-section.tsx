import { cn } from "@/lib/utils";

export function detailSectionClass(className?: string) {
  return cn("border-2 border-black bg-white p-3", className);
}

export function detailSectionTitleClass(className?: string) {
  return cn(
    "inline-flex items-center gap-1.5 border-2 border-black bg-brutal-primary px-2.5 py-1 font-heading text-[11px] font-black uppercase tracking-widest text-black shadow-brutal-sm",
    className,
  );
}

export function detailFieldLabelClass(className?: string) {
  return cn(
    "inline-block bg-brutal-primary-light border-2 border-black px-1.5 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider text-black",
    className,
  );
}

export function detailEditActionClass(className?: string) {
  return cn(
    "flex items-center gap-1 font-mono text-[10px] font-bold uppercase tracking-wider text-muted-foreground hover:text-black",
    className,
  );
}
