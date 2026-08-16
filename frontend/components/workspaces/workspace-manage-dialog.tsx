'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { Trash2 } from 'lucide-react';
import {
  Dialog,
  DialogCloseButton,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button, iconActionClass } from '@/components/ui/button';
import { useWorkspace, type ManageTabKey } from '@/lib/workspace-context';
import { WorkspaceSettingsCard } from '@/components/workspaces/workspace-members-dialog';
import { useToast } from '@/components/ui/toast';
import { t } from '@/lib/i18n';
import { cn } from '@/lib/utils';

const TABS = [
  { key: 'overview' as const, labelKey: 'workspaceManageOverview' },
  { key: 'members' as const, labelKey: 'workspaceManageMembers' },
  { key: 'invites' as const, labelKey: 'workspaceManageInvitations' },
] as const;

// Dialog driven by WorkspaceContext. Overview shows read-only summary;
// Members and Invites each own their matching settings sections. Notifications is omitted
// per P0-09 A — the rail does not gain an empty bell tab.
export function WorkspaceManageDialog() {
  const router = useRouter();
  const { activeWorkspace, deleteWorkspace, manageDialog, openManage, closeManage } = useWorkspace();
  const { showToast } = useToast();
  const [tab, setTab] = useState<ManageTabKey>(manageDialog.tab);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (manageDialog.open) setTab(manageDialog.tab);
  }, [manageDialog.open, manageDialog.tab]);

  const canDelete = activeWorkspace?.role === 'owner'
    && !activeWorkspace.is_default
    && !activeWorkspace.is_personal;

  const remove = async () => {
    if (!activeWorkspace) return;
    const confirmed = window.confirm(t('deleteWorkspaceConfirm'));
    if (!confirmed) return;
    setBusy(true);
    try {
      await deleteWorkspace(activeWorkspace.id);
      closeManage();
      router.push('/dashboard');
      showToast(t('workspaceDeleted'), 'success');
    } catch (error) {
      showToast(error instanceof Error ? error.message : t('workspaceDeleteFailed'), 'error');
    } finally {
      setBusy(false);
    }
  };

  if (!activeWorkspace) return null;

  return (
    <Dialog open={manageDialog.open} onOpenChange={(next) => next ? openManage(tab) : closeManage()} width="xl">
      <DialogHeader>
        <DialogTitle>{t('workspaceManageTitle')}</DialogTitle>
        <DialogCloseButton onClick={closeManage} />
      </DialogHeader>

      <div className="flex min-h-[320px] gap-4">
        <div role="tablist" className="flex w-[140px] flex-shrink-0 flex-col gap-1 border-r-2 border-black pr-3">
          {TABS.map((item) => (
            <button
              key={item.key}
              type="button"
              role="tab"
              aria-selected={tab === item.key}
              onClick={() => setTab(item.key)}
              className={cn(
                'border-2 px-3 py-2 text-left font-heading text-xs font-black',
                tab === item.key
                  ? 'border-black bg-brutal-primary shadow-brutal-sm'
                  : 'border-transparent hover:border-black hover:bg-white',
              )}
            >
              {t(item.labelKey)}
            </button>
          ))}
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto pr-1" role="tabpanel">
          {tab === 'overview' && (
            <div className="flex flex-col gap-4">
              <div className="flex items-start gap-3 border-2 border-black bg-white p-3 shadow-brutal-sm">
                <span className="flex h-12 w-12 shrink-0 items-center justify-center border-2 border-black bg-brutal-primary font-heading text-lg font-black">
                  {activeWorkspace.icon?.slice(0, 2) || 'S'}
                </span>
                <div className="min-w-0 flex-1">
                  <div className="font-heading text-base font-black">{activeWorkspace.name}</div>
                  <div className="mt-0.5 font-mono text-[10px] font-bold uppercase text-black/55">
                    {activeWorkspace.role} · {activeWorkspace.member_count} {t('workspaceMemberCount')}
                  </div>
                </div>
              </div>

              <div className="border-2 border-dashed border-black/30 bg-brutal-cream p-3 text-xs text-black/60">
                {t('workspaceManageOverviewPending')}
              </div>

              {canDelete && (
                <div className="mt-auto border-t-2 border-black pt-4">
                  <Button
                    variant="outline"
                    onClick={() => void remove()}
                    disabled={busy}
                    className={iconActionClass('border-red-700 text-red-700 hover:bg-red-50')}
                  >
                    <Trash2 className="mr-2 h-4 w-4" />
                    {t('deleteWorkspaceButton')}
                  </Button>
                </div>
              )}
            </div>
          )}

          {tab === 'members' && <WorkspaceSettingsCard bare view="members" />}
          {tab === 'invites' && <WorkspaceSettingsCard bare view="invites" />}
        </div>
      </div>
    </Dialog>
  );
}
