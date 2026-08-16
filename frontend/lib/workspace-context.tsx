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
import { apiClient, getStoredActiveWorkspaceIdForUser, PUBLIC_WORKSPACE_ID, setStoredActiveWorkspaceId } from './api-client';
import { useAuth } from './auth-context';

export type WorkspaceRole = 'owner' | 'admin' | 'member';

export interface Workspace {
  id: string;
  name: string;
  icon: string;
  visibility: 'public' | 'private';
  is_default: boolean;
  is_personal: boolean;
  lucy_channel_id?: string;
  member_count: number;
  role: WorkspaceRole;
  created_by?: string;
  created_at: string;
  updated_at: string;
}

export type ManageTabKey = 'overview' | 'members' | 'invites';

interface ManageDialogState {
  open: boolean;
  tab: ManageTabKey;
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
  manageDialog: ManageDialogState;
  openManage: (tab?: ManageTabKey) => void;
  closeManage: () => void;
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null);

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const { user, isAuthenticated, isLoading: authLoading } = useAuth();
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [activeId, setActiveId] = useState(PUBLIC_WORKSPACE_ID);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [manageDialog, setManageDialog] = useState<ManageDialogState>({ open: false, tab: 'overview' });

  const load = useCallback(async () => {
    if (!isAuthenticated) {
      setWorkspaces([]);
      setIsLoading(false);
      return;
    }
    setIsLoading(true);
    try {
      const items = await apiClient.get<Workspace[]>('/api/v1/workspaces');
      const remembered = user ? getStoredActiveWorkspaceIdForUser(user.id) : null;
      const next = items.find((item) => item.id === remembered)
        ?? items.find((item) => item.is_personal)
        ?? items.find((item) => item.is_default)
        ?? items[0]
        ?? null;
      if (next) setStoredActiveWorkspaceId(next.id, user?.id);
      setActiveId(next?.id ?? PUBLIC_WORKSPACE_ID);
      setWorkspaces(items);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load Workspaces');
    } finally {
      setIsLoading(false);
    }
  }, [isAuthenticated, user]);

  useEffect(() => {
    if (!authLoading) void load();
  }, [authLoading, load]);

  const switchWorkspace = useCallback((workspaceId: string) => {
    setStoredActiveWorkspaceId(workspaceId, user?.id);
    setActiveId(workspaceId);
  }, [user?.id]);

  const createWorkspace = useCallback(async (name: string, icon?: string) => {
    const workspace = await apiClient.post<Workspace>('/api/v1/workspaces', { name, icon });
    setWorkspaces((current) => [...current, workspace]);
    setStoredActiveWorkspaceId(workspace.id, user?.id);
    setActiveId(workspace.id);
    return workspace;
  }, [user?.id]);

  const deleteWorkspace = useCallback(async (workspaceId: string) => {
    await apiClient.delete(`/api/v1/workspaces/${workspaceId}`);
    setWorkspaces((current) => current.filter((item) => item.id !== workspaceId));
    if (activeId === workspaceId) {
      const fallback = workspaces.find((item) => item.id !== workspaceId && item.is_personal)
        ?? workspaces.find((item) => item.id !== workspaceId && item.is_default);
      setStoredActiveWorkspaceId(fallback?.id ?? PUBLIC_WORKSPACE_ID, user?.id);
      setActiveId(fallback?.id ?? PUBLIC_WORKSPACE_ID);
    }
  }, [activeId, user?.id, workspaces]);

  const activeWorkspace = workspaces.find((item) => item.id === activeId) ?? null;
  const openManage = useCallback((tab: ManageTabKey = 'overview') => {
    setManageDialog({ open: true, tab });
  }, []);
  const closeManage = useCallback(() => {
    setManageDialog((prev) => ({ ...prev, open: false }));
  }, []);
  const value = useMemo(() => ({
    workspaces, activeWorkspace, isLoading, error,
    switchWorkspace, createWorkspace, deleteWorkspace, refetch: load,
    manageDialog, openManage, closeManage,
  }), [workspaces, activeWorkspace, isLoading, error, switchWorkspace, createWorkspace, deleteWorkspace, load, manageDialog, openManage, closeManage]);

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
