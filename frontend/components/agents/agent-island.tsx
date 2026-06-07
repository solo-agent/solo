'use client';

// ============================================================================
// AgentIsland (SOLO-island PR2) — iPhone Dynamic Island-style floating UI
// that surfaces real-time agent status in the current channel.
//
// Visual: a brutalist bar fixed at the bottom of the sidebar (left second
// column), matching its 220px width — like how the iPhone Dynamic Island
// matches the notch width. Collapsed by default; shows the most recent
// activity for the first active agent. Expands (on click) into a list of
// all active agents, growing upward from the bottom.
//
// Disappears entirely when no agent is active (on-demand).
//
// Exit animation (SOLO-island PR-fix): when activeAgents empties, the bar
// stays mounted for 200ms while playing a height-collapse + fade animation
// (pure CSS via Tailwind transition classes — no framer-motion to keep the
// bundle lean), then unmounts.
// ============================================================================

import { useState, useEffect, useCallback, useRef } from 'react';
import {
  Brain,
  Loader2,
  Bot,
  AlertTriangle,
  Eye,
  EyeOff,
} from 'lucide-react';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { useAgentIsland, type IslandAgent, type IslandAgentStatus } from '@/lib/hooks/use-agent-island';
import { cn } from '@/lib/utils';

// ---- Final-state badge config (for agent.done with non-ok outcomes) ----

const FINAL_STATE_BADGE: Record<
  NonNullable<IslandAgent['finalState']>,
  { label: string; className: string }
> = {
  completed: { label: '完成', className: 'bg-brutal-success text-black' },
  failed: { label: '失败', className: 'bg-brutal-danger text-white' },
  aborted: { label: '中断', className: 'bg-brutal-muted text-white' },
  timeout: { label: '超时', className: 'bg-brutal-warning text-black' },
  cancelled: { label: '已取消', className: 'bg-brutal-muted text-white' },
};

// ---- Status visual config ----

interface StatusVisuals {
  /** Lucide icon component. */
  icon: typeof Brain;
  /** Color class for the status dot. */
  dotClass: string;
  /** Color class for the icon. */
  iconClass: string;
  /** Whether the icon should be spinning (Loader2-style). */
  spin: boolean;
  /** Whether the icon should pulse. */
  pulse: boolean;
  /** Short Chinese label used in badges and aria text. */
  label: string;
  /** Tailwind class for the inline status badge background. */
  badgeClass: string;
}

const STATUS_VISUALS: Record<IslandAgentStatus, StatusVisuals> = {
  idle: {
    icon: Bot,
    dotClass: 'bg-brutal-muted',
    iconClass: 'text-foreground',
    spin: false,
    pulse: false,
    label: '空闲',
    badgeClass: 'bg-brutal-muted text-white',
  },
  thinking: {
    icon: Brain,
    dotClass: 'bg-brutal-accent',
    iconClass: 'text-yellow-600',
    spin: false,
    pulse: true,
    label: '思考中',
    badgeClass: 'bg-brutal-accent text-black',
  },
  running: {
    icon: Loader2,
    dotClass: 'bg-brutal-info',
    iconClass: 'text-cyan-600',
    spin: true,
    pulse: false,
    label: '执行中',
    badgeClass: 'bg-brutal-info text-black',
  },
  streaming: {
    icon: Bot,
    dotClass: 'bg-brutal-success',
    iconClass: 'text-green-600',
    spin: false,
    pulse: true,
    label: '生成中',
    badgeClass: 'bg-brutal-success text-black',
  },
  // waiting_approval: reserved per PRD v1.x approval flow — UI not implemented yet
  waiting_approval: {
    icon: AlertTriangle,
    dotClass: 'bg-brutal-warning',
    iconClass: 'text-orange-600',
    spin: false,
    pulse: true,
    label: '等审批',
    badgeClass: 'bg-brutal-warning text-black',
  },
  error: {
    icon: AlertTriangle,
    dotClass: 'bg-brutal-danger',
    iconClass: 'text-red-600',
    spin: false,
    pulse: false,
    label: '出错',
    badgeClass: 'bg-brutal-danger text-white',
  },
};

interface AgentIslandProps {
  /**
   * The current channel/DM ID the island should reflect. Pass null to
   * disable the island entirely (e.g. on pages without a channel scope).
   */
  channelId: string | null;
  /**
   * SOLO-island PR3 — invoked when the user clicks an agent row in the
   * expanded panel. Parent is expected to open the AgentViewPanel and
   * focus the corresponding agent's trace.
   */
  onInvokeAgent?: (agentId: string) => void;
}

// ============================================================================
// AgentIsland — root component
// ============================================================================

