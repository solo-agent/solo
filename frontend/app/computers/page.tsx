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
import { useRouter, useSearchParams } from 'next/navigation';
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
  Globe,
  Cpu,
  ChevronDown,
  ChevronUp,
  ExternalLink,
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
  DialogDescription,
  DialogFooter,
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
  const router = useRouter();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const { computers, isLoading, error, updateComputer, deleteComputer, refetch } = useComputers();
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

  // Expanded card state
  const [expandedId, setExpandedId] = useState<string | null>(null);

  // Add computer dialog
  const [showAddDialog, setShowAddDialog] = useState(false);

  // Inline edit state
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editName, setEditName] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const editInputRef = useRef<HTMLInputElement>(null);

  // Delete confirmation state
  const [deleteTargetId, setDeleteTargetId] = useState<string | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);
  const deleteTargetName =
    deleteTargetId ? computers.find((c) => c.id === deleteTargetId)?.name : null;

  // Selected computer (driven by ComputersLeftColumn + URL)
  const searchParams = useSearchParams();
  const [selectedComputerId, setSelectedComputerId] = useState<string | null>(null);
  const selectedComputer = useMemo(
    () => (selectedComputerId ? computers.find((c) => c.id === selectedComputerId) : undefined),
    [computers, selectedComputerId],
  );

  // Initialize from URL param on load.
  const [urlInitialized, setUrlInitialized] = useState(false);
  useEffect(() => {
    if (urlInitialized || isLoading || computers.length === 0) return;
    const idParam = searchParams.get('id');
    if (idParam && computers.some((c) => c.id === idParam)) {
      setSelectedComputerId(idParam);
      setExpandedId(idParam);
    } else {
      const firstId = computers[0].id;
      setSelectedComputerId(firstId);
      setExpandedId(firstId);
      router.replace(`/computers?id=${firstId}`, { scroll: false });
    }
    setUrlInitialized(true);
  }, [urlInitialized, isLoading, computers, searchParams, router]);

  // Left-column click: re-click clears selection; switching resets edit/expand
  const handleComputerClick = useCallback((id: string) => {
    if (selectedComputerId === id) {
      setSelectedComputerId(null);
    } else {
      setSelectedComputerId(id);
      router.replace(`/computers?id=${id}`, { scroll: false });
    }
    setEditingId(null);
    setExpandedId(null);
  }, [selectedComputerId, router]);

  // Focus edit input when editing starts
  useEffect(() => {
    if (editingId && editInputRef.current) {
      editInputRef.current.focus();
      editInputRef.current.select();
    }
  }, [editingId]);

  const handleToggleExpand = useCallback((id: string) => {
    setExpandedId((prev) => (prev === id ? null : id));
    setEditingId(null);
  }, []);

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

  const handleDelete = useCallback(async () => {
    if (!deleteTargetId) return;
    setIsDeleting(true);
    try {
      await deleteComputer(deleteTargetId);
      // If the deleted computer was selected, clear the selection so the
      // left-column click can re-select it later (rather than the toggle
      // seeing it as already-selected and clearing).
      setSelectedComputerId((prev) => (prev === deleteTargetId ? null : prev));
      setExpandedId(null);
      showToast(t('computersRemoved'), 'success');
    } catch (err) {
      const message = err instanceof Error ? err.message : t('computersRemoveError');
      showToast(message, 'error');
    } finally {
      setIsDeleting(false);
      setDeleteTargetId(null);
    }
  }, [deleteTargetId, deleteComputer, showToast]);

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

      <main className="flex flex-1 flex-col overflow-hidden">
        {/* Top bar (page label lives in the left column) */}
        <div className="flex flex-shrink-0 items-center h-14 border-b-2 border-black bg-brutal-cream px-4" />
        <div className="flex-1 overflow-y-auto px-6 py-6">
          <div className="w-full">
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
                isExpanded={expandedId === selectedComputer.id}
                editingId={editingId}
                editName={editName}
                isSaving={isSaving}
                editInputRef={editInputRef}
                onToggleExpand={handleToggleExpand}
                onStartEdit={handleStartEdit}
                onCancelEdit={handleCancelEdit}
                onSaveName={handleSaveName}
                onEditKeyDown={handleEditKeyDown}
                onEditNameChange={setEditName}
                onDeleteClick={setDeleteTargetId}
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
  isExpanded: boolean;
  editingId: string | null;
  editName: string;
  isSaving: boolean;
  editInputRef: React.RefObject<HTMLInputElement | null>;
  onToggleExpand: (id: string) => void;
  onStartEdit: (computer: Computer) => void;
  onCancelEdit: () => void;
  onSaveName: (id: string) => void;
  onEditKeyDown: (e: React.KeyboardEvent<HTMLInputElement>, id: string) => void;
  onEditNameChange: (name: string) => void;
  onDeleteClick: (id: string) => void;
  onCreateAgent?: () => void;
  agentVersion?: number;
}

