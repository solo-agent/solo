'use client';

// ============================================================================
// useAgentIsland (SOLO-island PR1) — derives a per-agent status map from
// WebSocket events for the AgentIsland floating UI.
//
// Inputs (channel-scoped):
//   - agent.activity  — derived by the daemon, carries status + activity_text
//   - agent.done      — terminal signal (PR0); clears the active flag
//   - message.new     — agent's final message arrived; triggers the 5s
//                       "completed" short-flash before idle removal
//
// The hook owns the React state, the WS subscription, and the per-agent
// flash timers. It's intentionally scoped to a single channel (matching
// the product decision: the island is current-channel only, not global).
// ============================================================================

import { useState, useEffect, useRef, useCallback } from 'react';
import { useWebSocket } from '@/lib/ws-context';

export type IslandAgentStatus =
  | 'idle'
  | 'thinking'
  | 'running'
  | 'streaming'
  | 'waiting_approval'
  | 'error';

/** Terminal outcome of a task, mirrored from server-side final_state. */
export type IslandFinalState =
  | 'completed'
  | 'failed'
  | 'aborted'
  | 'timeout'
  | 'cancelled';

export interface IslandAgent {
  agentId: string;
  agentName: string;
  status: IslandAgentStatus;
  activityText: string;
  toolName: string | null;
  toolInputSummary: string | null;
  source: string | null;
  /** Last update timestamp (ms epoch). */
  updatedAt: number;
  /** True while the agent is in the "active" set shown by the island. */
  isActive: boolean;
  /** Set when the agent has finished a turn and is in the 5s "completed" flash. */
  completedAt: number | null;
  /** Mirrors agent.done's final_state. Set on terminal signal. */
  finalState: IslandFinalState | null;
}

const COMPLETED_FLASH_MS = 5000;

