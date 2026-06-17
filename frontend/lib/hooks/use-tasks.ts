// ============================================================================
// useTasks — Task CRUD hook backed by REST API
// - List tasks with optional status/claimer/channel filters
// - Get single task
// - Create, update, delete tasks
// - Claim / unclaim tasks
// - Convert message to task (asTask)
// ============================================================================

'use client';

import { t } from '@/lib/i18n';
import { useState, useEffect, useCallback, useRef } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';
import { useWebSocket } from '@/lib/ws-context';
import type { Task, TaskStatus } from '@/lib/types';

// ---- Backend response shapes (匹配 Phase 1 Claim API) ----

interface TaskResponse {
  id: string;
  task_number: number;
  channel_id: string;
  creator_id: string;
  title: string;
  description: string;
  status: string;
  claimer_id: string;
  claimer_name?: string;
  claimer_deleted?: boolean;
  creator_name?: string;
  priority: string;
  due_date: string | null;
  message_id: string;
  parent_task_id?: string | null;
  subtask_count?: number;
  done_subtask_count?: number;
  blocker_ids?: string[];
  blocked_by_count?: number;
  blocking_count?: number;
  worktree_path?: string;
  created_at: string;
  updated_at: string;
}

// ---- Mapping helpers ----

function mapTask(resp: TaskResponse): Task {
  return {
    id: resp.id,
    channel_id: resp.channel_id,
    title: resp.title,
    description: resp.description || '',
    status: (resp.status === 'cancelled' ? 'closed' : resp.status) as Task['status'],
    priority: resp.priority as Task['priority'],
    task_number: resp.task_number || undefined,
    claimer_id: resp.claimer_id || undefined,
    claimer_name: resp.claimer_name || undefined,
    claimer_deleted: resp.claimer_deleted,
    creator_id: resp.creator_id,
    creator_name: resp.creator_name || undefined,
    message_id: resp.message_id || undefined,
    reply_count: (resp as TaskResponse & { reply_count?: number }).reply_count,
    parent_task_id: resp.parent_task_id ?? undefined,
    subtask_count: resp.subtask_count,
    done_subtask_count: resp.done_subtask_count,
    due_date: resp.due_date || undefined,
    blocker_ids: resp.blocker_ids,
    blocked_by_count: resp.blocked_by_count,
    blocking_count: resp.blocking_count,
    worktree_path: resp.worktree_path,
    created_at: resp.created_at,
    updated_at: resp.updated_at,
  };
}

// ---- Hook ----

interface TaskFilters {
  status?: TaskStatus;
  claimer_id?: string;
  channel_id?: string;
}

