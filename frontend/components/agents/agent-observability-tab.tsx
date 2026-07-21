'use client';

import { useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { apiClient } from '@/lib/api-client';
import { displayAgentActivity } from '@/lib/agent-activity';
import type { AgentRunStatus } from '@/lib/agent-run-types';
import { useWebSocket } from '@/lib/ws-context';
import { cn } from '@/lib/utils';
import { detailSectionTitleClass } from '@/components/ui/detail-section';

interface AgentRun {
  id: string;
  agent_id: string;
  session_id?: string;
  status: AgentRunStatus;
  activity_text: string;
  tool_name?: string;
  tool_input_summary?: string;
  source?: string;
  transcript_path?: string;
  started_at: string;
  backend_started_at?: string;
  updated_at: string;
}

interface AgentSession {
  id: string;
  provider: string;
  external_session_id?: string;
  transcript_path?: string;
  status: string;
  started_at: string;
  last_active_at: string;
}

interface AgentTaskSummary {
  id: string;
  task_number: number;
  title: string;
  status: string;
  last_run_id: string;
  last_run_status: string;
  last_activity: string;
  last_run_at: string;
  linked_run_count: number;
}

interface AgentTranscriptEntry {
  seq: number;
  timestamp?: string;
  role: string;
  type: string;
  text?: string;
  tool_name?: string;
  input?: unknown;
}

interface AgentRunEvent {
  id: string;
  run_id: string;
  seq: number;
  type: string;
  message: string;
  tool_name?: string;
  payload?: Record<string, unknown>;
  created_at: string;
}

type Scope = 'sessions' | 'tasks' | 'runs';

export function AgentObservabilityTab({ agentId, initialRunId }: { agentId: string; initialRunId?: string | null }) {
  const { onEvent } = useWebSocket();
  const [scope, setScope] = useState<Scope>('sessions');
  const [sessions, setSessions] = useState<AgentSession[]>([]);
  const [runs, setRuns] = useState<AgentRun[]>([]);
  const [tasks, setTasks] = useState<AgentTaskSummary[]>([]);
  const [taskRuns, setTaskRuns] = useState<AgentRun[]>([]);
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null);
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);
  const [transcript, setTranscript] = useState<AgentTranscriptEntry[]>([]);
  const [events, setEvents] = useState<AgentRunEvent[]>([]);
  const [transcriptRefreshTick, setTranscriptRefreshTick] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setSelectedSessionId(null);
    setSelectedTaskId(null);
    setSelectedRunId(null);
    setTranscript([]);
    setEvents([]);
    Promise.all([
      apiClient.get<AgentSession[]>(`/api/v1/agents/${agentId}/sessions`).catch(() => []),
      apiClient.get<AgentRun[]>(`/api/v1/agents/${agentId}/runs`).catch(() => []),
      apiClient.get<AgentTaskSummary[]>(`/api/v1/agents/${agentId}/tasks`).catch(() => []),
    ]).then(([nextSessions, nextRuns, nextTasks]) => {
      if (cancelled) return;
      const safeSessions = Array.isArray(nextSessions) ? nextSessions : [];
      const safeRuns = Array.isArray(nextRuns) ? nextRuns : [];
      const safeTasks = Array.isArray(nextTasks) ? nextTasks : [];
      const preferredRun = initialRunId ? safeRuns.find((run) => run.id === initialRunId) : undefined;
      setSessions(safeSessions);
      setRuns(safeRuns);
      setTasks(safeTasks);
      setScope(preferredRun ? 'runs' : 'sessions');
      setSelectedSessionId(preferredRun?.session_id ?? safeSessions[0]?.id ?? null);
      setSelectedRunId(preferredRun?.id ?? safeRuns[0]?.id ?? null);
    });
    return () => {
      cancelled = true;
    };
  }, [agentId, initialRunId]);

  useEffect(() => onEvent((event) => {
    if (
      event.type === 'agent.run.started' ||
      event.type === 'agent.run.updated' ||
      event.type === 'agent.run.finished'
    ) {
      if (event.agent_id !== agentId) return;
      const nextRun: AgentRun = {
        id: event.run_id,
        agent_id: event.agent_id,
        session_id: event.session_id,
        status: event.status,
        activity_text: event.activity_text ?? '',
        tool_name: event.tool_name,
        tool_input_summary: event.tool_input_summary,
        source: event.source,
        transcript_path: event.transcript_path,
        started_at: event.timestamp ?? new Date().toISOString(),
        updated_at: event.timestamp ?? new Date().toISOString(),
      };
      setRuns((prev) => upsertRun(prev, nextRun));
      if (!selectedRunId) setSelectedRunId(event.run_id);
      if (event.run_id === selectedRunId) setTranscriptRefreshTick((tick) => tick + 1);
      if (event.type === 'agent.run.started' || event.type === 'agent.run.finished') {
        apiClient.get<AgentSession[]>(`/api/v1/agents/${agentId}/sessions`)
          .then((items) => setSessions(Array.isArray(items) ? items : []))
          .catch(() => {});
        apiClient.get<AgentTaskSummary[]>(`/api/v1/agents/${agentId}/tasks`)
          .then((items) => setTasks(Array.isArray(items) ? items : []))
          .catch(() => {});
      }
      return;
    }
    if (event.type === 'agent.run.event' && event.run_id === selectedRunId) {
      const nextEvent: AgentRunEvent = {
        id: event.id ?? `${event.run_id}-${event.seq}`,
        run_id: event.run_id,
        seq: event.seq,
        type: event.event_type,
        message: event.message ?? '',
        tool_name: event.tool_name,
        payload: event.payload,
        created_at: event.timestamp,
      };
      setEvents((prev) => upsertEvent(prev, nextEvent));
      setTranscriptRefreshTick((tick) => tick + 1);
    }
  }), [agentId, onEvent, selectedRunId]);

  useEffect(() => {
    if (!selectedTaskId) {
      setTaskRuns([]);
      return;
    }
    let cancelled = false;
    apiClient.get<AgentRun[]>(`/api/v1/tasks/${selectedTaskId}/runs`)
      .then((items) => {
        if (cancelled) return;
        setTaskRuns(items);
        setSelectedRunId(items[0]?.id ?? null);
      })
      .catch(() => {
        if (!cancelled) setTaskRuns([]);
      });
    return () => {
      cancelled = true;
    };
  }, [selectedTaskId]);

  useEffect(() => {
    if (!selectedRunId) {
      setTranscript([]);
      setEvents([]);
      return;
    }
    let cancelled = false;
    apiClient.get<AgentTranscriptEntry[]>(`/api/v1/agent-runs/${selectedRunId}/transcript`)
      .then((items) => {
        if (!cancelled) setTranscript(Array.isArray(items) ? items : []);
      })
      .catch(() => {
        if (!cancelled) setTranscript([]);
      });
    apiClient.get<AgentRunEvent[]>(`/api/v1/agent-runs/${selectedRunId}/events`)
      .then((items) => {
        if (!cancelled) setEvents(Array.isArray(items) ? items : []);
      })
      .catch(() => {
        if (!cancelled) setEvents([]);
      });
    return () => {
      cancelled = true;
    };
  }, [selectedRunId, transcriptRefreshTick]);

  const visibleRuns = useMemo(() => {
    if (scope === 'tasks' && selectedTaskId) return taskRuns;
    if (scope === 'sessions' && selectedSessionId) {
      return runs.filter((run) => run.session_id === selectedSessionId);
    }
    return runs;
  }, [runs, scope, selectedSessionId, selectedTaskId, taskRuns]);
  const selectedRun = useMemo(
    () => [...runs, ...taskRuns].find((run) => run.id === selectedRunId),
    [runs, selectedRunId, taskRuns],
  );
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <span className={detailSectionTitleClass()}>Observability</span>
        <div className="flex border-2 border-black font-heading text-xs font-bold">
          {(['sessions', 'tasks', 'runs'] as const).map((item) => (
            <button
              key={item}
              type="button"
              onClick={() => setScope(item)}
              className={cn('border-r-2 border-black px-2 py-1 last:border-r-0', scope === item ? 'bg-brutal-primary' : 'bg-white')}
            >
              {item}
            </button>
          ))}
        </div>
      </div>

      <div className="grid gap-3 lg:grid-cols-[280px_260px_1fr]">
        <ScopeList
          scope={scope}
          sessions={sessions}
          tasks={tasks}
          runs={runs}
          selectedSessionId={selectedSessionId}
          selectedTaskId={selectedTaskId}
          selectedRunId={selectedRunId}
          onSelectSession={(id) => {
            setSelectedSessionId(id);
            setSelectedTaskId(null);
            setSelectedRunId(runs.find((run) => run.session_id === id)?.id ?? null);
          }}
          onSelectTask={(id, runId) => {
            setSelectedTaskId(id);
            setSelectedSessionId(null);
            setSelectedRunId(runId);
          }}
          onSelectRun={setSelectedRunId}
        />
        <RunList runs={visibleRuns} selectedRunId={selectedRunId} onSelectRun={setSelectedRunId} />
        <TranscriptPanel entries={transcript} events={events} selectedRunId={selectedRunId} transcriptPath={selectedRun?.transcript_path} />
      </div>
    </section>
  );
}

