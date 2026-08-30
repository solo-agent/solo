// ============================================================================
// useChannels — channel CRUD hook backed by real API
// ============================================================================

'use client';

import { t } from '@/lib/i18n';
import { useState, useEffect, useCallback, useRef } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import type { Channel, CreateChannelInput } from '@/lib/types';
import { useWebSocket } from '@/lib/ws-context';

// ---- Backend response shape ----

interface ChannelResponse {
  id: string;
  name: string;
  description: string;
  type: string;
  created_by: string;
  is_archived: boolean;
  source_template_id?: string;
  created_at: string;
  updated_at: string;
}

// ---- Mapping helpers ----

function mapChannel(resp: ChannelResponse): Channel {
  return {
    id: resp.id,
    name: resp.name,
    description: resp.description || '',
    type: resp.type as Channel['type'],
    source_template_id: resp.source_template_id,
    member_count: 0, // Backend channel list doesn't include member_count
    created_at: resp.created_at,
    created_by: resp.created_by,
  };
}

// ---- Hook ----

export function useChannels() {
  const [channels, setChannels] = useState<Channel[]>([]);
  const [lucyChannel, setLucyChannel] = useState<Channel | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);
  const { onEvent } = useWebSocket();

  const loadChannels = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<ChannelResponse[]>('/api/v1/channels');
      if (mountedRef.current) {
        setChannels(res.map(mapChannel));
      }
      try {
        const lucy = await apiClient.get<ChannelResponse>('/api/v1/channels/lucy');
        if (mountedRef.current) {
          setLucyChannel({
            ...mapChannel(lucy),
            name: 'Lucy',
            type: 'lucy',
          });
        }
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) {
          if (mountedRef.current) setLucyChannel(null);
        } else {
          throw err;
        }
      }
    } catch (err) {
      const message = err instanceof ApiError ? err.message : `${t('channelLoadError')}`;
      setError(message);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    loadChannels();

    return () => {
      mountedRef.current = false;
    };
  }, [loadChannels]);

  useEffect(() => {
    return onEvent((event) => {
      if (event.type === 'team.formed') {
        void loadChannels();
      }
    });
  }, [loadChannels, onEvent]);

  const createChannel = useCallback(
    async (input: CreateChannelInput): Promise<Channel> => {
      const res = await apiClient.post<ChannelResponse>('/api/v1/channels', {
        name: input.name,
        description: input.description || '',
        template_id: input.template_id || undefined,
      });
      const channel = mapChannel(res);
      // Add to local state so the sidebar updates immediately
      setChannels((prev) => [...prev, channel]);
      return channel;
    },
    [],
  );

  const deleteChannel = useCallback(async (channelId: string) => {
    await apiClient.delete(`/api/v1/channels/${channelId}`);
    // Remove from local state immediately
    setChannels((prev) => prev.filter((c) => c.id !== channelId));
  }, []);

  return {
    channels,
    lucyChannel,
    isLoading,
    error,
    createChannel,
    deleteChannel,
    refetch: loadChannels,
  } as const;
}
