// ============================================================================
// useAgents — Agent CRUD hook backed by real API
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import { getNextPixelAvatarIndex } from '@/components/ui/pixel-avatar';
import type { Agent, CreateAgentInput, UpdateAgentInput, AgentInteractionMode } from '@/lib/types';

// ---- Backend response shape ----

interface AgentResponse {
  id: string;
  name: string;
  description: string;
  owner_id: string;
  model_provider: string;
  model_name: string;
  system_prompt: string;
  temperature: number;
  max_tokens: number;
  is_active: boolean;
  auto_join: boolean;
  avatar_url: string;
  enabled_tools: string[];
  interaction_mode: string;
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
    model_provider: resp.model_provider as Agent['model_provider'],
    model_name: resp.model_name,
    system_prompt: resp.system_prompt,
    temperature: resp.temperature,
    max_tokens: resp.max_tokens,
    is_active: resp.is_active,
    auto_join: resp.auto_join,
    avatar_url: resp.avatar_url || null,
    enabled_tools: resp.enabled_tools ?? [],
    interaction_mode: (resp.interaction_mode as AgentInteractionMode) ?? 'mention',
    created_at: resp.created_at,
    updated_at: resp.updated_at,
  };
}

// ---- Hook ----

export function useAgents() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  const loadAgents = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<AgentResponse[]>('/api/v1/agents');
      if (mountedRef.current) {
        setAgents(res.map(mapAgent));
      }
    } catch (err) {
      const message = err instanceof ApiError ? err.message : '加载 Agent 列表失败';
      setError(message);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    loadAgents();

    return () => {
      mountedRef.current = false;
    };
  }, [loadAgents]);

  const createAgent = useCallback(async (input: CreateAgentInput): Promise<Agent> => {
    const nextIndex = getNextPixelAvatarIndex(agents.map(a => a.avatar_url));
    const res = await apiClient.post<AgentResponse>('/api/v1/agents', {
      name: input.name,
      description: input.description || '',
      model_provider: input.model_provider,
      model_name: input.model_name,
      system_prompt: input.system_prompt || '',
      temperature: input.temperature,
      max_tokens: input.max_tokens,
      avatar_url: input.avatar_url || `pixel:${nextIndex}`,
    });
    const agent = mapAgent(res);
    setAgents((prev) => [...prev, agent]);
    return agent;
  }, []);

  const updateAgent = useCallback(async (id: string, input: UpdateAgentInput): Promise<Agent> => {
    const res = await apiClient.patch<AgentResponse>(`/api/v1/agents/${id}`, {
      name: input.name,
      description: input.description,
      model_provider: input.model_provider,
      model_name: input.model_name,
      system_prompt: input.system_prompt,
      temperature: input.temperature,
      max_tokens: input.max_tokens,
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
