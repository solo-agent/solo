'use client';

import { useCallback, useEffect, useState } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import { t } from '@/lib/i18n';
import type { Automation, AutomationInput, AutomationRun } from '@/lib/types';

export function useAutomations(channelId: string, active = true) {
  const [automations, setAutomations] = useState<Automation[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (showLoading = true) => {
    if (!active || !channelId) return;
    if (showLoading) {
      setIsLoading(true);
      setError(null);
    }
    try {
      const items = await apiClient.get<Automation[]>(`/api/v1/channels/${channelId}/automations`);
      setAutomations(Array.isArray(items) ? items : []);
    } catch (err) {
      if (showLoading) setError(err instanceof ApiError ? err.message : t('automationLoadError'));
    } finally {
      if (showLoading) setIsLoading(false);
    }
  }, [active, channelId]);

  useEffect(() => {
    void load();
    if (!active) return;
    const timer = window.setInterval(() => { void load(false); }, 15_000);
    return () => window.clearInterval(timer);
  }, [active, load]);

  const create = useCallback(async (input: AutomationInput) => {
    const created = await apiClient.post<Automation>(`/api/v1/channels/${channelId}/automations`, input);
    setAutomations((current) => [created, ...current]);
    return created;
  }, [channelId]);

  const update = useCallback(async (id: string, input: AutomationInput) => {
    const updated = await apiClient.patch<Automation>(`/api/v1/channels/${channelId}/automations/${id}`, input);
    setAutomations((current) => current.map((item) => item.id === id ? updated : item));
    return updated;
  }, [channelId]);

  const remove = useCallback(async (id: string) => {
    await apiClient.delete(`/api/v1/channels/${channelId}/automations/${id}`);
    setAutomations((current) => current.filter((item) => item.id !== id));
  }, [channelId]);

  const runNow = useCallback(async (id: string) => {
    try {
      return await apiClient.post<AutomationRun>(`/api/v1/channels/${channelId}/automations/${id}/run`);
    } finally {
      await load();
    }
  }, [channelId, load]);

  const listRuns = useCallback(async (id: string) => {
    return apiClient.get<AutomationRun[]>(`/api/v1/channels/${channelId}/automations/${id}/runs?limit=20`);
  }, [channelId]);

  return { automations, isLoading, error, load, create, update, remove, runNow, listRuns };
}
