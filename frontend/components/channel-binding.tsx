// ============================================================================
// ChannelBinding — bind/unbind a Git repository to a channel
// - Shows current binding info (repo URL, branch, path)
// - Form to bind a new repo
// - "Open Workspace" button to navigate to workspace file browser
// - Uses card-brutal + brutalist design tokens
// ============================================================================

'use client';

import { useState, useEffect, useCallback } from 'react';
import { GitBranch, FolderOpen, Unlink, Link2, Loader2, ExternalLink } from 'lucide-react';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { BrutalAlert } from '@/components/ui/brutal-alert';
import { Select } from '@/components/ui/select';
import { Dialog, DialogHeader, DialogTitle, DialogCloseButton, DialogFooter } from '@/components/ui/dialog';
import { useToast } from '@/components/ui/toast';
import { apiClient } from '@/lib/api-client';
import type { ChannelBinding as ChannelBindingType } from '@/lib/types';

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

  const fetchBinding = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const data = await apiClient.get<ChannelBindingType>(`/api/v1/channels/${channelId}/bind`);
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

  useEffect(() => {
    fetchBinding();
  }, [fetchBinding]);

  const handleBind = async () => {
    if (!repoUrl.trim()) return;
    setIsSubmitting(true);
    try {
      const data = await apiClient.post<ChannelBindingType>(
        `/api/v1/channels/${channelId}/bind`,
        { repo_url: repoUrl.trim(), repo_branch: repoBranch || 'main' },
      );
      setBinding(data);
      setIsBindOpen(false);
      setRepoUrl('');
      setRepoBranch('main');
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
      await apiClient.delete(`/api/v1/channels/${channelId}/bind`);
      setBinding(null);
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
