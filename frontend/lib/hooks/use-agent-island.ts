'use client';

import { useState, useEffect, useRef, useCallback } from 'react';
import { apiClient } from '@/lib/api-client';
import { useWebSocket } from '@/lib/ws-context';
import { t } from '@/lib/i18n';

export type AgentRunStatus =
  | 'queued'
  | 'thinking'
  | 'running'
  | 'streaming'
  | 'waiting_input'
  | 'waiting_approval'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'timeout';

export interface IslandAgent {
  runId: string;
  sessionId: string | null;
  agentId: string;
  agentName: string;
  taskId: string | null;
  channelId: string | null;
  threadId: string | null;
  status: AgentRunStatus;
  activityText: string;
  toolName: string | null;
  toolInputSummary: string | null;
  source: string | null;
  updatedAt: number;
  startedAt: number | null;
  completedAt: number | null;
}

interface AgentRunResponse {
  id: string;
  session_id?: string;
  agent_id: string;
  agent_name?: string;
  channel_id?: string;
  thread_id?: string;
  status: AgentRunStatus;
  activity_text?: string;
  tool_name?: string;
  tool_input_summary?: string;
  source?: string;
  started_at?: string;
  updated_at?: string;
}

const FLASH_STATUSES = new Set<AgentRunStatus>(['completed', 'failed', 'cancelled', 'timeout']);
const COMPLETED_FLASH_MS = 5000;

function timestampMs(value?: string): number {
  if (!value) return Date.now();
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? Date.now() : parsed;
}

function fromRunResponse(run: AgentRunResponse): IslandAgent {
  return {
    runId: run.id,
    sessionId: run.session_id ?? null,
    agentId: run.agent_id,
    agentName: run.agent_name ?? t('agent'),
    taskId: null,
    channelId: run.channel_id ?? null,
    threadId: run.thread_id ?? null,
    status: run.status,
    activityText: run.activity_text ?? '',
    toolName: run.tool_name ?? null,
    toolInputSummary: run.tool_input_summary ?? null,
    source: run.source ?? null,
    updatedAt: timestampMs(run.updated_at),
    startedAt: run.started_at ? timestampMs(run.started_at) : null,
    completedAt: FLASH_STATUSES.has(run.status) ? Date.now() : null,
  };
}

export function useAgentIsland() {
  const [agents, setAgents] = useState<Map<string, IslandAgent>>(new Map());
  const { onEvent } = useWebSocket();
  const completedTimers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  const scheduleRemoval = useCallback((runId: string) => {
    const existing = completedTimers.current.get(runId);
    if (existing) clearTimeout(existing);
    const timer = setTimeout(() => {
      setAgents((prev) => {
        const current = prev.get(runId);
        if (!current || !FLASH_STATUSES.has(current.status)) return prev;
        const next = new Map(prev);
        next.delete(runId);
        return next;
      });
      completedTimers.current.delete(runId);
    }, COMPLETED_FLASH_MS);
    completedTimers.current.set(runId, timer);
  }, []);

  const upsertRun = useCallback((event: {
    run_id: string;
    session_id?: string;
    agent_id: string;
    agent_name?: string;
    task_id?: string;
    channel_id?: string;
    thread_id?: string;
    status: AgentRunStatus;
    activity_text?: string;
    tool_name?: string;
    tool_input_summary?: string;
    source?: string;
    timestamp?: string;
  }) => {
    setAgents((prev) => {
      const next = new Map(prev);
      const existing = next.get(event.run_id);
      const updated: IslandAgent = {
        runId: event.run_id,
        sessionId: event.session_id ?? existing?.sessionId ?? null,
        agentId: event.agent_id,
        agentName: event.agent_name ?? existing?.agentName ?? t('agent'),
        taskId: event.task_id ?? existing?.taskId ?? null,
        channelId: event.channel_id ?? existing?.channelId ?? null,
        threadId: event.thread_id ?? existing?.threadId ?? null,
        status: event.status,
        activityText: event.activity_text ?? existing?.activityText ?? '',
        toolName: event.tool_name ?? existing?.toolName ?? null,
        toolInputSummary: event.tool_input_summary ?? existing?.toolInputSummary ?? null,
        source: event.source ?? existing?.source ?? null,
        updatedAt: timestampMs(event.timestamp),
        startedAt: existing?.startedAt ?? null,
        completedAt: FLASH_STATUSES.has(event.status) ? Date.now() : null,
      };
      next.set(event.run_id, updated);
      return next;
    });

    if (FLASH_STATUSES.has(event.status)) {
      scheduleRemoval(event.run_id);
    } else {
      const timer = completedTimers.current.get(event.run_id);
      if (timer) {
        clearTimeout(timer);
        completedTimers.current.delete(event.run_id);
      }
    }
  }, [scheduleRemoval]);

  useEffect(() => {
    let cancelled = false;
    apiClient.get<AgentRunResponse[]>('/api/v1/agent-runs/active')
      .then((runs) => {
        if (cancelled) return;
        setAgents(new Map(runs.map((run) => [run.id, fromRunResponse(run)])));
      })
      .catch(() => {
        if (!cancelled) setAgents(new Map());
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const unsub = onEvent((event) => {
      if (
        event.type === 'agent.run.started' ||
        event.type === 'agent.run.updated' ||
        event.type === 'agent.run.finished'
      ) {
        upsertRun(event);
      }
    });

    return () => {
      unsub();
      completedTimers.current.forEach((timer) => clearTimeout(timer));
      completedTimers.current.clear();
    };
  }, [onEvent, upsertRun]);

  const clearAgent = useCallback((runId: string) => {
    const timer = completedTimers.current.get(runId);
    if (timer) {
      clearTimeout(timer);
      completedTimers.current.delete(runId);
    }
    setAgents((prev) => {
      const next = new Map(prev);
      next.delete(runId);
      return next;
    });
  }, []);

  const clearAll = useCallback(() => {
    completedTimers.current.forEach((timer) => clearTimeout(timer));
    completedTimers.current.clear();
    setAgents(new Map());
  }, []);

  const priority: Record<AgentRunStatus, number> = {
    running: 0,
    streaming: 1,
    thinking: 2,
    queued: 3,
    waiting_input: 4,
    waiting_approval: 5,
    failed: 6,
    timeout: 7,
    completed: 8,
    cancelled: 9,
  };
  const sortedAgents = Array.from(agents.values()).sort((a, b) => {
    const byPriority = priority[a.status] - priority[b.status];
    return byPriority !== 0 ? byPriority : b.updatedAt - a.updatedAt;
  });
  const groupedAgents = new Map<string, IslandAgent>();
  for (const agent of sortedAgents) {
    if (!groupedAgents.has(agent.agentId)) {
      groupedAgents.set(agent.agentId, agent);
    }
  }
  const activeAgents = Array.from(groupedAgents.values());

  return { agents, activeAgents, clearAgent, clearAll };
}
