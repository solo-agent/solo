'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import type { ChannelWorkspace } from '@/lib/types';

export function useChannelWorkspace(channelId: string | null | undefined) {
  const [workspace, setWorkspace] = useState<ChannelWorkspace | null>(null);
  const [isLoading, setIsLoading] = useState(Boolean(channelId));
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  const loadWorkspace = useCallback(async () => {
    if (!channelId) {
      setWorkspace(null);
      setIsLoading(false);
      setError(null);
      return;
    }

    setIsLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<ChannelWorkspace>(`/api/v1/channels/${channelId}/workspace`);
      if (mountedRef.current) setWorkspace(res);
    } catch (err) {
      if (!mountedRef.current) return;
      setError(err instanceof ApiError ? err.message : 'Failed to load channel workspace');
    } finally {
      if (mountedRef.current) setIsLoading(false);
    }
  }, [channelId]);

  useEffect(() => {
    mountedRef.current = true;
    loadWorkspace();
    return () => {
      mountedRef.current = false;
    };
  }, [loadWorkspace]);

  return {
    workspace,
    context: workspace?.context ?? null,
    team: workspace?.team ?? null,
    isLoading,
    error,
    refetch: loadWorkspace,
  } as const;
}
