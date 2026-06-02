'use client';

import { useState, useEffect, useRef, useCallback } from 'react';
import { useWebSocket } from '@/lib/ws-context';

export interface AgentChunk {
  agentId: string;
  agentName: string;
  chunkType: 'thinking' | 'tool_use' | 'tool_result' | 'text' | 'error';
  content: string;
  tool?: { name: string; input?: string; output?: string; call_id?: string };
  timestamp: string;
}

const MAX_CHUNKS_PER_AGENT = 200;
const DONE_CLEANUP_MS = 3000;

export function useAgentChunks(channelId: string | null) {
  const [agentTracks, setAgentTracks] = useState<Map<string, AgentChunk[]>>(new Map());
  const [activeAgentIds, setActiveAgentIds] = useState<string[]>([]);
  const { onEvent } = useWebSocket();
  const doneTimers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  useEffect(() => {
    if (!channelId) {
      setAgentTracks(new Map());
      setActiveAgentIds([]);
      return;
    }

    setAgentTracks(new Map());
    setActiveAgentIds([]);

    const unsub = onEvent((event) => {
      // Handle agent.chunk
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
          const trimmed = existing.length >= MAX_CHUNKS_PER_AGENT ? existing.slice(1) : existing;
          next.set(chunk.agentId, [...trimmed, chunk]);
          return next;
        });

        setActiveAgentIds(prev => {
          if (!prev.includes(chunk.agentId)) {
            return [...prev, chunk.agentId];
          }
          return prev;
        });
      }

      // Handle message.new from agent: mark done, cleanup after delay
      if (
        event.type === 'message.new' &&
        event.sender_type === 'agent' &&
        event.channel_id === channelId &&
        event.sender_id
      ) {
        const agentId = event.sender_id;
        const existing = doneTimers.current.get(agentId);
        if (existing) clearTimeout(existing);
        const timer = setTimeout(() => {
          setActiveAgentIds(prev => prev.filter(id => id !== agentId));
          doneTimers.current.delete(agentId);
        }, DONE_CLEANUP_MS);
        doneTimers.current.set(agentId, timer);
      }
    });

    return () => {
      unsub();
      doneTimers.current.forEach(timer => clearTimeout(timer));
      doneTimers.current.clear();
    };
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
