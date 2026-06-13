// ============================================================================
// ChannelBinding — bind/unbind a Git repository to a channel
// - Shows current binding info (repo URL, branch, path)
// - Form to bind a new repo
// - "Open Workspace" button to navigate to workspace file browser
// - Uses card-brutal + brutalist design tokens
// ============================================================================

'use client';

import { useState, useEffect, useCallback } from 'react';
import { GitBranch, FolderOpen, Unlink, Link2, Loader2, ExternalLink, ChevronDown, AlertCircle, RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { BrutalAlert } from '@/components/ui/brutal-alert';
import { Dialog, DialogHeader, DialogTitle, DialogCloseButton, DialogFooter } from '@/components/ui/dialog';
import { useToast } from '@/components/ui/toast';
import { FileTree } from '@/components/workspace/file-tree';
import { apiClient } from '@/lib/api-client';
import type { ChannelBinding as ChannelBindingType, WorkspaceFileNode } from '@/lib/types';

interface ChannelBindingProps {
  channelId: string;
  channelName: string;
  isAdmin?: boolean;
}

export function ChannelBinding({ channelId, channelName, isAdmin = false }: ChannelBindingProps) {
  const [binding, setBinding] = useState<ChannelBindingType | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isBindOpen, setIsBindOpen] = useState(false);
  const [isUnbindOpen, setIsUnbindOpen] = useState(false);
  const [repoUrl, setRepoUrl] = useState('');
  const [repoBranch, setRepoBranch] = useState('main');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const { showToast } = useToast();

  // Workspace file tree state
  const [wsTree, setWsTree] = useState<WorkspaceFileNode[]>([]);
  const [wsLoading, setWsLoading] = useState(false);
  const [wsError, setWsError] = useState<string | null>(null);
  const [wsSelectedPath, setWsSelectedPath] = useState<string | null>(null);
  const [wsExpandedPaths, setWsExpandedPaths] = useState<Set<string>>(new Set());
  const [wsExpanded, setWsExpanded] = useState(false);

  const fetchBinding = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const data = await apiClient.get<ChannelBindingType>(`/api/v1/channels/${channelId}/binding`);
      setBinding(data);
    } catch (err: unknown) {
      const apiErr = err as { status?: number };
      if (apiErr.status === 404) {
        setBinding(null);
      } else {
        setError(t('channelBindingLoadError'));
      }
    } finally {
      setIsLoading(false);
    }
  }, [channelId]);

  // ---- API response type for channel workspace ----
  interface ChannelFileNode {
    name: string;
    path: string;
    is_dir: boolean;
    size?: number;
    children?: ChannelFileNode[];
  }

  function mapToWorkspaceNodes(nodes: ChannelFileNode[] | undefined): WorkspaceFileNode[] {
    if (!nodes) return [];
    return nodes.map((n) => ({
      name: n.name,
      path: n.path,
      type: n.is_dir ? 'directory' : 'file',
      size: n.size,
      children: mapToWorkspaceNodes(n.children),
    }));
  }

  const fetchWorkspace = useCallback(async () => {
    if (!channelId) return;
    setWsLoading(true);
    setWsError(null);
    try {
      const root = await apiClient.get<ChannelFileNode>(`/api/v1/channels/${channelId}/workspace`);
      setWsTree(mapToWorkspaceNodes(root.children));
    } catch {
      setWsError(t('channelBindingWorkspaceError'));
      setWsTree([]);
    } finally {
      setWsLoading(false);
    }
  }, [channelId]);

  useEffect(() => {
    fetchBinding();
    fetchWorkspace();
  }, [fetchBinding, fetchWorkspace]);

  const handleBind = async () => {
    if (!repoUrl.trim()) return;
    setIsSubmitting(true);
    try {
      const data = await apiClient.post<ChannelBindingType>(
        `/api/v1/channels/${channelId}/bind-project`,
        { repo_url: repoUrl.trim(), repo_branch: repoBranch || 'main' },
      );
      setBinding(data);
      setIsBindOpen(false);
      setRepoUrl('');
      setRepoBranch('main');
      setWsSelectedPath(null);
      setWsExpandedPaths(new Set());
      setWsExpanded(false);
      fetchWorkspace();
      showToast(t('channelBindingBind'), 'success');
    } catch {
      showToast(t('channelBindingBindError'), 'error');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleUnbind = async () => {
    setIsSubmitting(true);
    try {
      await apiClient.delete(`/api/v1/channels/${channelId}/bind-project`);
      setBinding(null);
      setWsTree([]);
      setWsSelectedPath(null);
      setWsExpandedPaths(new Set());
      setWsExpanded(false);
      setIsUnbindOpen(false);
      showToast(t('channelBindingUnbind'), 'info');
    } catch {
      showToast(t('channelBindingUnbindError'), 'error');
    } finally {
      setIsSubmitting(false);
    }
  };

  const formatDate = (iso?: string) => {
    if (!iso) return '';
    try {
      return new Date(iso).toLocaleString();
    } catch {
      return iso;
    }
  };

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 p-4">
        <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        <span className="font-mono text-xs text-muted-foreground">{t('loading')}</span>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {/* Section header */}
      <div className="flex items-center justify-between">
        <h3 className="font-heading text-sm font-bold uppercase tracking-wider text-foreground">
          <GitBranch className="inline h-4 w-4 mr-1.5 -mt-0.5" />
          {t('channelBindingTitle')}
        </h3>
      </div>

      {error && (
        <BrutalAlert variant="error" className="text-xs">
          {error}
          <button
            type="button"
            onClick={fetchBinding}
            className="ml-2 underline font-bold"
          >
            {t('retry')}
          </button>
        </BrutalAlert>
      )}

      {binding ? (
        /* Bound state */
        <div className="border-2 border-black bg-white p-4 shadow-brutal-sm space-y-3">
          <div className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
            <div>
              <span className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                {t('channelBindingRepoUrl')}
              </span>
              <p className="font-mono text-xs text-foreground truncate mt-0.5">
                {binding.repo_url}
              </p>
            </div>
            <div>
              <span className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                {t('channelBindingRepoBranch')}
              </span>
              <p className="font-mono text-xs text-foreground mt-0.5">
                {binding.repo_branch}
              </p>
            </div>
            <div className="col-span-2">
              <span className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                {t('channelBindingBindPath')}
              </span>
              <p className="font-mono text-xs text-foreground truncate mt-0.5">
                {binding.bind_path}
              </p>
            </div>
            <div className="col-span-2">
              <span className="font-heading text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                {t('channelBindingLastUpdated')}
              </span>
              <p className="font-mono text-xs text-foreground mt-0.5">
                {formatDate(binding.bound_at)}
              </p>
            </div>
          </div>

          {/* Actions */}
          <div className="flex items-center gap-2 pt-1">
            <Button
              variant="outline"
              size="sm"
              onClick={() => window.open(`/workspace`, '_blank')}
              className="text-xs"
            >
              <ExternalLink className="h-3 w-3 mr-1" />
              {t('channelBindingOpenWorkspace')}
            </Button>
            {isAdmin && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setIsUnbindOpen(true)}
                className="text-xs text-brutal-danger"
              >
                <Unlink className="h-3 w-3 mr-1" />
                {t('channelBindingUnbind')}
              </Button>
            )}
          </div>

          {/* Workspace file tree */}
          <div className="border-t-2 border-black pt-3 mt-3">
            <button
              type="button"
              onClick={() => setWsExpanded((v) => !v)}
              className="flex w-full items-center gap-1.5 text-left"
              aria-expanded={wsExpanded}
            >
              <ChevronDown
                className={cn(
                  'h-3.5 w-3.5 flex-shrink-0 transition-transform',
                  wsExpanded ? 'rotate-0' : '-rotate-90',
                )}
              />
              <span className="font-heading text-xs font-bold uppercase tracking-wider text-foreground">
                {t('channelBindingWorkspaceFiles')}
              </span>
              <div className="flex-1" />
              {wsExpanded && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={(e) => { e.stopPropagation(); fetchWorkspace(); }}
                  className="h-7 w-7 p-0"
                  aria-label={t('retry')}
                  disabled={wsLoading}
                >
                  <RefreshCw className={cn('h-3 w-3', wsLoading && 'animate-spin')} />
                </Button>
              )}
            </button>

            {wsExpanded && (
              <div className="mt-2 border-2 border-black bg-white min-h-[80px]">
                {wsLoading ? (
                  <div className="flex items-center gap-2 p-3">
                    <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                    <span className="font-mono text-xs text-muted-foreground">{t('loading')}</span>
                  </div>
                ) : wsError ? (
                  <div className="flex flex-col items-center gap-2 p-4">
                    <AlertCircle className="h-4 w-4 text-brutal-danger" />
                    <p className="font-mono text-[10px] text-brutal-danger text-center">{wsError}</p>
                    <Button variant="outline" size="sm" onClick={fetchWorkspace} className="text-[10px]">
                      {t('retry')}
                    </Button>
                  </div>
                ) : wsTree.length === 0 ? (
                  <div className="flex items-center justify-center p-4">
                    <p className="font-mono text-[10px] text-muted-foreground italic">
                      {t('channelBindingWorkspaceEmpty')}
                    </p>
                  </div>
                ) : (
                  <div className="max-h-[320px] overflow-y-auto">
                    <FileTree
                      tree={wsTree}
                      selectedPath={wsSelectedPath}
                      expandedPaths={wsExpandedPaths}
                      onSelect={(path) => {
                        setWsSelectedPath(path);
                      }}
                      onToggleExpand={(path) => {
                        setWsExpandedPaths((prev) => {
                          const next = new Set(prev);
                          next.has(path) ? next.delete(path) : next.add(path);
                          return next;
                        });
                      }}
                      onLoadDirectory={() => {
                        // full tree is already loaded from ScanWorkspace
                      }}
                    />
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      ) : (
        /* Unbound state */
        <div className="border-2 border-black border-dashed bg-brutal-cream p-4">
          <div className="text-center">
            <FolderOpen className="mx-auto h-6 w-6 text-muted-foreground mb-2" />
            <p className="font-heading text-sm font-bold text-foreground">
              {t('channelBindingNoBinding')}
            </p>
            <p className="mt-1 font-mono text-xs text-muted-foreground">
              {t('channelBindingNoBindingDesc')}
            </p>
            {isAdmin && (
              <Button
                variant="primary"
                size="sm"
                onClick={() => setIsBindOpen(true)}
                className="mt-3"
              >
                <Link2 className="h-3 w-3 mr-1" />
                {t('channelBindingBind')}
              </Button>
            )}
          </div>
        </div>
      )}

      {/* Bind dialog */}
      <Dialog open={isBindOpen} onOpenChange={setIsBindOpen}>
        <DialogHeader>
          <DialogTitle>
            <Link2 className="inline h-4 w-4 mr-1.5 -mt-0.5" />
            {t('channelBindingBind')}
          </DialogTitle>
          <DialogCloseButton onClick={() => setIsBindOpen(false)} />
        </DialogHeader>
        <div className="space-y-4">
          <div>
            <label className="block font-heading text-sm font-bold mb-1.5">
              {t('channelBindingRepoUrl')}
            </label>
            <Input
              value={repoUrl}
              onChange={(e) => setRepoUrl(e.target.value)}
              placeholder={t('channelBindingRepoUrlPlaceholder')}
            />
          </div>
          <div>
            <label className="block font-heading text-sm font-bold mb-1.5">
              {t('channelBindingRepoBranch')}
            </label>
            <Input
              value={repoBranch}
              onChange={(e) => setRepoBranch(e.target.value)}
              placeholder="main"
            />
          </div>
          <p className="font-mono text-xs text-muted-foreground">
            Channel: #{channelName}
          </p>
        </div>
        <DialogFooter>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setIsBindOpen(false)}
          >
            {t('cancel')}
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={handleBind}
            disabled={!repoUrl.trim() || isSubmitting}
          >
            {isSubmitting ? t('submitting') : t('channelBindingBind')}
          </Button>
        </DialogFooter>
      </Dialog>

      {/* Unbind confirmation dialog */}
      <Dialog open={isUnbindOpen} onOpenChange={setIsUnbindOpen}>
        <DialogHeader>
          <DialogTitle>{t('channelBindingUnbind')}</DialogTitle>
          <DialogCloseButton onClick={() => setIsUnbindOpen(false)} />
        </DialogHeader>
        <p className="text-sm text-muted-foreground">
          {t('channelBindingUnbindConfirm')}
        </p>
        <DialogFooter>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setIsUnbindOpen(false)}
          >
            {t('cancel')}
          </Button>
          <Button
            variant="danger"
            size="sm"
            onClick={handleUnbind}
            disabled={isSubmitting}
          >
            {isSubmitting ? t('submitting') : t('channelBindingUnbind')}
          </Button>
        </DialogFooter>
      </Dialog>
    </div>
  );
}
