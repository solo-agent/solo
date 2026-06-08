// ============================================================================
// SOLO-54-F: useDMs — DM conversation list hook with mock data fallback
// ============================================================================
// Backend API not yet available — uses mock data and simulates async fetch.
// When the backend is ready, replace mock data with real apiClient calls.
// Mock API endpoints (future):
//   GET  /api/v1/dms          → list DM conversations
//   POST /api/v1/dms          → create a new DM
//   GET  /api/v1/dms/{id}     → single DM conversation detail
// ============================================================================

'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import { t } from '@/lib/i18n';
import type { DMChannel, CreateDMInput } from '@/lib/types';

// ---- Mock data ----

const MOCK_DMS: DMChannel[] = [
  {
    id: 'dm-1',
    type: 'dm',
    other_user: { id: 'user-2', display_name: 'Alice' },
    last_message: {
      content: "Don't forget the meeting tomorrow afternoon",
      sender_id: 'user-2',
      sender_name: 'Alice',
      created_at: new Date(Date.now() - 1000 * 60 * 5).toISOString(),
    },
    last_reply_at: new Date(Date.now() - 1000 * 60 * 5).toISOString(),
    unread_count: 1,
    created_at: new Date(Date.now() - 86400000 * 3).toISOString(),
  },
  {
    id: 'dm-2',
    type: 'dm',
    other_agent: { id: 'agent-1', name: 'AI Assistant' },
    last_message: {
      content: "I've analyzed the data. Here's the detailed report...",
      sender_id: 'agent-1',
      sender_name: 'AI Assistant',
      created_at: new Date(Date.now() - 1000 * 60 * 30).toISOString(),
    },
    last_reply_at: new Date(Date.now() - 1000 * 60 * 30).toISOString(),
    unread_count: 0,
    created_at: new Date(Date.now() - 86400000 * 7).toISOString(),
  },
  {
    id: 'dm-3',
    type: 'dm',
    other_user: { id: 'user-3', display_name: 'Bob' },
    last_message: {
      content: 'Got it, thanks!',
      sender_id: 'user-1',
      sender_name: 'Me',
      created_at: new Date(Date.now() - 86400000).toISOString(),
    },
    last_reply_at: new Date(Date.now() - 86400000).toISOString(),
    unread_count: 0,
    created_at: new Date(Date.now() - 86400000 * 14).toISOString(),
  },
];

// ---- Participants list for DM creation (users + agents) ----

export interface DMCreateParticipant {
  id: string;
  type: 'user' | 'agent';
  display_name: string;
  online: boolean;
}

const MOCK_PARTICIPANTS: DMCreateParticipant[] = [
  { id: 'user-2', type: 'user', display_name: 'Alice', online: true },
  { id: 'user-3', type: 'user', display_name: 'Bob', online: false },
  { id: 'user-4', type: 'user', display_name: 'Charlie', online: true },
  { id: 'user-5', type: 'user', display_name: 'Diana', online: false },
  { id: 'agent-1', type: 'agent', display_name: 'AI Assistant', online: true },
  { id: 'agent-2', type: 'agent', display_name: 'Data Analyst', online: true },
  { id: 'agent-3', type: 'agent', display_name: 'Code Reviewer', online: false },
];

// ---- Simulate async network delay ----

function delay(ms = 400): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// ---- Hook ----

export function useDMs() {
  const [dms, setDms] = useState<DMChannel[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  const loadDMs = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      // TODO: Replace with real API call when backend is ready:
      //   const res = await apiClient.get<DMChannel[]>('/api/v1/dms');
      await delay();
      if (mountedRef.current) {
        setDms(MOCK_DMS);
      }
    } catch (err) {
      const message = err instanceof ApiError ? err.message : t('dmLoadError');
      if (mountedRef.current) {
        setError(message);
      }
    } finally {
      if (mountedRef.current) {
        setIsLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    loadDMs();
    return () => {
      mountedRef.current = false;
    };
  }, [loadDMs]);

  /** Create a new DM or return existing one if it already exists. */
  const createDM = useCallback(
    async (input: CreateDMInput): Promise<DMChannel> => {
      // TODO: Replace with real API call:
      //   const res = await apiClient.post<DMChannel>('/api/v1/dms', input);
      await delay();

      // Mock: check if DM already exists with this participant
      const existingId =
        input.user_id
          ? MOCK_DMS.find(
              (d) => d.other_user && d.other_user.id === input.user_id,
            )?.id
          : input.agent_id
            ? MOCK_DMS.find(
                (d) => d.other_agent && d.other_agent.id === input.agent_id,
              )?.id
            : undefined;

      if (existingId) {
        // Return existing DM
        const existing = MOCK_DMS.find((d) => d.id === existingId)!;
        // Add to local state if not already present
        setDms((prev) => {
          if (prev.some((d) => d.id === existing.id)) return prev;
          return [...prev, existing];
        });
        return existing;
      }

      // Create new mock DM
      const newId = `dm-${Date.now()}`;
      const participant = input.user_id
        ? MOCK_PARTICIPANTS.find((p) => p.id === input.user_id)
        : MOCK_PARTICIPANTS.find((p) => p.id === input.agent_id);
      const newDM: DMChannel = {
        id: newId,
        type: 'dm',
        ...(input.user_id
          ? { other_user: { id: input.user_id, display_name: participant?.display_name ?? t('user') } }
          : { other_agent: { id: input.agent_id!, name: participant?.display_name ?? 'Agent' } }),
        last_reply_at: new Date().toISOString(),
        unread_count: 0,
        created_at: new Date().toISOString(),
      };

      setDms((prev) => [...prev, newDM]);
      // Also add to mock list for future lookups
      MOCK_DMS.push(newDM);

      return newDM;
    },
    [],
  );

  /** Mark a DM's unread count as 0 (called when user opens a DM) */
  const markAsRead = useCallback((dmId: string) => {
    setDms((prev) =>
      prev.map((dm) =>
        dm.id === dmId ? { ...dm, unread_count: 0 } : dm,
      ),
    );
  }, []);

  return {
    dms,
    isLoading,
    error,
    createDM,
    markAsRead,
    refetch: loadDMs,
    /** Available participants for creating new DMs */
    participants: MOCK_PARTICIPANTS,
  } as const;
}
