// ============================================================================
// useSkills — 2 hooks for the skill system
// - useSkills: full catalog (daemon syncs via heartbeat, read-only)
// - useSkill(id): single skill with body + files
//
// Skills are discovered by the daemon and synced to the DB on each heartbeat
// (every 30s). The catalog is shared across all agents.
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import type { Skill, SkillSummary } from '@/lib/types';

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

