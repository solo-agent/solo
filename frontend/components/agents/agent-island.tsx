'use client';

// ============================================================================
// AgentIsland (SOLO-island PR2) — iPhone Dynamic Island-style floating UI
// that surfaces real-time agent status in the current channel.
//
// Visual: a brutalist pill fixed at the bottom-center of the screen. Collapsed
// by default; shows the most recent activity for the first active agent.
// Expands (on click) into a list of all active agents, each clickable to
// scroll back to the latest activity in the channel.
//
// Disappears entirely when no agent is active (on-demand).
//
// Exit animation (SOLO-island PR-fix): when activeAgents empties, the pill
// stays mounted for 200ms while playing a fade + slide-down + scale-shrink
// animation (pure CSS via Tailwind transition classes — no framer-motion
// to keep the bundle lean), then unmounts. The active set transitioning
// to the expanded panel uses a key-based remount with a height + opacity
// transition so the panel doesn't pop.
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
  completed: { label: '完成', className: 'bg-brutal-lime text-black' },
  failed: { label: '失败', className: 'bg-brutal-red text-white' },
  aborted: { label: '中断', className: 'bg-brutal-stone text-white' },
  timeout: { label: '超时', className: 'bg-brutal-orange text-black' },
  cancelled: { label: '已取消', className: 'bg-brutal-stone text-white' },
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
    dotClass: 'bg-brutal-stone',
    iconClass: 'text-muted-foreground',
    spin: false,
    pulse: false,
    label: '空闲',
    badgeClass: 'bg-brutal-stone text-white',
  },
  thinking: {
    icon: Brain,
    dotClass: 'bg-brutal-yellow',
    iconClass: 'text-yellow-600',
    spin: false,
    pulse: true,
    label: '思考中',
    badgeClass: 'bg-brutal-yellow text-black',
  },
  running: {
    icon: Loader2,
    dotClass: 'bg-brutal-cyan',
    iconClass: 'text-cyan-600',
    spin: true,
    pulse: false,
    label: '执行中',
    badgeClass: 'bg-brutal-cyan text-black',
  },
  streaming: {
    icon: Bot,
    dotClass: 'bg-brutal-lime',
    iconClass: 'text-green-600',
    spin: false,
    pulse: true,
    label: '生成中',
    badgeClass: 'bg-brutal-lime text-black',
  },
  // waiting_approval: reserved per PRD v1.x approval flow — UI not implemented yet
  waiting_approval: {
    icon: AlertTriangle,
    dotClass: 'bg-brutal-orange',
    iconClass: 'text-orange-600',
    spin: false,
    pulse: true,
    label: '等审批',
    badgeClass: 'bg-brutal-orange text-black',
  },
  error: {
    icon: AlertTriangle,
    dotClass: 'bg-brutal-red',
    iconClass: 'text-red-600',
    spin: false,
    pulse: false,
    label: '出错',
    badgeClass: 'bg-brutal-red text-white',
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
        'fixed bottom-6 left-1/2 z-50 -translate-x-1/2 transition-all duration-200 ease-out',
        // Enter: from "exited" state into the resting position.
        // Exit:  slide down, fade out, slight scale shrink — iPhone-style
        //        island curl-up. The -translate-y shifts the pill toward
        //        the bottom edge so it disappears off-screen feel.
        closing
          ? 'translate-y-3 scale-95 opacity-0'
          : 'translate-y-0 scale-100 opacity-100',
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
// Collapsed pill — single-agent summary
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
      className="group flex items-center gap-3 border-brutal-thick border-black bg-white px-4 py-2 shadow-brutal-xl transition-all hover:-translate-x-[2px] hover:-translate-y-[2px] hover:shadow-[12px_12px_0_0_#000] active:translate-x-[3px] active:translate-y-[3px] active:shadow-none"
      aria-label={`Agent ${primary.agentName} ${STATUS_VISUALS[primary.status].label},点击查看详情`}
    >
      {/* Pixel avatar */}
      <PixelAvatar agentId={primary.agentId} avatarUrl={null} size="sm" />

      {/* Status indicator dot */}
      <span
        className={cn(
          'h-2.5 w-2.5 rounded-full',
          vis.dotClass,
          vis.pulse && 'animate-pulse',
        )}
        aria-hidden
      />

      {/* Status icon (subtle) */}
      <Icon
        className={cn(
          'h-3.5 w-3.5 flex-shrink-0',
          vis.iconClass,
          vis.spin && 'animate-spin',
        )}
        aria-hidden
      />

      {/* Agent name + activity */}
      <div className="flex min-w-0 items-center gap-2">
        <span className="font-heading text-sm font-bold text-foreground">
          {primary.agentName}
        </span>
        <span className="font-mono text-[11px] text-muted-foreground">
          ·
        </span>
        <span className="truncate font-mono text-[11px] text-muted-foreground">
          {primary.activityText || STATUS_VISUALS[primary.status].label}
        </span>
        {primary.toolInputSummary && primary.status === 'running' && (
          <span className="ml-1 hidden truncate font-mono text-[10px] text-cyan-700 sm:inline">
            {primary.toolInputSummary}
          </span>
        )}
      </div>

      {/* Overflow indicator */}
      {overflow > 0 && (
        <span className="ml-1 flex h-5 min-w-[20px] items-center justify-center border-2 border-black bg-brutal-pink px-1 font-mono text-[10px] font-bold text-black">
          +{overflow}
        </span>
      )}

      {/* Expand hint */}
      <Eye className="ml-1 h-3 w-3 flex-shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
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
    <div className="w-[420px] max-w-[calc(100vw-3rem)] border-brutal-thick border-black bg-white shadow-brutal-xl">
      {/* Header */}
      <div className="flex items-center justify-between border-b-2 border-black bg-brutal-cream px-3 py-2">
        <div className="flex items-center gap-2">
          <span className="flex h-5 w-5 items-center justify-center border-2 border-black bg-brutal-pink">
            <Eye className="h-3 w-3 text-white" />
          </span>
          <span className="font-heading text-sm font-bold">Agent View</span>
          <span className="font-mono text-[10px] text-muted-foreground">
            {agents.length} {agents.length === 1 ? 'agent' : 'agents'}
          </span>
        </div>
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={onClearAll}
            className="btn-flat h-6 px-2 text-[10px]"
            aria-label="清除全部"
            title="清除全部"
          >
            清除
          </button>
          <button
            type="button"
            onClick={onCollapse}
            className="flex h-6 w-6 items-center justify-center border-2 border-black bg-white hover:bg-brutal-cream"
            aria-label="收起"
            title="收起"
          >
            <EyeOff className="h-3 w-3" />
          </button>
        </div>
      </div>

      {/* Agent list */}
      <div className="max-h-72 divide-y-2 divide-black overflow-y-auto">
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
        'flex items-start gap-2.5 px-3 py-2 transition-colors',
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
        <div className="flex items-center gap-1.5">
          <span
            className={cn(
              'h-2 w-2 flex-shrink-0 rounded-full',
              vis.dotClass,
              vis.pulse && 'animate-pulse',
            )}
            aria-hidden
          />
          <Icon
            className={cn(
              'h-3 w-3 flex-shrink-0',
              vis.iconClass,
              vis.spin && 'animate-spin',
            )}
            aria-hidden
          />
          <span className="truncate font-heading text-xs font-bold">
            {agent.agentName}
          </span>
          <span
            className={cn(
              'badge-brutal px-1.5 py-0 text-[9px]',
              STATUS_VISUALS[agent.status].badgeClass,
            )}
          >
            {STATUS_VISUALS[agent.status].label}
          </span>
          {/* Terminal outcome badge — only shown for idle entries that
              came in via agent.done. failed/aborted/etc. deserve a
              distinct color so the user can spot failures at a glance. */}
          {agent.status === 'idle' && agent.finalState && (
            <span
              className={cn(
                'badge-brutal px-1.5 py-0 text-[9px]',
                FINAL_STATE_BADGE[agent.finalState].className,
              )}
            >
              {FINAL_STATE_BADGE[agent.finalState].label}
            </span>
          )}
        </div>
        <p className="mt-0.5 truncate font-mono text-[11px] text-muted-foreground">
          {agent.activityText}
        </p>
        {agent.toolInputSummary && (
          <p className="mt-0.5 truncate font-mono text-[10px] text-cyan-700">
            {agent.toolInputSummary}
          </p>
        )}
      </div>
    </div>
  );
}
