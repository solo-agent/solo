// ============================================================================
// useChannels — channel CRUD hook backed by real API
// ============================================================================

'use client';

import { t } from '@/lib/i18n';
import { useState, useEffect, useCallback, useRef } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import type { Channel, CreateChannelInput } from '@/lib/types';

// ---- Backend response shape ----

interface ChannelResponse {
  id: string;
  name: string;
  description: string;
  type: string;
  created_by: string;
  is_archived: boolean;
  created_at: string;
  updated_at: string;
}

// ---- Mapping helpers ----

function mapChannel(resp: ChannelResponse): Channel {
  return {
    id: resp.id,
    name: resp.name,
    description: resp.description || '',
    member_count: 0, // Backend channel list doesn't include member_count
    created_at: resp.created_at,
    created_by: resp.created_by,
  };
}

// ---- Hook ----

export function useChannels() {
  const [channels, setChannels] = useState<Channel[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  const loadChannels = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<ChannelResponse[]>('/api/v1/channels');
      if (mountedRef.current) {
        setChannels(res.map(mapChannel));
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

  const createChannel = useCallback(
    async (input: CreateChannelInput): Promise<Channel> => {
      const res = await apiClient.post<ChannelResponse>('/api/v1/channels', {
        name: input.name,
        description: input.description || '',
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
    isLoading,
    error,
    createChannel,
    deleteChannel,
    refetch: loadChannels,
  } as const;
}
