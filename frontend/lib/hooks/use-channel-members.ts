// ============================================================================
// useChannelMembers — channel members hook backed by real API
// - CRUD via REST API
// - updateMemberStatus kept as local WS-driven state
// ============================================================================

'use client';

import { t } from '@/lib/i18n';
import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import { useWebSocket } from '@/lib/ws-context';
import type { ChannelMember } from '@/lib/types';

// ---- Backend response shape (from service.Member) ----

interface MemberResponse {
  channel_id: string;
  member_type: string;
  member_id: string;
  display_name: string;
  avatar_url?: string | null;
  email?: string;
  role: string;
  joined_at: string;
}

// ---- Mapping helpers ----

function mapMember(resp: MemberResponse): ChannelMember {
  return {
    channel_id: resp.channel_id,
    member_type: resp.member_type as 'user' | 'agent',
    member_id: resp.member_id,
    role: resp.role as 'owner' | 'admin' | 'member',
    // Backend only resolves display_name for user members.
    // For agent members, fall back to member_id.
    display_name: resp.display_name || resp.member_id,
    avatar_url: resp.avatar_url,
    status: 'offline' as const,
  };
}

// ---- Hook ----

export function useChannelMembers(channelId: string | null) {
  const [members, setMembers] = useState<ChannelMember[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);
  const channelIdRef = useRef(channelId);
  channelIdRef.current = channelId;
  const loadMembersRef = useRef<(() => Promise<void>) | null>(null);
  const { onEvent } = useWebSocket();

  // Keep channel membership in sync when another surface adds/removes agents.
  useEffect(() => {
    return onEvent((event) => {
      if (
        (event.type === 'member.added' || event.type === 'member.removed') &&
        event.channel_id === channelIdRef.current
      ) {
        loadMembersRef.current?.();
      }
      if (event.type === 'agent_deleted') {
        setMembers((prev) => prev.filter((member) => member.member_id !== event.agent_id));
        loadMembersRef.current?.();
      }
    });
  }, [onEvent]);

  const loadMembers = useCallback(async () => {
    if (!channelId) {
      setMembers([]);
      setIsLoading(false);
      return;
    }

    setIsLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<MemberResponse[]>(
        `/api/v1/channels/${channelId}/members`,
      );
      if (mountedRef.current) {
        setMembers(res.map(mapMember));
      }
    } catch (err) {
      const message = err instanceof ApiError ? err.message : `${t('memberLoadError')}`;
      setError(message);
    } finally {
      setIsLoading(false);
    }
  }, [channelId]);

  loadMembersRef.current = loadMembers;

  useEffect(() => {
    mountedRef.current = true;
    loadMembers();

    return () => {
      mountedRef.current = false;
    };
  }, [loadMembers]);

  const addAgentToChannel = useCallback(
    async (agentId: string, _agentName: string) => {
      if (!channelId) return;
      await apiClient.post(`/api/v1/channels/${channelId}/members`, {
        member_type: 'agent',
        member_id: agentId,
      });
      // Reload to get up-to-date member list including display names
      await loadMembers();
    },
    [channelId, loadMembers],
  );

  const removeMember = useCallback(
    async (_memberType: 'user' | 'agent', memberId: string) => {
      if (!channelId) return;
      await apiClient.delete(
        `/api/v1/channels/${channelId}/members/${memberId}`,
      );
      setMembers((prev) => prev.filter((m) => m.member_id !== memberId));
    },
    [channelId],
  );

  /**
   * 实时更新指定成员的状态（由 WebSocket agent.thinking / agent.typing 驱动）
   * 这是纯本地状态操作，不涉及后端 API
   */
  const updateMemberStatus = useCallback(
    (memberId: string, status: ChannelMember['status']) => {
      setMembers((prev) =>
        prev.map((m) =>
          m.member_id === memberId ? { ...m, status } : m,
        ),
      );
    },
    [],
  );

  const users = useMemo(() => members.filter((m) => m.member_type === 'user'), [members]);
  const agents = useMemo(() => members.filter((m) => m.member_type === 'agent'), [members]);

  return {
    members,
    users,
    agents,
    isLoading,
    error,
    addAgentToChannel,
    removeMember,
    updateMemberStatus,
    refetch: loadMembers,
  } as const;
}