export function AgentIsland({ channelId, onInvokeAgent }: AgentIslandProps) {
  const { activeAgents, clearAll } = useAgentIsland(channelId);
  const [expanded, setExpanded] = useState(false);

  // ---- Exit-animation state machine ----
  //
  // `mounted` controls whether the component renders DOM at all.
  // `closing` controls whether the exit animation is playing.
  //
  // Lifecycle:
  //   activeAgents.length > 0
  //     → mounted=true, closing=false (visible, entering or stable)
  //   activeAgents.length === 0
  //     → mounted=true, closing=true  (playing exit animation)
  //     → 200ms later: mounted=false   (fully unmounted)
  //
  // We mount first (closing=false → closing=true) so React renders the
  // element, then the next render adds the exit classes; without the
  // mount-first trick the browser would never see the entering frame
  // and couldn't transition to the exit frame.
  const [mounted, setMounted] = useState(activeAgents.length > 0);
  const [closing, setClosing] = useState(false);
  const unmountTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Remember the last primary agent so the closing frame still has
  // something to render. Without this, when activeAgents empties the
  // collapsed pill would briefly render with `primary = undefined` and
  // trip the PixelAvatar / STATUS_VISUALS lookups.
  const lastPrimaryRef = useRef<IslandAgent | null>(null);
  const lastOverflowRef = useRef<number>(0);

  useEffect(() => {
    if (activeAgents.length > 0) {
      // Re-entering — cancel any in-flight unmount and reveal.
      if (unmountTimerRef.current) {
        clearTimeout(unmountTimerRef.current);
        unmountTimerRef.current = null;
      }
      setMounted(true);
      setClosing(false);
      return;
    }

    // No active agents — play exit animation, then unmount.
    if (!mounted) return;
    setClosing(true);
    unmountTimerRef.current = setTimeout(() => {
      setMounted(false);
      setClosing(false);
      lastPrimaryRef.current = null;
      lastOverflowRef.current = 0;
      unmountTimerRef.current = null;
    }, 200);

    return () => {
      if (unmountTimerRef.current) {
        clearTimeout(unmountTimerRef.current);
        unmountTimerRef.current = null;
      }
    };
  }, [activeAgents.length, mounted]);

  if (!mounted) {
    return null;
  }

  // Use the current active agent when available, otherwise fall back to
  // the last-seen one so the exit animation can play with valid data.
  if (activeAgents.length > 0) {
    lastPrimaryRef.current = activeAgents[0];
    lastOverflowRef.current = activeAgents.length - 1;
  }
  const primary = activeAgents[0] ?? lastPrimaryRef.current;
  const overflow =
    activeAgents.length > 0 ? activeAgents.length - 1 : lastOverflowRef.current;

  // Defensive: first render with no last-primary (shouldn't happen since
  // `mounted` initialises to `activeAgents.length > 0`, but guards
  // against channel-switch races).
  if (!primary) {
    return null;
  }

  return (
    <div
      className={cn(
        // Anchored to the sidebar bottom-left: navbar (w-14 = 56px) +
        // sidebar width (220px). Fixed so it overlays and stays put
        // regardless of sidebar scroll.
        'fixed bottom-0 left-[56px] z-50 w-[220px] border-r-2 border-black transition-all duration-200 ease-out',
        closing
          ? 'max-h-0 opacity-0'
          : 'max-h-[500px] opacity-100',
      )}
      role="region"
      aria-label="Agent 实时状态"
      aria-hidden={closing}
    >
      {expanded ? (
        <ExpandedPanel
          // Remount on collapse→expand so the height transition plays
          // cleanly (CSS transition needs the entering frame to differ
          // from the leaving frame).
          key="expanded"
          agents={activeAgents}
          onCollapse={() => setExpanded(false)}
          onClearAll={clearAll}
          onInvokeAgent={onInvokeAgent}
        />
      ) : (
        <CollapsedPill
          key="collapsed"
          primary={primary}
          overflow={overflow}
          onClick={() => setExpanded(true)}
        />
      )}
    </div>
  );
}

// ============================================================================
// Collapsed bar — single-agent summary, full sidebar width
// ============================================================================

function CollapsedPill({
  primary,
  overflow,
  onClick,
}: {
  primary: IslandAgent;
  overflow: number;
  onClick: () => void;
}) {
  const vis = STATUS_VISUALS[primary.status];
  const Icon = vis.icon;

  return (
    <button
      type="button"
      onClick={onClick}
      className="group flex w-full items-center gap-2 border-t-2 border-black bg-brutal-cream px-3 py-2 transition-colors hover:bg-brutal-muted-light"
      aria-label={`Agent ${primary.agentName} ${STATUS_VISUALS[primary.status].label},点击查看详情`}
    >
      {/* Pixel avatar */}
      <PixelAvatar agentId={primary.agentId} avatarUrl={null} size="sm" />

      {/* Status indicator dot */}
      <span
        className={cn(
          'h-2 w-2 flex-shrink-0 rounded-full',
          vis.dotClass,
          vis.pulse && 'animate-pulse',
        )}
        aria-hidden
      />

      {/* Status icon */}
      <Icon
        className={cn(
          'h-3 w-3 flex-shrink-0',
          vis.iconClass,
          vis.spin && 'animate-spin',
        )}
        aria-hidden
      />

      {/* Agent name + activity — compact single line */}
      <div className="flex min-w-0 flex-1 items-center gap-1.5">
        <span className="truncate font-heading text-xs font-bold text-foreground">
          {primary.agentName}
        </span>
        <span className="truncate font-mono text-[10px] text-foreground">
          {primary.activityText || STATUS_VISUALS[primary.status].label}
        </span>
      </div>

      {/* Overflow indicator */}
      {overflow > 0 && (
        <span className="flex h-4 min-w-[16px] flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-primary px-1 font-mono text-[9px] font-bold text-black">
          +{overflow}
        </span>
      )}

      {/* Expand hint */}
      <Eye className="h-3 w-3 flex-shrink-0 text-foreground opacity-0 transition-opacity group-hover:opacity-100" />
    </button>
  );
}

