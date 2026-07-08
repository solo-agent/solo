'use client';

import { useState, useCallback, useEffect, useRef, lazy, Suspense } from 'react';
import { useRouter } from 'next/navigation';
import { Loader2, InboxIcon, Mail } from 'lucide-react';
import { useInbox } from '@/lib/hooks/use-inbox';
import { useInboxUnread } from '@/lib/hooks/use-inbox-unread';
import { InboxItem } from './inbox-item';
import { Button } from '@/components/ui/button';
import { TabBar } from '@/components/ui/tab-bar';
import { apiClient } from '@/lib/api-client';
import { buildDashboardHref } from '@/lib/dashboard-url';
import { useToast } from '@/components/ui/toast';
import type { TabBarTab } from '@/components/ui/tab-bar';
import type { InboxItem as InboxItemType, Message, TaskArtifact } from '@/lib/types';
import { t } from '@/lib/i18n';

const ThreadPanel = lazy(() =>
  import('@/components/dashboard/thread-panel').then((m) => ({ default: m.ThreadPanel })),
);

type ArtifactPreview = TaskArtifact & { previewUrl: string };

const INBOX_TABS: TabBarTab[] = [
  { key: 'all', label: t('inboxTabAll') },
  { key: 'mention', label: t('inboxTabMentions') },
  { key: 'thread_reply', label: t('inboxTabReplies') },
  { key: 'dm', label: t('inboxTabDMs') },
];

const KEY_TO_TYPE_FILTER: Record<string, string[]> = {
  all: [],
  mention: ['mention'],
  thread_reply: ['thread_reply'],
  dm: ['dm'],
};

function typeFilterToKey(tf: string[]) {
  if (tf.length === 0) return 'all';
  return tf[0];
}

