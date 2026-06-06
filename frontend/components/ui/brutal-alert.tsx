// STUB-FE2: fe1 will override with final API. Delete this comment when fe1 merges.
// ============================================================================
// BrutalAlert — bordered, shadowed alert banner with icon + message + action
// - Variants: info | warning | error | success
// - Each variant has hard-coded color tokens (no Tailwind defaults)
// ============================================================================

import type { ReactNode } from 'react';
import { AlertCircle, AlertTriangle, CheckCircle2, Info } from 'lucide-react';
import { cn } from '@/lib/utils';

type AlertVariant = 'info' | 'warning' | 'error' | 'success';

interface BrutalAlertProps {
  variant?: AlertVariant;
  children: ReactNode;
  action?: ReactNode;
  className?: string;
  icon?: ReactNode;
}

const VARIANT_STYLE: Record<
  AlertVariant,
  { border: string; bg: string; iconClass: string; Icon: typeof Info }
> = {
  info: {
    border: 'border-brutal-cyan',
    bg: 'bg-brutal-cyan-light',
    iconClass: 'text-brutal-cyan',
    Icon: Info,
  },
  warning: {
    border: 'border-brutal-orange',
    bg: 'bg-brutal-orange-light',
    iconClass: 'text-brutal-orange',
    Icon: AlertTriangle,
  },
  error: {
    border: 'border-brutal-red',
    bg: 'bg-brutal-red-light',
    iconClass: 'text-brutal-red',
    Icon: AlertCircle,
  },
  success: {
    border: 'border-brutal-lime',
    bg: 'bg-brutal-lime-light',
    iconClass: 'text-brutal-lime',
    Icon: CheckCircle2,
  },
};

export function BrutalAlert({
  variant = 'info',
  children,
  action,
  className,
  icon,
}: BrutalAlertProps) {
  const { border, bg, iconClass, Icon } = VARIANT_STYLE[variant];
  return (
    <div
      role="alert"
      className={cn(
        'flex items-center gap-3 border-2 p-3 shadow-brutal-sm',
        border,
        bg,
        className,
      )}
    >
      {icon ?? <Icon className={cn('h-4 w-4 flex-shrink-0', iconClass)} />}
      <div className="flex-1 font-body text-sm text-foreground">{children}</div>
      {action}
    </div>
  );
}
