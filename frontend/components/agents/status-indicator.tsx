// ============================================================================
// StatusIndicator — brutalist agent status indicator with pulse animation
// - Green (#a9d877)  = online,      pulse animation
// - Yellow (#ffd440) = thinking,    pulse animation
// - Blue  (#27ccf3)  = outputting,  no pulse
// - Gray  (#c0b9b1)  = offline,     no pulse
// ============================================================================

'use client';

import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';

export type AgentStatus = 'online' | 'thinking' | 'outputting' | 'offline';

interface StatusIndicatorProps {
  /** Agent current status */
  status: AgentStatus;
  className?: string;
  /** Whether to show the text label next to the dot. Defaults to true. */
  showLabel?: boolean;
  /** Accessible label override */
  label?: string;
}

const STATUS_CONFIG: Record<
  AgentStatus,
  { color: string; label: string; animate: boolean }
> = {
  online: { color: '#a9d877', label: t('statusOnline'), animate: true },
  thinking: { color: '#ffd440', label: t('statusThinking'), animate: true },
  outputting: { color: '#27ccf3', label: t('statusStreaming'), animate: false },
  offline: { color: '#c0b9b1', label: t('statusOffline'), animate: false },
};

export function StatusIndicator({
  status,
  className,
  showLabel = true,
  label,
}: StatusIndicatorProps) {
  const config = STATUS_CONFIG[status];

  return (
    <span className={cn('inline-flex items-center gap-1.5', className)}>
      {/* Status dot — square per neubrutalism, hard black border */}
      <span
        className={cn(
          'inline-block h-3 w-3 border-2 border-black',
          config.animate && 'animate-pulse',
        )}
        style={{ backgroundColor: config.color }}
        role="img"
        aria-label={label ?? config.label}
      />
      {showLabel && (
        <span className="font-mono text-[11px] text-muted-foreground leading-none">
          {label ?? config.label}
        </span>
      )}
    </span>
  );
}
