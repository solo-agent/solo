'use client';

import { useState, useCallback } from 'react';
import { apiClient } from '@/lib/api-client';
import type { CreateLucyRequest, CreateLucyResponse } from '@/lib/types';

export function useOnboarding() {
  const [isCreating, setIsCreating] = useState(false);
  const [createdAgent, setCreatedAgent] = useState<CreateLucyResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  const createLucy = useCallback(async (req: CreateLucyRequest): Promise<CreateLucyResponse> => {
    setIsCreating(true);
    setError(null);
    try {
      const res = await apiClient.post<CreateLucyResponse>('/api/v1/onboarding/create-lucy', req);
      setCreatedAgent(res);
      return res;
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'Failed to create Lucy';
      setError(message);
      throw err;
    } finally {
      setIsCreating(false);
    }
  }, []);

  return { createLucy, isCreating, createdAgent, error } as const;
}
