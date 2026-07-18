'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import { useWebSocket } from '@/lib/ws-context';
import { t } from '@/lib/i18n';
import type { ThinkingNode, ThinkingSpace } from '@/lib/types';

export function useThinkingSpace(
  channelId: string,
  enabled: boolean,
  initialNodeId?: string | null,
) {
  const [space, setSpace] = useState<ThinkingSpace | null>(null);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const { onEvent } = useWebSocket();
  const refreshTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const appliedInitialNodeId = useRef<string | null>(null);

  const adoptSpace = useCallback((next: ThinkingSpace) => {
    setSpace(next);
    setSelectedNodeId((current) => {
      if (current && next.nodes.some((node) => node.id === current)) return current;
      return next.nodes.find((node) => !node.parent_id)?.id ?? next.nodes[0]?.id ?? null;
    });
  }, []);

  const refresh = useCallback(async () => {
    if (!enabled) return;
    try {
      adoptSpace(await apiClient.get<ThinkingSpace>(`/api/v1/channels/${channelId}/thinking`));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('thinkingSpaceError'));
    }
  }, [adoptSpace, channelId, enabled]);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    setIsLoading(true);
    setError(null);
    apiClient.post<ThinkingSpace>(`/api/v1/channels/${channelId}/thinking`)
      .then((next) => { if (active) adoptSpace(next); })
      .catch((err) => {
        if (active) setError(err instanceof ApiError ? err.message : t('thinkingSpaceError'));
      })
      .finally(() => { if (active) setIsLoading(false); });
    return () => { active = false; };
  }, [adoptSpace, channelId, enabled]);

  useEffect(() => {
    appliedInitialNodeId.current = null;
  }, [channelId]);

  useEffect(() => {
    if (!initialNodeId || appliedInitialNodeId.current === initialNodeId) return;
    if (!space?.nodes.some((node) => node.id === initialNodeId)) return;
    appliedInitialNodeId.current = initialNodeId;
    setSelectedNodeId(initialNodeId);
  }, [initialNodeId, space]);

  useEffect(() => {
    if (!enabled) return;
    const unsubscribe = onEvent((event) => {
      const nodeMessage = event.type === 'message.new' && event.channel_id === channelId && Boolean(event.thinking_node_id);
      const topologyUpdate = event.type === 'thinking.updated' && event.channel_id === channelId;
      if (!nodeMessage && !topologyUpdate) return;
      if (refreshTimer.current) clearTimeout(refreshTimer.current);
      refreshTimer.current = setTimeout(() => { void refresh(); }, 150);
    });
    return () => {
      unsubscribe();
      if (refreshTimer.current) clearTimeout(refreshTimer.current);
    };
  }, [channelId, enabled, onEvent, refresh]);

  const createChild = useCallback(async (parentId: string, title: string) => {
    const node = await apiClient.post<ThinkingNode>(
      `/api/v1/channels/${channelId}/thinking/nodes/${parentId}/children`,
      { title },
    );
    setSpace((current) => current ? { ...current, nodes: [...current.nodes, node] } : current);
    setSelectedNodeId(node.id);
    return node;
  }, [channelId]);

  const returnNode = useCallback(async (nodeId: string) => {
    const node = await apiClient.post<ThinkingNode>(
      `/api/v1/channels/${channelId}/thinking/nodes/${nodeId}/return`,
    );
    setSpace((current) => current ? {
      ...current,
      nodes: current.nodes.map((item) => item.id === node.id ? node : item),
    } : current);
    return node;
  }, [channelId]);

  const retryForkHandoff = useCallback(async (nodeId: string) => {
    const node = await apiClient.post<ThinkingNode>(
      `/api/v1/channels/${channelId}/thinking/nodes/${nodeId}/handoff/retry`,
    );
    return node;
  }, [channelId]);

  return {
    space,
    selectedNodeId,
    setSelectedNodeId,
    isLoading,
    error,
    refresh,
    createChild,
    retryForkHandoff,
    returnNode,
  } as const;
}
