// ============================================================================
// TeamsAgentWorkspace — Workspace tab content for an agent on /teams.
// Top bar: agent's workspace path + refresh button.
// Body:   file tree (left, 260px) + file preview with Shiki (right, flex).
// Read-only — no edit, no upload, no delete.
// ============================================================================

'use client';

import { useState, useEffect, useCallback } from 'react';
import { RefreshCw, AlertCircle, FolderOpen, FileText } from 'lucide-react';
import { useWorkspaceFiles } from '@/lib/hooks/use-workspace-files';
import { FileTree } from '@/components/workspace/file-tree';
import { FilePreview } from '@/components/workspace/file-preview';
import { Skeleton } from '@/components/ui/skeleton';

interface TeamsAgentWorkspaceProps {
  agentId: string;
}

export function TeamsAgentWorkspace({ agentId }: TeamsAgentWorkspaceProps) {
  const { tree, isLoading, error, loadTree, fetchFileContent } = useWorkspaceFiles(agentId);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState<string | null>(null);
  const [isContentLoading, setIsContentLoading] = useState(false);
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());

  // Persist expanded paths per agent
  useEffect(() => {
    try {
      const saved = localStorage.getItem(`ws-expand-${agentId}`);
      if (saved) setExpandedPaths(new Set(JSON.parse(saved)));
    } catch { /* ignore */ }
  }, [agentId]);

  useEffect(() => {
    try {
      localStorage.setItem(`ws-expand-${agentId}`, JSON.stringify([...expandedPaths]));
    } catch { /* ignore */ }
  }, [expandedPaths, agentId]);

  const handleSelect = useCallback(
    async (filePath: string, _type?: 'file' | 'directory') => {
      setSelectedPath(filePath);
      setIsContentLoading(true);
      try {
        const content = await fetchFileContent(filePath);
        setFileContent(content);
      } catch {
        setFileContent(null);
      } finally {
        setIsContentLoading(false);
      }
    },
    [fetchFileContent],
  );

  const handleToggleExpand = useCallback((path: string) => {
    setExpandedPaths((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  }, []);

  const handleLoadDirectory = useCallback(
    (dirPath: string) => {
      loadTree(dirPath);
    },
    [loadTree],
  );

  // ---- Loading (initial) ----
  if (isLoading && tree.length === 0) {
    return (
      <div className="space-y-2 p-4">
        <Skeleton className="h-6 w-3/4 rounded-none" />
        <Skeleton className="h-6 w-1/2 rounded-none" />
        <Skeleton className="h-6 w-2/3 rounded-none" />
      </div>
    );
  }

  // ---- Error ----
  if (error && tree.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <div className="mb-3 flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-red-light shadow-brutal-sm">
          <AlertCircle className="h-6 w-6 text-brutal-red" />
        </div>
        <p className="font-body text-sm text-brutal-red">{error}</p>
        <button
          type="button"
          onClick={() => loadTree()}
          className="btn-brutal btn-brutal-sm mt-4"
        >
          <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
          重试
        </button>
      </div>
    );
  }

  // ---- Empty ----
  if (!isLoading && tree.length === 0) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="text-center">
          <FolderOpen className="mx-auto h-10 w-10 text-muted-foreground" />
          <p className="mt-2 font-heading text-sm font-bold">Agent workspace 尚无文件</p>
          <p className="mt-1 font-mono text-xs text-muted-foreground">
            运行 Agent 任务后文件将出现在此处
          </p>
        </div>
      </div>
    );
  }

  // ---- Normal: path bar + tree + preview ----
  return (
    <div className="flex h-full flex-col">
      {/* Path bar */}
      <div className="flex items-center justify-between border-b-2 border-black bg-white px-3 py-1.5">
        <div className="flex min-w-0 items-center gap-2 font-mono text-[11px] text-muted-foreground">
          <FolderOpen className="h-3.5 w-3.5 flex-shrink-0" />
          <span className="truncate">
            agents/<span className="font-bold text-foreground">{agentId.slice(0, 8)}</span>/workspace
          </span>
        </div>
        <button
          type="button"
          onClick={() => loadTree()}
          className="btn-brutal btn-brutal-xs"
          aria-label="刷新文件树"
        >
          <RefreshCw className="h-3 w-3" />
        </button>
      </div>

      {/* Tree + preview split */}
      <div className="flex flex-1 overflow-hidden">
        <div className="h-full w-[260px] flex-shrink-0 overflow-y-auto border-r-2 border-black bg-white">
          <div className="border-b-2 border-black px-3 py-2">
            <span className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
              文件
            </span>
          </div>
          <FileTree
            tree={tree}
            selectedPath={selectedPath}
            expandedPaths={expandedPaths}
            onSelect={handleSelect}
            onToggleExpand={handleToggleExpand}
            onLoadDirectory={handleLoadDirectory}
          />
        </div>
        <div className="h-full flex-1 overflow-y-auto bg-brutal-cream">
          {selectedPath ? (
            <FilePreview
              path={selectedPath}
              content={fileContent}
              isLoading={isContentLoading}
            />
          ) : (
            <div className="flex h-full items-center justify-center">
              <div className="text-center">
                <FileText className="mx-auto h-6 w-6 text-muted-foreground" />
                <p className="mt-2 font-mono text-xs text-muted-foreground">
                  选择文件以预览内容
                </p>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
