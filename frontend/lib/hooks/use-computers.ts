// ============================================================================
// useComputers — Computer CRUD hook backed by real API (SOLO-245-F, SOLO-246-F)
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import type { Computer, UpdateComputerInput } from '@/lib/types';

// ---- Backend response shape ----

interface ComputerResponse {
  id: string;
  name: string;
  owner_id: string;
  daemon_id?: string;
  daemon_url?: string;
  status: string;
  last_heartbeat?: string;
  agent_ids?: string[];
  agent_names?: string[];
  created_at: string;
  updated_at: string;
}

// ---- Mapping helpers ----

function mapComputer(resp: ComputerResponse): Computer {
  return {
    id: resp.id,
    name: resp.name,
    owner_id: resp.owner_id,
    daemon_id: resp.daemon_id,
    daemon_url: resp.daemon_url,
    status: resp.status as Computer['status'],
    last_heartbeat: resp.last_heartbeat,
    agent_ids: resp.agent_ids ?? [],
    agent_names: resp.agent_names ?? [],
    os: (resp as unknown as Record<string, unknown>).os as string | undefined,
    hostname: (resp as unknown as Record<string, unknown>).hostname as string | undefined,
    ip: (resp as unknown as Record<string, unknown>).ip as string | undefined,
    created_at: resp.created_at,
    updated_at: resp.updated_at,
  };
}

// ---- Hook ----

export function useComputers() {
  const [computers, setComputers] = useState<Computer[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  const loadComputers = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<ComputerResponse[]>('/api/v1/computers');
      if (mountedRef.current) {
        setComputers(res.map(mapComputer));
      }
    } catch (err) {
      const message = err instanceof ApiError ? err.message : '加载电脑列表失败';
      if (mountedRef.current) setError(message);
    } finally {
      if (mountedRef.current) setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    loadComputers();

    return () => {
      mountedRef.current = false;
    };
  }, [loadComputers]);

  const addComputer = useCallback(async (name: string): Promise<Computer> => {
    const res = await apiClient.post<ComputerResponse>('/api/v1/computers', { name });
    const computer = mapComputer(res);
    setComputers((prev) => [...prev, computer]);
    return computer;
  }, []);

  const updateComputer = useCallback(async (id: string, input: UpdateComputerInput): Promise<Computer> => {
    const res = await apiClient.patch<ComputerResponse>(`/api/v1/computers/${id}`, input);
    const updated = mapComputer(res);
    setComputers((prev) => prev.map((c) => (c.id === id ? updated : c)));
    return updated;
  }, []);

  const deleteComputer = useCallback(async (id: string) => {
    await apiClient.delete(`/api/v1/computers/${id}`);
    setComputers((prev) => prev.filter((c) => c.id !== id));
  }, []);

  return {
    computers,
    isLoading,
    error,
    addComputer,
    updateComputer,
    deleteComputer,
    refetch: loadComputers,
  } as const;
}
