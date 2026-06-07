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
import {
  Monitor,
  Plus,
  AlertCircle,
  Terminal,
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
} from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { useComputers } from '@/lib/hooks/use-computers';
import { useComputerAgents } from '@/lib/hooks/use-computer-agents';
import { Button } from '@/components/ui/button';
import { Spinner } from '@/components/ui/spinner';
import { Skeleton } from '@/components/ui/skeleton';
import { BrutalAlert } from '@/components/ui/brutal-alert';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { useToast } from '@/components/ui/toast';
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
  if (!os) return { icon: <MonitorDot className="h-4 w-4" />, label: '未知' };
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
    online: '空闲',
    thinking: '思考中',
    running: '运行中',
    offline: '离线',
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
  const { computers, isLoading, error, updateComputer, deleteComputer, refetch } = useComputers();
  const { showToast } = useToast();

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

  // Selected computer (driven by ComputersLeftColumn)
  const [selectedComputerId, setSelectedComputerId] = useState<string | null>(null);
  const selectedComputer = useMemo(
    () => (selectedComputerId ? computers.find((c) => c.id === selectedComputerId) : undefined),
    [computers, selectedComputerId],
  );

  // Left-column click: re-click clears selection; switching resets edit/expand
  const handleComputerClick = useCallback((id: string) => {
    setSelectedComputerId((prev) => (prev === id ? null : id));
    setEditingId(null);
    setExpandedId(null);
  }, []);

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
      showToast('名称已更新', 'success');
    } catch (err) {
      const message = err instanceof Error ? err.message : '更新名称失败';
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
      showToast('电脑已移除', 'success');
    } catch (err) {
      const message = err instanceof Error ? err.message : '移除电脑失败';
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
          <p className="font-mono text-sm text-muted-foreground">加载中...</p>
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
        {/* Top bar — add button only (page label lives in the left column) */}
        <div className="flex flex-shrink-0 items-center justify-end h-14 border-b-2 border-black px-4">
          <Button onClick={() => setShowAddDialog(true)}>
            <Plus className="mr-2 h-4 w-4" />
            添加电脑
          </Button>
        </div>
        <div className="flex-1 overflow-y-auto px-6 py-6">
          <div className="mx-auto w-full max-w-3xl">
            {/* Error state */}
            {error && (
              <div className="mb-6 space-y-2">
                <BrutalAlert variant="warning" className="p-4">
                  {error}
                </BrutalAlert>
                <Button variant="outline" size="sm" onClick={refetch}>
                  重试
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
              <div className="flex flex-col items-center justify-center border-2 border-dashed border-black py-20">
                <div className="mb-4 flex h-16 w-16 items-center justify-center border-2 border-black bg-brutal-info shadow-brutal">
                  <Monitor className="h-8 w-8 text-white" />
                </div>
                <h2 className="text-xl font-heading font-bold text-foreground">
                  还没有连接的电脑
                </h2>
                <p className="mt-2 font-body text-sm text-muted-foreground">
                  启动 Daemon 并注册后，电脑将出现在这里
                </p>
                <Button className="mt-6" onClick={() => setShowAddDialog(true)}>
                  <Plus className="mr-2 h-4 w-4" />
                  查看接入指引
                </Button>
              </div>
            )}

            {/* Empty state — computers exist, none selected */}
            {!isLoading && !error && computers.length > 0 && !selectedComputer && (
              <div className="flex flex-col items-center justify-center border-2 border-dashed border-black py-20">
                <p className="font-body text-sm text-muted-foreground">
                  请从左侧选择一台电脑
                </p>
              </div>
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
              />
            )}
          </div>
        </div>
      </main>

      {/* Add computer instruction dialog */}
      <Dialog
        open={showAddDialog}
        onOpenChange={(open) => {
          if (!open) setShowAddDialog(false);
        }}
      >
        <DialogHeader>
          <DialogTitle>添加电脑</DialogTitle>
          <DialogCloseButton onClick={() => setShowAddDialog(false)} />
        </DialogHeader>
        <DialogDescription>
          在目标机器上启动 Daemon 并注册到 Solo 服务器。
        </DialogDescription>
        <div className="mt-4 space-y-3">
          <div className="border-2 border-black bg-brutal-cream p-4">
            <div className="flex items-center gap-2 mb-2">
              <Terminal className="h-4 w-4" />
              <span className="font-heading text-sm font-bold">操作步骤</span>
            </div>
            <ol className="list-decimal list-inside space-y-1.5 font-mono text-xs text-foreground">
              <li>在目标机器上克隆项目代码</li>
              <li>设置 <code className="bg-brutal-black text-brutal-success px-1">.env</code> 中的 <code className="bg-brutal-black text-brutal-success px-1">DAEMON_PORT</code> 和 <code className="bg-brutal-black text-brutal-success px-1">SERVER_URL</code></li>
              <li>运行 <code className="bg-brutal-black text-brutal-success px-1">make daemon</code> 启动 Daemon</li>
              <li>Daemon 启动后会自动向服务器注册</li>
              <li>注册成功后，电脑将出现在列表中</li>
            </ol>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" size="sm" onClick={() => setShowAddDialog(false)}>
            知道了
          </Button>
        </DialogFooter>
      </Dialog>

      {/* Delete confirmation dialog */}
      <Dialog
        open={!!deleteTargetId}
        onOpenChange={(open) => {
          if (!open) setDeleteTargetId(null);
        }}
      >
        <DialogHeader>
          <DialogTitle>移除电脑</DialogTitle>
          <DialogCloseButton onClick={() => setDeleteTargetId(null)} />
        </DialogHeader>
        <DialogDescription>
          确定要移除{' '}
          <strong className="text-foreground">{deleteTargetName}</strong>{' '}
          吗？此操作不可撤销。该电脑将从系统中注销。
        </DialogDescription>
        <DialogFooter>
          <Button variant="outline" size="sm" onClick={() => setDeleteTargetId(null)}>
            取消
          </Button>
          <Button
            variant="destructive"
            size="sm"
            onClick={handleDelete}
            disabled={isDeleting}
          >
            {isDeleting ? '移除中...' : '确认移除'}
          </Button>
        </DialogFooter>
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
}

