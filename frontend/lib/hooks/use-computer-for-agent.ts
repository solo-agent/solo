// ============================================================================
// useComputerForAgent — reverse-lookup: given an agent id, find the
// computer that owns it. (v3.3)
//
// Strategy: fetch the computers list, then call GET /computers/{id}/agents
// for each, accumulating the first match. Result is { computer, loading }.
// Returns null while still searching, even after the first non-match.
// ============================================================================

'use client';

import { useState, useEffect } from 'react';
import { apiClient } from '@/lib/api-client';
import type { Computer, ComputerAgent } from '@/lib/types';

export interface ComputerForAgentResult {
  computer: Computer | null;
  isLoading: boolean;
  error: string | null;
}

export function useComputerForAgent(agentId: string | null): ComputerForAgentResult {
  const [computer, setComputer] = useState<Computer | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!agentId) {
      setComputer(null);
      return;
    }
    let cancelled = false;
    setIsLoading(true);
    setError(null);

    (async () => {
      try {
        const computers = await apiClient.get<Computer[]>('/api/v1/computers');
        for (const c of computers) {
          if (cancelled) return;
          try {
            const { agents } = await apiClient.get<{ agents: ComputerAgent[] }>(
              `/api/v1/computers/${c.id}/agents`,
            );
            if (cancelled) return;
            if (agents.some((a) => a.id === agentId)) {
              setComputer(c);
              setIsLoading(false);
              return;
            }
          } catch {
            // skip this computer
          }
        }
        if (!cancelled) {
          setComputer(null);
          setIsLoading(false);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Lookup failed');
          setIsLoading(false);
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [agentId]);

  return { computer, isLoading, error };
}