function ScopeList(props: {
  scope: Scope;
  sessions: AgentSession[];
  tasks: AgentTaskSummary[];
  runs: AgentRun[];
  selectedSessionId: string | null;
  selectedTaskId: string | null;
  selectedRunId: string | null;
  onSelectSession: (id: string) => void;
  onSelectTask: (id: string, runId: string) => void;
  onSelectRun: (id: string) => void;
}) {
  if (props.scope === 'sessions') {
    return (
      <Panel title="Sessions">
        {props.sessions.map((session) => (
          <Row key={session.id} active={props.selectedSessionId === session.id} onClick={() => props.onSelectSession(session.id)}>
            <strong>{session.provider}</strong>
            <span>{session.external_session_id ? session.external_session_id.slice(0, 8) : session.id.slice(0, 8)}</span>
            <small>{formatTime(session.last_active_at)}</small>
          </Row>
        ))}
      </Panel>
    );
  }
  if (props.scope === 'tasks') {
    return (
      <Panel title="Tasks">
        {props.tasks.map((task) => (
          <Row key={task.id} active={props.selectedTaskId === task.id} onClick={() => props.onSelectTask(task.id, task.last_run_id)}>
            <strong>#{task.task_number} {task.title}</strong>
            <span>{task.status} · {task.linked_run_count} runs</span>
            <small>{formatTime(task.last_run_at)}</small>
          </Row>
        ))}
      </Panel>
    );
  }
  return (
    <Panel title="Runs">
      {props.runs.map((run) => (
        <Row key={run.id} active={props.selectedRunId === run.id} onClick={() => props.onSelectRun(run.id)}>
          <strong>{run.status}</strong>
          <span>{displayAgentActivity(run.status, run.activity_text, run.tool_input_summary, run.id.slice(0, 8))}</span>
          <small>{formatTime(run.updated_at)}</small>
        </Row>
      ))}
    </Panel>
  );
}

