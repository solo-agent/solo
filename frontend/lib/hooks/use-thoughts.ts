'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import type { ThoughtSession } from '@/lib/types';

interface ThoughtListResponse {
  thoughts: ThoughtSession[];
}

export function useThoughts(channelId: string | null | undefined) {
  const [thoughts, setThoughts] = useState<ThoughtSession[]>([]);
  const [isLoading, setIsLoading] = useState(Boolean(channelId));
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  const loadThoughts = useCallback(async () => {
    if (!channelId) {
      setThoughts([]);
      setIsLoading(false);
      setError(null);
      return;
    }

    setIsLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<ThoughtListResponse>(`/api/v1/channels/${channelId}/thoughts`);
      if (mountedRef.current) setThoughts(res.thoughts ?? []);
    } catch (err) {
      if (!mountedRef.current) return;
      setError(err instanceof ApiError ? err.message : 'Failed to load thoughts');
    } finally {
      if (mountedRef.current) setIsLoading(false);
    }
  }, [channelId]);

  useEffect(() => {
    mountedRef.current = true;
    loadThoughts();
    return () => {
      mountedRef.current = false;
    };
  }, [loadThoughts]);

  const startThought = useCallback(async (input: { message_id?: string; title?: string }) => {
    if (!channelId) throw new Error('channel is required');
    const thought = await apiClient.post<ThoughtSession>(`/api/v1/channels/${channelId}/thoughts`, input);
    if (mountedRef.current) {
      setThoughts((current) => [thought, ...current.filter((item) => item.id !== thought.id)]);
    }
    return thought;
  }, [channelId]);

  const completeThought = useCallback(async (thoughtId: string, input: { message_id?: string; summary?: string } = {}) => {
    const thought = await apiClient.post<ThoughtSession>(`/api/v1/thoughts/${thoughtId}/complete`, input);
    if (mountedRef.current) {
      setThoughts((current) => current.map((item) => (item.id === thought.id ? thought : item)));
    }
    return thought;
  }, []);

  const requestThoughtReview = useCallback(async (thoughtId: string) => {
    const thought = await apiClient.post<ThoughtSession>(`/api/v1/thoughts/${thoughtId}/review`, {});
    if (mountedRef.current) {
      setThoughts((current) => current.map((item) => (item.id === thought.id ? thought : item)));
    }
    return thought;
  }, []);

  return {
    thoughts,
    activeThought: thoughts[0] ?? null,
    isLoading,
    error,
    refetch: loadThoughts,
    startThought,
    completeThought,
    requestThoughtReview,
  } as const;
}
