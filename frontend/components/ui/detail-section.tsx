import { cn } from "@/lib/utils";

export function detailSectionClass(className?: string) {
  return cn("detail-section border border-border rounded-xl bg-white p-3", className);
}

export function detailSectionTitleClass(className?: string) {
  return cn(
    "detail-section-title inline-flex items-center gap-1.5 rounded-lg border border-border bg-brutal-primary-light px-2.5 py-1 font-heading text-[11px] font-black uppercase tracking-widest text-foreground shadow-none",
    className,
  );
}

export function detailFieldLabelClass(className?: string) {
  return cn(
    "detail-field-label inline-block rounded-md border border-border bg-brutal-primary-light px-1.5 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider text-foreground",
    className,
  );
}

export function detailEditActionClass(className?: string) {
  return cn(
    "flex items-center gap-1 font-mono text-[10px] font-bold uppercase tracking-wider text-muted-foreground hover:text-black",
    className,
  );
}