export function useTasks(filters?: TaskFilters) {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  // Resolve filters to stable reference for useCallback deps
  const filtersKey = `${filters?.status ?? ''}|${filters?.claimer_id ?? ''}|${filters?.channel_id ?? ''}`;

  const loadTasks = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const params: Record<string, string> = {};
      if (filters?.status) params.status = filters.status;
      if (filters?.claimer_id) params.claimer_id = filters.claimer_id;
      if (filters?.channel_id) params.channel_id = filters.channel_id;

      const query = new URLSearchParams(params).toString();
      const path = query ? `/api/v1/tasks?${query}` : '/api/v1/tasks';
      const res = await apiClient.get<TaskResponse[]>(path);
      if (mountedRef.current) {
        setTasks(Array.isArray(res) ? res.map(mapTask) : []);
      }
    } catch (err) {
      const message = err instanceof ApiError ? err.message : `${t('taskLoadError')}`;
      if (mountedRef.current) setError(message);
    } finally {
      if (mountedRef.current) setIsLoading(false);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filtersKey]);

  useEffect(() => {
    mountedRef.current = true;
    loadTasks();
    return () => { mountedRef.current = false; };
  }, [loadTasks]);

  // ---- WebSocket subscription for real-time task events ----
  const channelFilter = filters?.channel_id;
  const { subscribe, unsubscribe, onEvent } = useWebSocket();

  // Ensure the client is subscribed to the channel so task events arrive.
  useEffect(() => {
    if (!channelFilter) return;
    subscribe(channelFilter);
    return () => { unsubscribe(channelFilter); };
  }, [channelFilter, subscribe, unsubscribe]);

  useEffect(() => {
    const unsub = onEvent((event) => {
      if (event.type === 'task.created') {
        // Filter by channel if one is specified
        if (channelFilter && event.channel_id !== channelFilter) return;
        // Filter by status if one is specified
        if (filters?.status && event.status !== filters.status) return;

        setTasks((prev) => {
          // Avoid duplicates from WS race
          if (prev.find((t) => t.id === event.id)) return prev;
          return [mapTask({
            id: event.id,
            task_number: event.task_number,
            channel_id: event.channel_id,
            creator_id: event.creator_id,
            creator_name: (event as { creator_name?: string }).creator_name || undefined,
            title: event.title,
            description: event.description ?? '',
            status: event.status,
            claimer_id: event.claimer_id ?? '',
            claimer_name: (event as { claimer_name?: string }).claimer_name || undefined,
            claimer_deleted: (event as { claimer_deleted?: boolean }).claimer_deleted,
            priority: event.priority ?? 'normal',
            due_date: event.due_date ?? null,
            message_id: event.message_id ?? '',
            parent_task_id: (event as { parent_task_id?: string }).parent_task_id ?? null,
            subtask_count: (event as { subtask_count?: number }).subtask_count ?? 0,
            done_subtask_count: (event as { done_subtask_count?: number }).done_subtask_count ?? 0,
            blocker_ids: (event as { blocker_ids?: string[] }).blocker_ids,
            blocked_by_count: (event as { blocked_by_count?: number }).blocked_by_count,
            created_at: event.created_at,
            updated_at: event.updated_at,
          }), ...prev];
        });
        return;
      }

      if (event.type === 'task.updated') {
        if (channelFilter && event.channel_id !== channelFilter) return;

        setTasks((prev) => {
          const existing = prev.find((t) => t.id === event.id);
          if (!existing) return prev; // task not in current filtered list

          const updated = { ...existing };
          updated.title = event.title;
          if (event.description !== undefined) updated.description = event.description;
          updated.status = (event.status === 'cancelled' ? 'closed' : event.status) as Task['status'];
          updated.task_number = event.task_number;
          if (event.claimer_id !== undefined) updated.claimer_id = event.claimer_id || undefined;
          if (event.claimer_name !== undefined) updated.claimer_name = event.claimer_name;
          if (event.claimer_deleted !== undefined) updated.claimer_deleted = event.claimer_deleted;
          if (event.priority !== undefined) updated.priority = event.priority as Task['priority'];
          if (event.due_date !== undefined) updated.due_date = event.due_date || undefined;
          if (event.message_id !== undefined) updated.message_id = event.message_id || undefined;
          const evt = event as { parent_task_id?: string; subtask_count?: number; done_subtask_count?: number; blocker_ids?: string[]; blocked_by_count?: number };
          if (evt.parent_task_id !== undefined) updated.parent_task_id = evt.parent_task_id || undefined;
          if (evt.subtask_count !== undefined) updated.subtask_count = evt.subtask_count;
          if (evt.done_subtask_count !== undefined) updated.done_subtask_count = evt.done_subtask_count;
          if (evt.blocker_ids !== undefined) updated.blocker_ids = evt.blocker_ids;
          if (evt.blocked_by_count !== undefined) updated.blocked_by_count = evt.blocked_by_count;
          updated.updated_at = event.updated_at;

          // If status filter is active and the updated task no longer matches, remove it
          if (filters?.status && updated.status !== filters.status) {
            return prev.filter((t) => t.id !== event.id);
          }

          return prev.map((t) => (t.id === event.id ? updated : t));
        });
        return;
      }

      if (event.type === 'task.deleted') {
        if (channelFilter && event.channel_id !== channelFilter) return;

        setTasks((prev) => prev.filter((t) => t.id !== event.id));
        return;
      }

      // Step 2: task.unblocked — granular update of the unblocked task
      // (T2.4.5) Auto-refresh task list when dependency is resolved.
      if (event.type === 'task.unblocked') {
        if (channelFilter && event.channel_id !== channelFilter) return;
        // Fetch just the unblocked task to get its updated blocker_ids/blocked_by_count
        const unblockedTaskId = event.blocked_task_id;
        apiClient.get<TaskResponse>(`/api/v1/tasks/${unblockedTaskId}`).then((resp) => {
          setTasks((prev) => {
            const idx = prev.findIndex((t) => t.id === unblockedTaskId);
            if (idx === -1) return prev;
            const updated = mapTask(resp);
            const copy = [...prev];
            copy[idx] = updated;
            return copy;
          });
        }).catch(() => {
          // Fall back to full refetch on fetch failure
          loadTasks();
        });
        return;
      }
    });

    return unsub;
  }, [channelFilter, filters?.status, onEvent, loadTasks]);

  const updateTask = useCallback(async (channelId: string, taskId: string, input: { status?: TaskStatus }): Promise<Task> => {
    const res = await apiClient.patch<TaskResponse>(`/api/v1/channels/${channelId}/tasks/${taskId}`, {
      status: input.status,
    });
    const updated = mapTask(res);
    setTasks((prev) => prev.map((t) => (t.id === taskId ? updated : t)));
    return updated;
  }, []);

  const deleteTask = useCallback(async (id: string) => {
    await apiClient.delete(`/api/v1/tasks/${id}`);
    setTasks((prev) => prev.filter((t) => t.id !== id));
  }, []);

  // ---- Claim / Unclaim ----

  const claimTask = useCallback(async (channelId: string, taskId: string): Promise<Task> => {
    const res = await apiClient.post<TaskResponse>(
      `/api/v1/channels/${channelId}/tasks/${taskId}/claim`,
    );
    const updated = mapTask(res);
    setTasks((prev) => prev.map((t) => (t.id === taskId ? updated : t)));
    return updated;
  }, []);

  const unclaimTask = useCallback(async (channelId: string, taskId: string): Promise<Task> => {
    const res = await apiClient.delete<TaskResponse>(
      `/api/v1/channels/${channelId}/tasks/${taskId}/claim`,
    );
    const updated = mapTask(res);
    setTasks((prev) => prev.map((t) => (t.id === taskId ? updated : t)));
    return updated;
  }, []);

  // ---- Convert message to task ----

  const convertMessageToTask = useCallback(
    async (channelId: string, messageId: string, title?: string): Promise<Task> => {
      const res = await apiClient.post<TaskResponse>(
        `/api/v1/channels/${channelId}/messages/${messageId}/convert-to-task`,
        { title },
      );
      const task = mapTask(res);
      setTasks((prev) => {
        // Avoid duplicate — WS task.created may have already added this task
        if (prev.find((t) => t.id === task.id)) return prev;
        return [task, ...prev];
      });
      return task;
    },
    [],
  );

  return {
    tasks,
    isLoading,
    error,
    updateTask,
    deleteTask,
    claimTask,
    unclaimTask,
    convertMessageToTask,
    refetch: loadTasks,
  } as const;
}

