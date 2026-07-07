'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import Link from 'next/link';
import {
  Activity,
  ArrowLeft,
  BarChart3,
  GitBranch,
  Layers,
  RefreshCw,
  X,
} from 'lucide-react';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { Button, iconActionClass } from '@/components/ui/button';
import { Spinner } from '@/components/ui/spinner';
import { tabButtonClass } from '@/components/ui/tab-bar';
import { agentRunStatusText, displayAgentActivity } from '@/lib/agent-activity';
import { apiClient } from '@/lib/api-client';
import type { AgentRunStatus } from '@/lib/hooks/use-agent-island';
import { useWebSocket } from '@/lib/ws-context';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';

interface DashboardLive {
  groups: DashboardLiveGroup[];
  totals: {
    agents: number;
    working: number;
    needs_attention: number;
    idle_recent: number;
  };
}

interface DashboardLiveGroup {
  key: string;
  label: string;
  count: number;
  items: DashboardLiveAgent[];
}

interface DashboardLiveAgent {
  agent_id: string;
  agent_name: string;
  avatar_url?: string;
  group: string;
  run_id?: string;
  session_id?: string;
  task_id?: string;
  status?: AgentRunStatus;
  activity_text?: string;
  tool_name?: string;
  tool_input_summary?: string;
  source?: string;
  updated_at?: string;
  active_count: number;
  attention_count: number;
  run_count: number;
}

interface AgentSession {
  id: string;
  provider: string;
  external_session_id?: string;
  transcript_path?: string;
  title?: string;
  status: string;
  started_at: string;
  last_active_at: string;
}

interface AgentTaskSummary {
  id: string;
  task_number: number;
  title: string;
  status: string;
  last_run_status: string;
  last_activity: string;
  last_run_at: string;
  linked_run_count: number;
}

interface AgentTimeline {
  scope: 'session' | 'task';
  id: string;
  session?: AgentSession;
  task?: {
    id: string;
    task_number: number;
    title: string;
    status: string;
  };
  runs: AgentTimelineRun[];
  entries: AgentTranscriptEntry[];
}

interface AgentTimelineRun {
  id: string;
  agent_id: string;
  session_id?: string;
  status: AgentRunStatus;
  activity_text: string;
  source?: string;
  started_at: string;
  finished_at?: string;
  entry_start_seq?: number;
  entry_end_seq?: number;
}

interface AgentRunDetail {
  id: string;
  agent_id: string;
  session_id?: string;
  status: AgentRunStatus;
  activity_text?: string;
  tool_name?: string;
  tool_input_summary?: string;
  source?: string;
  updated_at?: string;
}

interface AgentTranscriptEntry {
  seq: number;
  timestamp?: string;
  role: string;
  type: string;
  text?: string;
  tool_name?: string;
  tool_id?: string;
  input?: unknown;
  raw?: unknown;
}

const GROUP_HEADER_CLASSES: Record<string, string> = {
  working: 'bg-brutal-info',
  needs_attention: 'bg-brutal-violet',
  idle_recent: 'bg-brutal-success',
};

function groupLabel(key: string, fallback: string) {
  const labels: Record<string, string> = {
    working: t('observabilityGroupWorking'),
    needs_attention: t('observabilityGroupNeedsAttention'),
    idle_recent: t('observabilityGroupIdleRecent'),
  };
  return labels[key] ?? fallback;
}

const QUESTION_HUES = [
  { bg: 'bg-brutal-primary-light', swatch: 'bg-brutal-primary' },
  { bg: 'bg-brutal-info-light', swatch: 'bg-brutal-info' },
  { bg: 'bg-brutal-success-light', swatch: 'bg-brutal-success' },
  { bg: 'bg-brutal-warning-light', swatch: 'bg-brutal-warning' },
  { bg: 'bg-brutal-accent-light', swatch: 'bg-brutal-accent' },
  { bg: 'bg-brutal-violet-light', swatch: 'bg-brutal-violet' },
];

