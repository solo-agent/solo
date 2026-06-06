// ============================================================================
// SOLO-127-F: 任务创建页 — create task form with brutalist styling
// - Route: /tasks/new
// - Form: title, description, priority, due date, assignee
// - input-brutal, textarea, select-brutal
// - Back to task list navigation
// ============================================================================

'use client';

import { Suspense, useEffect, useState, useCallback } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { ArrowLeft, Hash } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { useTasks } from '@/lib/hooks/use-tasks';
import { useChannels } from '@/lib/hooks/use-channels';
import { useAgents } from '@/lib/hooks/use-agents';
import { TaskForm } from '@/components/tasks/task-form';
import { Button } from '@/components/ui/button';
import { Spinner } from '@/components/ui/spinner';
import { Select } from '@/components/ui/select';
import type { CreateTaskInput } from '@/lib/types';

function CreateTaskForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const { createTask } = useTasks();
  const { channels, isLoading: channelsLoading } = useChannels();
  const { agents } = useAgents();
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  // Pre-selected channel from query param (e.g., ?channel_id=xxx)
  const urlChannelId = searchParams.get('channel_id') || '';
  const [selectedChannelId, setSelectedChannelId] = useState(urlChannelId);

  // Use URL param if provided, otherwise use dropdown selection
  const effectiveChannelId = urlChannelId || selectedChannelId;

  // Build assignee options from agents
  const assigneeOptions = agents.map((a) => ({
    id: a.id,
    name: a.name,
    type: 'agent' as const,
  }));

  // Auth guard
  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.push('/auth/login');
    }
  }, [authLoading, isAuthenticated, router]);

  const handleSubmit = useCallback(
    async (input: CreateTaskInput) => {
      setIsSubmitting(true);
      setSubmitError(null);
      try {
        if (!effectiveChannelId) {
          setSubmitError('请选择一个频道');
          return;
        }
        await createTask({ ...input, channel_id: effectiveChannelId });
        router.push('/tasks');
      } catch (err) {
        setSubmitError(err instanceof Error ? err.message : '创建任务失败，请稍后再试');
      } finally {
        setIsSubmitting(false);
      }
    },
    [createTask, router, effectiveChannelId],
  );

  if (authLoading || !isAuthenticated) {
    return (
      <div className="flex h-screen items-center justify-center bg-brutal-cream">
        <div className="flex flex-col items-center gap-3">
          <Spinner size="lg" />
          <p className="font-mono text-sm text-muted-foreground">加载中...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-2xl px-6 py-8">
      {/* Back link */}
      <Button
        variant="ghost"
        className="mb-6 -ml-2 text-muted-foreground"
        onClick={() => router.push('/tasks')}
      >
        <ArrowLeft className="mr-2 h-4 w-4" />
        返回任务列表
      </Button>

      <div className="mb-8">
        <h1 className="text-2xl font-heading font-bold text-foreground">
          创建任务
        </h1>
        <p className="mt-1 font-body text-sm text-muted-foreground">
          创建一个新任务，指派给团队成员或 Agent
        </p>
      </div>

      <div className="card-brutal p-6">
        {/* Channel selector (only show when no channel_id in URL) */}
        {!urlChannelId && (
          <div className="mb-5">
            <label className="mb-1.5 block text-sm font-heading font-bold">
              <Hash className="mr-1 inline h-3.5 w-3.5" />
              所属频道 <span className="text-brutal-red">*</span>
            </label>
            <Select
              value={selectedChannelId}
              onChange={(e) => setSelectedChannelId(e.target.value)}
              className="h-10 w-full"
              disabled={channelsLoading || isSubmitting}
            >
              <option value="">选择频道...</option>
              {channels.map((ch) => (
                <option key={ch.id} value={ch.id}>
                  #{ch.name}
                </option>
              ))}
            </Select>
          </div>
        )}

        <TaskForm
          channelId={effectiveChannelId}
          assigneeOptions={assigneeOptions}
          onSubmit={handleSubmit}
          isSubmitting={isSubmitting}
          submitLabel="创建任务"
          error={submitError}
        />
      </div>
    </div>
  );
}

export default function CreateTaskPage() {
  return (
    <Suspense
      fallback={
        <div className="flex h-screen items-center justify-center bg-brutal-cream">
          <div className="flex flex-col items-center gap-3">
            <Spinner size="lg" />
            <p className="font-mono text-sm text-muted-foreground">加载中...</p>
          </div>
        </div>
      }
    >
      <CreateTaskForm />
    </Suspense>
  );
}