// ============================================================================
// Expanded panel — multi-agent list
// ============================================================================

function ExpandedPanel({
  agents,
  onCollapse,
  onClearAll,
  onInvokeAgent,
}: {
  agents: IslandAgent[];
  onCollapse: () => void;
  onClearAll: () => void;
  onInvokeAgent?: (agentId: string) => void;
}) {
  return (
    <div className="w-full border-t-2 border-black bg-brutal-cream">
      {/* Header */}
      <div className="flex items-center justify-between border-b-2 border-black bg-brutal-cream px-2.5 py-1.5">
        <div className="flex items-center gap-1.5">
          <span className="flex h-4 w-4 items-center justify-center border-2 border-black bg-brutal-primary">
            <Eye className="h-2.5 w-2.5 text-white" />
          </span>
          <span className="font-heading text-xs font-bold">Agent View</span>
          <span className="font-mono text-[9px] text-foreground">
            {agents.length}
          </span>
        </div>
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={onClearAll}
            className="btn-flat h-5 px-1.5 text-[9px]"
            aria-label="清除全部"
            title="清除全部"
          >
            清除
          </button>
          <button
            type="button"
            onClick={onCollapse}
            className="flex h-5 w-5 items-center justify-center border-2 border-black bg-brutal-cream hover:bg-brutal-muted-light"
            aria-label="收起"
            title="收起"
          >
            <EyeOff className="h-2.5 w-2.5" />
          </button>
        </div>
      </div>

      {/* Agent list */}
      <div className="max-h-64 divide-y-2 divide-black overflow-y-auto">
        {agents.map((agent) => (
          <AgentRow
            key={agent.agentId}
            agent={agent}
            onClick={onInvokeAgent ? () => onInvokeAgent(agent.agentId) : undefined}
          />
        ))}
      </div>
    </div>
  );
}

// ============================================================================
// Agent row in the expanded list
// ============================================================================

function AgentRow({ agent, onClick }: { agent: IslandAgent; onClick?: () => void }) {
  const vis = STATUS_VISUALS[agent.status];
  const Icon = vis.icon;

  const interactive = !!onClick;

  return (
    <div
      className={cn(
        'flex items-start gap-2 px-2.5 py-1.5 transition-colors',
        interactive && 'cursor-pointer hover:bg-brutal-cream',
      )}
      onClick={onClick}
      role={interactive ? 'button' : undefined}
      tabIndex={interactive ? 0 : undefined}
      onKeyDown={
        interactive
          ? (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onClick?.();
              }
            }
          : undefined
      }
      aria-label={interactive ? `查看 ${agent.agentName} 完整 trace` : undefined}
    >
      <PixelAvatar agentId={agent.agentId} avatarUrl={null} size="sm" />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1 flex-wrap">
          <span
            className={cn(
              'h-1.5 w-1.5 flex-shrink-0 rounded-full',
              vis.dotClass,
              vis.pulse && 'animate-pulse',
            )}
            aria-hidden
          />
          <Icon
            className={cn(
              'h-2.5 w-2.5 flex-shrink-0',
              vis.iconClass,
              vis.spin && 'animate-spin',
            )}
            aria-hidden
          />
          <span className="truncate font-heading text-[11px] font-bold">
            {agent.agentName}
          </span>
          <span
            className={cn(
              'badge-brutal px-1 py-0 text-[8px]',
              STATUS_VISUALS[agent.status].badgeClass,
            )}
          >
            {STATUS_VISUALS[agent.status].label}
          </span>
          {agent.status === 'idle' && agent.finalState && (
            <span
              className={cn(
                'badge-brutal px-1 py-0 text-[8px]',
                FINAL_STATE_BADGE[agent.finalState].className,
              )}
            >
              {FINAL_STATE_BADGE[agent.finalState].label}
            </span>
          )}
        </div>
        <p className="mt-0.5 truncate font-mono text-[10px] text-foreground">
          {agent.activityText}
        </p>
        {agent.toolInputSummary && (
          <p className="mt-0.5 truncate font-mono text-[9px] text-cyan-700">
            {agent.toolInputSummary}
          </p>
        )}
      </div>
    </div>
  );
}
