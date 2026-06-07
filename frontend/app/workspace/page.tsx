'use client';

import { useState, useEffect, useCallback } from 'react';
import { ChevronDown, FolderOpen, FileText, AlertCircle, RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';
import { NavBar } from '@/components/ui/navbar';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { FileTree } from '@/components/workspace/file-tree';
import { FilePreview } from '@/components/workspace/file-preview';
import { ResizablePanel } from '@/components/workspace/resizable-panel';
import { Breadcrumb } from '@/components/workspace/breadcrumb';
import { useWorkspaceFiles } from '@/lib/hooks/use-workspace-files';
import { useAgents } from '@/lib/hooks/use-agents';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { BrutalAlert } from '@/components/ui/brutal-alert';
import type { Agent } from '@/lib/types';

// ---- Left column: agent list ----

const SECTION_HEADER =
  'flex w-full items-center gap-1.5 px-3 py-2 text-left text-xs font-bold uppercase tracking-wider font-heading';
const SECTION_COUNT = 'ml-auto text-xs tabular-nums opacity-50';

function WorkspaceLeftColumn({
  agents,
  isLoading,
  error,
  onRetry,
  selectedAgentId,
  onSelectAgent,
}: {
  agents: Agent[];
  isLoading: boolean;
  error: string | null;
  onRetry: () => void;
  selectedAgentId: string | null;
  onSelectAgent: (agentId: string) => void;
}) {
  const [expanded, setExpanded] = useState(true);

  return (
    <div className="flex h-full flex-col overflow-hidden border-r-2 border-black bg-brutal-cream">
      {/* Page label */}
      <div className="flex items-center h-14 border-b-2 border-black px-4">
        <span className="font-heading text-lg font-bold">Workspace</span>
      </div>

      {/* Agents section */}
      <div className="flex-1 overflow-y-auto pt-0 pb-2">
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className={cn(SECTION_HEADER, 'text-muted-foreground')}
          aria-label="展开或折叠 Agents"
          aria-expanded={expanded}
        >
          <ChevronDown
            aria-hidden="true"
            className={cn(
              'h-3 w-3 transition-transform',
              expanded ? 'rotate-0' : '-rotate-90',
            )}
          />
          <span>Agents</span>
          <span className={SECTION_COUNT}>{agents.length}</span>
        </button>

        {expanded && (
          <div>
            {isLoading ? (
              <div className="space-y-1 px-2 py-1">
                {[1, 2, 3].map((i) => (
                  <Skeleton key={i} className="h-9 w-full rounded-none" />
                ))}
              </div>
            ) : error ? (
              <div className="px-4 py-2">
                <p className="font-mono text-[10px] text-brutal-red mb-1">{error}</p>
                <button
                  type="button"
                  onClick={onRetry}
                  className="font-mono text-[10px] font-bold underline hover:text-brutal-pink"
                >
                  重试
                </button>
              </div>
            ) : agents.length === 0 ? (
              <p className="px-6 py-2 font-mono text-[10px] italic text-muted-foreground">
                暂无 agent
              </p>
            ) : (
              agents.map((agent) => (
                <button
                  key={agent.id}
                  type="button"
                  onClick={() => onSelectAgent(agent.id)}
                  className={cn(
                    'flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm border-2',
                    selectedAgentId === agent.id
                      ? 'border-black bg-brutal-pink text-black shadow-brutal-sm'
                      : 'border-transparent hover:border-black',
                  )}
                  aria-current={selectedAgentId === agent.id ? 'true' : undefined}
                >
                  <PixelAvatar agentId={agent.id} size="sm" />
                  <span className="truncate font-body">{agent.name}</span>
                </button>
              ))
            )}
          </div>
        )}
      </div>
    </div>
  );
}

// ---- Main workspace page ----

export default function WorkspacePage() {
  const { agents, isLoading: agentsLoading, error: agentsError, refetch: refetchAgents } = useAgents();
  const [selectedAgentId, setSelectedAgentId] = useState<string | null>(null);

  const selectedAgent = agents.find((a) => a.id === selectedAgentId) ?? null;

  const { tree, isLoading: wsLoading, error: wsError, loadTree, fetchFileContent } = useWorkspaceFiles(selectedAgentId || '');

  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState<string | null>(null);
  const [isContentLoading, setIsContentLoading] = useState(false);
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());

  // Load tree when agent changes
  useEffect(() => {
    if (!selectedAgentId) return;
    setSelectedPath(null);
    setFileContent(null);
    loadTree();
  }, [selectedAgentId, loadTree]);

  // Persist expanded paths
  useEffect(() => {
    if (!selectedAgentId) return;
    try {
      const saved = localStorage.getItem(`ws-expand-${selectedAgentId}`);
      if (saved) setExpandedPaths(new Set(JSON.parse(saved)));
    } catch {}
  }, [selectedAgentId]);

  useEffect(() => {
    if (!selectedAgentId) return;
    try {
      localStorage.setItem(`ws-expand-${selectedAgentId}`, JSON.stringify([...expandedPaths]));
    } catch {}
  }, [expandedPaths, selectedAgentId]);

  const handleSelect = useCallback(async (filePath: string, _type: 'file' | 'directory') => {
    if (!selectedAgentId) return;
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
  }, [selectedAgentId, fetchFileContent]);

  const handleToggleExpand = useCallback((path: string) => {
    setExpandedPaths((prev) => {
      const next = new Set(prev);
      next.has(path) ? next.delete(path) : next.add(path);
      return next;
    });
  }, []);

  const handleLoadDirectory = useCallback((dirPath: string) => {
    if (!selectedAgentId) return;
    loadTree(dirPath);
  }, [selectedAgentId, loadTree]);

  return (
    <div className="flex h-screen min-w-[1024px] overflow-hidden bg-brutal-cream">
      <NavBar />
      <div className="w-[220px] flex-shrink-0">
        <WorkspaceLeftColumn
          agents={agents}
          isLoading={agentsLoading}
          error={agentsError}
          onRetry={refetchAgents}
          selectedAgentId={selectedAgentId}
          onSelectAgent={setSelectedAgentId}
        />
      </div>

      <main className="flex flex-1 flex-col overflow-hidden">
        {/* No agent selected */}
        {!selectedAgentId ? (
          <div className="flex flex-1 items-center justify-center">
            <div className="text-center">
              <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center border-2 border-black bg-brutal-pink shadow-brutal-sm">
                <FolderOpen className="h-8 w-8 text-black" />
              </div>
              <h2 className="font-heading text-lg font-bold text-foreground">Workspace</h2>
              <p className="mt-2 font-mono text-sm text-muted-foreground">
                从左侧选择一个 Agent 浏览其工作空间文件
              </p>
            </div>
          </div>
        ) : (
          <>
            {/* Header bar */}
            <div className="flex items-center gap-3 h-14 flex-shrink-0 border-b-2 border-black px-4 bg-brutal-cream">
              <PixelAvatar agentId={selectedAgent.id} size="md" />
              <h1 className="font-heading text-base font-bold text-foreground truncate">
                {selectedAgent.name}
              </h1>
              <span className="font-mono text-[10px] text-muted-foreground">Workspace</span>
              <div className="flex-1" />
              <Button
                variant="outline"
                size="sm"
                onClick={() => loadTree()}
                className="h-8 w-8 p-0"
                aria-label="刷新文件树"
              >
                <RefreshCw className="h-3.5 w-3.5" />
              </Button>
            </div>

            {/* Error */}
            {wsError && tree.length === 0 && !wsLoading && (
              <div className="flex flex-col items-center justify-center py-12">
                <div className="mb-3 flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-red-light shadow-brutal-sm">
                  <AlertCircle className="h-6 w-6 text-brutal-red" />
                </div>
                <BrutalAlert variant="error" className="mb-4 max-w-md">
                  {wsError}
                </BrutalAlert>
                <Button variant="outline" size="sm" onClick={() => loadTree()}>
                  <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
                  重试
                </Button>
              </div>
            )}

            {/* Loading */}
            {wsLoading && tree.length === 0 && !wsError && (
              <div className="p-4 space-y-2">
                <Skeleton className="h-6 w-48 rounded-none" />
                <Skeleton className="h-6 w-full rounded-none" />
                <Skeleton className="h-6 w-3/4 rounded-none" />
              </div>
            )}

            {/* Empty workspace */}
            {!wsLoading && !wsError && tree.length === 0 && (
              <div className="flex flex-1 items-center justify-center">
                <div className="text-center">
                  <FolderOpen className="mx-auto h-10 w-10 text-muted-foreground" />
                  <p className="mt-2 font-heading text-sm font-bold text-foreground">Agent workspace 尚无文件</p>
                  <p className="mt-1 font-mono text-xs text-muted-foreground">
                    运行 Agent 任务后文件将出现在此处
                  </p>
                </div>
              </div>
            )}

            {/* File tree + preview */}
            {tree.length > 0 && (
              <>
                {selectedPath && (
                  <Breadcrumb path={selectedPath} onNavigate={(p) => handleSelect(p, 'directory')} />
                )}
                <ResizablePanel
                  left={
                    <div className="h-full overflow-y-auto border-r-2 border-black bg-white">
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
                  }
                  right={
                    <div className="h-full overflow-y-auto bg-brutal-cream">
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
                  }
                />
              </>
            )}
          </>
        )}
      </main>
    </div>
  );
}
