'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { AlertTriangle, Bot, Brain, CheckCircle2, Clock, Eye, EyeOff, Loader2, XCircle } from 'lucide-react';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { useAgentIsland, type AgentRunStatus, type IslandAgent } from '@/lib/hooks/use-agent-island';
import { displayAgentActivity } from '@/lib/agent-activity';
import { cn } from '@/lib/utils';
import { t, type TranslationKey } from '@/lib/i18n';

type StatusVisuals = {
  icon: typeof Brain;
  dotClass: string;
  iconClass: string;
  badgeClass: string;
  labelKey: TranslationKey;
  spin?: boolean;
  pulse?: boolean;
};

const STATUS_VISUALS: Record<AgentRunStatus, StatusVisuals> = {
  queued: {
    icon: Clock,
    dotClass: 'bg-brutal-muted',
    iconClass: 'text-foreground',
    badgeClass: 'bg-brutal-muted text-white',
    labelKey: 'agentQueued',
  },
  thinking: {
    icon: Brain,
    dotClass: 'bg-brutal-accent',
    iconClass: 'text-yellow-600',
    badgeClass: 'bg-brutal-accent text-black',
    labelKey: 'agentThinking',
    pulse: true,
  },
  running: {
    icon: Loader2,
    dotClass: 'bg-brutal-info',
    iconClass: 'text-cyan-600',
    badgeClass: 'bg-brutal-info text-black',
    labelKey: 'agentExecuting',
    spin: true,
  },
  streaming: {
    icon: Bot,
    dotClass: 'bg-brutal-success',
    iconClass: 'text-green-600',
    badgeClass: 'bg-brutal-success text-black',
    labelKey: 'agentGenerating',
    pulse: true,
  },
  waiting_input: {
    icon: AlertTriangle,
    dotClass: 'bg-brutal-warning',
    iconClass: 'text-orange-600',
    badgeClass: 'bg-brutal-warning text-black',
    labelKey: 'agentWaitingInput',
    pulse: true,
  },
  waiting_approval: {
    icon: AlertTriangle,
    dotClass: 'bg-brutal-warning',
    iconClass: 'text-orange-600',
    badgeClass: 'bg-brutal-warning text-black',
    labelKey: 'agentWaitingApproval',
    pulse: true,
  },
  completed: {
    icon: CheckCircle2,
    dotClass: 'bg-brutal-success',
    iconClass: 'text-green-700',
    badgeClass: 'bg-brutal-success text-black',
    labelKey: 'agentDone',
  },
  failed: {
    icon: XCircle,
    dotClass: 'bg-brutal-danger',
    iconClass: 'text-red-600',
    badgeClass: 'bg-brutal-danger text-white',
    labelKey: 'runFailed',
  },
  cancelled: {
    icon: XCircle,
    dotClass: 'bg-brutal-muted',
    iconClass: 'text-foreground',
    badgeClass: 'bg-brutal-muted text-white',
    labelKey: 'runCancelled',
  },
  timeout: {
    icon: AlertTriangle,
    dotClass: 'bg-brutal-danger',
    iconClass: 'text-red-600',
    badgeClass: 'bg-brutal-danger text-white',
    labelKey: 'runTimeout',
  },
};

export function AgentIsland() {
  const router = useRouter();
  const { activeAgents, clearAll } = useAgentIsland();
  const [expanded, setExpanded] = useState(false);
  const [, setTick] = useState(0);

  useEffect(() => {
    const id = setInterval(() => setTick((v) => v + 1), 1000);
    return () => clearInterval(id);
  }, []);

  if (activeAgents.length === 0) return null;

  const primary = activeAgents[0];
  const overflow = activeAgents.length - 1;

  const openRun = (runId: string) => {
    router.push(`/observability/live?run_id=${encodeURIComponent(runId)}`);
  };

  return (
    <div className="fixed bottom-0 left-[56px] z-50 w-[220px] border-r-2 border-black">
      {expanded ? (
        <ExpandedPanel
          agents={activeAgents}
          onOpenRun={openRun}
          onCollapse={() => setExpanded(false)}
          onClearAll={clearAll}
        />
      ) : (
        <CollapsedPill
          primary={primary}
          overflow={overflow}
          onExpand={() => setExpanded(true)}
          onOpenRun={() => openRun(primary.runId)}
        />
      )}
    </div>
  );
}

