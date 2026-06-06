'use client';

import { Suspense, useEffect, useState, useCallback } from 'react';
import { useSearchParams } from 'next/navigation';
import { FolderOpen, FileText, AlertCircle, RefreshCw } from 'lucide-react';
import { FileTree } from '@/components/workspace/file-tree';
import { FilePreview } from '@/components/workspace/file-preview';
import { AgentSelector } from '@/components/workspace/agent-selector';
import { Breadcrumb } from '@/components/workspace/breadcrumb';
import { useWorkspaceFiles } from '@/lib/hooks/use-workspace-files';
import { Skeleton } from '@/components/ui/skeleton';

function WorkspacePageContent() {
  const searchParams = useSearchParams();
  const agentId = searchParams.get('agent');
  const { tree, isLoading, error, loadTree, fetchFileContent } = useWorkspaceFiles(agentId || '');

  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState<string | null>(null);
  const [isContentLoading, setIsContentLoading] = useState(false);
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());

  // Load tree on mount / agent change
  useEffect(() => {
    if (!agentId) return;
    loadTree();
  }, [agentId, loadTree]);

  // Persist expanded paths
  useEffect(() => {
    if (!agentId) return;
    try {
      const saved = localStorage.getItem(`ws-expand-${agentId}`);
      if (saved) setExpandedPaths(new Set(JSON.parse(saved)));
    } catch {}
  }, [agentId]);

  useEffect(() => {
    if (!agentId) return;
    try {
      localStorage.setItem(`ws-expand-${agentId}`, JSON.stringify([...expandedPaths]));
    } catch {}
  }, [expandedPaths, agentId]);

  const handleSelect = useCallback(async (filePath: string, _type: 'file' | 'directory') => {
    if (!agentId) return;
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
  }, [agentId, fetchFileContent]);

  const handleToggleExpand = useCallback((path: string) => {
    setExpandedPaths((prev) => {
      const next = new Set(prev);
      next.has(path) ? next.delete(path) : next.add(path);
      return next;
    });
  }, []);

  const handleLoadDirectory = useCallback((dirPath: string) => {
    if (!agentId) return;
    loadTree(dirPath);
  }, [agentId, loadTree]);

  // ---- Empty state: no agent selected ----
  if (!agentId) {
    return (
      <div className="flex flex-col items-center justify-center py-24">
        <div className="mb-4 flex h-16 w-16 items-center justify-center border-2 border-black bg-brutal-pink shadow-brutal-sm">
          <FolderOpen className="h-8 w-8 text-white" />
        </div>
        <h2 className="font-heading text-lg font-bold text-foreground">Workspace</h2>
        <p className="mt-2 font-mono text-sm text-muted-foreground">
          选择一个 Agent 浏览其工作空间文件
        </p>
        <div className="mt-6">
          <AgentSelector agentId={null} />
        </div>
      </div>
    );
  }

  // ---- Loading state ----
  if (isLoading && tree.length === 0) {
    return (
      <div className="p-4 space-y-2">
        <Skeleton className="h-6 w-48 rounded-none" />
        <Skeleton className="h-6 w-full rounded-none" />
        <Skeleton className="h-6 w-3/4 rounded-none" />
        <Skeleton className="h-6 w-2/3 rounded-none" />
      </div>
    );
  }

  // ---- Error state ----
  if (error && tree.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <div className="mb-3 flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-red-light shadow-brutal-sm">
          <AlertCircle className="h-6 w-6 text-brutal-red" />
        </div>
        <p className="font-body text-sm text-brutal-red">{error}</p>
        <button type="button" onClick={() => loadTree()} className="btn-brutal btn-brutal-sm mt-4">
          <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
          重试
        </button>
      </div>
    );
  }

  // ---- Empty workspace ----
  if (!isLoading && tree.length === 0) {
    return (
      <div className="flex flex-col h-[calc(100vh-64px)]">
        <div className="flex items-center justify-between px-4 py-2 bg-white border-b-2 border-black">
          <AgentSelector agentId={agentId} />
        </div>
        <div className="flex flex-1 items-center justify-center">
          <div className="text-center">
            <FolderOpen className="mx-auto h-10 w-10 text-muted-foreground" />
            <p className="mt-2 font-heading text-sm font-bold text-foreground">Agent workspace 尚无文件</p>
            <p className="mt-1 font-mono text-xs text-muted-foreground">
              运行 Agent 任务后文件将出现在此处
            </p>
          </div>
        </div>
      </div>
    );
  }

  // ---- Normal: tree + preview ----
  return (
    <div className="flex flex-col h-[calc(100vh-64px)]">
      <div className="flex items-center justify-between px-4 py-1.5 bg-white border-b-2 border-black">
        <AgentSelector agentId={agentId} />
        <button type="button" onClick={() => loadTree()} className="btn-brutal btn-brutal-xs">
          <RefreshCw className="h-3 w-3" />
        </button>
      </div>
      {selectedPath && (
        <Breadcrumb path={selectedPath} onNavigate={(p) => handleSelect(p, 'directory')} />
      )}
      <div className="flex flex-1 overflow-hidden">
        <div className="w-[220px] flex-shrink-0 overflow-y-auto border-r-2 border-black bg-white">
          <div className="border-b-2 border-black px-3 py-2">
            <span className="font-heading text-[10px] font-bold text-muted-foreground uppercase tracking-wider">
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
        <div className="flex-1 overflow-y-auto bg-brutal-cream">
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

export default function WorkspacePage() {
  return (
    <Suspense fallback={<div className="p-8"><Skeleton className="h-96 w-full rounded-none" /></div>}>
      <WorkspacePageContent />
    </Suspense>
  );
}
