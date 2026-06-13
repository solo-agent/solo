// ============================================================================
// useRelationships — Agent relationship CRUD hook with WebSocket sync (T5.1.3)
// - Fetch relationships from REST API with optional filters
// - WebSocket subscription for create/update/delete events
// - Granular cache invalidation on WS events
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import { useWebSocket } from '@/lib/ws-context';
import type { AgentRelationship, CreateRelationshipInput, RelationshipType } from '@/lib/types';

// ---- Backend response shape ----

interface RelationshipResponse {
  id: string;
  from_agent_id: string;
  from_agent_name?: string;
  from_agent_active?: boolean;
  to_agent_id: string;
  to_agent_name?: string;
  to_agent_active?: boolean;
  rel_type: string;
  channel_id?: string;
  channel_name?: string;
  weight?: number;
  created_at?: string;
}

// ---- Mapping helpers ----

function mapRelationship(resp: RelationshipResponse): AgentRelationship {
  return {
    id: resp.id,
    from_agent_id: resp.from_agent_id,
    from_agent_name: resp.from_agent_name,
    from_agent_active: resp.from_agent_active,
    to_agent_id: resp.to_agent_id,
    to_agent_name: resp.to_agent_name,
    to_agent_active: resp.to_agent_active,
    rel_type: resp.rel_type as RelationshipType,
    channel_id: resp.channel_id,
    channel_name: resp.channel_name,
    weight: resp.weight,
    created_at: resp.created_at,
  };
}

// ---- Hook ----

export interface RelationshipFilters {
  rel_type?: RelationshipType;
  channel_id?: string;
  agent_id?: string;
}

export function useRelationships(filters?: RelationshipFilters) {
  const [relationships, setRelationships] = useState<AgentRelationship[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  const loadRelationships = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const params: Record<string, string> = {};
      if (filters?.rel_type) params.rel_type = filters.rel_type;
      if (filters?.channel_id) params.channel_id = filters.channel_id;
      if (filters?.agent_id) {
        // If agent_id is provided, use the agent-specific endpoint
        const res = await apiClient.get<RelationshipResponse[]>(
          `/api/v1/agents/${filters.agent_id}/relationships`,
        );
        if (mountedRef.current) {
          setRelationships(Array.isArray(res) ? res.map(mapRelationship) : []);
        }
        return;
      }

      const query = new URLSearchParams(params).toString();
      const path = query
        ? `/api/v1/agent-relationships?${query}`
        : '/api/v1/agent-relationships';
      const res = await apiClient.get<RelationshipResponse[]>(path);
      if (mountedRef.current) {
        setRelationships(Array.isArray(res) ? res.map(mapRelationship) : []);
      }
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to load relationships';
      if (mountedRef.current) setError(message);
    } finally {
      if (mountedRef.current) setIsLoading(false);
    }
  }, [filters?.rel_type, filters?.channel_id, filters?.agent_id]);

  useEffect(() => {
    mountedRef.current = true;
    loadRelationships();
    return () => { mountedRef.current = false; };
  }, [loadRelationships]);

  // ---- WebSocket subscriptions for real-time sync ----

  const { onEvent } = useWebSocket();

  useEffect(() => {
    const unsub = onEvent((event) => {
      // relationship_created
      if (event.type === 'relationship_created') {
        // Apply filters
        if (filters?.rel_type && event.rel_type !== filters.rel_type) return;
        if (filters?.channel_id && event.channel_id !== filters.channel_id) return;
        if (filters?.agent_id &&
          event.from_agent_id !== filters.agent_id &&
          event.to_agent_id !== filters.agent_id) return;

        setRelationships((prev) => {
          if (prev.find((r) => r.id === event.id)) return prev;
          return [...prev, {
            id: event.id,
            from_agent_id: event.from_agent_id,
            to_agent_id: event.to_agent_id,
            rel_type: event.rel_type as RelationshipType,
            channel_id: event.channel_id,
          }];
        });
        return;
      }

      // relationship_updated
      if (event.type === 'relationship_updated') {
        setRelationships((prev) => {
          const existing = prev.find((r) => r.id === event.id);
          if (!existing) return prev;
          return prev.map((r) =>
            r.id === event.id
              ? {
                  ...r,
                  rel_type: event.rel_type as RelationshipType,
                  channel_id: event.channel_id,
                }
              : r,
          );
        });
        return;
      }

      // relationship_deleted
      if (event.type === 'relationship_deleted') {
        setRelationships((prev) => prev.filter((r) => r.id !== event.id));
      }
    });
    return unsub;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [onEvent, filters?.rel_type, filters?.channel_id, filters?.agent_id]);

  // ---- CRUD operations ----

  const createRelationship = useCallback(
    async (input: CreateRelationshipInput): Promise<AgentRelationship> => {
      const res = await apiClient.post<RelationshipResponse>(
        '/api/v1/agent-relationships',
        {
          from_agent_id: input.from_agent_id,
          to_agent_id: input.to_agent_id,
          rel_type: input.rel_type,
          channel_id: input.channel_id,
          weight: input.weight,
        },
      );
      const rel = mapRelationship(res);
      setRelationships((prev) => {
        if (prev.find((r) => r.id === rel.id)) return prev;
        return [...prev, rel];
      });
      return rel;
    },
    [],
  );

  const updateRelationship = useCallback(
    async (id: string, input: Partial<Pick<CreateRelationshipInput, 'rel_type' | 'weight'>>): Promise<AgentRelationship> => {
      const res = await apiClient.patch<RelationshipResponse>(
        `/api/v1/agent-relationships/${id}`,
        input,
      );
      const updated = mapRelationship(res);
      setRelationships((prev) =>
        prev.map((r) => (r.id === id ? updated : r)),
      );
      return updated;
    },
    [],
  );

  const deleteRelationship = useCallback(async (id: string) => {
    await apiClient.delete(`/api/v1/agent-relationships/${id}`);
    setRelationships((prev) => prev.filter((r) => r.id !== id));
  }, []);

  return {
    relationships,
    isLoading,
    error,
    createRelationship,
    updateRelationship,
    deleteRelationship,
    refetch: loadRelationships,
  } as const;
}
