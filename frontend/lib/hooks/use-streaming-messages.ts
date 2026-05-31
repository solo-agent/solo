// ============================================================================
// useStreamingMessages — tracks which agents are actively streaming content
// - Used by channel-view.tsx for the enhanced typing indicator
// - Only tracks agent IDs (not content — content is in use-messages.ts)
// ============================================================================

'use client';

import { useEffect, useRef, useState } from 'react';
import { useWebSocket } from '@/lib/ws-context';

interface StreamingStatusResult {
  /** IDs of agents currently streaming content */
  activeStreamingAgentIds: string[];
}

export function useStreamingMessages(
  channelId: string | null,
): StreamingStatusResult {
  const { onEvent } = useWebSocket();
  const streamingAgentIdsRef = useRef<Set<string>>(new Set());
  const [activeStreamingAgentIds, setActiveStreamingAgentIds] = useState<string[]>([]);

  useEffect(() => {
    if (!channelId) return;

    streamingAgentIdsRef.current.clear();
    setActiveStreamingAgentIds([]);

    const unsub = onEvent((event) => {
      if (event.type === 'message.agent_typing') {
        if (event.channel_id !== channelId) return;
        // An agent started streaming (or is continuing)
        streamingAgentIdsRef.current.add(event.sender_id);
        setActiveStreamingAgentIds(Array.from(streamingAgentIdsRef.current));
      }

      if (event.type === 'message.new' && event.channel_id === channelId && event.sender_id) {
        // Agent finished streaming — remove from active set
        streamingAgentIdsRef.current.delete(event.sender_id);
        setActiveStreamingAgentIds(Array.from(streamingAgentIdsRef.current));
      }
    });

    return unsub;
  }, [channelId, onEvent]);

  return { activeStreamingAgentIds };
}
