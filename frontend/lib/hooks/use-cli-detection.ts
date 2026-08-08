// ============================================================================
// useCliDetection — fetches CLI availability from /api/v1/agent-backends/detect
// Used to show which provider CLIs are installed on the runtime computer
// v1.4: returns Record<string, AgentBackendDetectItem> keyed by type
// ============================================================================

'use client';

import { t } from '@/lib/i18n';
import { useState, useEffect, useRef } from 'react';
import { apiClient } from '@/lib/api-client';
import type { AgentBackendDetectItem } from '@/lib/types';

export interface CliDetectionState {
  /** Map of type -> detection result */
  results: Record<string, AgentBackendDetectItem>;
  isLoaded: boolean;
  isLoading: boolean;
  error: string | null;
}

/** Whitelist of runtimes available for agent creation. */
const ALLOWED_RUNTIMES = new Set(["openclaw", "hermes", "claude", "opencode", "codex"]);

/** Raw shape from backend — matches GET /api/v1/agent-backends/detect */
interface DetectResponseItem {
  type: string;
  display_name: string;
  binary: string;
  available: boolean;
  version?: string;
  error?: string;
}

export function useCliDetection(computerId?: string, cached: DetectResponseItem[] = []): CliDetectionState {
  const [results, setResults] = useState<
    Record<string, AgentBackendDetectItem>
  >({});
  const [isLoaded, setIsLoaded] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);
  const cachedKey = JSON.stringify(cached);

  useEffect(() => {
    const cachedItems = JSON.parse(cachedKey) as DetectResponseItem[];
    mountedRef.current = true;
    setIsLoading(true);
    setError(null);
    setResults({});

    apiClient
      .get<DetectResponseItem[]>(`/api/v1/agent-backends/detect${computerId ? `?computer_id=${encodeURIComponent(computerId)}` : ''}`)
      .then((data) => {
        if (!mountedRef.current) return;
        const map: Record<string, AgentBackendDetectItem> = {};
        for (const item of data) {
          if (!ALLOWED_RUNTIMES.has(item.type)) continue;
          map[item.type] = {
            type: item.type,
            display_name: item.display_name,
            binary: item.binary,
            available: item.available,
            version: item.version,
            error: item.error,
          };
        }
        setResults(map);
        setIsLoaded(true);
      })
      .catch((err) => {
        if (!mountedRef.current) return;
        if (cachedItems.length > 0) {
          const map: Record<string, AgentBackendDetectItem> = {};
          for (const item of cachedItems) {
            if (ALLOWED_RUNTIMES.has(item.type)) map[item.type] = item;
          }
          setResults(map);
          setError(null);
          setIsLoaded(true);
          return;
        }
        setError(
          err instanceof Error ? err.message : `${t('cliDetectionError')}`,
        );
        setIsLoaded(true);
      })
      .finally(() => {
        if (mountedRef.current) setIsLoading(false);
      });

    return () => {
      mountedRef.current = false;
    };
  }, [computerId, cachedKey]);

  return { results, isLoaded, isLoading, error };
}
