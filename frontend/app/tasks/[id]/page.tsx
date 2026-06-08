// ============================================================================
// Task Detail Page — redirects to dashboard with thread context
// - Route: /tasks/[id]
// - Per v1.2 task-system-analysis: task detail = ThreadPanel
// - Loads task info, then redirects to /dashboard?channel={cid}&message={msgId}
// ============================================================================

'use client';

import { useEffect } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { Loader2 } from 'lucide-react';
import { t } from '@/lib/i18n';
import { apiClient } from '@/lib/api-client';

interface TaskRedirectInfo {
  channel_id: string;
  message_id?: string;
}

export default function TaskDetailPage() {
  const params = useParams();
  const router = useRouter();
  const taskId = params.id as string;

  useEffect(() => {
    let cancelled = false;

    async function redirectToThread() {
      try {
        const task = await apiClient.get<TaskRedirectInfo>(
          `/api/v1/tasks/${taskId}`,
        );

        if (!cancelled) {
          if (task.message_id) {
            router.replace(
              `/dashboard?channel=${task.channel_id}&message=${task.message_id}`,
            );
          } else {
            router.replace(`/dashboard?channel=${task.channel_id}`);
          }
        }
      } catch (err: unknown) {
        if (!cancelled) router.push('/tasks');
      }
    }

    redirectToThread();

    return () => {
      cancelled = true;
    };
  }, [taskId, router]);

  return (
    <div className="flex h-screen items-center justify-center bg-brutal-cream">
      <div className="flex flex-col items-center gap-3">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        <p className="font-mono text-sm text-muted-foreground">{t('taskRedirecting')}</p>
      </div>
    </div>
  );
}