// ---- DM Tasks hook ----
// Uses DM-specific REST endpoints:
//   GET    /api/v1/dm/{dmID}/tasks          → list
//   POST   /api/v1/dm/{dmID}/tasks          → create
//   POST   /api/v1/dm/{dmID}/tasks/{id}/claim   → claim
//   DELETE /api/v1/dm/{dmID}/tasks/{id}/claim   → unclaim

interface DMTaskResponse {
  id: string;
  task_number: number;
  dm_id: string;
  creator_id: string;
  title: string;
  description: string;
  status: string;
  claimer_id: string;
  claimer_name?: string;
  claimer_deleted?: boolean;
  creator_name?: string;
  priority: string;
  due_date: string | null;
  message_id: string;
  created_at: string;
  updated_at: string;
}

function mapDMTask(resp: DMTaskResponse): Task {
  return {
    id: resp.id,
    channel_id: resp.dm_id,
    title: resp.title,
    description: resp.description || '',
    status: (resp.status === 'cancelled' ? 'closed' : resp.status) as Task['status'],
    priority: resp.priority as Task['priority'],
    task_number: resp.task_number || undefined,
    claimer_id: resp.claimer_id || undefined,
    claimer_name: resp.claimer_name || undefined,
    claimer_deleted: resp.claimer_deleted,
    creator_id: resp.creator_id,
    creator_name: resp.creator_name || undefined,
    message_id: resp.message_id || undefined,
    reply_count: (resp as DMTaskResponse & { reply_count?: number }).reply_count,
    due_date: resp.due_date || undefined,
    created_at: resp.created_at,
    updated_at: resp.updated_at,
  };
}

