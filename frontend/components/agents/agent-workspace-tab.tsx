// ============================================================================
// AgentWorkspaceTab — file tree + readonly preview (v1.5)
// - Left: collapsible directory tree
// - Right: file preview (markdown for .md, plain/code for others)
// - Read-only — no edit/upload/delete
// ============================================================================

'use client';

import { useEffect, useState, useCallback } from 'react';
import Link from 'next/link';
import { FolderOpen, AlertCircle, RefreshCw, FileText, ExternalLink } from 'lucide-react';
import { useWorkspaceFiles } from '@/lib/hooks/use-workspace-files';
import { FileTree } from '@/components/workspace/file-tree';
import { FilePreview } from '@/components/workspace/file-preview';
import type { WorkspaceFileNode } from '@/lib/types';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';

interface AgentWorkspaceTabProps {
  agentId: string;
}

// ---- Component ----

export function AgentWorkspaceTab({ agentId }: AgentWorkspaceTabProps) {
  const { tree, isLoading, error, loadTree, fetchFileContent } = useWorkspaceFiles(agentId);

  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState<string | null>(null);
  const [isContentLoading, setIsContentLoading] = useState(false);
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());

  // Load tree on mount
  useEffect(() => {
    loadTree();
  }, [loadTree]);

  const handleSelect = useCallback(
    async (filePath: string, _type: 'file' | 'directory') => {
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
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  }, []);

  const handleLoadDirectory = useCallback(
    (dirPath: string) => {
      loadTree(dirPath);
    },
    [loadTree],
  );

  // Loading state
  if (isLoading && tree.length === 0) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-6 w-full rounded-none" />
        <Skeleton className="h-6 w-3/4 rounded-none" />
        <Skeleton className="h-6 w-2/3 rounded-none" />
        <Skeleton className="h-6 w-5/6 rounded-none" />
      </div>
    );
  }

  // Error state
  if (error && tree.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <div className="mb-3 flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-danger-light shadow-brutal-sm">
          <AlertCircle className="h-6 w-6 text-brutal-danger" />
        </div>
        <p className="font-body text-sm text-brutal-danger">{error}</p>
        <button type="button" onClick={() => loadTree()} className="btn-brutal btn-brutal-sm mt-4">
          <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
          {t('retry')}
        </button>
      </div>
    );
  }

  // Empty state
  if (!isLoading && tree.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <div className="mb-3 flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-primary shadow-brutal-sm">
          <FolderOpen className="h-6 w-6 text-white" />
        </div>
        <p className="font-heading text-sm font-bold text-foreground">{t('workspaceNoFiles')}</p>
        <p className="mt-1 font-mono text-[11px] text-muted-foreground">
          {t('workspaceNoFilesDesc')}
        </p>
      </div>
    );
  }

  return (
    <div className="-mx-4 -mb-4 flex h-[calc(100vh-200px)]">
      {/* Left: File tree */}
      <div className="w-[220px] flex-shrink-0 overflow-y-auto border-r-2 border-black bg-white">
        <div className="border-b-2 border-black px-3 py-2 flex items-center">
          <span className="font-heading text-[10px] font-bold text-muted-foreground uppercase tracking-wider">
            {t('workspaceFiles')}
          </span>
          <Link
            href={`/workspace?agent=${agentId}`}
            className="btn-brutal btn-brutal-xs ml-auto"
            target="_blank"
            rel="noopener noreferrer"
          >
            <ExternalLink className="mr-1 h-3 w-3" />
            {t('workspaceOpenNewTab')}
          </Link>
        </div>
        <FileTree
          tree={tree}
          selectedPath={selectedPath}
          expandedPaths={expandedPaths}
          onSelect={(path, type) => {
            if (type === 'file') handleSelect(path, type);
          }}
          onToggleExpand={handleToggleExpand}
          onLoadDirectory={handleLoadDirectory}
        />
      </div>

      {/* Right: File preview */}
      <div className="flex-1 overflow-y-auto bg-brutal-cream">
        {selectedPath ? (
          <>
            <div className="border-b-2 border-black bg-white px-3 py-1.5">
              <span className="font-mono text-[10px] text-muted-foreground">
                {selectedPath}
              </span>
            </div>
            <FilePreview
              path={selectedPath}
              content={fileContent}
              isLoading={isContentLoading}
            />
          </>
        ) : (
          <div className="flex h-full items-center justify-center">
            <div className="text-center">
              <FileText className="mx-auto h-6 w-6 text-muted-foreground" />
              <p className="mt-2 font-mono text-xs text-muted-foreground">
                {t('workspaceSelectFile')}
              </p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
