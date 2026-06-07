'use client';

import { FolderOpen, ChevronDown, ChevronRight, Folder, FileText } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { WorkspaceFileNode } from '@/lib/types';

interface FileTreeNodeProps {
  node: WorkspaceFileNode;
  depth: number;
  onSelect: (path: string, type: 'file' | 'directory') => void;
  selectedPath: string | null;
  expandedPaths: Set<string>;
  onToggleExpand: (path: string) => void;
  onLoadDirectory: (path: string) => void;
}

function FileTreeNode({
  node, depth, onSelect, selectedPath, expandedPaths, onToggleExpand, onLoadDirectory,
}: FileTreeNodeProps) {
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

  const Icon = isDir ? (isExpanded ? FolderOpen : Folder) : FileText;
  const iconColor = isDir ? 'text-brutal-violet' : 'text-muted-foreground';

  return (
    <div>
      <button
        type="button"
        onClick={handleClick}
        className={cn(
          'flex w-full items-center gap-1 px-1 py-1 text-left transition-colors',
          isSelected && 'bg-brutal-primary',
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
        <span className="truncate font-mono text-[11px] ml-1">{node.name}</span>
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

// ---- Public FileTree wrapper ----

interface FileTreeProps {
  tree: WorkspaceFileNode[];
  selectedPath: string | null;
  expandedPaths: Set<string>;
  onSelect: (path: string, type: 'file' | 'directory') => void;
  onToggleExpand: (path: string) => void;
  onLoadDirectory: (path: string) => void;
}

export function FileTree({ tree, selectedPath, expandedPaths, onSelect, onToggleExpand, onLoadDirectory }: FileTreeProps) {
  return (
    <div className="py-1">
      {tree.map((node) => (
        <FileTreeNode
          key={node.path}
          node={node}
          depth={0}
          onSelect={onSelect}
          selectedPath={selectedPath}
          expandedPaths={expandedPaths}
          onToggleExpand={onToggleExpand}
          onLoadDirectory={onLoadDirectory}
        />
      ))}
    </div>
  );
}
