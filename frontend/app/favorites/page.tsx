'use client';

import { useCallback, useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { Bookmark, ExternalLink, Trash2 } from 'lucide-react';
import { PersonalFrame } from '@/components/layout/personal-frame';
import { Button } from '@/components/ui/button';
import { Spinner } from '@/components/ui/spinner';
import { EmptyState } from '@/components/ui/empty-state';
import { UserAvatar } from '@/components/ui/user-avatar';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { useToast } from '@/components/ui/toast';
import { useAuth } from '@/lib/auth-context';
import { apiClient } from '@/lib/api-client';
import { t } from '@/lib/i18n';
import { formatDateTime } from '@/lib/utils/time';
import type { Message } from '@/lib/types';
import { useWorkspace } from '@/lib/workspace-context';

type Favorite = {
  message: Message & {
    sender_id?: string;
    sender_name?: string;
    sender_avatar?: string | null;
  };
  workspace_id: string;
  workspace_name: string;
  channel_name: string;
  channel_type: 'channel' | 'dm';
  thread_root_message_id?: string;
  favorited_at: string;
};

export default function FavoritesPage() {
  const router = useRouter();
  const { showToast } = useToast();
  const { switchWorkspace } = useWorkspace();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const [items, setItems] = useState<Favorite[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const favorites = await apiClient.get<Favorite[]>('/api/v1/favorites');
      setItems(favorites.map((item) => ({
        ...item,
        message: {
          ...item.message,
          user_id: item.message.user_id || item.message.sender_id || '',
          display_name: item.message.display_name || item.message.sender_name || '',
          avatar_url: item.message.avatar_url || item.message.sender_avatar,
          status: 'sent',
        },
      })));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t('loadError'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!authLoading && !isAuthenticated) router.push('/auth/login');
    if (!authLoading && isAuthenticated) void load();
  }, [authLoading, isAuthenticated, load, router]);

  const remove = async (item: Favorite) => {
    try {
      await apiClient.delete(`/api/v1/channels/${item.message.channel_id}/messages/${item.message.id}/favorite`);
      setItems((current) => current.filter((candidate) => candidate.message.id !== item.message.id));
    } catch (reason) {
      showToast(reason instanceof Error ? reason.message : t('messageReuseFailed'), 'error');
    }
  };

  const openOriginal = (item: Favorite) => {
    switchWorkspace(item.workspace_id);
    const source = item.channel_type === 'dm' ? 'dm' : 'channel';
    const thread = item.thread_root_message_id ? `&thread=${encodeURIComponent(item.thread_root_message_id)}` : '';
    router.push(`/dashboard?${source}=${encodeURIComponent(item.message.channel_id)}${thread}&message=${encodeURIComponent(item.message.id)}`);
  };

  if (authLoading || !isAuthenticated) return <div className="flex h-screen items-center justify-center bg-brutal-cream"><Spinner size="md" /></div>;

  return (
    <PersonalFrame>
      <div className="h-full overflow-y-auto bg-skin-canvas">
        <div className="mx-auto max-w-4xl px-6 py-8">
          <header className="mb-8 flex items-start gap-3">
            <span className="flex h-10 w-10 items-center justify-center rounded-lg border border-brutal-border bg-brutal-muted-light shadow-card"><Bookmark className="h-5 w-5" /></span>
            <div>
              <h1 className="font-heading text-2xl font-black">{t('favoritesTitle')}</h1>
              <p className="mt-1 font-body text-sm text-muted-foreground">{t('favoritesDescription')}</p>
            </div>
          </header>
          {loading ? <div className="flex justify-center py-20"><Spinner size="md" /></div> : error ? (
            <div className="rounded-lg border border-brutal-border bg-card p-6 text-center"><p className="font-body text-sm text-brutal-danger">{error}</p><Button className="mt-4" size="sm" onClick={() => { void load(); }}>{t('retry')}</Button></div>
          ) : items.length === 0 ? (
            <EmptyState title={t('favoritesEmpty')} icon={<Bookmark className="h-6 w-6" />} />
          ) : (
            <div className="space-y-3">
              {items.map((item) => (
                <article key={item.message.id} className="rounded-xl border border-brutal-border bg-card p-5 shadow-card">
                  <div className="flex items-start gap-3">
                    {item.message.sender_type === 'agent' ? <PixelAvatar agentId={item.message.user_id} avatarUrl={item.message.avatar_url} size="md" /> : <UserAvatar userId={item.message.user_id} name={item.message.display_name} avatarUrl={item.message.avatar_url} size="md" />}
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                        <span className="font-heading text-sm font-bold">{item.message.display_name}</span>
                        <span className="font-mono text-[11px] text-muted-foreground">{formatDateTime(item.message.created_at)}</span>
                      </div>
                      <p className="mt-2 whitespace-pre-wrap break-words font-body text-sm leading-relaxed">{item.message.content}</p>
                      <p className="mt-3 font-body text-xs text-muted-foreground">{item.workspace_name} / {item.channel_type === 'dm' ? t('directMessages') : `#${item.channel_name}`}</p>
                    </div>
                  </div>
                  <div className="mt-4 flex justify-end gap-2">
                    <Button type="button" variant="outline" size="sm" onClick={() => { void remove(item); }}><Trash2 className="mr-1.5 h-3.5 w-3.5" />{t('removeFavorite')}</Button>
                    <Button type="button" size="sm" onClick={() => openOriginal(item)}><ExternalLink className="mr-1.5 h-3.5 w-3.5" />{t('openOriginalMessage')}</Button>
                  </div>
                </article>
              ))}
            </div>
          )}
        </div>
      </div>
    </PersonalFrame>
  );
}
