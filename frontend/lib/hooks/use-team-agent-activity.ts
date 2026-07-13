'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { apiClient } from '@/lib/api-client';
import { displayAgentActivity } from '@/lib/agent-activity';
import { useWebSocket } from '@/lib/ws-context';
import type { WSServerEvent } from '@/lib/ws-types';
import type { AgentRunStatus } from '@/lib/agent-run-types';

const FINISHED_STATUSES = new Set<AgentRunStatus>(['completed', 'failed', 'cancelled', 'timeout']);
const FINISHED_VISIBLE_MS = 3000;
const MAX_MESSAGE_CACHE = 120;

interface AgentRunResponse {
  id: string;
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
  started_at?: string;
  updated_at?: string;
}

export interface LiveAgentState {
  agentId: string;
  agentName: string;
  currentRun?: {
    runId: string;
    sessionId?: string;
    taskId?: string;
    channelId?: string;
    threadId?: string;
    status: AgentRunStatus;
    activityText?: string;
    toolName?: string;
    toolInputSummary?: string;
    source?: string;
    startedAt: number;
    updatedAt: number;
  };
  currentTool?: { name: string; args?: string; startedAt: number };
  currentActivity?: { text: string; startedAt: number };
  currentHumanMsg?: { messageId: string; text: string; authorName: string; channelId: string; arrivedAt: number };
}

type RunEvent = Extract<WSServerEvent, { type: 'agent.run.started' | 'agent.run.updated' | 'agent.run.finished' }>;
type AgentRunEvent = Extract<WSServerEvent, { type: 'agent.run.event' }>;
type MessageEvent = Extract<WSServerEvent, { type: 'message.new' }>;

function timestampMs(value?: string): number {
  if (!value) return Date.now();
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? Date.now() : parsed;
}

function trimText(value: string | undefined, fallback = ''): string {
  return value?.trim() || fallback;
}

function payloadText(payload: Record<string, unknown> | undefined, key: string): string {
  const value = payload?.[key];
  return typeof value === 'string' ? value : '';
}

function runToLive(run: AgentRunResponse): LiveAgentState {
  const updatedAt = timestampMs(run.updated_at);
  const activity = displayAgentActivity(run.status, run.activity_text, run.tool_input_summary);
  return {
    agentId: run.agent_id,
    agentName: run.agent_name || 'Agent',
    currentRun: {
      runId: run.id,
      sessionId: run.session_id,
      taskId: run.task_id,
      channelId: run.channel_id,
      threadId: run.thread_id,
      status: run.status,
      activityText: run.activity_text,
      toolName: run.tool_name,
      toolInputSummary: run.tool_input_summary,
      source: run.source,
      startedAt: timestampMs(run.started_at),
      updatedAt,
    },
    currentTool: run.tool_name ? { name: run.tool_name, args: run.tool_input_summary, startedAt: updatedAt } : undefined,
    currentActivity: activity ? { text: activity, startedAt: updatedAt } : undefined,
  };
}

