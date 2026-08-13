'use client';

import { useEffect, useRef, useState } from 'react';
import { Check, ChevronDown, Plus, Settings, Trash2 } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { Button } from '@/components/ui/button';
import { Dialog, DialogCloseButton, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import { useWorkspace } from '@/lib/workspace-context';

export function WorkspaceSwitcher() {
  const router = useRouter();
  const { workspaces, activeWorkspace, switchWorkspace, createWorkspace, deleteWorkspace } = useWorkspace();
  const rootRef = useRef<HTMLDivElement>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!menuOpen) return;
    const close = (event: MouseEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setMenuOpen(false);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMenuOpen(false);
    };
    document.addEventListener('mousedown', close);
    document.addEventListener('keydown', escape);
    return () => {
      document.removeEventListener('mousedown', close);
      document.removeEventListener('keydown', escape);
    };
  }, [menuOpen]);

  const select = (workspaceID: string) => {
    switchWorkspace(workspaceID);
    setMenuOpen(false);
    router.push('/dashboard');
  };

  const create = async () => {
    if (!name.trim()) return;
    setBusy(true);
    try {
      await createWorkspace(name.trim());
      setName('');
      setCreateOpen(false);
      setMenuOpen(false);
      router.push('/dashboard');
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!activeWorkspace) return;
    setBusy(true);
    try {
      await deleteWorkspace(activeWorkspace.id);
      setDeleteOpen(false);
      setMenuOpen(false);
      router.push('/dashboard');
    } finally {
      setBusy(false);
    }
  };

  const canDelete = activeWorkspace?.role === 'owner'
    && !activeWorkspace.is_default
    && !activeWorkspace.is_personal;

  return (
    <>
      <div ref={rootRef} className="relative min-w-0 flex-1">
        <button
          type="button"
          onClick={() => setMenuOpen((value) => !value)}
          className="flex w-full min-w-0 items-center gap-3 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-black"
          aria-label="Workspace menu"
          aria-expanded={menuOpen}
        >
          <span className="flex h-9 w-9 shrink-0 items-center justify-center border-2 border-black bg-brutal-primary font-heading text-sm font-black shadow-brutal-sm">
            {activeWorkspace?.icon?.slice(0, 2) || 'S'}
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate font-heading text-base font-black text-black">{activeWorkspace?.name ?? 'Solo'}</span>
            <span className="block truncate font-mono text-[9px] font-bold uppercase tracking-wider text-black/50">{activeWorkspace?.role ?? 'Workspace'}</span>
          </span>
          <ChevronDown className={cn('h-4 w-4 shrink-0 transition-transform', menuOpen && 'rotate-180')} />
        </button>

        {menuOpen && (
          <div className="absolute left-0 top-[calc(100%+10px)] z-40 w-[300px] border-2 border-black bg-white p-2 shadow-brutal-lg" role="menu">
            <div className="mb-2 flex items-center gap-3 border-b-2 border-black px-2 pb-3 pt-1">
              <span className="flex h-11 w-11 shrink-0 items-center justify-center border-2 border-black bg-brutal-primary font-heading text-lg font-black">
                {activeWorkspace?.icon?.slice(0, 2) || 'S'}
              </span>
              <div className="min-w-0 flex-1">
                <div className="truncate font-heading text-lg font-black">{activeWorkspace?.name}</div>
                <div className="font-mono text-[10px] font-bold uppercase text-black/50">{activeWorkspace?.visibility} · {activeWorkspace?.role}</div>
              </div>
            </div>

            <div className="max-h-52 space-y-1 overflow-y-auto">
              {workspaces.map((workspace) => (
                <button
                  key={workspace.id}
                  type="button"
                  onClick={() => select(workspace.id)}
                  className={cn(
                    'flex w-full items-center gap-3 px-2 py-2 text-left hover:bg-brutal-cream',
                    workspace.id === activeWorkspace?.id && 'bg-brutal-cream',
                  )}
                  aria-label={`Switch to ${workspace.name}`}
                  role="menuitem"
                >
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center border-2 border-black bg-white font-heading text-xs font-black">{workspace.icon.slice(0, 2)}</span>
                  <span className="min-w-0 flex-1 truncate font-body text-sm font-bold">{workspace.name}</span>
                  {workspace.id === activeWorkspace?.id && <Check className="h-4 w-4 shrink-0" />}
                </button>
              ))}
            </div>

            <div className="mt-2 space-y-1 border-t-2 border-black pt-2">
              <button type="button" onClick={() => setCreateOpen(true)} className="flex w-full items-center gap-3 px-2 py-2 text-left font-body text-sm font-bold hover:bg-brutal-primary-light" role="menuitem">
                <Plus className="h-5 w-5" /> New Workspace
              </button>
              <button type="button" onClick={() => { setMenuOpen(false); router.push('/settings#workspace'); }} className="flex w-full items-center gap-3 px-2 py-2 text-left font-body text-sm font-bold hover:bg-brutal-cream" role="menuitem">
                <Settings className="h-5 w-5" /> Workspace settings
              </button>
              {canDelete && (
                <button type="button" onClick={() => setDeleteOpen(true)} className="flex w-full items-center gap-3 px-2 py-2 text-left font-body text-sm font-bold text-brutal-danger hover:bg-brutal-danger-light" role="menuitem">
                  <Trash2 className="h-5 w-5" /> Delete Workspace
                </button>
              )}
            </div>
          </div>
        )}
      </div>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogHeader>
          <DialogTitle>Create Workspace</DialogTitle>
          <DialogCloseButton onClick={() => setCreateOpen(false)} />
        </DialogHeader>
        <label className="block font-mono text-xs font-bold uppercase tracking-wider" htmlFor="workspace-name">Workspace name</label>
        <Input id="workspace-name" value={name} onChange={(event) => setName(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') void create(); }} placeholder="Design studio" maxLength={100} autoFocus />
        <p className="mt-2 font-body text-sm text-black/60">A private space with its own People, Channels, Agents, and Lucy.</p>
        <DialogFooter>
          <Button variant="outline" onClick={() => setCreateOpen(false)}>Cancel</Button>
          <Button onClick={() => void create()} disabled={busy || !name.trim()}>Create Workspace</Button>
        </DialogFooter>
      </Dialog>

      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogHeader>
          <DialogTitle>Delete {activeWorkspace?.name}?</DialogTitle>
          <DialogCloseButton onClick={() => setDeleteOpen(false)} />
        </DialogHeader>
        <p className="font-body text-sm text-black/70">Channels and Agents in this Workspace will be archived. Your Computers stay connected.</p>
        <DialogFooter>
          <Button variant="outline" onClick={() => setDeleteOpen(false)}>Cancel</Button>
          <Button variant="danger" onClick={() => void remove()} disabled={busy}>Delete Workspace</Button>
        </DialogFooter>
      </Dialog>
    </>
  );
}
