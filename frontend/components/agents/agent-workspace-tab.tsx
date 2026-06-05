// ============================================================================
// AgentWorkspaceTab — file tree + readonly preview (v1.5)
// - Left: collapsible directory tree
// - Right: file preview (markdown for .md, plain/code for others)
// - Read-only — no edit/upload/delete
// ============================================================================

'use client';

import { useEffect, useState, useCallback } from 'react';
import { FolderOpen, File, ChevronDown, ChevronRight, AlertCircle, RefreshCw, Folder, FileText, Loader2 } from 'lucide-react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { useWorkspaceFiles, type WorkspaceFileNode } from '@/lib/hooks/use-workspace-files';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';

interface AgentWorkspaceTabProps {
  agentId: string;
}

// ---- File tree node ----

function FileTreeNode({
  node,
  depth,
  onSelect,
  selectedPath,
  expandedPaths,
  onToggleExpand,
  onLoadDirectory,
}: {
  node: WorkspaceFileNode;
  depth: number;
  onSelect: (path: string, type: 'file' | 'directory') => void;
  selectedPath: string | null;
  expandedPaths: Set<string>;
  onToggleExpand: (path: string) => void;
  onLoadDirectory: (path: string) => void;
}) {
  const isExpanded = expandedPaths.has(node.path);
  const isSelected = selectedPath === node.path;
  const isDir = node.type === 'directory';

  const handleClick = () => {
    if (isDir) {
      onToggleExpand(node.path);
      if (!isExpanded && (!node.children || node.children.length === 0)) {
        onLoadDirectory(node.path);
      }
    } else {
      onSelect(node.path, 'file');
    }
  };

  const Icon = isDir
    ? isExpanded
      ? FolderOpen
      : Folder
    : FileText;

  const iconColor = isDir ? 'text-brutal-lavender' : 'text-muted-foreground';

  return (
    <div>
      <button
        type="button"
        onClick={handleClick}
        className={cn(
          'flex w-full items-center gap-1 px-1 py-1 text-left transition-colors',
          isSelected && 'bg-brutal-pink',
          !isSelected && 'hover:bg-muted',
        )}
        style={{ paddingLeft: `${depth * 16 + 4}px` }}
        aria-expanded={isDir ? isExpanded : undefined}
      >
        {isDir && (
          isExpanded
            ? <ChevronDown className="h-3 w-3 flex-shrink-0 text-muted-foreground" />
            : <ChevronRight className="h-3 w-3 flex-shrink-0 text-muted-foreground" />
        )}
        <Icon className={cn('h-3.5 w-3.5 flex-shrink-0', iconColor)} />
        <span className="truncate font-mono text-[11px] ml-1">
          {node.name}
        </span>
      </button>

      {isDir && isExpanded && node.children && (
        <div>
          {node.children.map((child) => (
            <FileTreeNode
              key={child.path}
              node={child}
              depth={depth + 1}
              onSelect={onSelect}
              selectedPath={selectedPath}
              expandedPaths={expandedPaths}
              onToggleExpand={onToggleExpand}
              onLoadDirectory={onLoadDirectory}
            />
          ))}
        </div>
      )}
    </div>
  );
}

// ---- File preview ----

function FilePreview({
  path,
  content,
  isLoading,
}: {
  path: string;
  content: string | null;
  isLoading: boolean;
}) {
  const isMarkdown = path.endsWith('.md');

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (content === null) {
    return (
      <div className="flex items-center justify-center h-full">
        <p className="font-mono text-xs text-muted-foreground">加载文件内容失败</p>
      </div>
    );
  }

  if (isMarkdown) {
    return (
      <div className="p-4 prose prose-sm max-w-none prose-headings:font-heading prose-headings:font-bold prose-code:font-mono prose-pre:bg-black prose-pre:text-brutal-lime prose-pre:border-2 prose-pre:border-black prose-pre:shadow-brutal-sm">
        <ReactMarkdown remarkPlugins={[remarkGfm]}>
          {content}
        </ReactMarkdown>
      </div>
    );
  }

  // Plain text / code preview
  return (
    <pre className="p-4 font-mono text-xs leading-relaxed whitespace-pre-wrap overflow-auto h-full bg-brutal-cream border-l-2 border-black">
      <code>{content}</code>
    </pre>
  );
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

  // Empty state
  if (!isLoading && tree.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <div className="mb-3 flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-pink shadow-brutal-sm">
          <FolderOpen className="h-6 w-6 text-white" />
        </div>
        <p className="font-heading text-sm font-bold text-foreground">Workspace 为空</p>
        <p className="mt-1 font-mono text-[11px] text-muted-foreground">
          Agent 的工作空间中还没有文件
        </p>
      </div>
    );
  }

  return (
    <div className="-mx-4 -mb-4 flex h-[calc(100vh-200px)]">
      {/* Left: File tree */}
      <div className="w-[220px] flex-shrink-0 overflow-y-auto border-r-2 border-black bg-white">
        <div className="border-b-2 border-black px-3 py-2">
          <span className="font-heading text-[10px] font-bold text-muted-foreground uppercase tracking-wider">
            文件
          </span>
        </div>
        <div className="py-1">
          {tree.map((node) => (
            <FileTreeNode
              key={node.path}
              node={node}
              depth={0}
              onSelect={(path, type) => {
                if (type === 'file') handleSelect(path, type);
              }}
              selectedPath={selectedPath}
              expandedPaths={expandedPaths}
              onToggleExpand={handleToggleExpand}
              onLoadDirectory={handleLoadDirectory}
            />
          ))}
        </div>
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
                选择文件以预览内容
              </p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
