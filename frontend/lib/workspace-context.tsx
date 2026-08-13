'use client';

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { apiClient, getActiveWorkspaceId, PUBLIC_WORKSPACE_ID, setStoredActiveWorkspaceId } from './api-client';
import { useAuth } from './auth-context';

export type WorkspaceRole = 'owner' | 'admin' | 'member';

export interface Workspace {
  id: string;
  name: string;
  icon: string;
  visibility: 'public' | 'private';
  is_default: boolean;
  is_personal: boolean;
  member_count: number;
  role: WorkspaceRole;
  created_by?: string;
  created_at: string;
  updated_at: string;
}

interface WorkspaceContextValue {
  workspaces: Workspace[];
  activeWorkspace: Workspace | null;
  isLoading: boolean;
  error: string | null;
  switchWorkspace: (workspaceId: string) => void;
  createWorkspace: (name: string, icon?: string) => Promise<Workspace>;
  deleteWorkspace: (workspaceId: string) => Promise<void>;
  refetch: () => Promise<void>;
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null);

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [activeId, setActiveId] = useState(PUBLIC_WORKSPACE_ID);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!isAuthenticated) {
      setWorkspaces([]);
      setIsLoading(false);
      return;
    }
    setIsLoading(true);
    try {
      const items = await apiClient.get<Workspace[]>('/api/v1/workspaces');
      const remembered = getActiveWorkspaceId();
      const next = items.find((item) => item.id === remembered)
        ?? items.find((item) => item.is_personal)
        ?? items.find((item) => item.is_default)
        ?? items[0]
        ?? null;
      if (next) setStoredActiveWorkspaceId(next.id);
      setActiveId(next?.id ?? PUBLIC_WORKSPACE_ID);
      setWorkspaces(items);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load Workspaces');
    } finally {
      setIsLoading(false);
    }
  }, [isAuthenticated]);

  useEffect(() => {
    if (!authLoading) void load();
  }, [authLoading, load]);

  const switchWorkspace = useCallback((workspaceId: string) => {
    if (!workspaces.some((item) => item.id === workspaceId)) return;
    setStoredActiveWorkspaceId(workspaceId);
    setActiveId(workspaceId);
  }, [workspaces]);

  const createWorkspace = useCallback(async (name: string, icon?: string) => {
    const workspace = await apiClient.post<Workspace>('/api/v1/workspaces', { name, icon });
    setWorkspaces((current) => [...current, workspace]);
    setStoredActiveWorkspaceId(workspace.id);
    setActiveId(workspace.id);
    return workspace;
  }, []);

  const deleteWorkspace = useCallback(async (workspaceId: string) => {
    await apiClient.delete(`/api/v1/workspaces/${workspaceId}`);
    setWorkspaces((current) => current.filter((item) => item.id !== workspaceId));
    if (activeId === workspaceId) {
      const fallback = workspaces.find((item) => item.id !== workspaceId && item.is_personal)
        ?? workspaces.find((item) => item.id !== workspaceId && item.is_default);
      setStoredActiveWorkspaceId(fallback?.id ?? PUBLIC_WORKSPACE_ID);
      setActiveId(fallback?.id ?? PUBLIC_WORKSPACE_ID);
    }
  }, [activeId, workspaces]);

  const activeWorkspace = workspaces.find((item) => item.id === activeId) ?? null;
  const value = useMemo(() => ({ workspaces, activeWorkspace, isLoading, error, switchWorkspace, createWorkspace, deleteWorkspace, refetch: load }), [workspaces, activeWorkspace, isLoading, error, switchWorkspace, createWorkspace, deleteWorkspace, load]);

  return (
    <WorkspaceContext.Provider value={value}>
      <div key={activeId} className="contents">{children}</div>
    </WorkspaceContext.Provider>
  );
}

export function useWorkspace(): WorkspaceContextValue {
  const value = useContext(WorkspaceContext);
  if (!value) throw new Error('useWorkspace must be used within WorkspaceProvider');
  return value;
}
