'use client';

import { useCallback, useEffect, useState } from 'react';
import { ChevronDown, UserPlus } from 'lucide-react';
import { apiClient } from '@/lib/api-client';
import { useAuth } from '@/lib/auth-context';
import { useWorkspace } from '@/lib/workspace-context';
import { UserAvatar } from '@/components/ui/user-avatar';
import { selectableRowClass } from '@/components/ui/selectable-row';
import { cn } from '@/lib/utils';
import { useToast } from '@/components/ui/toast';
import { Button } from '@/components/ui/button';
import { t } from '@/lib/i18n';
import {
  Dialog,
  DialogCloseButton,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';

interface WorkspaceMember {
  user_id: string;
  email: string;
  display_name: string;
  avatar_url?: string;
  role: 'owner' | 'admin' | 'member';
}

export function WorkspacePeople() {
  const { user } = useAuth();
  const { activeWorkspace, refetch, openManage } = useWorkspace();
  const { showToast } = useToast();
  const [expanded, setExpanded] = useState(true);
  const [inviteOpen, setInviteOpen] = useState(false);
  const [email, setEmail] = useState('');
  const [role, setRole] = useState<'member' | 'admin'>('member');
  const [members, setMembers] = useState<WorkspaceMember[]>([]);
  const [busy, setBusy] = useState(false);
  const canAdmin = activeWorkspace?.role === 'owner' || activeWorkspace?.role === 'admin';
  const memberCount = activeWorkspace?.member_count ?? members.length;

  const load = useCallback(async () => {
    if (!activeWorkspace) return;
    try {
      setMembers(await apiClient.get<WorkspaceMember[]>(`/api/v1/workspaces/${activeWorkspace.id}/members?limit=5`));
    } catch (error) {
      showToast(error instanceof Error ? error.message : t('workspacePeopleLoadFailed'), 'error');
    }
  }, [activeWorkspace, showToast]);

  useEffect(() => { void load(); }, [load]);

  const invite = async () => {
    if (!activeWorkspace || !email.trim()) return;
    setBusy(true);
    try {
      await apiClient.post(`/api/v1/workspaces/${activeWorkspace.id}/members`, { email: email.trim(), role });
      setEmail('');
      setInviteOpen(false);
      await load();
      await refetch();
      showToast(t('workspacePersonAdded'), 'success');
    } catch (error) {
      showToast(error instanceof Error ? error.message : t('workspaceInviteFailed'), 'error');
    } finally {
      setBusy(false);
    }
  };

  if (!activeWorkspace) return null;

  return (
    <section className="pt-2">
      <div className="flex items-center gap-2 px-3 py-2">
        <button type="button" onClick={() => setExpanded((value) => !value)} className="flex min-w-0 flex-1 items-center gap-2 text-left" aria-expanded={expanded}>
          <ChevronDown className={cn('h-3.5 w-3.5 shrink-0 transition-transform', !expanded && '-rotate-90')} />
          <span className="min-w-0 flex-1 font-heading text-xs font-black uppercase tracking-wider text-black/70">{t('workspacePeopleHeading')}</span>
          <span className="font-mono text-xs font-bold tabular-nums text-black/45">{memberCount}</span>
        </button>
        {canAdmin && (
          <button type="button" onClick={() => { setExpanded(true); setInviteOpen(true); }} className="flex h-7 w-7 shrink-0 items-center justify-center border-2 border-black bg-white shadow-brutal-sm transition-[transform,box-shadow] hover:-translate-y-px hover:shadow-brutal" aria-label={t('workspaceInviteAria')} title={t('workspaceInviteAria')}>
            <UserPlus className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      {expanded && (
        <div className="pb-2">
          <div className="space-y-0.5">
            {members.map((member) => (
              <div key={member.user_id} data-testid="workspace-person" className={selectableRowClass(false, 'cursor-default bg-transparent hover:bg-white/50')}>
                <UserAvatar userId={member.user_id} name={member.display_name} avatarUrl={member.avatar_url} size="sm" />
                <span className="min-w-0 flex-1 truncate font-body text-sm">{member.display_name}</span>
                <span className="font-mono text-[9px] font-bold uppercase text-black/55">{t(`workspaceRole${member.role.charAt(0).toUpperCase()}${member.role.slice(1)}` as 'workspaceRoleOwner' | 'workspaceRoleAdmin' | 'workspaceRoleMember')}</span>
                {member.user_id === user?.id && <span className="font-mono text-[9px] text-black/50">{t('workspaceYouSuffix')}</span>}
              </div>
            ))}
          </div>
          {memberCount > members.length && (
            canAdmin ? (
              <button type="button" onClick={() => openManage('members')} className="mx-3 mt-1 block py-1 text-left font-mono text-[10px] font-bold uppercase tracking-wider text-black/50 hover:text-black hover:underline">
                {t('workspaceManageAllLink', { count: memberCount })}
              </button>
            ) : (
              <p className="mx-3 mt-1 py-1 font-mono text-[10px] font-bold uppercase tracking-wider text-black/45">
                {t('workspaceMorePeople', { count: memberCount - members.length })}
              </p>
            )
          )}
        </div>
      )}

      <Dialog open={inviteOpen} onOpenChange={(open) => { if (!busy) setInviteOpen(open); }} width="sm">
        <DialogHeader>
          <DialogTitle>{t('workspaceInviteTitle', { name: activeWorkspace.name })}</DialogTitle>
          <DialogCloseButton onClick={() => setInviteOpen(false)} />
        </DialogHeader>
        <DialogDescription>
          {t('workspaceInviteDesc')}
        </DialogDescription>
        <label htmlFor="workspace-invite-email" className="mt-5 block font-mono text-xs font-bold uppercase tracking-wider">{t('workspaceInviteEmailLabel')}</label>
        <Input
          id="workspace-invite-email"
          type="email"
          value={email}
          onChange={(event) => setEmail(event.target.value)}
          onKeyDown={(event) => { if (event.key === 'Enter') void invite(); }}
          placeholder={t('workspaceInviteEmailPlaceholder')}
          aria-label={t('workspaceInviteEmailAria')}
          className="mt-2"
          autoFocus
        />
        <fieldset className="mt-5">
          <legend className="font-mono text-xs font-bold uppercase tracking-wider">{t('workspaceInviteRoleLegend')}</legend>
          <div className="mt-2 grid grid-cols-2 gap-2">
            {(['member', 'admin'] as const).map((value) => (
              activeWorkspace.role === 'owner' || value === 'member' ?
              <button
                key={value}
                type="button"
                onClick={() => setRole(value)}
                className={cn(
                  'border-2 border-black px-3 py-3 text-left shadow-brutal-sm',
                  role === value ? 'bg-brutal-primary' : 'bg-white hover:bg-brutal-cream',
                )}
                aria-pressed={role === value}
              >
                <span className="block font-heading text-sm font-black">{t(`workspaceRole${value.charAt(0).toUpperCase()}${value.slice(1)}` as 'workspaceRoleAdmin' | 'workspaceRoleMember')}</span>
                <span className="mt-1 block font-body text-xs text-black/60">
                  {t(value === 'admin' ? 'workspaceRoleAdminDesc' : 'workspaceRoleMemberDesc')}
                </span>
              </button>
              : null
            ))}
          </div>
        </fieldset>
        <DialogFooter>
          <Button variant="outline" onClick={() => setInviteOpen(false)} disabled={busy}>{t('cancel')}</Button>
          <Button onClick={() => void invite()} disabled={busy || !email.trim()}>
            {busy ? t('workspaceInvitingButton') : t('workspaceInvitePersonButton')}
          </Button>
        </DialogFooter>
      </Dialog>
    </section>
  );
}