function RunList({ runs, selectedRunId, onSelectRun }: { runs: AgentRun[]; selectedRunId: string | null; onSelectRun: (id: string) => void }) {
  return (
    <Panel title="Related Runs">
      {runs.map((run) => (
        <Row key={run.id} active={selectedRunId === run.id} onClick={() => onSelectRun(run.id)}>
          <strong>{run.status}</strong>
          <span>{displayAgentActivity(run.status, run.activity_text, run.tool_input_summary, run.id.slice(0, 8))}</span>
          <small>{formatTime(run.updated_at)}</small>
        </Row>
      ))}
    </Panel>
  );
}

function TranscriptPanel({ entries, events, selectedRunId, transcriptPath }: { entries: AgentTranscriptEntry[]; events: AgentRunEvent[]; selectedRunId: string | null; transcriptPath?: string }) {
  const fallback = <EventsTimeline events={events} />;
  return (
    <Panel title="JSONL Transcript">
      {!selectedRunId ? (
        <div className="p-3 text-sm text-muted-foreground">选择一个 run</div>
      ) : entries.length > 0 ? (
        <div className="space-y-2 p-2">
          {transcriptPath && (
            <div className="truncate border-2 border-black bg-white px-2 py-1 font-mono text-[11px] text-muted-foreground">
              {transcriptPath}
            </div>
          )}
          {entries.map((entry) => (
            <details key={`${entry.seq}-${entry.type}`} className="border-2 border-black bg-white" open={entry.type !== 'tool_use' && entry.type !== 'tool_result'}>
              <summary className="cursor-pointer px-2 py-1 font-heading text-xs font-bold">
                {entryLabel(entry)} <span className="font-mono font-normal text-muted-foreground">{formatTime(entry.timestamp)}</span>
              </summary>
              <div className="border-t-2 border-black p-2 text-sm">
                {entry.input ? (
                  <pre className="max-h-56 overflow-auto whitespace-pre-wrap break-words bg-black p-2 font-mono text-xs text-white">
                    {JSON.stringify(entry.input, null, 2)}
                  </pre>
                ) : (
                  <div className="whitespace-pre-wrap break-words">{entry.text || entry.type}</div>
                )}
              </div>
            </details>
          ))}
        </div>
      ) : !transcriptPath ? (
        <>
          <div className="border-b-2 border-black p-3 text-sm text-muted-foreground">当前 run 还没有关联外部 jsonl 路径，先展示轻量 events fallback。</div>
          {fallback}
        </>
      ) : (
        <>
          <div className="border-b-2 border-black p-3 text-sm text-muted-foreground">已关联 jsonl，但暂无可解析内容：{transcriptPath}</div>
          {fallback}
        </>
      )}
    </Panel>
  );
}

