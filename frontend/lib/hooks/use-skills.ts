// ============================================================================
// useSkills — 5 hooks for the skill system (Phase1)
// - useSkills: full catalog from disk (via rescan-then-list)
// - useSkill(id): single skill with body + files
// - useAgentSkills(agentId): agent's current bindings
// - useRescanSkills(): trigger disk rescan (mutation)
// - useSetAgentSkills(agentId): replace agent bindings (mutation)
//
// Uses raw useState/useEffect (project does not have @tanstack/react-query).
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import type { Skill, SkillSummary, RescanResult } from '@/lib/types';

export function useSkills() {
  const [skills, setSkills] = useState<SkillSummary[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  const load = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<SkillSummary[]>('/api/v1/skills');
      if (mountedRef.current) setSkills(res);
    } catch (err) {
      if (mountedRef.current) {
        setError(err instanceof ApiError ? err.message : '加载 Skills 失败');
      }
    } finally {
      if (mountedRef.current) setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    load();
    return () => {
      mountedRef.current = false;
    };
  }, [load]);

  return { skills, isLoading, error, refetch: load } as const;
}

export function useSkill(id: string | null) {
  const [skill, setSkill] = useState<Skill | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!id) {
      setSkill(null);
      setIsLoading(false);
      setError(null);
      return;
    }
    let cancelled = false;
    setIsLoading(true);
    setError(null);
    apiClient
      .get<Skill>(`/api/v1/skills/${id}`)
      .then((res) => {
        if (!cancelled) setSkill(res);
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : '加载 Skill 失败');
          setSkill(null);
        }
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [id]);

  return { skill, isLoading, error } as const;
}

export function useAgentSkills(agentId: string | null) {
  const [skills, setSkills] = useState<SkillSummary[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  const load = useCallback(async () => {
    if (!agentId) {
      setSkills([]);
      return;
    }
    setIsLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<SkillSummary[]>(`/api/v1/agents/${agentId}/skills`);
      if (mountedRef.current) setSkills(res);
    } catch (err) {
      if (mountedRef.current) {
        setError(err instanceof ApiError ? err.message : '加载 Agent Skills 失败');
      }
    } finally {
      if (mountedRef.current) setIsLoading(false);
    }
  }, [agentId]);

  useEffect(() => {
    mountedRef.current = true;
    load();
    return () => {
      mountedRef.current = false;
    };
  }, [load]);

  return { skills, isLoading, error, refetch: load } as const;
}

export function useRescanSkills() {
  const [isPending, setIsPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const mutate = useCallback(async (): Promise<RescanResult | null> => {
    setIsPending(true);
    setError(null);
    try {
      const res = await apiClient.post<RescanResult>('/api/v1/skills/rescan', {});
      return res;
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Rescan 失败');
      return null;
    } finally {
      setIsPending(false);
    }
  }, []);

  return { mutate, isPending, error } as const;
}

export function useSetAgentSkills(agentId: string | null) {
  const [isPending, setIsPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const mutate = useCallback(
    async (skillIds: string[]): Promise<SkillSummary[] | null> => {
      if (!agentId) return null;
      setIsPending(true);
      setError(null);
      try {
        const res = await apiClient.put<SkillSummary[]>(
          `/api/v1/agents/${agentId}/skills`,
          { skill_ids: skillIds },
        );
        return res;
      } catch (err) {
        setError(err instanceof ApiError ? err.message : '设置 Skills 失败');
        return null;
      } finally {
        setIsPending(false);
      }
    },
    [agentId],
  );

  return { mutate, isPending, error } as const;
}
