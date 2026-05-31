// ============================================================================
// SOLO-245-F & SOLO-246-F: Computers list page with inline detail expansion
// - Brutalist card grid (2 cols desktop, 1 col mobile)
// - Status indicators (online green / offline gray pulsing)
// - Inline expand on card click for detail view
// - Inline name editing with PATCH
// - Delete confirmation with brutalist dialog
// - Loading skeleton, error state with retry, empty state
// ============================================================================

'use client';

import { useEffect, useState, useCallback, useRef } from 'react';
import { useRouter } from 'next/navigation';
import { Monitor, Plus, AlertCircle, Terminal, Edit3, Check, X } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { AppFrame } from '@/components/layout/app-frame';
import { useComputers } from '@/lib/hooks/use-computers';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useToast } from '@/components/ui/toast';
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

export default function ComputersPage() {
  const router = useRouter();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const { computers, isLoading, error, updateComputer, deleteComputer, refetch } = useComputers();
  const { showToast } = useToast();

  // Auth guard
  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.push('/auth/login');
    }
  }, [authLoading, isAuthenticated, router]);

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

  // Focus edit input when editing starts
  useEffect(() => {
    if (editingId && editInputRef.current) {
      editInputRef.current.focus();
      editInputRef.current.select();
    }
  }, [editingId]);

  const handleToggleExpand = useCallback((id: string) => {
    setExpandedId((prev) => (prev === id ? null : id));
    // Cancel any in-progress edit when toggling
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
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-brutal-pink border-t-transparent" />
          <p className="font-mono text-sm text-muted-foreground">加载中...</p>
        </div>
      </div>
    );
  }

  return (
    <AppFrame>
      <div className="mx-auto w-full max-w-5xl px-6 py-8 overflow-y-auto flex-1">
      {/* Page header */}
      <div className="mb-8 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-heading font-bold text-foreground">
            电脑管理
          </h1>
          <p className="mt-1 font-body text-sm text-muted-foreground">
            管理已连接的电脑和 Daemon 实例
          </p>
        </div>
        <Button onClick={() => setShowAddDialog(true)}>
          <Plus className="mr-2 h-4 w-4" />
          添加电脑
        </Button>
      </div>

      {/* Error state */}
      {error && (
        <div className="mb-6 flex items-center gap-3 border-2 border-brutal-orange bg-brutal-orange-light p-4 shadow-brutal-sm">
          <AlertCircle className="h-5 w-5 flex-shrink-0 text-brutal-orange" />
          <span className="flex-1 font-body text-sm text-foreground">
            {error}
          </span>
          <button
            type="button"
            onClick={refetch}
            className="btn-brutal btn-brutal-sm"
          >
            重试
          </button>
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

      {/* Empty state */}
      {!isLoading && !error && computers.length === 0 && (
        <div className="flex flex-col items-center justify-center border-2 border-dashed border-black py-20">
          <div className="mb-4 flex h-16 w-16 items-center justify-center border-2 border-black bg-brutal-cyan shadow-brutal">
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

      {/* Computer cards grid */}
      {!isLoading && !error && computers.length > 0 && (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {computers.map((computer) => {
            const isExpanded = expandedId === computer.id;
            const isOnline = computer.status === 'online';

            return (
              <div
                key={computer.id}
                className={cn(
                  'border-2 border-black bg-white transition-all duration-300',
                  isExpanded ? 'shadow-brutal-lg' : 'shadow-brutal card-brutal',
                )}
              >
                {/* Card header — click to expand */}
                <button
                  type="button"
                  className="w-full p-6 text-left"
                  onClick={() => handleToggleExpand(computer.id)}
                  aria-expanded={isExpanded}
                  aria-label={`${computer.name} — ${isOnline ? '在线' : '离线'}`}
                >
                  <div className="flex items-start gap-3">
                    <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-cyan shadow-brutal-sm">
                      <Monitor className="h-5 w-5 text-black" />
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

                  {/* Quick info */}
                  <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 font-body text-xs text-muted-foreground">
                    {computer.agent_names && computer.agent_names.length > 0 ? (
                      <span>
                        Agent: {computer.agent_names.join(', ')}
                      </span>
                    ) : (
                      <span>无绑定 Agent</span>
                    )}
                    {computer.daemon_id && (
                      <span className="font-mono text-xs">
                        Daemon: {computer.daemon_id}
                      </span>
                    )}
                  </div>
                </button>

                {/* Expanded detail panel */}
                <div
                  className={cn(
                    'overflow-hidden transition-all duration-300 ease-in-out',
                    isExpanded ? 'max-h-[800px] opacity-100' : 'max-h-0 opacity-0',
                  )}
                >
                  <div className="border-t-2 border-black px-6 pb-6 pt-4">
                    {/* Section: Basic Info */}
                    <SectionHeader label="基本信息" />
                    <div className="mt-3 space-y-2 font-body text-sm">
                      <InfoRow label="名称">
                        {editingId === computer.id ? (
                          <div className="flex items-center gap-2">
                            <input
                              ref={editInputRef}
                              type="text"
                              value={editName}
                              onChange={(e) => setEditName(e.target.value)}
                              onKeyDown={(e) => handleEditKeyDown(e, computer.id)}
                              className="input-brutal h-8 w-48 py-1 text-sm"
                              disabled={isSaving}
                            />
                            <button
                              type="button"
                              onClick={() => handleSaveName(computer.id)}
                              disabled={isSaving || !editName.trim()}
                              className="btn-brutal btn-brutal-sm h-8 w-8 p-0"
                              aria-label="保存名称"
                            >
                              <Check className="h-3.5 w-3.5" />
                            </button>
                            <button
                              type="button"
                              onClick={handleCancelEdit}
                              disabled={isSaving}
                              className="btn-brutal btn-brutal-sm h-8 w-8 p-0 bg-white"
                              aria-label="取消编辑"
                            >
                              <X className="h-3.5 w-3.5" />
                            </button>
                          </div>
                        ) : (
                          <div className="flex items-center gap-2">
                            <span className="font-bold">{computer.name}</span>
                            <button
                              type="button"
                              onClick={() => handleStartEdit(computer)}
                              className="btn-brutal btn-brutal-sm h-7 px-2 text-xs"
                              aria-label="编辑名称"
                            >
                              <Edit3 className="h-3 w-3" />
                            </button>
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

                    {/* Section: Bound Agents */}
                    <SectionHeader label="绑定 Agent" className="mt-6" />
                    <div className="mt-3">
                      {computer.agent_names && computer.agent_names.length > 0 ? (
                        <ul className="space-y-1">
                          {computer.agent_names.map((name, idx) => (
                            <li
                              key={idx}
                              className="flex items-center gap-2 font-body text-sm"
                            >
                              <span className="h-1.5 w-1.5 flex-shrink-0 rounded-full bg-brutal-pink" />
                              {name}
                            </li>
                          ))}
                        </ul>
                      ) : (
                        <p className="font-body text-sm text-muted-foreground">
                          暂无绑定 Agent
                        </p>
                      )}
                    </div>

                    {/* Remove button */}
                    <div className="mt-6">
                      <button
                        type="button"
                        onClick={() => {
                          setDeleteTargetId(computer.id);
                        }}
                        className="btn-brutal btn-brutal-sm bg-brutal-red text-white"
                      >
                        移除电脑
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

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
              <li>设置 <code className="bg-brutal-black text-brutal-lime px-1">.env</code> 中的 <code className="bg-brutal-black text-brutal-lime px-1">DAEMON_PORT</code> 和 <code className="bg-brutal-black text-brutal-lime px-1">SERVER_URL</code></li>
              <li>运行 <code className="bg-brutal-black text-brutal-lime px-1">make daemon</code> 启动 Daemon</li>
              <li>Daemon 启动后会自动向服务器注册</li>
              <li>注册成功后，电脑将出现在列表中</li>
            </ol>
          </div>
        </div>
        <DialogFooter>
          <button
            type="button"
            onClick={() => setShowAddDialog(false)}
            className="btn-brutal btn-brutal-sm"
          >
            知道了
          </button>
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
          <button
            type="button"
            onClick={() => setDeleteTargetId(null)}
            className="btn-brutal btn-brutal-sm"
          >
            取消
          </button>
          <button
            type="button"
            onClick={handleDelete}
            disabled={isDeleting}
            className="btn-brutal btn-brutal-sm bg-brutal-red text-white"
          >
            {isDeleting ? '移除中...' : '确认移除'}
          </button>
        </DialogFooter>
      </Dialog>
    </div>
    </AppFrame>
  );
}

// ---- Sub-components ----

function StatusDot({ isOnline }: { isOnline: boolean }) {
  return (
    <span
      className={cn(
        'inline-block h-2.5 w-2.5 flex-shrink-0 rounded-full border border-black',
        isOnline ? 'bg-green-500' : 'bg-gray-400 animate-pulse',
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
      <span className="h-1 w-1 rounded-full bg-brutal-pink" />
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