function EventsTimeline({ events }: { events: AgentRunEvent[] }) {
  if (events.length === 0) {
    return <div className="p-3 text-sm text-muted-foreground">暂无 events fallback</div>;
  }
  const toolNameByCallId = new Map<string, string>();
  for (const event of events) {
    const callID = typeof event.payload?.call_id === 'string' ? event.payload.call_id : '';
    if (callID && event.tool_name) {
      toolNameByCallId.set(callID, event.tool_name);
    }
  }
  return (
    <div className="space-y-2 p-2">
      {events.map((event) => {
        const message = readableEventText(event.message || payloadText(event.payload) || '暂无摘要');
        const meta = eventMeta(event.payload);
        const callID = typeof event.payload?.call_id === 'string' ? event.payload.call_id : '';
        const toolName = event.tool_name || toolNameByCallId.get(callID) || '';
        return (
          <details key={event.id} className="border-2 border-black bg-white">
            <summary className="flex cursor-pointer items-center gap-2 px-2 py-1 font-heading text-xs font-bold">
              <span>#{event.seq}</span>
              <span>{eventLabel(event.type, toolName)}</span>
              <span className="ml-auto font-mono font-normal text-muted-foreground">{formatTime(event.created_at)}</span>
            </summary>
            <div className="space-y-2 border-t-2 border-black p-2">
              {meta.length > 0 && (
                <div className="flex flex-wrap gap-1">
                  {meta.map(([key, value]) => (
                    <span key={key} className="border-2 border-black bg-brutal-cream px-1.5 py-0.5 font-mono text-[11px]">
                      {key}: {value}
                    </span>
                  ))}
                </div>
              )}
              <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words bg-brutal-cream p-2 font-mono text-xs leading-5">
                {message}
              </pre>
            </div>
          </details>
        );
      })}
    </div>
  );
}

function readableEventText(value: string) {
  const trimmed = value.trim();
  if (trimmed.startsWith('"') && trimmed.endsWith('"')) {
    try {
      const parsed = JSON.parse(trimmed);
      if (typeof parsed === 'string') return parsed;
    } catch {
      // fall through
    }
  }
  return value.replaceAll('\\n', '\n').replaceAll('\\"', '"');
}

function payloadText(payload?: Record<string, unknown>) {
  const value = payload?.output ?? payload?.input;
  if (typeof value === 'string') return value;
  if (value == null) return '';
  return JSON.stringify(value, null, 2);
}

function eventMeta(payload?: Record<string, unknown>) {
  if (!payload) return [];
  return Object.entries(payload)
    .filter(([key]) => key !== 'output' && key !== 'input' && key !== 'call_id')
    .map(([key, value]) => [key, typeof value === 'string' ? readableEventText(value) : String(value)] as const);
}

function eventLabel(type: string, toolName: string) {
  if (type === 'tool_started') return toolName ? `${toolName} 调用` : '工具调用';
  if (type === 'tool_finished') return toolName ? `${toolName} 结果` : '工具结果';
  if (type === 'thinking') return '思考过程';
  if (type === 'assistant_message') return 'Assistant';
  if (type === 'user_message_received') return '用户消息';
  if (type === 'task_linked') return '关联 task';
  if (type === 'run_started') return '创建 run';
  if (type === 'done') return '完成';
  if (type === 'error') return '错误';
  return type;
}

function upsertRun(runs: AgentRun[], nextRun: AgentRun) {
  const existing = runs.find((run) => run.id === nextRun.id);
  const merged = existing ? { ...existing, ...nextRun, started_at: existing.started_at } : nextRun;
  const rest = runs.filter((run) => run.id !== nextRun.id);
  return [merged, ...rest].sort((a, b) => Date.parse(b.updated_at) - Date.parse(a.updated_at));
}

function upsertEvent(events: AgentRunEvent[], nextEvent: AgentRunEvent) {
  const rest = events.filter((event) => event.id !== nextEvent.id);
  return [...rest, nextEvent].sort((a, b) => a.seq - b.seq);
}

function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="min-h-[260px] border-2 border-black bg-brutal-cream">
      <div className="border-b-2 border-black bg-white px-2 py-1 font-heading text-xs font-bold">{title}</div>
      <div className="max-h-[520px] overflow-auto">{children}</div>
    </div>
  );
}

function Row({ active, onClick, children }: { active: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn('block w-full border-b-2 border-black px-2 py-2 text-left last:border-b-0 hover:bg-white', active && 'bg-white')}
    >
      <div className="flex min-w-0 flex-col gap-0.5 font-mono text-xs">{children}</div>
    </button>
  );
}

function entryLabel(entry: AgentTranscriptEntry) {
  if (entry.type === 'thinking') return '思考过程';
  if (entry.type === 'tool_use') return `${entry.tool_name || 'Tool'} 调用`;
  if (entry.type === 'tool_result') return `${entry.tool_name || 'Tool'} 结果`;
  return entry.role === 'user' ? '用户消息' : 'Assistant';
}

function formatTime(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return date.toLocaleString('zh-CN', { hour12: false });
}
