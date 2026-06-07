// ============================================================================
// useComputerAgents — fetch agents running on a computer (v1.5)
// - GET /api/v1/computers/{id}/agents
// - Returns agents array + loading/error
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import type { ComputerAgent } from '@/lib/types';

interface ComputerAgentsResponse {
  computer_id: string;
  agents: ComputerAgent[];
}

export function useComputerAgents(computerId: string | null) {
  const [agents, setAgents] = useState<ComputerAgent[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  const fetchAgents = useCallback(async () => {
    if (!computerId) {
      setAgents([]);
      return;
    }

    setIsLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<ComputerAgentsResponse>(
        `/api/v1/computers/${computerId}/agents`,
      );
      if (mountedRef.current) {
        setAgents(Array.isArray(res.agents) ? res.agents : []);
      }
    } catch (err) {
      const message = err instanceof ApiError ? err.message : '加载 Agent 列表失败';
      if (mountedRef.current) setError(message);
    } finally {
      if (mountedRef.current) setIsLoading(false);
    }
  }, [computerId]);

  useEffect(() => {
    mountedRef.current = true;
    fetchAgents();
    return () => { mountedRef.current = false; };
  }, [fetchAgents]);

  return { agents, isLoading, error, refetch: fetchAgents } as const;
}