export function InboxView() {
  const router = useRouter();
  const { showToast } = useToast();
  const { items, hasMore, isLoading, isLoadingMore, loadMore, markRead, markAllRead, clearAll, typeFilter, setTypeFilter, senderFilter, setSenderFilter } = useInbox();
  useInboxUnread();

  const handleClearAll = useCallback(async () => {
    await clearAll();
    window.dispatchEvent(new Event('inbox-refresh-unread'));
  }, [clearAll]);

  const handleMarkAllRead = useCallback(async () => {
    await markAllRead();
    window.dispatchEvent(new Event('inbox-refresh-unread'));
  }, [markAllRead]);

  const [threadMessage, setThreadMessage] = useState<Message | null>(null);
  const [threadTargetMessageId, setThreadTargetMessageId] = useState<string | undefined>(undefined);
  const [threadSource, setThreadSource] = useState<{ type: 'channel' | 'dm'; id: string } | null>(null);
  const [threadPanelWidth, setThreadPanelWidth] = useState(400);
  const [artifactPreview, setArtifactPreview] = useState<ArtifactPreview | null>(null);
  const [artifactReviewBusy, setArtifactReviewBusy] = useState(false);
  const artifactFrameRef = useRef<HTMLIFrameElement>(null);
  const artifactPreviewUrlRef = useRef<string | null>(null);

  const closeArtifactPreview = useCallback(() => {
    if (artifactPreviewUrlRef.current) {
      URL.revokeObjectURL(artifactPreviewUrlRef.current);
      artifactPreviewUrlRef.current = null;
    }
    setArtifactPreview(null);
  }, []);

  useEffect(() => () => {
    if (artifactPreviewUrlRef.current) {
      URL.revokeObjectURL(artifactPreviewUrlRef.current);
    }
  }, []);

  const handleItemClick = useCallback(
    async (item: InboxItemType) => {
      // Mark this specific message as read, then update badge
      await markRead(item.message_id);
      window.dispatchEvent(new Event('inbox-refresh-unread'));

      switch (item.type) {
        case 'thread_reply':
          if ((item.channel_id || item.dm_id) && item.thread_id) {
            const source = item.channel_id
              ? { type: 'channel' as const, id: item.channel_id }
              : { type: 'dm' as const, id: item.dm_id as string };
            const isAgent = item.parent_sender_type === 'agent';
            setThreadTargetMessageId(item.message_id);
            setThreadSource(source);
            setThreadMessage({
              id: item.thread_id,
              channel_id: source.id,
              user_id: item.parent_sender_id || '',
              display_name: item.parent_sender_name || '',
              content: item.parent_content || '',
              created_at: item.created_at,
              status: 'sent',
              sender_type: isAgent ? 'agent' : 'user',
              task_number: item.parent_task_number || undefined,
            });
          }
          break;
        case 'dm':
          if (item.dm_id) {
            router.push(`/dashboard?dm=${item.dm_id}&message=${item.message_id}`);
          }
          break;
        case 'mention':
          if (item.channel_id) {
            router.push(`/dashboard?channel=${item.channel_id}&message=${item.message_id}`);
          } else if (item.dm_id) {
            router.push(`/dashboard?dm=${item.dm_id}&message=${item.message_id}`);
          }
          break;
      }
    },
    [router, markRead],
  );

  const handleThreadClose = useCallback(() => {
    setThreadMessage(null);
    setThreadTargetMessageId(undefined);
    setThreadSource(null);
  }, []);

  const handleViewThreadInSource = useCallback(() => {
    if (!threadMessage || !threadSource) return;
    if (threadSource.type === 'channel') {
      router.push(buildDashboardHref(threadSource.id, { panel: 'thread', threadId: threadMessage.id }));
      return;
    }
    const key = threadSource.type === 'dm' ? 'dm' : 'channel';
    router.push(`/dashboard?${key}=${threadSource.id}&message=${threadMessage.id}&thread=${threadMessage.id}`);
  }, [router, threadMessage, threadSource]);

  const handleOpenArtifactReference = useCallback(async (ref: string) => {
    try {
      let artifact: TaskArtifact | null = null;
      const url = new URL(ref, window.location.origin);
      if (url.pathname.startsWith('/api/v1/artifacts/')) {
        artifact = await apiClient.get<TaskArtifact>(`${url.pathname.replace(/\/meta$/, '')}/meta`);
      } else {
        const match = ref.match(/\/\.solo\/artifacts\/([^/\s]+)\/([^/\s]+\.html)/);
        if (match) {
          const [, taskId, filename] = match;
          const artifacts = await apiClient.get<TaskArtifact[] | null>(`/api/v1/tasks/${taskId}/artifacts`);
          artifact = (artifacts ?? []).find((item) => item.summary !== 'pending' && item.html_path.endsWith(`/${filename}`)) ?? null;
        }
      }
      if (!artifact) throw new Error('artifact not found');
      const html = await apiClient.getText(artifact.url);
      const blobUrl = URL.createObjectURL(new Blob([html], { type: 'text/html' }));
      if (artifactPreviewUrlRef.current) {
        URL.revokeObjectURL(artifactPreviewUrlRef.current);
      }
      artifactPreviewUrlRef.current = blobUrl;
      setArtifactPreview({ ...artifact, previewUrl: blobUrl });
    } catch {
      showToast('Could not open artifact link.', 'error');
    }
  }, [showToast]);

  useEffect(() => {
    if (!artifactPreview) return;

    const handleArtifactMessage = async (event: MessageEvent) => {
      if (event.source !== artifactFrameRef.current?.contentWindow) return;
      const data = event.data;
      if (!data || typeof data !== 'object' || data.type !== 'artifact.reviewAction') return;
      const taskId = typeof data.taskId === 'string' && data.taskId.trim() !== ''
        ? data.taskId.trim()
        : artifactPreview.task_id;
      if (!taskId || (data.action !== 'accept' && data.action !== 'reject') || artifactReviewBusy) return;

      const reason = typeof data.reason === 'string' ? data.reason.trim() : '';
      if (data.action === 'reject' && reason === '') {
        showToast('Reject comment is required.', 'error');
        return;
      }

      setArtifactReviewBusy(true);
      try {
        const path = `/api/v1/tasks/${taskId}/${data.action === 'accept' ? 'accept' : 'reject'}`;
        await apiClient.post(path, data.action === 'reject' ? { reason } : undefined);
        closeArtifactPreview();
        showToast(data.action === 'accept' ? 'Task accepted.' : 'Task rejected.', 'success');
      } catch {
        showToast(data.action === 'accept' ? 'Could not accept task.' : 'Could not reject task.', 'error');
      } finally {
        setArtifactReviewBusy(false);
      }
    };

    window.addEventListener('message', handleArtifactMessage);
    return () => window.removeEventListener('message', handleArtifactMessage);
  }, [artifactPreview, artifactReviewBusy, closeArtifactPreview, showToast]);

  return (
    <div className="flex flex-1 overflow-hidden">
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <div className="sidebar-collapse-offset flex h-14 flex-shrink-0 items-center border-b-2 border-black px-4">
          <div className="flex items-center gap-2 flex-1 min-w-0">
            <Mail className="h-5 w-5 flex-shrink-0 text-muted-foreground" />
            <h2 className="font-bold text-foreground truncate">Inbox</h2>
          </div>
          {items.length > 0 && (
            <div className="flex items-center gap-2">
              <Button
                type="button"
                onClick={handleMarkAllRead}
                variant="outline"
                size="sm"
                className="px-3 text-xs"
              >
                {t('markAllRead')}
              </Button>
              <Button
                type="button"
                onClick={handleClearAll}
                variant="primary"
                size="sm"
                className="px-3 text-xs"
              >
                {t('clearInbox')}
              </Button>
            </div>
          )}
        </div>

        {/* Filter bar */}
        <TabBar
          tabs={INBOX_TABS}
          activeKey={typeFilterToKey(typeFilter)}
          onChange={(key) => setTypeFilter(KEY_TO_TYPE_FILTER[key])}
          variant="pill"
        >
          <input
            type="text"
            placeholder={t('filterSender')}
            value={senderFilter}
            onChange={(e) => setSenderFilter(e.target.value)}
            className="focus-brutal-compact ml-auto h-7 w-36 border-2 border-black bg-white px-2 py-1 font-body text-xs shadow-brutal-sm outline-none transition-shadow placeholder:text-muted-foreground"
          />
        </TabBar>

        <div className="flex flex-1 flex-col overflow-hidden bg-brutal-cream">
          {isLoading ? (
            <div className="flex flex-1 items-center justify-center">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : items.length === 0 ? (
            <div className="flex flex-1 flex-col items-center justify-center px-4">
              <div className="mb-4 flex h-16 w-16 items-center justify-center border-2 border-black bg-brutal-primary shadow-brutal-sm">
                <InboxIcon className="h-8 w-8 text-black" />
              </div>
              <h2 className="font-heading text-lg font-bold text-foreground">
                {t('noNewMessages')}
              </h2>
              <p className="mt-1 font-body text-sm text-muted-foreground">
                {t('inboxEmptyDesc')}
              </p>
            </div>
          ) : (
            <div className="flex-1 overflow-y-auto">
              {items.map((item) => (
                <InboxItem
                  key={`${item.type}-${item.id}`}
                  item={item}
                  onClick={handleItemClick}
                />
              ))}

              {hasMore && (
                <div className="flex justify-center py-4">
                  <button
                    type="button"
                    onClick={loadMore}
                    disabled={isLoadingMore}
                    className="text-xs font-medium text-muted-foreground hover:text-foreground transition-colors disabled:opacity-50"
                  >
                    {isLoadingMore ? (
                      <span className="flex items-center gap-1.5">
                        <Loader2 className="h-3 w-3 animate-spin" />
                        {t('loading')}
                      </span>
                    ) : (
                      t('loadMore')
                    )}
                  </button>
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {/* ThreadPanel */}
      <div
        className="flex-shrink-0 bg-brutal-cream overflow-hidden relative transition-all duration-500 ease-[cubic-bezier(0.16,1,0.3,1)] border-l-2 border-transparent"
        style={{ width: threadMessage ? threadPanelWidth : 0, borderLeftColor: threadMessage ? '#000' : 'transparent' }}
      >
        {threadMessage && (
          <div
            className="absolute left-0 top-0 bottom-0 w-1.5 cursor-col-resize hover:bg-brutal-primary/50 transition-colors z-10"
            onMouseDown={(e) => {
              e.preventDefault();
              const startX = e.clientX;
              const startWidth = threadPanelWidth;
              const onMove = (ev: MouseEvent) => {
                const newWidth = Math.max(280, Math.min(800, startWidth + startX - ev.clientX));
                setThreadPanelWidth(newWidth);
              };
              const onUp = () => {
                document.removeEventListener('mousemove', onMove);
                document.removeEventListener('mouseup', onUp);
              };
              document.addEventListener('mousemove', onMove);
              document.addEventListener('mouseup', onUp);
            }}
          />
        )}
        {threadMessage && (
          <Suspense
            fallback={
              <div className="flex h-full items-center justify-center">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              </div>
            }
          >
            <ThreadPanel
              parentMessage={threadMessage}
              onClose={handleThreadClose}
              targetMessageId={threadTargetMessageId}
              replyCount={0}
              onViewInChannel={handleViewThreadInSource}
              onOpenArtifactReference={handleOpenArtifactReference}
            />
          </Suspense>
        )}
      </div>

      {artifactPreview && (
        <div
          role="dialog"
          aria-modal="true"
          aria-labelledby="inbox-artifact-preview-title"
          className="fixed inset-4 z-50 flex flex-col border-4 border-black bg-white shadow-brutal-xl"
        >
          <div className="flex items-center justify-between border-b-4 border-black px-4 py-2">
            <div id="inbox-artifact-preview-title" className="font-heading text-sm font-black uppercase">{artifactPreview.title}</div>
            <button
              type="button"
              onClick={closeArtifactPreview}
              className="border-2 border-black bg-white px-2 py-1 font-mono text-xs font-bold uppercase shadow-brutal-sm"
              aria-label="Close artifact preview"
            >
              Close
            </button>
          </div>
          <iframe ref={artifactFrameRef} title={artifactPreview.title} src={artifactPreview.previewUrl} className="min-h-0 flex-1 bg-white" />
        </div>
      )}
    </div>
  );
}
