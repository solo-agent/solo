// ============================================================================
// useWorkspaceFiles — fetch Agent workspace file tree (v1.5)
// - GET /api/v1/agents/{id}/workspace?path=
// - Returns file tree structure, loading/error states
// ============================================================================

'use client';

import { useState, useCallback, useRef } from 'react';
import { apiClient, ApiError } from '@/lib/api-client';

export interface WorkspaceFileNode {
  name: string;
  path: string;
  type: 'file' | 'directory';
  size?: number;
  children?: WorkspaceFileNode[];
}

interface WorkspaceListResponse {
  entries: WorkspaceFileNode[];
}

export function useWorkspaceFiles(agentId: string) {
  const [tree, setTree] = useState<WorkspaceFileNode[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const loadedRef = useRef(false);

  const loadTree = useCallback(async (dirPath?: string) => {
    if (!agentId) return;
    setIsLoading(true);
    setError(null);
    try {
      const params: Record<string, string> = {};
      if (dirPath) params.path = dirPath;
      const res = await apiClient.get<WorkspaceListResponse>(
        `/api/v1/agents/${agentId}/workspace`,
        Object.keys(params).length > 0 ? params : undefined,
      );
      if (dirPath) {
        // Partial update: replace children of the directory
        setTree((prev) => replaceChildrenInTree(prev, dirPath, res.entries ?? []));
      } else {
        setTree(res.entries ?? []);
      }
      loadedRef.current = true;
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError('加载工作空间文件失败');
      }
      if (!loadedRef.current) {
        setTree([]);
      }
    } finally {
      setIsLoading(false);
    }
  }, [agentId]);

  const fetchFileContent = useCallback(async (filePath: string): Promise<string> => {
    const res = await apiClient.get<{ content: string }>(
      `/api/v1/agents/${agentId}/workspace`,
      { path: filePath },
    );
    return res.content ?? '';
  }, [agentId]);

  return { tree, isLoading, error, loadTree, fetchFileContent } as const;
}

// ---- Helper: replace children of a directory node in the tree ----

function replaceChildrenInTree(
  nodes: WorkspaceFileNode[],
  dirPath: string,
  newChildren: WorkspaceFileNode[],
): WorkspaceFileNode[] {
  return nodes.map((node) => {
    if (node.type === 'directory' && node.path === dirPath) {
      return { ...node, children: newChildren };
    }
    if (node.children && dirPath.startsWith(node.path + '/')) {
      return { ...node, children: replaceChildrenInTree(node.children, dirPath, newChildren) };
    }
    return node;
  });
}
