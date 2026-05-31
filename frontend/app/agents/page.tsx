// ============================================================================
// SOLO-29-F: Agents list page — brutalist card grid with loading/empty/error
// - Card grid with card-brutal style
// - btn-brutal-pink for create button
// - Delete confirmation with brutalist dialog
// ============================================================================

'use client';

import { useEffect, useState, useCallback } from 'react';
import { useRouter } from 'next/navigation'
import { Bot, Plus, AlertCircle } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { AppFrame } from '@/components/layout/app-frame';
import { useAgents } from '@/lib/hooks/use-agents';
import { AgentCard } from '@/components/agents/agent-card';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogCloseButton,
} from '@/components/ui/dialog';

export default function AgentsPage() {
  const router = useRouter();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const { agents, isLoading, error, deleteAgent, refetch } = useAgents();

  // Delete confirmation state
  const [deleteTargetId, setDeleteTargetId] = useState<string | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);
  const deleteTargetName =
    deleteTargetId && agents.find((a) => a.id === deleteTargetId)?.name;

  // Auth guard
  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.push('/auth/login');
    }
  }, [authLoading, isAuthenticated, router]);

  const handleDelete = useCallback(async () => {
    if (!deleteTargetId) return;
    setIsDeleting(true);
    try {
      await deleteAgent(deleteTargetId);
    } finally {
      setIsDeleting(false);
      setDeleteTargetId(null);
    }
  }, [deleteTargetId, deleteAgent]);

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
            Agent 管理
          </h1>
          <p className="mt-1 font-body text-sm text-muted-foreground">
            创建和管理你的 AI Agent
          </p>
        </div>
        <Button onClick={() => router.push('/agents/new')}>
          <Plus className="mr-2 h-4 w-4" />
          创建 Agent
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

      {/* Loading skeleton grid */}
      {isLoading && (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
          {[1, 2, 3, 4, 5, 6].map((i) => (
            <div
              key={i}
              className="border-2 border-black bg-white p-6 shadow-brutal"
            >
              <div className="flex items-center gap-3">
                <Skeleton className="h-12 w-12 rounded-none" />
                <div className="flex-1 space-y-2">
                  <Skeleton className="h-4 w-24 rounded-none" />
                  <Skeleton className="h-3 w-32 rounded-none" />
                </div>
              </div>
              <div className="mt-4 space-y-2">
                <Skeleton className="h-3 w-full rounded-none" />
                <Skeleton className="h-3 w-3/4 rounded-none" />
              </div>
              <div className="mt-3">
                <Skeleton className="h-12 w-full rounded-none" />
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Empty state */}
      {!isLoading && !error && agents.length === 0 && (
        <div className="flex flex-col items-center justify-center border-2 border-dashed border-black py-20">
          <div className="mb-4 flex h-16 w-16 items-center justify-center border-2 border-black bg-brutal-pink shadow-brutal">
            <Bot className="h-8 w-8 text-white" />
          </div>
          <h2 className="text-xl font-heading font-bold text-foreground">
            还没有 Agent
          </h2>
          <p className="mt-2 font-body text-sm text-muted-foreground">
            创建你的第一个 Agent，让它加入频道协作
          </p>
          <Button className="mt-6" onClick={() => router.push('/agents/new')}>
            <Plus className="mr-2 h-4 w-4" />
            创建第一个 Agent
          </Button>
        </div>
      )}

      {/* Agent card grid */}
      {!isLoading && !error && agents.length > 0 && (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
          {agents.map((agent) => (
            <AgentCard
              key={agent.id}
              agent={agent}
              onClick={(id) => router.push(`/agents/${id}`)}
              onEdit={(id) => router.push(`/agents/${id}/edit`)}
              onDelete={(id) => setDeleteTargetId(id)}
            />
          ))}
        </div>
      )}

      {/* Delete confirmation dialog */}
      <Dialog
        open={!!deleteTargetId}
        onOpenChange={(open) => {
          if (!open) setDeleteTargetId(null);
        }}
      >
        <DialogHeader>
          <DialogTitle>删除 Agent</DialogTitle>
          <DialogCloseButton onClick={() => setDeleteTargetId(null)} />
        </DialogHeader>
        <DialogDescription>
          确定要删除{' '}
          <strong className="text-foreground">{deleteTargetName}</strong>{' '}
          吗？此操作不可撤销。该 Agent 将从所有频道中移除。
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
            {isDeleting ? '删除中...' : '确认删除'}
          </button>
        </DialogFooter>
      </Dialog>
    </div>
    </AppFrame>
  );
}
