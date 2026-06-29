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

import { useEffect, useState, useCallback, useRef, useMemo } from 'react';
import { useRouter } from 'next/navigation';
import {
  Monitor,
  Plus,
  AlertCircle,
  Edit3,
  Check,
  X,
  Apple,
  MonitorDot,
  Server,
  Cpu,
  ChevronDown,
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
import { useAgents } from '@/lib/hooks/use-agents';
import { AgentForm, type AgentFormValues } from '@/components/agents/agent-form';
import { NavBar } from '@/components/ui/navbar';
import { ComputersLeftColumn } from '@/components/computers/computers-left-column';
import { relativeTime, formatDateTime } from '@/lib/utils/time';
import { cn } from '@/lib/utils';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogCloseButton,
} from '@/components/ui/dialog';
import type { Computer } from '@/lib/types';

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
  const { computers, isLoading, error, updateComputer, refetch } = useComputers();
  const { createAgent } = useAgents();
  const { showToast } = useToast();

  // Create agent dialog
  const [showCreateAgent, setShowCreateAgent] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  // Increment after agent creation to force ConnectedAgents remount
  const [agentVersion, setAgentVersion] = useState(0);

  const handleCreateAgent = useCallback(async (values: AgentFormValues) => {
    setIsCreating(true);
    try {
      await createAgent(values);
      setShowCreateAgent(false);
      showToast(t('teamsAgentCreated'), 'success');
      refetch();
      setAgentVersion((v) => v + 1);
    } catch {
      showToast(t('teamsAgentCreateError'), 'error');
    } finally {
      setIsCreating(false);
    }
  }, [isCreating, createAgent, showToast, refetch]);

  // Add computer dialog
  const [showAddDialog, setShowAddDialog] = useState(false);

  // Inline edit state
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editName, setEditName] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const editInputRef = useRef<HTMLInputElement>(null);

  // Selected computer (driven by ComputersLeftColumn)
  const [selectedComputerId, setSelectedComputerId] = useState<string | null>(null);
  const selectedComputer = useMemo(
    () => (selectedComputerId ? computers.find((c) => c.id === selectedComputerId) : undefined),
    [computers, selectedComputerId],
  );

  // Auto-select and expand the first computer on initial load
  const [autoSelected, setAutoSelected] = useState(false);
  useEffect(() => {
    if (!autoSelected && !isLoading && computers.length > 0 && !selectedComputerId) {
      const firstId = computers[0].id;
      setSelectedComputerId(firstId);
      setAutoSelected(true);
    }
  }, [autoSelected, isLoading, computers, selectedComputerId]);

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
    <div className="flex h-screen min-w-[1024px] overflow-hidden bg-brutal-cream">
      <NavBar />
      <div className="w-[220px] flex-shrink-0">
        <ComputersLeftColumn
          computers={computers}
          isLoading={isLoading}
          error={error}
          onRetry={refetch}
          selectedComputerId={selectedComputerId}
          onComputerClick={handleComputerClick}
        />
      </div>

      <main className="flex flex-1 flex-col overflow-hidden bg-white">
        {/* Top bar (page label lives in the left column) */}
        <div className="flex flex-shrink-0 items-center h-14 border-b-2 border-black bg-brutal-cream px-4" />
        <div className="flex-1 overflow-y-auto bg-white">
          <div className={cn('w-full', selectedComputer ? '' : 'px-6 py-6')}>
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
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                {[1, 2, 3, 4].map((i) => (
                  <div
                    key={i}
                    className="border-2 border-black bg-white p-6 shadow-brutal"
                  >
                    <div className="flex items-center gap-3">
                      <Skeleton className="h-10 w-10 rounded-none" />
                      <div className="flex-1 space-y-2">
                        <Skeleton className="h-4 w-28 rounded-none" />
                        <Skeleton className="h-3 w-20 rounded-none" />
                      </div>
                      <Skeleton className="h-3 w-3 rounded-full" />
                    </div>
                    <div className="mt-4 space-y-2">
                      <Skeleton className="h-3 w-40 rounded-none" />
                      <Skeleton className="h-3 w-32 rounded-none" />
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

            {/* Empty state — computers exist, none selected */}
            {!isLoading && !error && computers.length > 0 && !selectedComputer && (
              <EmptyState
                variant="dashed"
                title={t('computersSelectOne')}
              />
            )}

            {/* Computer detail card */}
            {!isLoading && !error && selectedComputer && (
              <ComputerCard
                key={selectedComputer.id}
                computer={selectedComputer}
                editingId={editingId}
                editName={editName}
                isSaving={isSaving}
                editInputRef={editInputRef}
                onStartEdit={handleStartEdit}
                onCancelEdit={handleCancelEdit}
                onSaveName={handleSaveName}
                onEditKeyDown={handleEditKeyDown}
                onEditNameChange={setEditName}
                onCreateAgent={() => setShowCreateAgent(true)}
                agentVersion={agentVersion}
              />
            )}
          </div>
        </div>
      </main>

      {/* Create Agent dialog */}
      <Dialog
        open={showCreateAgent}
        onOpenChange={(open) => { if (!open) setShowCreateAgent(false); }}
      >
        <DialogHeader>
          <DialogTitle>{t('teamsCreateAgent')}</DialogTitle>
          <DialogCloseButton onClick={() => setShowCreateAgent(false)} />
        </DialogHeader>
        <AgentForm
          onSubmit={handleCreateAgent}
          isSubmitting={isCreating}
          submitLabel={t('teamsCreateAgent')}
        />
      </Dialog>

    </div>
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
  onCreateAgent?: () => void;
  agentVersion?: number;
}

function ComputerCard({
  onCreateAgent,
  agentVersion,
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
}: ComputerCardProps) {
  const isOnline = computer.status === 'online';
  const osInfo = getOsIcon(computer.os);

  return (
    <div className="bg-white">
      <div className="border-b-2 border-black bg-white px-4 py-3">
        <div className="flex items-start gap-3">
          <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-info shadow-brutal-sm">
            {osInfo.icon}
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <h3 className="truncate text-base font-heading font-bold text-foreground">
                {computer.name}
              </h3>
              {/* v3.3: chunky status pill replacing the bare dot. */}
              <span
                className={cn(
                  'inline-flex items-center gap-1.5 border-2 border-black px-1.5 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider',
                  isOnline ? 'bg-brutal-success text-black' : 'bg-brutal-muted text-black',
                )}
              >
                <span
                  className={cn(
                    'h-1.5 w-1.5 border border-black',
                    isOnline ? 'bg-white' : 'bg-black',
                  )}
                  aria-hidden
                />
                {isOnline ? t('online') : t('offline')}
              </span>
            </div>
            <p className="mt-1 font-mono text-[11px] text-muted-foreground">
              {isOnline
                ? `${t('computersLastHeartbeat')}: ${relativeTime(computer.last_heartbeat)}`
                : `${t('offline')} ${relativeTime(computer.last_heartbeat, false)}`}
            </p>
          </div>
        </div>
      </div>

      {/* Detail panel */}
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
              <InfoRow label={t('computersLastHeartbeat')}>
                <span>
                  {computer.last_heartbeat
                    ? formatDateTime(computer.last_heartbeat)
                    : t('never')}
                </span>
              </InfoRow>
              <InfoRow label={t('computersRegistered')}>
                <span>{formatDateTime(computer.created_at)}</span>
              </InfoRow>
            </div>
          </section>

          {/* Section: Connected Agents (v1.5) */}
          <section className={detailSectionClass()}>
            <ConnectedAgents key={agentVersion} computerId={computer.id} onCreateAgent={onCreateAgent} />
          </section>

        </div>
      </div>
    </div>
  );
}

// ---- Connected Agents sub-component (lazy-loaded on expand) ----

function ConnectedAgents({ computerId, onCreateAgent }: { computerId: string | null; onCreateAgent?: () => void }) {
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
      <div className="flex items-center justify-between">
        <p className="font-body text-sm text-muted-foreground">
          {t('computersAgentCount', { n: agents.length })}
        </p>
        <Button
          variant="outline"
          size="sm"
          className="h-7 px-2"
          onClick={() => onCreateAgent?.()}
        >
          <Plus className="h-3.5 w-3.5" />
        </Button>
      </div>
      {agents.length === 0 ? (
        <p className="font-body text-sm text-muted-foreground">{t('computersNoConnectedAgents')}</p>
      ) : (
        <ul className="space-y-2">
          {agents.map((agent) => (
            <li key={agent.id}>
              <button
                type="button"
                className="flex w-full items-center gap-3 border-2 border-black bg-brutal-cream p-2.5 text-left transition-all hover:-translate-x-0.5 hover:-translate-y-0.5 hover:shadow-brutal"
                onClick={() => router.push('/teams')}
              >
                <PixelAvatar agentId={agent.id} size="sm" />
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
