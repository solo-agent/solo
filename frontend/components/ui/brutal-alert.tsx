// ============================================================================
// BrutalAlert — inline alert banner (error / warning / success / info)
// - Uses the 8% tint of the matching semantic color as background
//   (--color-brutal-{variant}-light).
// - 2px black border, no shadow (alerts are usually inline so no lift).
// - Title rendered bold; body content rendered below.
// ============================================================================

import * as React from "react";
import { AlertCircle, AlertTriangle, CheckCircle2, Info } from "lucide-react";
import { cn } from "@/lib/utils";

export type BrutalAlertVariant = "error" | "warning" | "success" | "info";

export interface BrutalAlertProps {
  variant?: BrutalAlertVariant;
  title?: string;
  children?: React.ReactNode;
  /** Hide the leading icon (default false). */
  hideIcon?: boolean;
  className?: string;
  /** Role override; default "alert" for error/warning, "status" for success/info. */
  role?: "alert" | "status";
}

const VARIANT_BG: Record<BrutalAlertVariant, string> = {
  error: "bg-brutal-danger-light",
  warning: "bg-brutal-warning-light",
  success: "bg-brutal-success-light",
  info: "bg-brutal-info-light",
};

const VARIANT_TEXT: Record<BrutalAlertVariant, string> = {
  error: "text-brutal-danger",
  warning: "text-brutal-warning",
  success: "text-brutal-success",
  info: "text-brutal-info",
};

const ICONS: Record<BrutalAlertVariant, React.ComponentType<{ className?: string }>> = {
  error: AlertCircle,
  warning: AlertTriangle,
  success: CheckCircle2,
  info: Info,
};

export function BrutalAlert({
  variant = "info",
  title,
  children,
  hideIcon = false,
  className,
  role,
}: BrutalAlertProps) {
  const Icon = ICONS[variant];
  const defaultRole: "alert" | "status" =
    variant === "error" || variant === "warning" ? "alert" : "status";

  return (
    <div
      role={role ?? defaultRole}
      className={cn(
        "flex items-start gap-2.5 border-2 border-black px-3 py-2.5",
        VARIANT_BG[variant],
        className,
      )}
    >
      {!hideIcon && (
        <Icon className={cn("h-4 w-4 flex-shrink-0 mt-0.5", VARIANT_TEXT[variant])} />
      )}
      <div className="min-w-0 flex-1">
        {title && (
          <p className={cn("font-heading text-sm font-bold", VARIANT_TEXT[variant])}>
            {title}
          </p>
        )}
        {children && (
          <div className="mt-0.5 font-body text-sm text-foreground/80">
            {children}
          </div>
        )}
      </div>
    </div>
  );
}
