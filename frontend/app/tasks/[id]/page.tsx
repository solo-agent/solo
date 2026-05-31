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

export default function TaskDetailPage() {
  const params = useParams();
  const router = useRouter();
  const taskId = params.id as string;

  useEffect(() => {
    // Fetch the task to get channel_id + message_id, then redirect
    let cancelled = false;

    async function redirectToThread() {
      try {
        const token = localStorage.getItem('access_token');
        if (!token) {
          router.push('/auth/login');
          return;
        }

        const res = await fetch(`/api/v1/tasks/${taskId}`, {
          headers: { Authorization: `Bearer ${token}` },
        });

        if (!res.ok) {
          // Task not found or error — redirect to tasks board
          if (!cancelled) router.push('/tasks');
          return;
        }

        const task = await res.json();

        if (!cancelled) {
          if (task.message_id) {
            router.replace(`/dashboard?channel=${task.channel_id}&message=${task.message_id}`);
          } else {
            router.replace(`/dashboard?channel=${task.channel_id}`);
          }
        }
      } catch {
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
        <p className="font-mono text-sm text-muted-foreground">正在跳转到讨论...</p>
      </div>
    </div>
  );
}
