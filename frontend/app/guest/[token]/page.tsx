'use client';

import { useEffect, useState } from 'react';
import { useParams } from 'next/navigation';
import { Eye, LockKeyhole } from 'lucide-react';
import { Spinner } from '@/components/ui/spinner';

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
    const body = await response.json().catch(() => ({})) as { message?: string };
    throw new Error(body.message ?? 'Guest link unavailable');
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
      .catch((reason) => setError(reason instanceof Error ? reason.message : 'Guest link unavailable'));
  }, [token]);

  useEffect(() => {
    if (!token || !channelID) return;
    guestFetch<{ messages: GuestMessage[] }>(`/api/v1/guest/channels/${channelID}/messages`, token)
      .then((next) => setMessages(next.messages))
      .catch((reason) => setError(reason instanceof Error ? reason.message : 'Messages unavailable'));
  }, [token, channelID]);

  if (error) {
    return <main className="flex min-h-screen items-center justify-center bg-brutal-cream p-6"><div className="max-w-md border-4 border-black bg-white p-8 text-center shadow-brutal-xl"><LockKeyhole className="mx-auto mb-4 h-10 w-10" /><h1 className="font-heading text-xl font-black">Guest link unavailable</h1><p className="mt-2 text-sm text-black/60">{error}</p></div></main>;
  }
  if (!info) {
    return <main className="flex min-h-screen items-center justify-center bg-brutal-cream"><Spinner size="md" label="Loading Guest Workspace" /></main>;
  }

  const activeChannel = info.channels.find((channel) => channel.id === channelID);
  return (
    <main className="min-h-screen bg-brutal-cream p-4 sm:p-8">
      <div className="mx-auto max-w-4xl">
        <header className="mb-5 flex items-center gap-4 border-4 border-black bg-[#2563EB] p-4 text-white shadow-brutal-lg">
          <div className="flex h-12 w-12 items-center justify-center border-2 border-black bg-white font-heading text-xl font-black text-black">{info.workspace_icon}</div>
          <div className="min-w-0 flex-1"><h1 className="truncate font-heading text-xl font-black">{info.workspace_name}</h1><p className="mt-0.5 flex items-center gap-1 font-mono text-[10px] font-bold uppercase"><Eye className="h-3 w-3" /> Read-only Guest view</p></div>
          <div className="hidden font-mono text-[10px] sm:block">expires {new Date(info.expires_at).toLocaleDateString()}</div>
        </header>

        <div className="grid gap-4 sm:grid-cols-[180px_minmax(0,1fr)]">
          <nav aria-label="Shared Channels" className="h-fit border-2 border-black bg-white p-2 shadow-brutal-sm">
            {info.channels.map((channel) => <button key={channel.id} type="button" onClick={() => setChannelID(channel.id)} className={`block w-full truncate border-2 border-transparent px-2 py-2 text-left text-sm font-bold ${channel.id === channelID ? 'border-black bg-[#DBEAFE]' : 'hover:bg-black/5'}`}># {channel.name}</button>)}
          </nav>
          <section className="min-h-[520px] border-2 border-black bg-white shadow-brutal-sm">
            <div className="border-b-2 border-black px-4 py-3 font-heading font-black"># {activeChannel?.name}</div>
            <div className="space-y-3 p-4">
              {messages.length === 0 && <p className="py-16 text-center text-sm text-black/45">No shared messages yet.</p>}
              {messages.map((message) => <article key={message.id} className="border-l-4 border-[#2563EB] bg-[#F8FAFC] px-3 py-2"><div className="flex items-baseline gap-2"><span className="text-sm font-black">{message.sender_name}</span><span className="font-mono text-[9px] text-black/45">{new Date(message.created_at).toLocaleString()}</span></div><p className="mt-1 whitespace-pre-wrap break-words text-sm leading-6">{message.content}</p></article>)}
            </div>
          </section>
        </div>
        <p className="mt-4 text-center font-mono text-[10px] font-bold uppercase text-black/50">Guests cannot post messages or invoke Agents</p>
      </div>
    </main>
  );
}