export function useAgentIsland(channelId: string | null) {
  const [agents, setAgents] = useState<Map<string, IslandAgent>>(new Map());
  const { onEvent } = useWebSocket();
  const completedTimers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  useEffect(() => {
    if (!channelId) {
      setAgents(new Map());
      return;
    }
    setAgents(new Map());

    const unsub = onEvent((event) => {
      if (!('channel_id' in event) || event.channel_id !== channelId) {
        // DEBUG: log why events are being filtered out for DM island
        if (event.type === 'agent.activity' || event.type === 'agent.thinking' || event.type === 'agent.done' || (event.type === 'message.new' && event.sender_type === 'agent')) {
          console.debug('[useAgentIsland] FILTERED event', {
            eventType: event.type,
            eventChannelId: ('channel_id' in event) ? event.channel_id : 'MISSING',
            hookChannelId: channelId,
            hasChannelId: 'channel_id' in event,
          });
        }
        return;
      }

      // agent.thinking — initial trigger and LLM thinking chunks.
      // Shows the island immediately when the agent receives a message,
      // before the first agent.activity chunk arrives.
      if (event.type === 'agent.thinking' && event.agent_id) {
        console.debug('[useAgentIsland] RECEIVED agent.thinking', {
          agentId: event.agent_id,
          agentName: event.agent_name,
          channelId: event.channel_id,
          thought: event.thought,
        });
        const agentId = event.agent_id;
        setAgents((prev) => {
          const next = new Map(prev);
          const existing = next.get(agentId);
          const timer = completedTimers.current.get(agentId);
          if (timer) {
            clearTimeout(timer);
            completedTimers.current.delete(agentId);
          }
          next.set(agentId, {
            agentId,
            agentName: event.agent_name ?? existing?.agentName ?? '...',
            status: 'thinking',
            activityText: event.thought ?? '思考中…',
            toolName: existing?.toolName ?? null,
            toolInputSummary: existing?.toolInputSummary ?? null,
            source: existing?.source ?? null,
            updatedAt: Date.now(),
            isActive: true,
            completedAt: null,
            finalState: null,
          });
          return next;
        });
        return;
      }

      // agent.activity — primary event. Update or insert the per-agent view.
      if (event.type === 'agent.activity' && event.agent_id) {
        console.debug('[useAgentIsland] RECEIVED agent.activity', {
          agentId: event.agent_id,
          agentName: event.agent_name,
          channelId: event.channel_id,
          status: event.status,
        });
        const agentId = event.agent_id;
        setAgents((prev) => {
          const next = new Map(prev);
          const existing = next.get(agentId);
          // Clear any pending completed-flash timer — new activity means
          // the agent is back in business.
          const timer = completedTimers.current.get(agentId);
          if (timer) {
            clearTimeout(timer);
            completedTimers.current.delete(agentId);
          }
          next.set(agentId, {
            agentId,
            agentName: event.agent_name ?? existing?.agentName ?? '...',
            status: event.status,
            activityText: event.activity_text,
            toolName: event.tool_name ?? null,
            toolInputSummary: event.tool_input_summary ?? null,
            source: event.source ?? null,
            updatedAt: Date.now(),
            isActive: true,
            completedAt: null,
            // New activity overrides any prior terminal outcome.
            finalState: null,
          });
          return next;
        });
        return;
      }

      // message.new from agent — short-flash "completed" state for 5s.
      // We treat this as a separate signal because the user gets immediate
      // visual feedback ("agent finished!") before the daemon's
      // agent.done lands a moment later.
      if (
        event.type === 'message.new' &&
        event.sender_type === 'agent' &&
        event.sender_id
      ) {
        const agentId = event.sender_id;
        setAgents((prev) => {
          const next = new Map(prev);
          const existing = next.get(agentId);
          if (!existing) {
            return prev; // No prior activity — nothing to flash for
          }
          next.set(agentId, {
            ...existing,
            status: 'idle',
            isActive: false,
            activityText: '完成',
            completedAt: Date.now(),
            // message.new alone doesn't tell us completed vs failed — leave
            // finalState untouched (could be set later by agent.done).
          });
          return next;
        });
        // Schedule removal after the flash window. If a new activity event
        // arrives in the meantime, the timer is cleared by the activity
        // handler above and the agent re-enters the active set.
        const existingTimer = completedTimers.current.get(agentId);
        if (existingTimer) clearTimeout(existingTimer);
        const timer = setTimeout(() => {
          setAgents((prev) => {
            const a = prev.get(agentId);
            // Only delete if still in the completed/idle state — if a new
            // activity bumped it back, leave it alone.
            if (!a || a.isActive || a.completedAt == null) {
              return prev;
            }
            const next = new Map(prev);
            next.delete(agentId);
            return next;
          });
          completedTimers.current.delete(agentId);
        }, COMPLETED_FLASH_MS);
        completedTimers.current.set(agentId, timer);
        return;
      }

      // agent.done — terminal signal. Replaces the 3s inactivity heuristic.
      // The agent is no longer active; we keep it visible (idle) until the
      // next message.new (which triggers the 5s flash) or until the user
      // navigates away. We also cancel any pending flash so we don't
      // double-remove the entry.
      if (event.type === 'agent.done' && event.agent_id) {
        const agentId = event.agent_id;
        const existingTimer = completedTimers.current.get(agentId);
        if (existingTimer) {
          clearTimeout(existingTimer);
          completedTimers.current.delete(agentId);
        }
        setAgents((prev) => {
          const next = new Map(prev);
          const a = next.get(agentId);
          if (a) {
            next.set(agentId, {
              ...a,
              status: 'idle',
              isActive: false,
              completedAt: Date.now(),
              // Record the terminal outcome for UI differentiation
              // (failure/aborted should look distinct from success).
              finalState: event.final_state,
            });
          }
          return next;
        });
        // If no message.new follows, the entry would linger forever. Start
        // the same 5s removal timer so a silent-done still gets cleaned up.
        const timer = setTimeout(() => {
          setAgents((prev) => {
            const a = prev.get(agentId);
            if (!a || a.isActive) return prev;
            const next = new Map(prev);
            next.delete(agentId);
            return next;
          });
          completedTimers.current.delete(agentId);
        }, COMPLETED_FLASH_MS);
        completedTimers.current.set(agentId, timer);
      }
    });

    return () => {
      unsub();
      completedTimers.current.forEach((t) => clearTimeout(t));
      completedTimers.current.clear();
    };
  }, [channelId, onEvent]);

  const clearAgent = useCallback((agentId: string) => {
    const timer = completedTimers.current.get(agentId);
    if (timer) {
      clearTimeout(timer);
      completedTimers.current.delete(agentId);
    }
    setAgents((prev) => {
      const next = new Map(prev);
      next.delete(agentId);
      return next;
    });
  }, []);

  const clearAll = useCallback(() => {
    completedTimers.current.forEach((t) => clearTimeout(t));
    completedTimers.current.clear();
    setAgents(new Map());
  }, []);

  // Derive the active subset for the island pill. Idle/completed entries
  // are filtered out — only currently-working agents are surfaced.
  const activeAgents = Array.from(agents.values()).filter((a) => a.isActive);

  return { agents, activeAgents, clearAgent, clearAll };
}
