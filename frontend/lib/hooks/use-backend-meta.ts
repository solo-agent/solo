// ============================================================================
// useBackendMeta — fetches registered agent backends from /api/v1/agent-backends
// Returns Record<string, AgentBackendMeta> keyed by type.
// v1.4: used to show available models for each runtime.
// ============================================================================

'use client';

import { useState, useEffect, useRef } from 'react';
import { apiClient } from '@/lib/api-client';
import type { AgentBackendMeta } from '@/lib/types';

export interface BackendMetaState {
  /** Map of type -> backend metadata */
  metas: Record<string, AgentBackendMeta>;
  isLoaded: boolean;
  isLoading: boolean;
  error: string | null;
}

/** Raw shape from GET /api/v1/agent-backends */
interface BackendResponseItem {
  type: string;
  display_name: string;
  requires_binary: string;
  protocols: string[];
  default_model: string;
  models: { id: string; label: string; default: boolean }[];
}

export function useBackendMeta(): BackendMetaState {
  const [metas, setMetas] = useState<Record<string, AgentBackendMeta>>({});
  const [isLoaded, setIsLoaded] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    setIsLoading(true);

    apiClient
      .get<BackendResponseItem[]>('/api/v1/agent-backends')
      .then((data) => {
        if (!mountedRef.current) return;
        const map: Record<string, AgentBackendMeta> = {};
        for (const item of data) {
          map[item.type] = {
            type: item.type,
            display_name: item.display_name,
            requires_binary: item.requires_binary,
            protocols: item.protocols,
            default_model: item.default_model,
            models: item.models,
          };
        }
        setMetas(map);
        setIsLoaded(true);
      })
      .catch((err) => {
        if (!mountedRef.current) return;
        setError(
          err instanceof Error ? err.message : '加载 Backend 元数据失败',
        );
      })
      .finally(() => {
        if (mountedRef.current) setIsLoading(false);
      });

    return () => {
      mountedRef.current = false;
    };
  }, []);

  return { metas, isLoaded, isLoading, error };
}
