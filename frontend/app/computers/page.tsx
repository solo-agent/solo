// ============================================================================
// SOLO-245-F & SOLO-246-F & v1.5: Computers list page with inline detail expansion
// - Brutalist card grid (2 cols desktop, 1 col mobile)
// - v1.5: OS icon, hostname, IP, detected runtimes, connected agents
// - Status indicators (online green / offline gray pulsing)
// - Inline expand on card click for detail view
// - Inline name editing with PATCH
// - Delete confirmation with brutalist dialog
// - Loading skeleton, error state with retry, empty state
// ============================================================================

'use client';

import { useEffect, useState, useCallback, useRef } from 'react';
import { useRouter } from 'next/navigation';
import {
  Monitor,
  Edit3,
  Check,
  X,
  Apple,
  MonitorDot,
  Server,
  ChevronDown,
  Plus,
  Copy,
  Trash2,
  RefreshCw,
  Unplug,
} from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { t } from '@/lib/i18n';
import { useComputers } from '@/lib/hooks/use-computers';
import { useComputerAgents } from '@/lib/hooks/use-computer-agents';
import { Button } from '@/components/ui/button';
import { Spinner } from '@/components/ui/spinner';
import { Skeleton } from '@/components/ui/skeleton';
import { BrutalAlert } from '@/components/ui/brutal-alert';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { useToast } from '@/components/ui/toast';
import { EmptyState } from '@/components/ui/empty-state';
import {
  detailEditActionClass,
  detailFieldLabelClass,
  detailSectionClass,
  detailSectionTitleClass,
} from '@/components/ui/detail-section';
import { PersonalFrame } from '@/components/layout/personal-frame';
import { relativeTime, formatDateTime } from '@/lib/utils/time';
import { cn } from '@/lib/utils';
import type { Computer } from '@/lib/types';
import { Dialog, DialogCloseButton, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { computerPairingCommands } from '@/lib/computer-pairing';
import { RuntimeLogo } from '@/components/agents/runtime-logo';
import { isSupportedAgentRuntime } from '@/lib/agent-runtimes';

// ---- OS icon helper ----

function getOsIcon(os?: string): { icon: React.ReactNode; label: string } {
  if (!os) return { icon: <MonitorDot className="h-4 w-4" />, label: t('unknown') };
  const lower = os.toLowerCase();
  if (lower.includes('darwin') || lower.includes('mac')) {
    return { icon: <Apple className="h-4 w-4" />, label: 'macOS' };
  }
  if (lower.includes('linux')) {
    return { icon: <Server className="h-4 w-4" />, label: 'Linux' };
  }
  if (lower.includes('windows') || lower.includes('win')) {
    return { icon: <Monitor className="h-4 w-4" />, label: 'Windows' };
  }
  return { icon: <MonitorDot className="h-4 w-4" />, label: os };
}

function runtimeBadgeClass(type: string): string {
  if (type === 'claude') return 'bg-brutal-warning-light';
  if (type === 'codex') return 'bg-brutal-info-light';
  if (type === 'opencode') return 'bg-brutal-violet-light';
  if (type === 'openclaw') return 'bg-brutal-accent-light';
  return 'bg-brutal-cream';
}

// Agent status indicator
function AgentStatusDot({ status }: { status: string }) {
  const colorMap: Record<string, string> = {
    online: 'bg-brutal-success',
    thinking: 'bg-brutal-accent',
    running: 'bg-brutal-info',
    offline: 'bg-brutal-muted',
  };
  const labelMap: Record<string, string> = {
    online: t('agentIdle'),
    thinking: t('agentThinkingShort'),
    running: t('agentExecuting'),
    offline: t('offline'),
  };
  return (
    <span className="flex items-center gap-1.5 text-xs">
      <span
        className={cn(
          'inline-block h-2 w-2 flex-shrink-0 border border-black',
          colorMap[status] || 'bg-brutal-muted',
        )}
      />
      <span className="text-muted-foreground">{labelMap[status] || status}</span>
    </span>
  );
}

export default function ComputersPage() {
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const { computers, isLoading, error, addComputer, updateComputer, deleteComputer, createEnrollment, revokeCredential, refetch } = useComputers();
  const { showToast } = useToast();

  // Inline edit state
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editName, setEditName] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [pairDialogOpen, setPairDialogOpen] = useState(false);
  const [pairingComputer, setPairingComputer] = useState<Computer | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Computer | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const editInputRef = useRef<HTMLInputElement>(null);

  // Selected computer (driven by ComputersLeftColumn)
  const [selectedComputerId, setSelectedComputerId] = useState<string | null>(null);

  // Left-column click: re-click clears selection; switching resets edit
  const handleComputerClick = useCallback((id: string) => {
    setSelectedComputerId((prev) => (prev === id ? null : id));
    setEditingId(null);
  }, []);

  // Focus edit input when editing starts
  useEffect(() => {
    if (editingId && editInputRef.current) {
      editInputRef.current.focus();
      editInputRef.current.select();
    }
  }, [editingId]);

  const handleStartEdit = useCallback((computer: Computer) => {
    setEditingId(computer.id);
    setEditName(computer.name);
  }, []);

  const handleCancelEdit = useCallback(() => {
    setEditingId(null);
    setEditName('');
  }, []);

  const handleSaveName = useCallback(async (computerId: string) => {
    if (!editName.trim()) return;
    setIsSaving(true);
    try {
      await updateComputer(computerId, { name: editName.trim() });
      setEditingId(null);
      showToast(t('computersNameUpdated'), 'success');
    } catch (err) {
      const message = err instanceof Error ? err.message : t('computersNameUpdateError');
      showToast(message, 'error');
    } finally {
      setIsSaving(false);
    }
  }, [editName, updateComputer, showToast]);

  const handleEditKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>, computerId: string) => {
      if (e.key === 'Enter') {
        handleSaveName(computerId);
      } else if (e.key === 'Escape') {
        handleCancelEdit();
      }
    },
    [handleSaveName, handleCancelEdit],
  );

  const openPairDialog = useCallback(async () => {
    setPairingComputer(null);
    setPairDialogOpen(true);
    try {
      const pending = computers.find((computer) => computer.pairing_status === 'pending');
      const computer = pending ? await createEnrollment(pending.id) : await addComputer(t('computersDefaultName'));
      setPairingComputer(computer);
      setSelectedComputerId(computer.id);
    } catch (err) {
      showToast(err instanceof Error ? err.message : t('computersPairError'), 'error');
      setPairDialogOpen(false);
    }
  }, [addComputer, computers, createEnrollment, showToast]);

  const showEnrollment = useCallback(async (computer: Computer) => {
    setPairingComputer(null);
    setPairDialogOpen(true);
    try {
      setPairingComputer(await createEnrollment(computer.id));
    } catch (err) {
      showToast(err instanceof Error ? err.message : t('computersPairError'), 'error');
      setPairDialogOpen(false);
    }
  }, [createEnrollment, showToast]);

  const copyPairingCommand = useCallback(async (command: string) => {
    try {
      await navigator.clipboard.writeText(command);
      showToast(t('computersCommandCopied'), 'success');
    } catch {
      showToast(t('computersCommandCopyError'), 'error');
    }
  }, [showToast]);

  const handleDeleteComputer = useCallback(async () => {
    if (!deleteTarget) return;
    setIsDeleting(true);
    setDeleteError(null);
    try {
      await deleteComputer(deleteTarget.id);
      setSelectedComputerId((current) => current === deleteTarget.id ? null : current);
      setDeleteTarget(null);
      showToast(t('computersDeleted'), 'success');
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : t('computersDeleteError'));
    } finally {
      setIsDeleting(false);
    }
  }, [deleteComputer, deleteTarget, showToast]);

  const pairingCommands = pairingComputer?.enrollment_token
    ? computerPairingCommands(pairingComputer.id, pairingComputer.enrollment_token)
    : null;

  // Auth loading state
  if (authLoading || !isAuthenticated) {
    return (
      <div className="flex h-screen items-center justify-center bg-brutal-cream">
        <div className="flex flex-col items-center gap-3">
          <Spinner size="md" />
          <p className="font-mono text-sm text-muted-foreground">{t('loading')}</p>
        </div>
      </div>
    );
  }

  return (
    <PersonalFrame>
      <div className="flex h-full flex-col overflow-hidden">
        <div className="flex flex-shrink-0 items-center justify-between gap-4 border-b-2 border-black bg-brutal-cream px-8 py-6">
          <div>
            <h1 className="font-heading text-2xl font-black">{t('personalComputers')}</h1>
            <p className="mt-1 font-body text-sm text-muted-foreground">{t('personalComputerDesc')}</p>
          </div>
          <Button type="button" size="sm" data-onboarding="connect-computer" onClick={() => void openPairDialog()}><Plus className="mr-1.5 h-4 w-4" />{t('computersAddComputer')}</Button>
        </div>
        <div className="flex-1 overflow-y-auto bg-white">
          <div className="mx-auto w-full max-w-5xl px-8 py-6">
            {/* Error state */}
            {error && (
              <div className="mb-6 space-y-2">
                <BrutalAlert variant="warning" className="p-4">
                  {error}
                </BrutalAlert>
                <Button variant="outline" size="sm" onClick={refetch}>
                  {t('retry')}
                </Button>
              </div>
            )}

            {/* Loading skeleton */}
            {isLoading && (
              <div className="space-y-3">
                {[1, 2].map((i) => (
                  <div
                    key={i}
                    className="border-2 border-black bg-white p-5 shadow-brutal"
                  >
                    <div className="flex items-center gap-3">
                      <Skeleton className="h-10 w-10 rounded-none" />
                      <div className="flex-1 space-y-2">
                        <Skeleton className="h-4 w-28 rounded-none" />
                        <Skeleton className="h-3 w-20 rounded-none" />
                      </div>
                      <Skeleton className="h-3 w-3 rounded-full" />
                    </div>
                  </div>
                ))}
              </div>
            )}

            {/* Empty state — no computers at all */}
            {!isLoading && !error && computers.length === 0 && (
              <EmptyState
                variant="dashed"
                rotation={-0.5}
                icon={
                  <div className="flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-info shadow-brutal-sm">
                    <Monitor className="h-6 w-6 text-white" />
                  </div>
                }
                title={t('computersNoComputers')}
                description={t('computersNoComputersDesc')}
              />
            )}

            {!isLoading && !error && computers.length > 0 && (
              <div className="space-y-3">
                {computers.map((computer) => {
                  const isOnline = computer.status === 'online';
                  const osInfo = getOsIcon(computer.os);
                  const runtimes = (computer.runtime_inventory ?? []).filter((runtime) => isSupportedAgentRuntime(runtime.type));
                  return (
                    <div key={computer.id} className="overflow-hidden border-2 border-black bg-white shadow-brutal-sm">
                      <button
                        type="button"
                        onClick={() => handleComputerClick(computer.id)}
                        aria-expanded={computer.id === selectedComputerId}
                        className="flex w-full items-start gap-4 bg-white px-5 py-4 text-left transition-colors hover:bg-brutal-cream/40"
                      >
                        <span className={cn('flex h-12 w-12 shrink-0 items-center justify-center border-2 border-black shadow-brutal-sm', isOnline ? 'bg-brutal-success-light' : 'bg-brutal-muted-light')}>
                          {osInfo.icon}
                        </span>
                        <span className="min-w-0 flex-1">
                          <span className="flex flex-wrap items-center gap-2">
                            <span className="truncate font-heading text-base font-black">{computer.name}</span>
                            <span className={cn('inline-flex items-center gap-1.5 border border-black px-2 py-0.5 font-body text-xs', isOnline ? 'bg-brutal-success-light' : 'bg-brutal-muted-light')}>
                              <span className={cn('h-2 w-2 rounded-full border border-black', isOnline ? 'bg-brutal-success' : 'bg-brutal-muted')} aria-hidden="true" />
                              {isOnline ? t('online') : t('offline')}
                            </span>
                          </span>
                          <span className="mt-1 block truncate font-mono text-[11px] text-muted-foreground">
                            {[osInfo.label, computer.hostname, computer.daemon_version, relativeTime(computer.last_heartbeat)].filter(Boolean).join(' · ')}
                          </span>
                          {runtimes.length > 0 && (
                            <span className="mt-3 flex flex-wrap gap-2">
                              {runtimes.map((runtime) => (
                                <span
                                  key={runtime.type}
                                  aria-disabled={!runtime.available}
                                  title={!runtime.available ? t('computersRuntimeUnavailable') : undefined}
                                  className={cn(
                                    'inline-flex items-center gap-1.5 border px-2 py-1 font-body text-xs',
                                    runtime.available
                                      ? cn('border-black', runtimeBadgeClass(runtime.type))
                                      : 'border-black/20 bg-brutal-muted-light text-muted-foreground opacity-45 grayscale',
                                  )}
                                >
                                  <RuntimeLogo runtime={runtime.type} className="h-3.5 w-3.5 shrink-0" />
                                  {runtime.display_name || runtime.type}
                                </span>
                              ))}
                            </span>
                          )}
                        </span>
                        <ChevronDown className={cn('mt-1 h-4 w-4 shrink-0 transition-transform', computer.id === selectedComputerId && 'rotate-180')} aria-hidden="true" />
                      </button>
                      {!isOnline && computer.my_role === 'owner' && (
                        <div className="flex justify-end px-5 pb-4">
                          <Button type="button" size="sm" variant="outline" onClick={() => void showEnrollment(computer)}>
                            <RefreshCw className="mr-1.5 h-4 w-4" />
                            {t('computersReconnect')}
                          </Button>
                        </div>
                      )}
                      {computer.id === selectedComputerId && (
                        <ComputerCard
                          computer={computer}
                          editingId={editingId}
                          editName={editName}
                          isSaving={isSaving}
                          editInputRef={editInputRef}
                          onStartEdit={handleStartEdit}
                          onCancelEdit={handleCancelEdit}
                          onSaveName={handleSaveName}
                          onEditKeyDown={handleEditKeyDown}
                          onEditNameChange={setEditName}
                          onRevokeCredential={async (item) => { await revokeCredential(item.id); }}
                          onDelete={(item) => {
                            setDeleteError(null);
                            setDeleteTarget(item);
                          }}
                        />
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      </div>

      <Dialog open={pairDialogOpen} onOpenChange={setPairDialogOpen} width="lg">
        <DialogHeader>
          <DialogTitle>{t('computersPairTitle')}</DialogTitle>
          <DialogCloseButton onClick={() => setPairDialogOpen(false)} />
        </DialogHeader>
        {!pairingCommands ? (
          <div className="flex items-center gap-3 py-8 font-body text-sm text-muted-foreground">
            <Spinner size="sm" />{t('computersCreatingConnection')}
          </div>
        ) : (
          <div className="space-y-3">
            <p className="font-body text-sm">{t('computersPairInstructions')}</p>
            <div className="flex items-center justify-between gap-2">
              <p className="font-heading text-xs font-bold uppercase">{t('computersFreshInstall')}</p>
              <Button type="button" size="sm" variant="outline" onClick={() => void copyPairingCommand(pairingCommands.fresh)}><Copy className="mr-1.5 h-4 w-4" />{t('copy')}</Button>
            </div>
            <pre className="overflow-x-auto border-2 border-black bg-black p-3 font-mono text-xs text-white">{pairingCommands.fresh}</pre>
            <div className="flex items-center justify-between gap-2">
              <p className="font-heading text-xs font-bold uppercase">{t('computersAlreadyInstalled')}</p>
              <Button type="button" size="sm" variant="outline" onClick={() => void copyPairingCommand(pairingCommands.installed)}><Copy className="mr-1.5 h-4 w-4" />{t('copy')}</Button>
            </div>
            <pre className="overflow-x-auto border-2 border-black bg-black p-3 font-mono text-xs text-white">{pairingCommands.installed}</pre>
            <p className="font-mono text-[11px] text-brutal-danger">{t('computersPairOnce')}</p>
          </div>
        )}
      </Dialog>

      <Dialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open && !isDeleting) {
            setDeleteTarget(null);
            setDeleteError(null);
          }
        }}
        width="md"
      >
        <DialogHeader>
          <DialogTitle>{t('computersDeleteTitle')}</DialogTitle>
          <DialogCloseButton onClick={() => {
            if (!isDeleting) {
              setDeleteTarget(null);
              setDeleteError(null);
            }
          }} />
        </DialogHeader>
        {deleteTarget && (
          <div className="space-y-4">
            <BrutalAlert variant="warning" title={t('computersDeleteWarning')}>
              {t('computersDeleteDescription')}
            </BrutalAlert>
            <div className="border-2 border-black bg-white px-3 py-2">
              <p className="font-heading text-sm font-bold">{deleteTarget.name}</p>
              <p className="mt-1 break-all font-mono text-[10px] text-muted-foreground">{deleteTarget.id}</p>
            </div>
            {deleteError && <BrutalAlert variant="error">{deleteError}</BrutalAlert>}
          </div>
        )}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setDeleteTarget(null)} disabled={isDeleting}>{t('cancel')}</Button>
          <Button type="button" variant="danger" onClick={() => void handleDeleteComputer()} disabled={isDeleting}>
            <Trash2 className="mr-1.5 h-4 w-4" />
            {isDeleting ? t('deleting') : t('computersDelete')}
          </Button>
        </DialogFooter>
      </Dialog>
    </PersonalFrame>
  );
}

// ---- Computer Card component (extracted for clarity) ----

interface ComputerCardProps {
  computer: Computer;
  editingId: string | null;
  editName: string;
  isSaving: boolean;
  editInputRef: React.RefObject<HTMLInputElement | null>;
  onStartEdit: (computer: Computer) => void;
  onCancelEdit: () => void;
  onSaveName: (id: string) => void;
  onEditKeyDown: (e: React.KeyboardEvent<HTMLInputElement>, id: string) => void;
  onEditNameChange: (name: string) => void;
  onRevokeCredential: (computer: Computer) => void;
  onDelete: (computer: Computer) => void;
}

function ComputerCard({
  computer,
  editingId,
  editName,
  isSaving,
  editInputRef,
  onStartEdit,
  onCancelEdit,
  onSaveName,
  onEditKeyDown,
  onEditNameChange,
  onRevokeCredential,
  onDelete,
}: ComputerCardProps) {
  const isOnline = computer.status === 'online';
  const osInfo = getOsIcon(computer.os);

  return (
    <div className="border-t-2 border-black bg-brutal-cream/40">
      <div>
        <div className="space-y-4 bg-white p-4">
          {/* Section: System Info */}
          <section className={detailSectionClass()}>
            <SectionHeader label={t('computersSystemInfo')} />
            <div className="space-y-1 font-body text-sm">
              {computer.os && (
                <InfoRow label={t('computersOS')}>
                  <span className="flex items-center gap-1.5">
                    {osInfo.icon}
                    {osInfo.label}
                  </span>
                </InfoRow>
              )}
              {computer.hostname && (
                <InfoRow label={t('computersHostname')}>
                  <span className="font-mono text-xs">{computer.hostname}</span>
                </InfoRow>
              )}
              {computer.ip && (
                <InfoRow label={t('computersIP')}>
                  <span className="font-mono text-xs">{computer.ip}</span>
                </InfoRow>
              )}
            </div>
          </section>

          <section className={detailSectionClass()}>
            <SectionHeader label={t('computersBasicInfo')} />
            <div className="space-y-1 font-body text-sm">
              <InfoRow label={t('computersName')}>
                {editingId === computer.id ? (
                  <div className="flex w-full items-center justify-between gap-3">
                    <input
                      ref={editInputRef}
                      type="text"
                      value={editName}
                      onChange={(e) => onEditNameChange(e.target.value)}
                      onKeyDown={(e) => onEditKeyDown(e, computer.id)}
                      className="input-brutal h-8 w-full max-w-sm py-1 text-sm"
                      disabled={isSaving}
                    />
                    <div className="flex flex-shrink-0 items-center gap-1.5">
                      <Button
                        type="button"
                        variant="success"
                        size="sm"
                        onClick={() => onSaveName(computer.id)}
                        disabled={isSaving || !editName.trim()}
                        aria-label={t('computersSaveName')}
                        className="gap-1 text-[10px] uppercase tracking-wider"
                      >
                        <Check className="h-3 w-3" />
                        {isSaving ? t('saving') : t('save')}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={onCancelEdit}
                        disabled={isSaving}
                        aria-label={t('computersCancelEdit')}
                        className="gap-1 text-[10px] uppercase tracking-wider"
                      >
                        <X className="h-3 w-3" />
                        {t('cancel')}
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className="flex w-full items-center justify-between gap-3">
                    <span className="min-w-0 truncate font-bold">{computer.name}</span>
                    <button
                      type="button"
                      onClick={() => onStartEdit(computer)}
                      className={detailEditActionClass()}
                      aria-label={t('computersEditName')}
                    >
                      <Edit3 className="h-3 w-3" />
                      {t('edit')}
                    </button>
                  </div>
                )}
              </InfoRow>
              <InfoRow label="ID">
                <span className="font-mono text-xs">{computer.id}</span>
              </InfoRow>
              {computer.daemon_id && (
                <InfoRow label={t('computersDaemonID')}>
                  <span className="font-mono text-xs">{computer.daemon_id}</span>
                </InfoRow>
              )}
              {computer.daemon_url && (
                <InfoRow label={t('computersDaemonURL')}>
                  <span className="font-mono text-xs">{computer.daemon_url}</span>
                </InfoRow>
              )}
            </div>
          </section>

          {/* Section: Status */}
          <section className={detailSectionClass()}>
            <SectionHeader label={t('computersStatus')} />
            <div className="space-y-1 font-body text-sm">
              <InfoRow label={t('computersCurrent')}>
                <div className="flex items-center gap-2">
                  <StatusDot isOnline={isOnline} />
                  <span>{isOnline ? t('online') : t('offline')}</span>
                </div>
              </InfoRow>
              <InfoRow label={t('computersPairingStatus')}>
                <div className="flex items-center gap-2">
                  <span className="font-mono text-xs">{computer.pairing_status}</span>
                  {computer.my_role === 'owner' && isOnline && computer.pairing_status === 'paired' && (
                    <Button type="button" size="sm" variant="danger" onClick={() => onRevokeCredential(computer)}>
                      <Unplug className="mr-1.5 h-4 w-4" />
                      {t('computersDisconnect')}
                    </Button>
                  )}
                </div>
              </InfoRow>
              <InfoRow label={t('computersLastHeartbeat')}>
                <span>
                  {computer.last_heartbeat
                    ? formatDateTime(computer.last_heartbeat)
                    : t('never')}
                </span>
              </InfoRow>
              {computer.daemon_version && (
                <InfoRow label={t('computersDaemonVersion')}>
                  <span className="font-mono text-xs">{computer.daemon_version}</span>
                </InfoRow>
              )}
              {!!computer.protocol_version && (
                <InfoRow label={t('computersProtocolVersion')}>
                  <span className="font-mono text-xs">v{computer.protocol_version}</span>
                </InfoRow>
              )}
              {computer.last_connected_at && (
                <InfoRow label={t('computersLastConnected')}>
                  <span>{formatDateTime(computer.last_connected_at)}</span>
                </InfoRow>
              )}
              <InfoRow label={t('computersRegistered')}>
                <span>{formatDateTime(computer.created_at)}</span>
              </InfoRow>
            </div>
          </section>

          {/* Section: Connected Agents (v1.5) */}
          <section className={detailSectionClass()}>
            <ConnectedAgents computerId={computer.id} />
          </section>

          {computer.my_role === 'owner' && (
            <section className={detailSectionClass('border-brutal-danger')}>
              <SectionHeader label={t('computersDangerZone')} />
              <div className="flex flex-wrap items-center justify-between gap-3">
                <p className="max-w-2xl font-body text-sm text-muted-foreground">{t('computersDeleteHint')}</p>
                <Button type="button" variant="danger" onClick={() => onDelete(computer)}>
                  <Trash2 className="mr-1.5 h-4 w-4" />
                  {t('computersDelete')}
                </Button>
              </div>
            </section>
          )}

        </div>
      </div>
    </div>
  );
}

// ---- Connected Agents sub-component (lazy-loaded on expand) ----

function ConnectedAgents({ computerId }: { computerId: string | null }) {
  const { agents, isLoading, error } = useComputerAgents(computerId);
  const router = useRouter();
  const [isOpen, setIsOpen] = useState(false);

  if (!computerId) {
    return <p className="font-body text-sm text-muted-foreground">{t('computersExpandCard')}</p>;
  }

  const Header = (
    <button
      type="button"
      onClick={() => setIsOpen((open) => !open)}
      className="flex w-full items-center justify-between gap-2 text-left"
    >
      <span className={detailSectionTitleClass()}>
        ★ {t('computersConnectedAgents')}
        {!isLoading && !error && (
          <span className="ml-1 inline-block border-2 border-black bg-white px-1 font-mono text-[9px] text-black">
            {agents.length}
          </span>
        )}
      </span>
      <ChevronDown className={cn('h-4 w-4 transition-transform', isOpen && 'rotate-180')} />
    </button>
  );

  if (!isOpen) {
    return Header;
  }

  if (isLoading) {
    return (
      <>
        {Header}
        <div className="mt-3 flex items-center gap-2 py-2">
          <Spinner size="sm" />
          <span className="text-sm text-muted-foreground">{t('loading')}</span>
        </div>
      </>
    );
  }

  if (error) {
    return (
      <>
        {Header}
        <p className="mt-3 font-body text-sm text-muted-foreground">{error}</p>
      </>
    );
  }

  return (
    <div className="space-y-3">
      {Header}
      <p className="font-body text-sm text-muted-foreground">
        {t('computersAgentCount', { n: agents.length })}
      </p>
      {agents.length === 0 ? (
        <p className="font-body text-sm text-muted-foreground">{t('computersNoConnectedAgents')}</p>
      ) : (
        <ul className="space-y-2">
          {agents.map((agent) => (
            <li key={agent.id}>
              <button
                type="button"
                className="flex w-full items-center gap-3 border-2 border-black bg-brutal-cream p-2.5 text-left transition-all hover:-translate-x-0.5 hover:-translate-y-0.5 hover:shadow-brutal"
                onClick={() => router.push('/dashboard')}
              >
                <PixelAvatar agentId={agent.id} avatarUrl={agent.avatar_url} size="sm" />
                <div className="flex-1 min-w-0">
                  <span className="block truncate font-body text-sm font-medium text-foreground">
                    {agent.name}
                  </span>
                  <AgentStatusDot status={agent.status} />
                </div>
                <div className="flex-shrink-0 text-right">
                  <span className="text-[11px] text-muted-foreground">
                    {t('computersActiveTasks')}
                  </span>
                  <span className="block font-mono text-sm font-bold text-foreground">
                    {agent.active_tasks}
                  </span>
                </div>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// ---- Sub-components ----

function StatusDot({ isOnline }: { isOnline: boolean }) {
  return (
    <span
      className={cn(
        'inline-block h-2.5 w-2.5 flex-shrink-0 border border-black',
        isOnline ? 'bg-brutal-success' : 'bg-brutal-muted animate-pulse',
      )}
      role="status"
      aria-label={isOnline ? t('online') : t('offline')}
    />
  );
}

function SectionHeader({ label, className }: { label: string; className?: string }) {
  return (
    <h3 className={cn('mb-3', className)}>
      <span className={detailSectionTitleClass()}>
        ★ {label}
      </span>
    </h3>
  );
}

function InfoRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-3 py-1.5">
      <span className={detailFieldLabelClass('flex-shrink-0')}>
        {label}
      </span>
      <div className="flex-1 min-w-0">{children}</div>
    </div>
  );
}
