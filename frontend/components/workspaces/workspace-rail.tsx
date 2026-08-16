'use client';

import Image from 'next/image';
import { useState } from 'react';
import { Plus } from 'lucide-react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { Button } from '@/components/ui/button';
import { Dialog, DialogCloseButton, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { useWorkspace } from '@/lib/workspace-context';
import { t } from '@/lib/i18n';
import { isPersonalArea } from '@/lib/personal-navigation';
import { cn } from '@/lib/utils';

const railItemClass = (isActive: boolean) =>
  cn(
    'group relative flex h-10 w-10 cursor-pointer items-center justify-center border-2 border-black font-heading text-sm font-black shadow-brutal-sm',
    'transition-[transform,box-shadow] hover:-translate-x-px hover:-translate-y-px hover:shadow-brutal',
    isActive ? 'bg-brutal-primary' : 'bg-white hover:bg-brutal-cream',
  );

// Vertical 56px rail. Items stay in API order so switching does not visually reorder them;
// the active workspace is just highlighted.
export function WorkspaceRail() {
  const router = useRouter();
  const pathname = usePathname();
  const { workspaces, activeWorkspace, switchWorkspace, createWorkspace, openManage } = useWorkspace();
  const personalArea = isPersonalArea(pathname);
  const [createOpen, setCreateOpen] = useState(false);
  const [workspaceName, setWorkspaceName] = useState('');
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  const switchTo = (id: string) => {
    switchWorkspace(id);
    router.push('/dashboard');
  };

  const openCreate = () => {
    setWorkspaceName('');
    setCreateError(null);
    setCreateOpen(true);
  };

  const create = async () => {
    if (!workspaceName.trim()) return;
    setCreating(true);
    setCreateError(null);
    try {
      await createWorkspace(workspaceName.trim());
      setCreateOpen(false);
      router.push('/dashboard');
    } catch (error) {
      setCreateError(error instanceof Error ? error.message : t('workspaceCreateFailed'));
    } finally {
      setCreating(false);
    }
  };

  return (
    <>
    <nav
      aria-label={t('workspaceRailLabel')}
      className="navbar-brutal flex w-14 flex-shrink-0 flex-col items-center gap-2 border-r-2 border-black py-3"
    >
      <Link
        href="/home"
        aria-label={t('personalHome')}
        title={t('personalHome')}
        aria-current={personalArea ? 'page' : undefined}
        className={railItemClass(personalArea)}
      >
        {personalArea && (
          <span aria-hidden className="absolute -left-3 top-1/2 h-6 w-1 -translate-y-1/2 bg-black" />
        )}
        <Image src="/favicon.svg" alt="" width={32} height={32} priority />
      </Link>

      <div className="h-px w-8 shrink-0 bg-black/20" />

      <div className="flex max-h-[60vh] w-full flex-col items-center gap-2 overflow-x-hidden overflow-y-auto">
        {workspaces.map((item) => {
          const isCurrentWorkspace = item.id === activeWorkspace?.id;
          const isActive = !personalArea && isCurrentWorkspace;
          return (
            <button
              key={item.id}
              type="button"
              onClick={isActive ? () => openManage() : () => switchTo(item.id)}
              aria-label={isActive
                ? t('workspaceRailOpenManage', { name: item.name })
                : t('workspaceRailSwitchTo', { name: item.name })}
              title={item.name}
              aria-current={isActive ? 'true' : undefined}
              className={railItemClass(isActive)}
            >
              {isActive && (
                <span aria-hidden className="absolute -left-3 top-1/2 h-6 w-1 -translate-y-1/2 bg-black" />
              )}
              {item.icon?.slice(0, 2) || '·'}
            </button>
          );
        })}

        <button
          type="button"
          onClick={openCreate}
          data-onboarding="create-workspace"
          aria-label={t('workspaceRailCreate')}
          title={t('workspaceRailCreate')}
          className="group relative flex h-10 w-10 shrink-0 cursor-pointer items-center justify-center border-2 border-dashed border-black bg-white text-black shadow-brutal-sm transition-[transform,box-shadow] hover:-translate-x-px hover:-translate-y-px hover:border-solid hover:shadow-brutal"
        >
          <Plus className="h-4 w-4" />
        </button>
      </div>
    </nav>

    <Dialog open={createOpen} onOpenChange={(open) => { if (!creating) setCreateOpen(open); }} width="sm">
      <DialogHeader>
        <DialogTitle>{t('workspaceRailCreate')}</DialogTitle>
        <DialogCloseButton onClick={() => setCreateOpen(false)} />
      </DialogHeader>
      <form onSubmit={(event) => { event.preventDefault(); void create(); }}>
        <label htmlFor="new-workspace-name" className="font-heading text-sm font-bold">
          {t('createWorkspacePrompt')}
        </label>
        <Input
          id="new-workspace-name"
          value={workspaceName}
          onChange={(event) => setWorkspaceName(event.target.value)}
          className="mt-2"
          autoFocus
        />
        {createError && <p role="alert" className="mt-3 font-body text-xs font-bold text-brutal-danger">{createError}</p>}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setCreateOpen(false)} disabled={creating}>{t('cancel')}</Button>
          <Button type="submit" disabled={creating || !workspaceName.trim()}>{creating ? t('saving') : t('create')}</Button>
        </DialogFooter>
      </form>
    </Dialog>
    </>
  );
}
