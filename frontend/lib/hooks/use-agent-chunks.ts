'use client';

import { useState, useEffect, useCallback } from 'react';
import { useWebSocket } from '@/lib/ws-context';

export interface AgentChunk {
  agentId: string;
  agentName: string;
  chunkType: 'thinking' | 'tool_use' | 'tool_result' | 'text' | 'error' | 'context';
  content: string;
  tool?: { name: string; input?: string; output?: string; call_id?: string };
  timestamp: string;
}

const MAX_CHUNKS_PER_AGENT = 200;

/** Merge consecutive same-type chunks (thinking, text) to avoid fragment spam. */
function mergeChunks(existing: AgentChunk[], incoming: AgentChunk): AgentChunk[] {
  const last = existing[existing.length - 1];
  if (
    last &&
    (last.chunkType === 'thinking' || last.chunkType === 'text') &&
    last.chunkType === incoming.chunkType
  ) {
    const merged = { ...last, content: last.content + incoming.content, timestamp: incoming.timestamp };
    const trimmed = existing.length >= MAX_CHUNKS_PER_AGENT ? existing.slice(1) : existing.slice(0, -1);
    return [...trimmed, merged];
  }
  const trimmed = existing.length >= MAX_CHUNKS_PER_AGENT ? existing.slice(1) : existing;
  return [...trimmed, incoming];
}

export function useAgentChunks(channelId: string | null) {
  const [agentTracks, setAgentTracks] = useState<Map<string, AgentChunk[]>>(new Map());
  const [activeAgentIds, setActiveAgentIds] = useState<string[]>([]);
  const { onEvent } = useWebSocket();

  useEffect(() => {
    if (!channelId) {
      setAgentTracks(new Map());
      setActiveAgentIds([]);
      return;
    }

    setAgentTracks(new Map());
    setActiveAgentIds([]);

    const unsub = onEvent((event) => {
      // Handle agent.chunk — accumulate into per-agent track, mark active
      if (event.type === 'agent.chunk' && event.channel_id === channelId) {
        const chunk: AgentChunk = {
          agentId: event.agent_id,
          agentName: event.agent_name,
          chunkType: event.chunk_type as AgentChunk['chunkType'],
          content: event.content,
          tool: event.tool,
          timestamp: event.timestamp,
        };

        setAgentTracks(prev => {
          const next = new Map(prev);
          const existing = next.get(chunk.agentId) || [];
          next.set(chunk.agentId, mergeChunks(existing, chunk));
          return next;
        });

        setActiveAgentIds(prev => {
          if (!prev.includes(chunk.agentId)) {
            return [...prev, chunk.agentId];
          }
          return prev;
        });
        return;
      }

      // Handle agent.done (SOLO-island PR0) — authoritative terminal signal.
      // Immediately remove the agent from the active list. Replaces the
      // previous 3s inactivity heuristic that was both slow and prone to
      // premature eviction during long-running tool calls.
      if (event.type === 'agent.done' && event.channel_id === channelId && event.agent_id) {
        const agentId = event.agent_id;
        setActiveAgentIds(prev => prev.filter(id => id !== agentId));
        // Note: we keep agentTracks populated for now so the user can scroll
        // back through the trace. The next channel navigation or explicit
        // clearAgentChunks() call removes it.
      }
    });

    return unsub;
  }, [channelId, onEvent]);

  const clearAgentChunks = useCallback((agentId: string) => {
    setAgentTracks(prev => {
      const next = new Map(prev);
      next.delete(agentId);
      return next;
    });
    setActiveAgentIds(prev => prev.filter(id => id !== agentId));
  }, []);

  const clearAllChunks = useCallback(() => {
    setAgentTracks(new Map());
    setActiveAgentIds([]);
  }, []);

  return { agentTracks, activeAgentIds, clearAgentChunks, clearAllChunks };
}
