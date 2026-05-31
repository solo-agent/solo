// ============================================================================
// TypingIndicator — shows which agents are thinking, typing, or streaming
// - Positioned below the message list, above the input
// - Three distinct states with different colors and animations
// ============================================================================

'use client';

import { Bot, Brain, Loader2 } from 'lucide-react';

/**
 * Describes a single agent's current activity state.
 */
export interface AgentActivity {
  agentId: string;
  name: string;
  state: 'thinking' | 'typing' | 'streaming';
}

interface TypingIndicatorProps {
  /**
   * List of agents and their current activity states.
   * Empty array hides the indicator.
   */
  agents: AgentActivity[];
}

/** State-specific visual config */
const STATE_CONFIG: Record<
  AgentActivity['state'],
  {
    icon: React.ReactNode;
    label: string;
    colorClass: string;
    bgClass: string;
  }
> = {
  thinking: {
    icon: <Brain className="h-3.5 w-3.5" />,
    label: '思考中',
    colorClass: 'text-yellow-600 dark:text-yellow-400',
    bgClass: 'bg-yellow-500/10',
  },
  typing: {
    icon: <Loader2 className="h-3.5 w-3.5 animate-spin" />,
    label: '输入中',
    colorClass: 'text-blue-600 dark:text-blue-400',
    bgClass: 'bg-blue-500/10',
  },
  streaming: {
    icon: <Bot className="h-3.5 w-3.5 animate-pulse" />,
    label: '流式输出中',
    colorClass: 'text-green-600 dark:text-green-400',
    bgClass: 'bg-green-500/10',
  },
};

/** Animated dots */
function AnimatedDots() {
  return (
    <span className="flex gap-0.5">
      <span className="h-1 w-1 animate-bounce rounded-full bg-current opacity-60 [animation-delay:0ms]" />
      <span className="h-1 w-1 animate-bounce rounded-full bg-current opacity-60 [animation-delay:150ms]" />
      <span className="h-1 w-1 animate-bounce rounded-full bg-current opacity-60 [animation-delay:300ms]" />
    </span>
  );
}

/** Single agent activity badge */
function AgentBadge({ activity }: { activity: AgentActivity }) {
  const config = STATE_CONFIG[activity.state];
  return (
    <div
      className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ${config.bgClass} ${config.colorClass}`}
    >
      {config.icon}
      <span className="truncate max-w-[120px]">{activity.name}</span>
      <span>{config.label}</span>
    </div>
  );
}

export function TypingIndicator({ agents }: TypingIndicatorProps) {
  if (agents.length === 0) return null;

  return (
    <div className="flex items-center gap-1.5 px-6 py-2 text-xs text-muted-foreground" aria-live="polite" aria-atomic="true">
      <div className="flex flex-wrap items-center gap-1.5">
        {agents.slice(0, 3).map((agent) => (
          <AgentBadge key={`${agent.agentId}-${agent.state}`} activity={agent} />
        ))}
        {agents.length > 3 && (
          <span className="text-muted-foreground/60">
            +{agents.length - 3}
          </span>
        )}
      </div>
      <AnimatedDots />
    </div>
  );
}