function CollapsedPill({
  primary,
  overflow,
  onExpand,
  onOpenRun,
}: {
  primary: IslandAgent;
  overflow: number;
  onExpand: () => void;
  onOpenRun: () => void;
}) {
  const visual = STATUS_VISUALS[primary.status];
  const Icon = visual.icon;
  const label = t(visual.labelKey);
  const activity = displayAgentActivity(primary.status, primary.activityText, primary.toolInputSummary, label);

  return (
    <div className="border-t-2 border-black bg-brutal-cream px-3 py-2">
      <button
        type="button"
        onClick={onOpenRun}
        className="group flex w-full flex-col text-left"
        aria-label={`${primary.agentName} ${label}`}
      >
        <div className="flex w-full items-center gap-2">
          <PixelAvatar agentId={primary.agentId} avatarUrl={null} size="sm" />
          <span className="min-w-0 flex-1 truncate font-heading text-xs font-bold text-foreground">
            {primary.agentName}
          </span>
          <span className={cn('badge-brutal px-1.5 py-0 text-[9px]', visual.badgeClass)}>
            {label}
          </span>
          {overflow > 0 && (
            <span className="border-2 border-black bg-brutal-primary px-1 font-mono text-[9px] font-bold text-black">
              +{overflow}
            </span>
          )}
        </div>

        <div className="mt-1 flex w-full items-center gap-2">
          <span className="inline-flex w-7 flex-shrink-0 items-center justify-center gap-0.5">
            <span className={cn('h-2.5 w-2.5 rounded-full', visual.dotClass, visual.pulse && 'animate-pulse')} />
            <Icon className={cn('h-3.5 w-3.5', visual.iconClass, visual.spin && 'animate-spin-slow')} />
          </span>
          <span className="min-w-0 flex-1 truncate font-heading text-xs font-bold text-foreground">
            {activity}
          </span>
          <span className="font-mono text-[9px] text-foreground">{elapsed(primary)}</span>
        </div>

        <div className="mt-1 flex items-center gap-1 pl-9 font-mono text-[9px] text-foreground">
          {primary.taskId ? <span>{t('agentTaskRef', { id: primary.taskId.slice(0, 8) })}</span> : <span>{t('agentSessionRef', { id: primary.sessionId?.slice(0, 8) ?? '-' })}</span>}
        </div>
      </button>
      {overflow > 0 && (
        <button
          type="button"
          onClick={onExpand}
          className="mt-1 flex w-full items-center justify-center border-t-2 border-black pt-1 font-heading text-[10px] font-bold hover:text-brutal-primary"
        >
          <Eye className="mr-1 h-3 w-3" />
          {t('agentExpandAll')}
        </button>
      )}
    </div>
  );
}

function ExpandedPanel({
  agents,
  onOpenRun,
  onCollapse,
  onClearAll,
}: {
  agents: IslandAgent[];
  onOpenRun: (runId: string) => void;
  onCollapse: () => void;
  onClearAll: () => void;
}) {
  return (
    <div className="border-t-2 border-black bg-brutal-cream">
      <div className="flex items-center justify-between border-b-2 border-black px-2.5 py-1.5">
        <span className="font-heading text-xs font-bold">{t('agentLive')}</span>
        <div className="flex items-center gap-1">
          <button type="button" onClick={onClearAll} className="btn-flat h-5 px-1.5 text-[9px]">
            {t('agentClearAll')}
          </button>
          <button
            type="button"
            onClick={onCollapse}
            className="flex h-5 w-5 items-center justify-center border-2 border-black bg-brutal-cream hover:bg-brutal-muted-light"
            aria-label={t('agentCollapse')}
          >
            <EyeOff className="h-2.5 w-2.5" />
          </button>
        </div>
      </div>

      <div className="max-h-72 divide-y-2 divide-black overflow-y-auto">
        {agents.map((agent) => (
          <AgentRow key={agent.runId} agent={agent} onClick={() => onOpenRun(agent.runId)} />
        ))}
      </div>
    </div>
  );
}

function AgentRow({ agent, onClick }: { agent: IslandAgent; onClick: () => void }) {
  const visual = STATUS_VISUALS[agent.status];
  const Icon = visual.icon;
  const label = t(visual.labelKey);
  const activity = displayAgentActivity(agent.status, agent.activityText, agent.toolInputSummary, label);

  return (
    <button type="button" onClick={onClick} className="flex w-full items-start gap-2 px-2.5 py-1.5 text-left hover:bg-brutal-muted-light">
      <PixelAvatar agentId={agent.agentId} avatarUrl={null} size="sm" />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1">
          <span className={cn('h-1.5 w-1.5 rounded-full', visual.dotClass, visual.pulse && 'animate-pulse')} />
          <Icon className={cn('h-2.5 w-2.5', visual.iconClass, visual.spin && 'animate-spin-slow')} />
          <span className="truncate font-heading text-[11px] font-bold">{agent.agentName}</span>
          <span className={cn('badge-brutal px-1 py-0 text-[8px]', visual.badgeClass)}>{label}</span>
          <span className="ml-auto font-mono text-[9px]">{elapsed(agent)}</span>
        </div>
        <p className="mt-0.5 truncate font-mono text-[10px] text-foreground">
          {activity}
        </p>
        <p className="mt-0.5 truncate font-mono text-[9px] text-cyan-700">
          {agent.taskId ? t('agentTaskRef', { id: agent.taskId.slice(0, 8) }) : t('agentSessionRef', { id: agent.sessionId?.slice(0, 8) ?? '-' })}
        </p>
      </div>
    </button>
  );
}

function elapsed(agent: IslandAgent): string {
  const from = agent.startedAt ?? agent.updatedAt;
  const seconds = Math.max(0, Math.floor((Date.now() - from) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const rest = seconds % 60;
  return `${minutes}m${rest.toString().padStart(2, '0')}s`;
}