export function useTeamAgentActivity() {
  const [liveByAgent, setLiveByAgent] = useState<Map<string, LiveAgentState>>(new Map());
  const { onEvent, isConnected } = useWebSocket();
  const mountedRef = useRef(false);
  const hasConnectedRef = useRef(false);
  const latestRunByAgentRef = useRef<Map<string, string>>(new Map());
  const messageCacheRef = useRef<Map<string, MessageEvent>>(new Map());
  const finishedTimersRef = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  const rememberMessage = useCallback((event: MessageEvent) => {
    const cache = messageCacheRef.current;
    cache.set(event.id, event);
    if (cache.size > MAX_MESSAGE_CACHE) {
      const oldest = cache.keys().next().value;
      if (oldest) cache.delete(oldest);
    }
  }, []);

  const clearFinishedTimer = useCallback((agentId: string) => {
    const timer = finishedTimersRef.current.get(agentId);
    if (timer) clearTimeout(timer);
    finishedTimersRef.current.delete(agentId);
  }, []);

  const scheduleFinishedClear = useCallback((agentId: string) => {
    clearFinishedTimer(agentId);
    const timer = setTimeout(() => {
      setLiveByAgent((prev) => {
        const current = prev.get(agentId);
        if (!current?.currentRun || !FINISHED_STATUSES.has(current.currentRun.status)) return prev;
        const next = new Map(prev);
        next.delete(agentId);
        return next;
      });
      finishedTimersRef.current.delete(agentId);
    }, FINISHED_VISIBLE_MS);
    finishedTimersRef.current.set(agentId, timer);
  }, [clearFinishedTimer]);

  const loadActiveRuns = useCallback(() => {
    return apiClient.get<AgentRunResponse[]>('/api/v1/agent-runs/active')
      .then((runs) => {
        if (!mountedRef.current) return;
        const sorted = [...runs].sort((a, b) => timestampMs(b.updated_at) - timestampMs(a.updated_at));
        const next = new Map<string, LiveAgentState>();
        for (const run of sorted) {
          if (!next.has(run.agent_id)) {
            latestRunByAgentRef.current.set(run.agent_id, run.id);
            next.set(run.agent_id, runToLive(run));
          }
        }
        setLiveByAgent(next);
      })
      .catch(() => {
        if (!mountedRef.current) return;
        setLiveByAgent(new Map());
      });
  }, []);

  const upsertRun = useCallback((event: RunEvent) => {
    const latestRunId = latestRunByAgentRef.current.get(event.agent_id);
    if (event.type !== 'agent.run.started' && latestRunId && latestRunId !== event.run_id) return;
    if (event.type === 'agent.run.finished' && !latestRunId) return;

    const updatedAt = timestampMs(event.timestamp);
    latestRunByAgentRef.current.set(event.agent_id, event.run_id);
    setLiveByAgent((prev) => {
      const existing = prev.get(event.agent_id);
      const status = event.status;
      const nextToolName = event.tool_name !== undefined ? event.tool_name : existing?.currentRun?.toolName;
      const nextToolInputSummary = event.tool_input_summary !== undefined ? event.tool_input_summary : existing?.currentRun?.toolInputSummary;
      const activity = displayAgentActivity(status, event.activity_text ?? existing?.currentRun?.activityText, nextToolInputSummary);
      const currentTool = FINISHED_STATUSES.has(status)
        ? undefined
        : nextToolName
          ? { name: nextToolName, args: nextToolInputSummary, startedAt: updatedAt }
          : event.tool_name !== undefined
            ? undefined
            : existing?.currentTool;
      const next = new Map(prev);
      next.set(event.agent_id, {
        agentId: event.agent_id,
        agentName: event.agent_name ?? existing?.agentName ?? 'Agent',
        currentRun: {
          runId: event.run_id,
          sessionId: event.session_id ?? existing?.currentRun?.sessionId,
          taskId: event.task_id ?? existing?.currentRun?.taskId,
          channelId: event.channel_id ?? existing?.currentRun?.channelId,
          threadId: event.thread_id ?? existing?.currentRun?.threadId,
          status,
          activityText: event.activity_text ?? existing?.currentRun?.activityText,
          toolName: nextToolName,
          toolInputSummary: nextToolInputSummary,
          source: event.source ?? existing?.currentRun?.source,
          startedAt: existing?.currentRun?.startedAt ?? updatedAt,
          updatedAt,
        },
        currentTool,
        currentActivity: activity ? { text: activity, startedAt: updatedAt } : existing?.currentActivity,
        currentHumanMsg: existing?.currentHumanMsg,
      });
      return next;
    });

    if (FINISHED_STATUSES.has(event.status)) scheduleFinishedClear(event.agent_id);
    else clearFinishedTimer(event.agent_id);
  }, [clearFinishedTimer, scheduleFinishedClear]);

  const applyMessageToHumanCard = useCallback((message: MessageEvent) => {
    setLiveByAgent((prev) => {
      let changed = false;
      const next = new Map(prev);
      for (const [agentId, live] of prev) {
        if (live.currentHumanMsg?.messageId !== message.id) continue;
        next.set(agentId, {
          ...live,
          currentHumanMsg: {
            messageId: message.id,
            text: trimText(message.content, live.currentHumanMsg.text),
            authorName: trimText(message.sender_name, live.currentHumanMsg.authorName),
            channelId: message.channel_id,
            arrivedAt: timestampMs(message.created_at),
          },
        });
        changed = true;
      }
      return changed ? next : prev;
    });
  }, []);

  const handleRunEvent = useCallback((event: AgentRunEvent) => {
    const latestRunId = latestRunByAgentRef.current.get(event.agent_id);
    if (latestRunId && latestRunId !== event.run_id) return;

    const time = timestampMs(event.timestamp);
    latestRunByAgentRef.current.set(event.agent_id, event.run_id);
    setLiveByAgent((prev) => {
      const existing = prev.get(event.agent_id);
      const next = new Map(prev);
      const live: LiveAgentState = existing ?? {
        agentId: event.agent_id,
        agentName: event.agent_name ?? 'Agent',
        currentRun: {
          runId: event.run_id,
          sessionId: event.session_id,
          channelId: event.channel_id,
          threadId: event.thread_id,
          status: 'running',
          startedAt: time,
          updatedAt: time,
        },
      };

      if (event.event_type === 'tool_started') {
        next.set(event.agent_id, {
          ...live,
          currentTool: {
            name: event.tool_name || event.message || 'Tool',
            args: payloadText(event.payload, 'input'),
            startedAt: time,
          },
        });
        return next;
      }

      if (event.event_type === 'tool_finished') {
        next.set(event.agent_id, {
          ...live,
          currentTool: undefined,
          currentActivity: { text: trimText(event.message, event.tool_name || 'Tool finished'), startedAt: time },
        });
        return next;
      }

      if (event.event_type === 'user_message_received') {
        const messageId = payloadText(event.payload, 'message_id');
        if (!messageId) return prev;
        const msg = messageCacheRef.current.get(messageId);
        next.set(event.agent_id, {
          ...live,
          currentHumanMsg: {
            messageId,
            text: trimText(msg?.content, event.message || 'New message'),
            authorName: trimText(msg?.sender_name, 'User'),
            channelId: msg?.channel_id || payloadText(event.payload, 'channel_id') || event.channel_id || '',
            arrivedAt: timestampMs(msg?.created_at ?? event.timestamp),
          },
        });
        return next;
      }

      if (event.event_type === 'activity' || event.event_type === 'assistant_message') {
        const text = trimText(event.message);
        if (!text) return prev;
        next.set(event.agent_id, {
          ...live,
          currentActivity: { text, startedAt: time },
        });
        return next;
      }

      return prev;
    });
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    void loadActiveRuns();
    const handleFocus = () => void loadActiveRuns();
    window.addEventListener('focus', handleFocus);
    return () => {
      mountedRef.current = false;
      window.removeEventListener('focus', handleFocus);
      finishedTimersRef.current.forEach((timer) => clearTimeout(timer));
      finishedTimersRef.current.clear();
    };
  }, [loadActiveRuns]);

  useEffect(() => {
    if (!isConnected) return;
    if (hasConnectedRef.current) {
      void loadActiveRuns();
      return;
    }
    hasConnectedRef.current = true;
  }, [isConnected, loadActiveRuns]);

  useEffect(() => onEvent((event) => {
    if (event.type === 'message.new') {
      rememberMessage(event);
      applyMessageToHumanCard(event);
      return;
    }
    if (event.type === 'agent.run.started' || event.type === 'agent.run.updated' || event.type === 'agent.run.finished') {
      upsertRun(event);
      return;
    }
    if (event.type === 'agent.run.event') {
      handleRunEvent(event);
      return;
    }
    if (event.type === 'agent_deleted') {
      clearFinishedTimer(event.agent_id);
      setLiveByAgent((prev) => {
        const next = new Map(prev);
        next.delete(event.agent_id);
        return next;
      });
      latestRunByAgentRef.current.delete(event.agent_id);
    }
  }), [applyMessageToHumanCard, clearFinishedTimer, handleRunEvent, onEvent, rememberMessage, upsertRun]);

  const getLatestRunId = useCallback((agentId: string) => latestRunByAgentRef.current.get(agentId), []);

  return { liveByAgent, getLatestRunId };
}
