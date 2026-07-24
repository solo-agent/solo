// ============================================================================
// useAgents — Agent CRUD hook backed by real API
// ============================================================================

'use client';

import { t } from '@/lib/i18n';
import { useState, useEffect, useCallback, useRef } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import type { Agent, CreateAgentInput, UpdateAgentInput } from '@/lib/types';

// ---- Backend response shape ----

interface AgentResponse {
  id: string;
  name: string;
  description: string;
  owner_id: string;
  home_channel_id: string;
  kind: 'agent' | 'lucy';
  model_provider: string;
  model_name: string;
  system_prompt: string;
  is_active: boolean;
  avatar_url: string;
  custom_env: Record<string, string> | null;
  custom_args: string[] | null;
  created_at: string;
  updated_at: string;
}

// ---- Mapping helpers ----

function mapAgent(resp: AgentResponse): Agent {
  return {
    id: resp.id,
    name: resp.name,
    description: resp.description || '',
    owner_id: resp.owner_id,
    home_channel_id: resp.home_channel_id,
    kind: resp.kind,
    model_provider: resp.model_provider as Agent['model_provider'],
    model_name: resp.model_name,
    system_prompt: resp.system_prompt,
    is_active: resp.is_active,
    avatar_url: resp.avatar_url || null,
    custom_env: resp.custom_env ?? {},
    custom_args: resp.custom_args ?? [],
    created_at: resp.created_at,
    updated_at: resp.updated_at,
  };
}

// ---- Hook ----

export function useAgents(channelId?: string | null) {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  const loadAgents = useCallback(async () => {
    if (!channelId) {
      setAgents([]);
      setIsLoading(false);
      return;
    }
    setIsLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<AgentResponse[]>(`/api/v1/channels/${channelId}/agents`);
      if (mountedRef.current) {
        setAgents(res.map(mapAgent));
      }
    } catch (err) {
      const message = err instanceof ApiError ? err.message : `${t('agentLoadError')}`;
      setError(message);
    } finally {
      setIsLoading(false);
    }
  }, [channelId]);

  useEffect(() => {
    mountedRef.current = true;
    loadAgents();

    return () => {
      mountedRef.current = false;
    };
  }, [loadAgents]);

  const createAgent = useCallback(async (input: CreateAgentInput): Promise<Agent> => {
    if (!channelId) {
      throw new Error('A home Channel is required to create an Agent.');
    }
    const res = await apiClient.post<AgentResponse>(`/api/v1/channels/${channelId}/agents`, {
      name: input.name,
      description: input.description || '',
      model_provider: input.model_provider,
      model_name: input.model_name,
      system_prompt: input.system_prompt || '',
      custom_env: input.custom_env || {},
      custom_args: input.custom_args || [],
    });
    const agent = mapAgent(res);
    setAgents((prev) => [...prev, agent]);
    return agent;
  }, [channelId]);

  const updateAgent = useCallback(async (id: string, input: UpdateAgentInput): Promise<Agent> => {
    const res = await apiClient.patch<AgentResponse>(`/api/v1/agents/${id}`, {
      name: input.name,
      description: input.description,
      model_provider: input.model_provider,
      model_name: input.model_name,
      system_prompt: input.system_prompt,
      custom_env: input.custom_env,
      custom_args: input.custom_args,
    });
    const updated = mapAgent(res);
    setAgents((prev) => prev.map((a) => (a.id === id ? updated : a)));
    return updated;
  }, []);

  const deleteAgent = useCallback(async (id: string) => {
    await apiClient.delete(`/api/v1/agents/${id}`);
    setAgents((prev) => prev.filter((a) => a.id !== id));
  }, []);

  const getAgent = useCallback(async (id: string): Promise<Agent | null> => {
    try {
      const res = await apiClient.get<AgentResponse>(`/api/v1/agents/${id}`);
      return mapAgent(res);
    } catch {
      return null;
    }
  }, []);

  return {
    agents,
    isLoading,
    error,
    createAgent,
    updateAgent,
    deleteAgent,
    getAgent,
    refetch: loadAgents,
  } as const;
}
