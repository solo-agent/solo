'use client';

import { useEffect, useState } from 'react';
import { useParams } from 'next/navigation';
import { Eye, LockKeyhole, MessageSquareText } from 'lucide-react';
import { Spinner } from '@/components/ui/spinner';
import { getLocale, t } from '@/lib/i18n';
import { cn } from '@/lib/utils';

interface GuestChannel { id: string; name: string }
interface GuestInfo {
  workspace_id: string;
  workspace_name: string;
  workspace_icon: string;
  expires_at: string;
  channels: GuestChannel[];
}
interface GuestMessage {
  id: string;
  sender_type: string;
  sender_name: string;
  content: string;
  created_at: string;
}

const apiBase = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080';

async function guestFetch<T>(path: string, token: string): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, { headers: { Authorization: `Guest ${token}` } });
  if (!response.ok) {
    throw new Error(t('guestUnavailableDescription'));
  }
  return response.json() as Promise<T>;
}

export default function GuestWorkspacePage() {
  const { token } = useParams<{ token: string }>();
  const [info, setInfo] = useState<GuestInfo | null>(null);
  const [channelID, setChannelID] = useState('');
  const [messages, setMessages] = useState<GuestMessage[]>([]);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!token) return;
    guestFetch<GuestInfo>('/api/v1/guest/embed', token)
      .then((next) => {
        setInfo(next);
        setChannelID(next.channels[0]?.id ?? '');
      })
      .catch((reason) => setError(reason instanceof Error ? reason.message : t('guestUnavailableTitle')));
  }, [token]);

  useEffect(() => {
    if (!token || !channelID) return;
    guestFetch<{ messages: GuestMessage[] }>(`/api/v1/guest/channels/${channelID}/messages`, token)
      .then((next) => setMessages(next.messages))
      .catch((reason) => setError(reason instanceof Error ? reason.message : t('guestUnavailableTitle')));
  }, [token, channelID]);

  if (error) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-brutal-cream p-6">
        <div className="max-w-md rounded-2xl border-2 border-black bg-white p-8 text-center shadow-brutal-lg">
          <LockKeyhole className="mx-auto mb-4 h-10 w-10" />
          <h1 className="font-heading text-xl font-black">{t('guestUnavailableTitle')}</h1>
          <p className="mt-2 text-sm text-black/60">{error}</p>
        </div>
      </main>
    );
  }
  if (!info) {
    return <main className="flex min-h-screen items-center justify-center bg-brutal-cream"><Spinner size="md" label={t('guestLoading')} /></main>;
  }

  const activeChannel = info.channels.find((channel) => channel.id === channelID);
  const expires = new Intl.DateTimeFormat(getLocale(), { dateStyle: 'medium' }).format(new Date(info.expires_at));
  return (
    <main className="min-h-screen bg-brutal-cream p-4 sm:p-8">
      <div className="mx-auto max-w-4xl">
        <header className="mb-5 flex items-center gap-4 rounded-2xl border-2 border-black bg-brutal-primary p-4 shadow-brutal-lg">
          <div className="flex h-12 w-12 items-center justify-center rounded-xl border-2 border-black bg-white font-heading text-xl font-black">{info.workspace_icon}</div>
          <div className="min-w-0 flex-1">
            <h1 className="truncate font-heading text-xl font-black">{info.workspace_name}</h1>
            <p className="mt-0.5 flex items-center gap-1 font-body text-xs font-semibold text-black/60"><Eye className="h-3 w-3" /> {t('guestReadOnly')}</p>
          </div>
          <div className="hidden font-body text-xs tabular-nums text-black/60 sm:block">{t('guestExpires', { date: expires })}</div>
        </header>

        <div className="grid gap-4 sm:grid-cols-[180px_minmax(0,1fr)]">
          <nav aria-label={t('guestSharedChannels')} className="h-fit rounded-xl border-2 border-black bg-white p-2 shadow-brutal-sm">
            {info.channels.map((channel) => (
              <button
                key={channel.id}
                type="button"
                onClick={() => setChannelID(channel.id)}
                className={cn(
                  'block w-full truncate rounded-lg border-2 px-2 py-2 text-left text-sm font-bold',
                  channel.id === channelID
                    ? 'border-black bg-brutal-primary-light'
                    : 'border-transparent hover:bg-brutal-cream',
                )}
              >
                # {channel.name}
              </button>
            ))}
          </nav>
          <section className="min-h-[520px] overflow-hidden rounded-xl border-2 border-black bg-white shadow-brutal-sm">
            <div className="border-b-2 border-black px-4 py-3 font-heading font-black"># {activeChannel?.name}</div>
            <div className="space-y-3 p-4">
              {messages.length === 0 && (
                <div className="py-16 text-center text-black/45">
                  <MessageSquareText className="mx-auto mb-3 h-8 w-8" />
                  <p className="text-sm">{t('guestNoMessages')}</p>
                </div>
              )}
              {messages.map((message) => (
                <article key={message.id} className="rounded-r-lg border-l-2 border-brutal-accent bg-brutal-cream px-3 py-2">
                  <div className="flex items-baseline gap-2">
                    <span className="text-sm font-black">{message.sender_name}</span>
                    <time className="font-mono text-[9px] text-black/45" dateTime={message.created_at}>
                      {new Date(message.created_at).toLocaleString(getLocale())}
                    </time>
                  </div>
                  <p className="mt-1 whitespace-pre-wrap break-words text-sm leading-6">{message.content}</p>
                </article>
              ))}
            </div>
          </section>
        </div>
        <p className="mt-4 text-center font-body text-xs font-medium text-black/50">{t('guestReadOnlyFooter')}</p>
      </div>
    </main>
  );
}