function ComputerCard({
  onCreateAgent,
  agentVersion,
  computer,
  isExpanded,
  editingId,
  editName,
  isSaving,
  editInputRef,
  onToggleExpand,
  onStartEdit,
  onCancelEdit,
  onSaveName,
  onEditKeyDown,
  onEditNameChange,
  onDeleteClick,
}: ComputerCardProps) {
  const isOnline = computer.status === 'online';
  const osInfo = getOsIcon(computer.os);

  return (
    <div className="transition-all duration-300">
      {/* Card header — click to expand */}
      <button
        type="button"
        className="w-full p-6 text-left"
        onClick={() => onToggleExpand(computer.id)}
        aria-expanded={isExpanded}
        aria-label={`${computer.name} — ${isOnline ? t('online') : t('offline')}`}
      >
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

        {/* Quick info — v3.3: inline chunky pill rows instead of bare muted text. */}
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <span className="inline-flex items-center gap-1.5 border-2 border-black bg-brutal-cream px-1.5 py-0.5 font-mono text-[11px] text-black">
            {osInfo.icon}
            <span className="truncate max-w-[120px]">
              {computer.hostname || osInfo.label}
            </span>
          </span>
          {computer.ip && (
            <span className="inline-flex items-center gap-1 border-2 border-black bg-brutal-cream px-1.5 py-0.5 font-mono text-[11px] text-black">
              <Globe className="h-3 w-3" />
              {computer.ip}
            </span>
          )}
        </div>

        {/* Expand indicator — v3.3: chunky 2px-bordered pill, not a thin chevron. */}
        <div className="mt-3 flex justify-center">
          <span className="inline-flex h-5 w-5 items-center justify-center border-2 border-black bg-white text-[10px] font-bold text-black">
            {isExpanded ? '−' : '+'}
          </span>
        </div>
      </button>

      {/* Expanded detail panel */}
      <div
        className={cn(
          'overflow-hidden transition-all duration-300 ease-in-out',
          isExpanded ? 'max-h-[1200px] opacity-100' : 'max-h-0 opacity-0',
        )}
      >
        <div className="border-t-2 border-black px-6 pb-6 pt-4 space-y-6">
          {/* Section: System Info */}
          <section>
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

          {/* Section: Basic Info — separated by a full-width 2px divider
              to match teams detail's tab-bar-style structure. */}
          <hr className="border-t-2 border-black" />
          <section>
            <SectionHeader label={t('computersBasicInfo')} />
            <div className="space-y-1 font-body text-sm">
            <InfoRow label={t('computersName')}>
              {editingId === computer.id ? (
                <div className="flex items-center gap-2">
                  <input
                    ref={editInputRef}
                    type="text"
                    value={editName}
                    onChange={(e) => onEditNameChange(e.target.value)}
                    onKeyDown={(e) => onEditKeyDown(e, computer.id)}
                    className="input-brutal h-8 w-48 py-1 text-sm"
                    disabled={isSaving}
                  />
                  <Button
                    variant="default"
                    size="icon"
                    onClick={() => onSaveName(computer.id)}
                    disabled={isSaving || !editName.trim()}
                    aria-label={t('computersSaveName')}
                    className="h-8 w-8"
                  >
                    <Check className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    variant="outline"
                    size="icon"
                    onClick={onCancelEdit}
                    disabled={isSaving}
                    aria-label={t('computersCancelEdit')}
                    className="h-8 w-8"
                  >
                    <X className="h-3.5 w-3.5" />
                  </Button>
                </div>
              ) : (
                <div className="flex items-center gap-2">
                  <span className="font-bold">{computer.name}</span>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => onStartEdit(computer)}
                    aria-label={t('computersEditName')}
                    className="h-7 px-2 text-xs"
                  >
                    <Edit3 className="h-3 w-3" />
                  </Button>
                </div>
              )}
            </InfoRow>
            <InfoRow label="ID">
              <span className="font-mono text-xs">{computer.id}</span>
            </InfoRow>
            {computer.daemon_id && (
              <InfoRow label="Daemon ID">
                <span className="font-mono text-xs">{computer.daemon_id}</span>
              </InfoRow>
            )}
            {computer.daemon_url && (
              <InfoRow label="Daemon URL">
                <span className="font-mono text-xs">{computer.daemon_url}</span>
              </InfoRow>
            )}
          </div>
          </section>

          {/* Section: Status */}
          <hr className="border-t-2 border-black" />
          <section>
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
          <hr className="border-t-2 border-black" />
          <section>
            <SectionHeader label={t('computersConnectedAgents')} />
            <div className="mt-3">
              <ConnectedAgents key={agentVersion} computerId={isExpanded ? computer.id : null} onCreateAgent={onCreateAgent} />
            </div>
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

  if (!computerId) {
    return <p className="font-body text-sm text-muted-foreground">{t('computersExpandCard')}</p>;
  }

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 py-2">
        <Spinner size="sm" />
        <span className="text-sm text-muted-foreground">{t('loading')}</span>
      </div>
    );
  }

  if (error) {
    return <p className="font-body text-sm text-muted-foreground">{error}</p>;
  }

  return (
    <div className="space-y-2">
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
                onClick={() => router.push(`/teams?agent=${agent.id}&tab=profile`)}
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
      <span
        className="inline-flex items-center gap-1.5 border-2 border-black bg-brutal-primary px-2.5 py-1 font-heading text-[11px] font-black uppercase tracking-widest text-black shadow-brutal-sm"
        style={{ transform: 'rotate(-0.8deg)' }}
      >
        ★ {label}
      </span>
    </h3>
  );
}

function InfoRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-3 py-1.5">
      <span className="inline-block bg-brutal-primary-light border-2 border-black px-1.5 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider text-black flex-shrink-0">
        {label}
      </span>
      <div className="flex-1 min-w-0">{children}</div>
    </div>
  );
}