function ComputerCard({
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
    <div
      className={cn(
        'border-2 border-black bg-white transition-all duration-300',
        isExpanded ? 'shadow-brutal-lg' : 'shadow-brutal card-brutal',
      )}
    >
      {/* Card header — click to expand */}
      <button
        type="button"
        className="w-full p-6 text-left"
        onClick={() => onToggleExpand(computer.id)}
        aria-expanded={isExpanded}
        aria-label={`${computer.name} — ${isOnline ? '在线' : '离线'}`}
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
              <StatusDot isOnline={isOnline} />
            </div>
            <p className="mt-1 font-body text-xs text-muted-foreground">
              {isOnline
                ? `最后心跳: ${relativeTime(computer.last_heartbeat)}`
                : `离线 ${relativeTime(computer.last_heartbeat, false)}`}
            </p>
          </div>
        </div>

        {/* Quick info — enhanced with system info */}
        <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 font-body text-xs text-muted-foreground">
          {/* OS + hostname */}
          <span className="flex items-center gap-1.5">
            {osInfo.icon}
            <span className="truncate max-w-[120px]">
              {computer.hostname || osInfo.label}
            </span>
          </span>
          {/* IP */}
          {computer.ip && (
            <span className="flex items-center gap-1 font-mono">
              <Globe className="h-3 w-3" />
              {computer.ip}
            </span>
          )}
          {/* Agent count */}
          {computer.agent_names && computer.agent_names.length > 0 ? (
            <span>
              <Cpu className="inline h-3 w-3 mr-0.5 -mt-0.5" />
              Agents: {computer.agent_names.length}
            </span>
          ) : (
            <span>无绑定 Agent</span>
          )}
        </div>

        {/* Expand indicator */}
        <div className="mt-2 flex justify-center">
          {isExpanded ? (
            <ChevronUp className="h-4 w-4 text-muted-foreground" />
          ) : (
            <ChevronDown className="h-4 w-4 text-muted-foreground" />
          )}
        </div>
      </button>

      {/* Expanded detail panel */}
      <div
        className={cn(
          'overflow-hidden transition-all duration-300 ease-in-out',
          isExpanded ? 'max-h-[1200px] opacity-100' : 'max-h-0 opacity-0',
        )}
      >
        <div className="border-t-2 border-black px-6 pb-6 pt-4">
          {/* Section: System Info */}
          <SectionHeader label="系统信息" />
          <div className="mt-3 space-y-2 font-body text-sm">
            {computer.os && (
              <InfoRow label="系统">
                <span className="flex items-center gap-1.5">
                  {osInfo.icon}
                  {osInfo.label}
                </span>
              </InfoRow>
            )}
            {computer.hostname && (
              <InfoRow label="主机名">
                <span className="font-mono text-xs">{computer.hostname}</span>
              </InfoRow>
            )}
            {computer.ip && (
              <InfoRow label="IP 地址">
                <span className="font-mono text-xs">{computer.ip}</span>
              </InfoRow>
            )}
          </div>

          {/* Section: Basic Info */}
          <SectionHeader label="基本信息" className="mt-6" />
          <div className="mt-3 space-y-2 font-body text-sm">
            <InfoRow label="名称">
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
                    aria-label="保存名称"
                    className="h-8 w-8"
                  >
                    <Check className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    variant="outline"
                    size="icon"
                    onClick={onCancelEdit}
                    disabled={isSaving}
                    aria-label="取消编辑"
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
                    aria-label="编辑名称"
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

          {/* Section: Status */}
          <SectionHeader label="状态" className="mt-6" />
          <div className="mt-3 space-y-2 font-body text-sm">
            <InfoRow label="当前">
              <div className="flex items-center gap-2">
                <StatusDot isOnline={isOnline} />
                <span>{isOnline ? '在线' : '离线'}</span>
              </div>
            </InfoRow>
            <InfoRow label="最后心跳">
              <span>
                {computer.last_heartbeat
                  ? formatDateTime(computer.last_heartbeat)
                  : '从未'}
              </span>
            </InfoRow>
            <InfoRow label="注册时间">
              <span>{formatDateTime(computer.created_at)}</span>
            </InfoRow>
          </div>

          {/* Section: Connected Agents (v1.5) */}
          <SectionHeader label="连接的 Agent" className="mt-6" />
          <div className="mt-3">
            <ConnectedAgents computerId={isExpanded ? computer.id : null} />
          </div>

          {/* Remove button */}
          <div className="mt-6">
            <Button
              variant="destructive"
              size="sm"
              onClick={() => onDeleteClick(computer.id)}
            >
              移除电脑
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

// ---- Connected Agents sub-component (lazy-loaded on expand) ----

function ConnectedAgents({ computerId }: { computerId: string | null }) {
  const { agents, isLoading, error } = useComputerAgents(computerId);

  if (!computerId) {
    return <p className="font-body text-sm text-muted-foreground">展开卡片查看</p>;
  }

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 py-2">
        <Spinner size="sm" />
        <span className="text-sm text-muted-foreground">加载中...</span>
      </div>
    );
  }

  if (error) {
    return <p className="font-body text-sm text-muted-foreground">{error}</p>;
  }

  if (agents.length === 0) {
    return <p className="font-body text-sm text-muted-foreground">暂无连接的 Agent</p>;
  }

  return (
    <div className="space-y-2">
      <p className="font-body text-sm text-muted-foreground">
        共 {agents.length} 个 Agent 连接在此电脑
      </p>
      <ul className="space-y-2">
        {agents.map((agent) => (
          <li
            key={agent.id}
            className="flex items-center gap-3 border-2 border-black bg-brutal-cream p-2.5"
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
                活跃任务
              </span>
              <span className="block font-mono text-sm font-bold text-foreground">
                {agent.active_tasks}
              </span>
            </div>
          </li>
        ))}
      </ul>
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
      aria-label={isOnline ? '在线' : '离线'}
    />
  );
}

function SectionHeader({ label, className }: { label: string; className?: string }) {
  return (
    <h3
      className={cn(
        'flex items-center gap-2 font-heading text-sm font-bold text-foreground',
        className,
      )}
    >
      <span className="h-1 w-1 bg-brutal-primary" />
      {label}
    </h3>
  );
}

function InfoRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-3">
      <span className="w-20 flex-shrink-0 text-xs text-muted-foreground">{label}</span>
      <div className="flex-1 min-w-0">{children}</div>
    </div>
  );
}