export function LiveMonitor({ selectedRunId }: { selectedRunId?: string | null }) {
  const { onEvent } = useWebSocket();
  const [live, setLive] = useState<DashboardLive | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [selectedAgent, setSelectedAgent] = useState<DashboardLiveAgent | null>(null);
  const seenRunSessions = useRef(new Set<string>());

  const loadLive = useCallback(async () => {
    const next = await apiClient.get<DashboardLive>('/api/v1/dashboard/live');
    setLive(next);
    setSelectedAgent((current) => current ? findAgent(next, current.agent_id) ?? current : current);
  }, []);

  useEffect(() => {
    let cancelled = false;
    setIsLoading(true);
    loadLive()
      .catch(() => {
        if (!cancelled) setLive({ groups: [], totals: { agents: 0, working: 0, needs_attention: 0, idle_recent: 0 } });
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [loadLive]);

  useEffect(() => onEvent((event) => {
    if (event.type === 'agent.run.started' || event.type === 'agent.run.finished') {
      loadLive().catch(() => undefined);
      return;
    }
    if (event.type === 'agent.run.updated' && event.session_id && !seenRunSessions.current.has(event.run_id)) {
      seenRunSessions.current.add(event.run_id);
      loadLive().catch(() => undefined);
    }
  }), [loadLive, onEvent]);

  useEffect(() => {
    if (!selectedRunId || !live) return;
    const agent = live.groups.flatMap((group) => group.items).find((item) => item.run_id === selectedRunId);
    if (agent) setSelectedAgent(agent);
  }, [live, selectedRunId]);

  useEffect(() => {
    if (!selectedRunId) return;
    let cancelled = false;
    apiClient.get<AgentRunDetail>(`/api/v1/agent-runs/${selectedRunId}`)
      .then((run) => {
        if (cancelled) return;
        setSelectedAgent(agentFromRun(run, live));
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [live, selectedRunId]);

  const groups = live?.groups ?? [];
  const totals = live?.totals ?? { agents: 0, working: 0, needs_attention: 0, idle_recent: 0 };

  return (
    <div className="flex h-full min-h-0 bg-brutal-cream">
      <section className="min-w-0 flex-1 overflow-auto">
        <DashboardTopTabs active="live" />
        <div className="border-b-2 border-black bg-brutal-cream px-5 py-4">
          <div className="flex items-center justify-between gap-4">
            <div>
              <h1 className="font-heading text-2xl font-black">{t('observabilityLiveTitle')}</h1>
              <p className="mt-1 font-mono text-xs text-muted-foreground">{t('observabilityLiveDesc')}</p>
            </div>
            <Button variant="outline" size="sm" onClick={() => loadLive().catch(() => undefined)}>
              <RefreshCw className="mr-2 h-4 w-4" />
              {t('refresh')}
            </Button>
          </div>
          <div className="mt-4 grid grid-cols-4 gap-3">
            <LiveMetric label={t('observabilityAgents')} value={totals.agents} />
            <LiveMetric label={t('observabilityWorking')} value={totals.working} />
            <LiveMetric label={t('observabilityAttention')} value={totals.needs_attention} />
            <LiveMetric label={t('observabilityIdle')} value={totals.idle_recent} />
          </div>
        </div>

        {isLoading ? (
          <div className="flex h-64 items-center justify-center">
            <Spinner size="md" />
          </div>
        ) : (
          <div className="space-y-4 p-5">
            {groups.map((group) => (
              <LiveGroup
                key={group.key}
                group={group}
                selectedAgentId={selectedAgent?.agent_id ?? null}
                onSelect={setSelectedAgent}
              />
            ))}
          </div>
        )}
      </section>

      {selectedAgent && (
        <AgentWorkPanel
          agent={selectedAgent}
          autoOpenRunId={selectedRunId ?? null}
          onClose={() => setSelectedAgent(null)}
        />
      )}
    </div>
  );
}

export function DashboardTopTabs({ active }: { active: 'live' | 'insight' }) {
  return (
    <div className="sidebar-collapse-offset flex h-14 items-center gap-2 border-b-2 border-black bg-brutal-cream px-5">
      <Link
        href="/observability/live"
        className={tabButtonClass(active === 'live', 'h-9 text-sm')}
      >
        <Activity className="h-4 w-4" />
        {t('observabilityLive')}
      </Link>
      <Link
        href="/observability/insight"
        className={tabButtonClass(active === 'insight', 'h-9 text-sm')}
      >
        <BarChart3 className="h-4 w-4" />
        {t('observabilityInsight')}
      </Link>
    </div>
  );
}

function LiveGroup({ group, selectedAgentId, onSelect }: { group: DashboardLiveGroup; selectedAgentId: string | null; onSelect: (agent: DashboardLiveAgent) => void }) {
  if (group.items.length === 0) {
    return (
      <section className="border-2 border-black bg-brutal-cream shadow-brutal-sm">
        <div className={cn('flex items-center justify-between px-3 py-2', groupHeaderClass(group.key))}>
          <h2 className="font-heading text-base font-black">{groupLabel(group.key, group.label)}</h2>
          <span className="font-mono text-xs">{group.count} · {t('observabilityNone')}</span>
        </div>
      </section>
    );
  }

  return (
    <section className="min-w-0 border-2 border-black bg-brutal-cream shadow-brutal-sm">
      <div className={cn('flex items-center justify-between border-b-2 border-black px-3 py-2', groupHeaderClass(group.key))}>
        <h2 className="font-heading text-base font-black">{groupLabel(group.key, group.label)}</h2>
        <span className="font-mono text-xs">{group.count}</span>
      </div>
      <div className="grid gap-3 p-3 md:grid-cols-2 2xl:grid-cols-3">
        {group.items.map((agent) => (
          <AgentLiveCard
            key={agent.agent_id}
            agent={agent}
            selected={selectedAgentId === agent.agent_id}
            onClick={() => onSelect(agent)}
          />
        ))}
      </div>
    </section>
  );
}

function AgentLiveCard({ agent, selected, onClick }: { agent: DashboardLiveAgent; selected: boolean; onClick: () => void }) {
  const activity = displayAgentActivity(agent.status, agent.activity_text, agent.tool_input_summary);
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={`${agent.agent_name} ${agentRunStatusText(agent.status)} ${activity}`}
      className={cn(
        'group block w-full border-2 border-black p-3 text-left shadow-brutal-sm transition-all hover:-translate-y-px hover:shadow-brutal active:translate-x-0.5 active:translate-y-0.5 active:shadow-none',
        selected
          ? 'bg-brutal-primary hover:bg-brutal-primary'
          : 'bg-brutal-cream hover:bg-brutal-accent-light',
      )}
    >
      <div className="flex items-start gap-3">
        <PixelAvatar agentId={agent.agent_id} avatarUrl={agent.avatar_url} size="md" />
        <div className="min-w-0 flex-1">
          <div className="flex items-center justify-between gap-3">
            <div className="min-w-0">
              <div className="truncate font-heading text-base font-black">{agent.agent_name}</div>
              <div className="mt-0.5 font-mono text-[11px] text-muted-foreground">{agent.source || t('observabilityRuntime')} · {formatRelative(agent.updated_at)}</div>
            </div>
            <StatusBadge status={agent.status} />
          </div>
          <p
            title={activity}
            className="mt-3 line-clamp-2 font-mono text-sm group-hover:line-clamp-none group-focus:line-clamp-none"
          >
            {activity}
          </p>
          <div className="mt-3 flex flex-wrap gap-2 font-mono text-[11px] text-muted-foreground">
            {agent.task_id && <span className="text-brutal-info">{t('agentTaskRef', { id: shortID(agent.task_id) })}</span>}
            {agent.session_id && <span>{t('agentSessionRef', { id: shortID(agent.session_id) })}</span>}
            <span>{t('observabilityRuns', { n: agent.run_count })}</span>
          </div>
        </div>
      </div>
    </button>
  );
}

function AgentWorkPanel({ agent, autoOpenRunId, onClose }: { agent: DashboardLiveAgent; autoOpenRunId: string | null; onClose: () => void }) {
  const { onEvent } = useWebSocket();
  const [mode, setMode] = useState<'session' | 'task'>('session');
  const [sessions, setSessions] = useState<AgentSession[]>([]);
  const [tasks, setTasks] = useState<AgentTaskSummary[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [timeline, setTimeline] = useState<AgentTimeline | null>(null);
  const [timelineLoading, setTimelineLoading] = useState(false);
  const seenRunSessions = useRef(new Set<string>());

  const loadLists = useCallback(async () => {
    const [nextSessions, nextTasks] = await Promise.all([
      apiClient.get<AgentSession[]>(`/api/v1/agents/${agent.agent_id}/sessions`).catch(() => []),
      apiClient.get<AgentTaskSummary[]>(`/api/v1/agents/${agent.agent_id}/tasks`).catch(() => []),
    ]);
    setSessions(Array.isArray(nextSessions) ? nextSessions : []);
    setTasks(Array.isArray(nextTasks) ? nextTasks : []);
  }, [agent.agent_id]);

  useEffect(() => {
    let cancelled = false;
    setMode('session');
    setTimeline(null);
    setIsLoading(true);
    loadLists().finally(() => {
      if (!cancelled) setIsLoading(false);
    });
    return () => {
      cancelled = true;
    };
  }, [agent.agent_id, loadLists]);

  useEffect(() => onEvent((event) => {
    if (
      (event.type === 'agent.run.started' || event.type === 'agent.run.finished') &&
      event.agent_id === agent.agent_id
    ) {
      loadLists().catch(() => undefined);
      return;
    }
    if (
      event.type === 'agent.run.updated' &&
      event.agent_id === agent.agent_id &&
      event.session_id &&
      !seenRunSessions.current.has(event.run_id)
    ) {
      seenRunSessions.current.add(event.run_id);
      loadLists().catch(() => undefined);
    }
  }), [agent.agent_id, loadLists, onEvent]);

  const openSession = async (session: AgentSession) => {
    setTimelineLoading(true);
    try {
      const next = await apiClient.get<AgentTimeline>(`/api/v1/agent-sessions/${session.id}/timeline`, { limit: '5000' });
      setTimeline(next);
    } finally {
      setTimelineLoading(false);
    }
  };

  useEffect(() => {
    if (!autoOpenRunId || agent.run_id !== autoOpenRunId || !agent.session_id) return;
    setTimelineLoading(true);
    apiClient.get<AgentTimeline>(`/api/v1/agent-sessions/${agent.session_id}/timeline`, { limit: '5000' })
      .then(setTimeline)
      .catch(() => undefined)
      .finally(() => setTimelineLoading(false));
  }, [agent.run_id, agent.session_id, autoOpenRunId]);

  const openTask = async (task: AgentTaskSummary) => {
    setTimelineLoading(true);
    try {
      const next = await apiClient.get<AgentTimeline>(`/api/v1/tasks/${task.id}/agent-timeline`, { agent_id: agent.agent_id, limit: '5000' });
      setTimeline(next);
    } finally {
      setTimelineLoading(false);
    }
  };

  return (
    <aside className="flex h-full w-[min(980px,58vw)] min-w-[620px] flex-shrink-0 flex-col border-l-2 border-black bg-brutal-cream shadow-brutal">
      <div className="flex h-14 items-center justify-between border-b-2 border-black bg-brutal-cream px-4">
        <div className="flex min-w-0 items-center gap-3">
          {timeline && (
            <button type="button" className={iconActionClass('rounded-none shadow-brutal-sm')} onClick={() => setTimeline(null)} aria-label={t('observabilityBackToList')}>
              <ArrowLeft className="h-4 w-4" />
            </button>
          )}
          <PixelAvatar agentId={agent.agent_id} avatarUrl={agent.avatar_url} size="sm" />
          <div className="min-w-0">
            <div className="truncate font-heading text-lg font-black">{agent.agent_name}</div>
            <div className="font-mono text-xs text-muted-foreground">{statusText(agent.status)}</div>
          </div>
        </div>
        <button type="button" className={iconActionClass('rounded-none shadow-brutal-sm hover:bg-brutal-danger focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brutal-danger focus-visible:ring-offset-2')} onClick={onClose} aria-label={t('observabilityClosePanel')}>
          <X className="h-4 w-4" />
        </button>
      </div>

      {!timeline && (
        <div className="flex h-14 items-center gap-2 border-b-2 border-black bg-brutal-cream px-4">
          <button
            type="button"
            className={tabButtonClass(mode === 'session', 'h-9 text-sm')}
            onClick={() => setMode('session')}
          >
            <Layers className="h-4 w-4" />
            {t('observabilitySession')}
          </button>
          <button
            type="button"
            className={tabButtonClass(mode === 'task', 'h-9 text-sm')}
            onClick={() => setMode('task')}
          >
            <GitBranch className="h-4 w-4" />
            {t('observabilityTask')}
          </button>
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-hidden">
        {timelineLoading ? (
          <div className="flex h-full items-center justify-center">
            <Spinner size="md" />
          </div>
        ) : timeline ? (
          <TimelineViewer timeline={timeline} />
        ) : isLoading ? (
          <div className="flex h-full items-center justify-center">
            <Spinner size="md" />
          </div>
        ) : mode === 'session' ? (
          <SessionList sessions={sessions} onOpen={openSession} />
        ) : (
          <TaskList tasks={tasks} onOpen={openTask} />
        )}
      </div>
    </aside>
  );
}

function SessionList({ sessions, onOpen }: { sessions: AgentSession[]; onOpen: (session: AgentSession) => void }) {
  if (sessions.length === 0) return <EmptyPanel text={t('observabilityNoSessions')} />;
  return (
    <div className="h-full overflow-auto bg-brutal-cream">
      {sessions.map((session) => (
        <button
          key={session.id}
          type="button"
          onClick={() => onOpen(session)}
          className="block w-full border-b-2 border-black bg-brutal-cream p-4 text-left transition-all hover:-translate-y-px hover:bg-brutal-accent-light hover:shadow-brutal-sm active:translate-y-0.5 active:shadow-none"
        >
          <div className="flex items-center justify-between gap-3">
            <div className="min-w-0">
              <div className="truncate font-heading text-base font-black">{session.title || t('agentSessionRef', { id: shortID(session.id) })}</div>
              <div className="mt-1 font-mono text-xs text-muted-foreground">{session.provider} · {formatDate(session.last_active_at)}</div>
            </div>
            <span className="badge-brutal bg-brutal-primary px-2 py-1 text-[11px] text-black">{session.status}</span>
          </div>
          <div className="mt-2 truncate font-mono text-xs text-muted-foreground">{session.external_session_id || session.transcript_path || t('observabilityUnboundTranscript')}</div>
        </button>
      ))}
    </div>
  );
}

function TaskList({ tasks, onOpen }: { tasks: AgentTaskSummary[]; onOpen: (task: AgentTaskSummary) => void }) {
  if (tasks.length === 0) return <EmptyPanel text={t('observabilityNoTasks')} />;
  return (
    <div className="h-full overflow-auto bg-brutal-cream">
      {tasks.map((task) => (
        <button
          key={task.id}
          type="button"
          onClick={() => onOpen(task)}
          className="block w-full border-b-2 border-black bg-brutal-cream p-4 text-left transition-all hover:-translate-y-px hover:bg-brutal-accent-light hover:shadow-brutal-sm active:translate-y-0.5 active:shadow-none"
        >
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="line-clamp-2 font-heading text-base font-black">#{task.task_number} {task.title}</div>
              <div className="mt-2 font-mono text-xs text-muted-foreground">{t('observabilityRuns', { n: task.linked_run_count })} · {formatDate(task.last_run_at)}</div>
            </div>
            <span className="badge-brutal bg-white px-2 py-1 text-[11px] text-black">{task.status}</span>
          </div>
          <p className="mt-2 truncate font-mono text-xs">{task.last_activity || task.last_run_status}</p>
        </button>
      ))}
    </div>
  );
}

function TimelineViewer({ timeline }: { timeline: AgentTimeline }) {
  const title = timeline.scope === 'session'
    ? (timeline.session?.title || t('agentSessionRef', { id: shortID(timeline.id) }))
    : `#${timeline.task?.task_number ?? ''} ${timeline.task?.title ?? t('observabilityTask')}`;
  const questions = useMemo(() => timelineQuestionOutline(timeline), [timeline]);
  const questionIndexBySeq = useMemo(() => {
    const map = new Map<number, number>();
    questions.forEach((question, index) => map.set(question.seq, index));
    return map;
  }, [questions]);
  const [activeQuestionSeq, setActiveQuestionSeq] = useState<number | null>(questions[0]?.seq ?? null);

  useEffect(() => {
    setActiveQuestionSeq(questions[0]?.seq ?? null);
  }, [questions]);

  const scrollToSeq = (seq: number) => {
    setActiveQuestionSeq(seq);
    document.getElementById(`timeline-entry-${seq}`)?.scrollIntoView({ block: 'start', behavior: 'smooth' });
  };

  return (
    <div className="grid h-full min-h-0 grid-cols-[220px_1fr]">
      <div className="min-h-0 border-r-2 border-black bg-brutal-cream">
        <div className="flex h-24 flex-col justify-center border-b-2 border-black bg-brutal-cream p-3">
          <div className="font-heading text-base font-black">{t('observabilityQuestionOutline')}</div>
          <div className="mt-1 font-mono text-xs text-muted-foreground">{t('observabilityQuestionStats', { questions: questions.length, entries: timeline.entries.length })}</div>
        </div>
        <div className="h-[calc(100%-6rem)] overflow-auto">
          {questions.map((question, index) => {
            const hue = questionHue(index);
            const active = activeQuestionSeq === question.seq;
            return (
              <button
                key={`${question.seq}-${index}`}
                type="button"
                onClick={() => scrollToSeq(question.seq)}
                className={cn(
                  'block w-full border-b-2 border-black p-3 text-left transition-all active:translate-y-0.5 active:shadow-none',
                  active
                    ? 'bg-brutal-primary'
                    : 'bg-brutal-cream hover:-translate-y-px hover:bg-brutal-accent-light hover:shadow-brutal-sm',
                )}
              >
                <div className="flex items-center gap-2 font-heading text-sm font-black">
                  <span className={cn('flex h-6 min-w-6 items-center justify-center border-2 border-black font-mono text-[10px] text-black', hue.swatch)}>{index + 1}</span>
                  <span className="line-clamp-2">{question.title}</span>
                </div>
                <div className="mt-2 font-mono text-[11px] text-muted-foreground">{formatDate(question.timestamp)}</div>
              </button>
            );
          })}
        </div>
      </div>

      <div className="min-h-0 overflow-auto bg-brutal-cream">
        <div className="sticky top-0 z-10 flex h-24 flex-col justify-center border-b-2 border-black bg-brutal-cream px-4">
          <div className="truncate font-heading text-lg font-black">{title}</div>
          <div className="mt-1 font-mono text-xs text-muted-foreground">{t('observabilityTranscriptViewer')}</div>
        </div>
        <div className="space-y-4 p-5">
          {timeline.entries.length === 0 ? (
            <EmptyPanel text={t('observabilityNoReadableTranscript')} />
          ) : timeline.entries.map((entry) => (
            <TranscriptEntryCard key={`${entry.seq}-${entry.type}-${entry.timestamp ?? ''}`} entry={entry} questionIndex={questionIndexBySeq.get(entry.seq)} />
          ))}
        </div>
      </div>
    </div>
  );
}

function TranscriptEntryCard({ entry, questionIndex }: { entry: AgentTranscriptEntry; questionIndex?: number }) {
  const isTool = entry.type === 'tool_use' || entry.type === 'tool_result';
  const isQuestion = entry.role === 'user' && entry.type === 'text' && questionIndex !== undefined;
  const hue = isQuestion ? questionHue(questionIndex) : null;
  const title = entry.type === 'thinking'
    ? t('observabilityThinking')
    : entry.type === 'tool_use'
      ? t('observabilityToolCall', { name: entry.tool_name || t('observabilityTool') })
      : entry.type === 'tool_result'
        ? t('observabilityToolResult', { name: entry.tool_name || t('observabilityTool') })
        : entry.role === 'user'
          ? t('observabilityUser')
          : t('observabilityAssistant');

  return (
    <article id={`timeline-entry-${entry.seq}`} className={cn('border-2 border-black bg-white shadow-brutal-sm', hue?.bg)}>
      <header className="flex items-center justify-between border-b-2 border-black px-3 py-2">
        <div className="flex min-w-0 items-center gap-2">
          <span className={cn('flex h-6 min-w-6 items-center justify-center border-2 border-black bg-white font-heading text-xs font-black text-black', hue?.swatch)}>{entry.seq}</span>
          <span className="truncate font-heading text-sm font-black">{title}</span>
        </div>
        <span className="whitespace-nowrap font-mono text-xs text-muted-foreground">{formatDate(entry.timestamp)}</span>
      </header>
      {isTool ? (
        <details open className="p-3">
          <summary className="cursor-pointer font-mono text-sm">{entry.tool_id || entry.tool_name || entry.type}</summary>
          {entry.input ? <CodeBlock value={prettyJSON(entry.input)} /> : null}
          {entry.text ? <CodeBlock value={entry.text} /> : null}
        </details>
      ) : (
        <div className="whitespace-pre-wrap p-4 font-sans text-sm leading-6 text-foreground">
          {entry.text || prettyJSON(entry.raw)}
        </div>
      )}
    </article>
  );
}

function CodeBlock({ value }: { value: string }) {
  return (
    <pre className="mt-3 max-h-[420px] overflow-auto bg-black p-3 font-mono text-xs leading-5 text-white">
      {value}
    </pre>
  );
}

function EmptyPanel({ text }: { text: string }) {
  return <div className="flex h-full min-h-40 items-center justify-center font-mono text-sm text-muted-foreground">{text}</div>;
}

function LiveMetric({ label, value }: { label: string; value: number }) {
  return (
    <div className="border-2 border-black bg-brutal-cream px-3 py-2 shadow-brutal-sm">
      <div className="font-heading text-xl font-black">{value}</div>
      <div className="font-mono text-[11px] text-muted-foreground">{label}</div>
    </div>
  );
}

function groupHeaderClass(key: string) {
  return GROUP_HEADER_CLASSES[key] ?? 'bg-brutal-muted-light';
}

function questionHue(index: number) {
  return QUESTION_HUES[index % QUESTION_HUES.length];
}

function StatusBadge({ status }: { status?: AgentRunStatus }) {
  return (
    <span className={cn('badge-brutal px-2 py-1 text-[11px] text-black', statusClass(status))}>
      {statusText(status)}
    </span>
  );
}

function statusClass(status?: AgentRunStatus) {
  switch (status) {
  case 'queued':
  case 'thinking':
  case 'running':
  case 'streaming':
    return 'bg-brutal-info';
  case 'waiting_input':
  case 'waiting_approval':
  case 'failed':
  case 'timeout':
    return 'bg-brutal-warning';
  case 'completed':
    return 'bg-brutal-success';
  default:
    return 'bg-white';
  }
}

function statusText(status?: AgentRunStatus) {
  return agentRunStatusText(status);
}

function findAgent(live: DashboardLive, agentID: string) {
  return live.groups.flatMap((group) => group.items).find((item) => item.agent_id === agentID) ?? null;
}

function agentFromRun(run: AgentRunDetail, live: DashboardLive | null): DashboardLiveAgent {
  const base = live ? findAgent(live, run.agent_id) : null;
  return {
    agent_id: run.agent_id,
    agent_name: base?.agent_name ?? t('agent'),
    avatar_url: base?.avatar_url,
    group: base?.group ?? 'working',
    run_id: run.id,
    session_id: run.session_id,
    status: run.status,
    activity_text: run.activity_text,
    tool_name: run.tool_name,
    tool_input_summary: run.tool_input_summary,
    source: run.source,
    updated_at: run.updated_at,
    active_count: base?.active_count ?? 0,
    attention_count: base?.attention_count ?? 0,
    run_count: base?.run_count ?? 1,
  };
}

function shortID(id?: string) {
  return id ? id.slice(0, 8) : '-';
}

function formatDate(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return date.toLocaleString('zh-CN', { hour12: false });
}

function formatRelative(value?: string) {
  if (!value) return t('observabilityNoRecord');
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return t('observabilityNoRecord');
  const seconds = Math.max(0, Math.floor((Date.now() - date.getTime()) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  return `${Math.floor(hours / 24)}d`;
}

function prettyJSON(value: unknown) {
  if (value == null) return '';
  if (typeof value === 'string') {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }
  return JSON.stringify(value, null, 2);
}

function timelineQuestionOutline(timeline: AgentTimeline) {
  const questions = timeline.entries
    .filter((entry) => entry.role === 'user' && entry.type === 'text' && entry.text)
    .map((entry, index) => ({
      seq: entry.seq,
      timestamp: entry.timestamp,
      title: cleanPromptTitle(entry.text || t('observabilityQuestionTitle', { n: index + 1 })),
    }));
  if (questions.length > 0) return questions;
  return timeline.runs.map((run, index) => ({
    seq: run.entry_start_seq || 1,
    timestamp: run.started_at,
    title: cleanPromptTitle(displayAgentActivity(run.status, run.activity_text, undefined, t('observabilityRoundTitle', { n: index + 1 }))),
  }));
}

function cleanPromptTitle(value: string) {
  return value
    .replace(/^User:\s*/i, '')
    .replace(/^New message received:\s*/i, '')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 80);
}