export function useDMTasks(dmId: string | null) {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  const loadTasks = useCallback(async () => {
    if (!dmId) {
      setTasks([]);
      setIsLoading(false);
      return;
    }
    setTasks([]);
    setError(null);
    setIsLoading(true);
    try {
      const res = await apiClient.get<DMTaskResponse[]>(`/api/v1/dm/${dmId}/tasks`);
      if (mountedRef.current) {
        setTasks(Array.isArray(res) ? res.map(mapDMTask) : []);
      }
    } catch (err) {
      const message = err instanceof ApiError ? err.message : `${t('taskLoadError')}`;
      if (mountedRef.current) setError(message);
    } finally {
      if (mountedRef.current) setIsLoading(false);
    }
  }, [dmId]);

  useEffect(() => {
    mountedRef.current = true;
    loadTasks();
    return () => { mountedRef.current = false; };
  }, [loadTasks]);

  // ---- WebSocket subscription for real-time DM task events ----
  const { subscribeDM, unsubscribeDM, onEvent: dmOnEvent } = useWebSocket();

  // Ensure the client is subscribed to the DM so task events arrive.
  useEffect(() => {
    if (!dmId) return;
    subscribeDM(dmId);
    return () => { unsubscribeDM(dmId); };
  }, [dmId, subscribeDM, unsubscribeDM]);

  useEffect(() => {
    const unsub = dmOnEvent((event) => {
      if (!dmId) return;

      if (event.type === 'task.created') {
        if (event.channel_id !== dmId) return;
        setTasks((prev) => {
          if (prev.find((t) => t.id === event.id)) return prev;
          return [mapDMTask({
            id: event.id,
            task_number: event.task_number,
            dm_id: event.channel_id,
            creator_id: event.creator_id,
            creator_name: (event as { creator_name?: string }).creator_name || undefined,
            title: event.title,
            description: event.description ?? '',
            status: event.status,
            claimer_id: event.claimer_id ?? '',
            claimer_name: (event as { claimer_name?: string }).claimer_name || undefined,
            claimer_deleted: (event as { claimer_deleted?: boolean }).claimer_deleted,
            priority: event.priority ?? 'normal',
            due_date: event.due_date ?? '',
            message_id: event.message_id ?? '',
            created_at: event.created_at,
            updated_at: event.updated_at,
          }), ...prev];
        });
        return;
      }

      if (event.type === 'task.updated') {
        if (process.env.NODE_ENV === 'development') {
          console.log('[useDMTasks] task.updated received:', { eventChannelId: event.channel_id, dmId, match: event.channel_id === dmId, eventId: event.id, eventStatus: event.status });
        }
        if (event.channel_id !== dmId) return;
        setTasks((prev) => {
          const existing = prev.find((t) => t.id === event.id);
          if (process.env.NODE_ENV === 'development') {
            console.log('[useDMTasks] task.updated existing:', !!existing, 'prevCount:', prev.length);
          }
          if (!existing) return prev;
          const updated = { ...existing };
          updated.title = event.title;
          if (event.description !== undefined) updated.description = event.description;
          updated.status = (event.status === 'cancelled' ? 'closed' : event.status) as Task['status'];
          updated.task_number = event.task_number;
          if (event.claimer_id !== undefined) updated.claimer_id = event.claimer_id || undefined;
          if (event.claimer_name !== undefined) updated.claimer_name = event.claimer_name;
          if (event.claimer_deleted !== undefined) updated.claimer_deleted = event.claimer_deleted;
          if (event.priority !== undefined) updated.priority = event.priority as Task['priority'];
          if (event.due_date !== undefined) updated.due_date = event.due_date || undefined;
          if (event.message_id !== undefined) updated.message_id = event.message_id || undefined;
          updated.updated_at = event.updated_at;
          return prev.map((t) => (t.id === event.id ? updated : t));
        });
        return;
      }

      if (event.type === 'task.deleted') {
        if (event.channel_id !== dmId) return;
        setTasks((prev) => prev.filter((t) => t.id !== event.id));
      }
    });
    return unsub;
  }, [dmId, dmOnEvent]);

  const updateTask = useCallback(async (id: string, input: { status?: TaskStatus }): Promise<Task> => {
    // DM tasks go through the global endpoint since they exist in a DM scope
    const res = await apiClient.patch<TaskResponse>(`/api/v1/tasks/${id}`, {
      status: input.status,
    });
    const updated = mapTask(res);
    setTasks((prev) => prev.map((t) => (t.id === id ? updated : t)));
    return updated;
  }, []);

  const claimTask = useCallback(async (dmId: string, taskId: string): Promise<Task> => {
    const res = await apiClient.post<DMTaskResponse>(
      `/api/v1/dm/${dmId}/tasks/${taskId}/claim`,
    );
    const updated = mapDMTask(res);
    setTasks((prev) => prev.map((t) => (t.id === taskId ? updated : t)));
    return updated;
  }, []);

  const unclaimTask = useCallback(async (dmId: string, taskId: string): Promise<Task> => {
    const res = await apiClient.delete<DMTaskResponse>(
      `/api/v1/dm/${dmId}/tasks/${taskId}/claim`,
    );
    const updated = mapDMTask(res);
    setTasks((prev) => prev.map((t) => (t.id === taskId ? updated : t)));
    return updated;
  }, []);

  // ---- DM convert message to task ----
  // POST /api/v1/dm/{dmID}/messages/{messageID}/convert-to-task

  const convertMessageToTask = useCallback(
    async (dmId: string, messageId: string, title?: string): Promise<Task> => {
      const res = await apiClient.post<DMTaskResponse>(
        `/api/v1/dm/${dmId}/messages/${messageId}/convert-to-task`,
        { title },
      );
      const task = mapDMTask(res);
      setTasks((prev) => {
        if (prev.find((t) => t.id === task.id)) return prev;
        return [task, ...prev];
      });
      return task;
    },
    [],
  );

  return {
    tasks,
    isLoading,
    error,
    updateTask,
    claimTask,
    unclaimTask,
    convertMessageToTask,
    refetch: loadTasks,
  } as const;
}

