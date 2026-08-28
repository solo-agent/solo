'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { CheckCircle2, ChevronDown, LoaderCircle, MessageCircleMore, QrCode, RefreshCw, Unplug } from 'lucide-react';
import { QRCodeSVG } from 'qrcode.react';
import { apiClient } from '@/lib/api-client';
import { useAgents } from '@/lib/hooks/use-agents';
import { useChannels } from '@/lib/hooks/use-channels';
import { t } from '@/lib/i18n';
import { Button } from '@/components/ui/button';
import { useToast } from '@/components/ui/toast';

interface LarkBinding {
  id: string;
  channel_id: string;
  agent_id: string;
  platform: 'feishu' | 'lark';
  external_chat_id?: string;
  last_status?: string;
  last_error?: string;
  connection_status: 'connecting' | 'connected' | 'error';
  connection_error?: string;
}

interface LarkRegistration {
  session_id: string;
  status: 'starting' | 'waiting' | 'connected' | 'expired' | 'error';
  qr_code_url?: string;
  expires_at?: string;
  error?: string;
}

export function WorkspaceLarkSettings({ workspaceId }: { workspaceId: string }) {
  const { showToast } = useToast();
  const { channels, lucyChannel, isLoading: channelsLoading } = useChannels();
  const [binding, setBinding] = useState<LarkBinding | null>(null);
  const [registration, setRegistration] = useState<LarkRegistration | null>(null);
  const [platform, setPlatform] = useState<'feishu' | 'lark'>('feishu');
  const [channelId, setChannelId] = useState('');
  const [agentId, setAgentId] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const availableChannels = useMemo(
    () => lucyChannel ? [lucyChannel, ...channels] : channels,
    [channels, lucyChannel],
  );
  const { agents, isLoading: agentsLoading } = useAgents(channelId);

  const loadBinding = useCallback(async () => {
    const value = await apiClient.get<LarkBinding | null>(`/api/v1/workspaces/${workspaceId}/lark-binding`);
    setBinding(value);
    if (value) {
      setPlatform(value.platform);
      setChannelId(value.channel_id);
      setAgentId(value.agent_id);
    }
  }, [workspaceId]);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    loadBinding()
      .catch((error) => { if (!cancelled) showToast(error instanceof Error ? error.message : t('larkLoadFailed'), 'error'); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [loadBinding, showToast]);

  useEffect(() => {
    if (!channelId && availableChannels[0]) setChannelId(availableChannels[0].id);
  }, [availableChannels, channelId]);

  useEffect(() => {
    if (agents.length && !agents.some((agent) => agent.id === agentId)) setAgentId(agents[0].id);
  }, [agentId, agents]);

  useEffect(() => {
    if (!registration || !['starting', 'waiting'].includes(registration.status)) return;
    const timer = window.setTimeout(async () => {
      try {
        const value = await apiClient.get<LarkRegistration>(`/api/v1/workspaces/${workspaceId}/lark-binding/registration/${registration.session_id}`);
        setRegistration(value);
        if (value.status === 'connected') {
          await loadBinding();
          showToast(t('larkQRConnected'), 'success');
        }
      } catch (error) {
        setRegistration((value) => value ? { ...value, status: 'error', error: error instanceof Error ? error.message : t('larkQRFailed') } : null);
      }
    }, 2000);
    return () => window.clearTimeout(timer);
  }, [loadBinding, registration, showToast, workspaceId]);

  const startRegistration = async () => {
    setBusy(true);
    try {
      const value = await apiClient.post<LarkRegistration>(`/api/v1/workspaces/${workspaceId}/lark-binding/registration`, {
        platform, channel_id: channelId, agent_id: agentId,
      });
      setRegistration(value);
    } catch (error) {
      showToast(error instanceof Error ? error.message : t('larkQRFailed'), 'error');
    } finally {
      setBusy(false);
    }
  };

  const disconnect = async () => {
    if (!window.confirm(t('larkDisconnectConfirm'))) return;
    setBusy(true);
    try {
      await apiClient.delete(`/api/v1/workspaces/${workspaceId}/lark-binding`);
      setBinding(null);
      setRegistration(null);
      showToast(t('larkDisconnected'), 'success');
    } catch (error) {
      showToast(error instanceof Error ? error.message : t('larkDisconnectFailed'), 'error');
    } finally {
      setBusy(false);
    }
  };

  const retry = async () => {
    setBusy(true);
    try {
      await apiClient.post(`/api/v1/workspaces/${workspaceId}/lark-binding/retry`);
      showToast(t('larkRetried'), 'success');
    } catch (error) {
      showToast(error instanceof Error ? error.message : t('larkRetryFailed'), 'error');
    } finally {
      setBusy(false);
    }
  };

  if (loading || channelsLoading) return <p className="text-sm text-muted-foreground">{t('loading')}</p>;

  const connectionError = registration?.error || binding?.connection_error;
  const waitingForScan = registration?.status === 'starting' || registration?.status === 'waiting';

  return (
    <div className="space-y-5">
      <div>
        <div className="flex items-center gap-2 font-heading text-base font-bold">
          <MessageCircleMore className="h-5 w-5" />
          {t('larkSettingsTitle')}
        </div>
        <p className="mt-1 text-sm text-muted-foreground">{t('larkQRSettingsDesc')}</p>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <label className="space-y-1">
          <span className="text-xs font-bold">{t('larkPlatform')}</span>
          <div className="relative">
            <select className="input-brutal h-9 w-full appearance-none px-3 pr-11 text-sm" value={platform} onChange={(event) => setPlatform(event.target.value as 'feishu' | 'lark')}>
              <option value="feishu">{t('larkPlatformFeishu')}</option>
              <option value="lark">{t('larkPlatformLark')}</option>
            </select>
            <ChevronDown aria-hidden className="pointer-events-none absolute right-4 top-1/2 h-4 w-4 -translate-y-1/2" />
          </div>
        </label>
        <label className="space-y-1">
          <span className="text-xs font-bold">{t('larkChannel')}</span>
          <div className="relative">
            <select className="input-brutal h-9 w-full appearance-none px-3 pr-11 text-sm" value={channelId} onChange={(event) => { setChannelId(event.target.value); setAgentId(''); }}>
              {availableChannels.map((channel) => <option key={channel.id} value={channel.id}># {channel.name}</option>)}
            </select>
            <ChevronDown aria-hidden className="pointer-events-none absolute right-4 top-1/2 h-4 w-4 -translate-y-1/2" />
          </div>
        </label>
        <label className="space-y-1 sm:col-span-2">
          <span className="text-xs font-bold">{t('larkAgent')}</span>
          <div className="relative">
            <select className="input-brutal h-9 w-full appearance-none px-3 pr-11 text-sm" value={agentId} onChange={(event) => setAgentId(event.target.value)} disabled={agentsLoading || agents.length === 0}>
              {agents.map((agent) => <option key={agent.id} value={agent.id}>{agent.name}</option>)}
            </select>
            <ChevronDown aria-hidden className="pointer-events-none absolute right-4 top-1/2 h-4 w-4 -translate-y-1/2" />
          </div>
          {!agentsLoading && agents.length === 0 && <p className="text-xs text-red-700">{t('larkNoAgent')}</p>}
        </label>
      </div>

      <div className="flex flex-wrap justify-end gap-2">
        {binding && <Button variant="outline" onClick={() => void disconnect()} disabled={busy}><Unplug className="mr-2 h-4 w-4" />{t('larkDisconnect')}</Button>}
        <Button onClick={() => void startRegistration()} disabled={busy || !channelId || !agentId || waitingForScan}>
          {waitingForScan ? <LoaderCircle className="mr-2 h-4 w-4 animate-spin" /> : <QrCode className="mr-2 h-4 w-4" />}
          {binding ? t('larkQRReconnect') : t('larkQRConnect')}
        </Button>
      </div>

      {registration?.qr_code_url && waitingForScan && (
        <div className="rounded-2xl border border-border bg-brutal-cream p-6 text-center">
          <div className="font-heading text-lg font-bold">{t('larkQRTitle')}</div>
          <p className="mt-1 text-sm text-muted-foreground">{t('larkQRDesc')}</p>
          <div className="mx-auto mt-5 w-fit rounded-2xl border border-border bg-white p-4 shadow-sm">
            <QRCodeSVG value={registration.qr_code_url} size={220} />
          </div>
          <p className="mt-4 flex items-center justify-center gap-2 text-sm font-medium">
            <LoaderCircle className="h-4 w-4 animate-spin" />{t('larkQRWaiting')}
          </p>
        </div>
      )}

      {binding && (
        <div className="rounded-xl border border-border bg-brutal-cream p-4">
          <div className="flex items-center gap-2 text-sm font-bold">
            {binding.connection_status === 'connected' ? <CheckCircle2 className="h-4 w-4 text-green-700" /> : <LoaderCircle className="h-4 w-4 animate-spin" />}
            {binding.connection_status === 'connected' ? t('larkConnectionConnected') : t('larkConnectionConnecting')}
          </div>
          <p className="mt-2 text-sm text-muted-foreground">{binding.external_chat_id ? t('larkConnectedChat') : t('larkWaitingChat')}</p>
          {binding.last_status === 'failed' && (
            <div className="mt-3 flex items-start justify-between gap-3 rounded-lg border border-red-300 bg-red-50 p-3 text-xs text-red-800">
              <span className="break-all">{binding.last_error || t('larkDeliveryFailed')}</span>
              <Button variant="outline" size="sm" onClick={() => void retry()} disabled={busy}><RefreshCw className="mr-1 h-3.5 w-3.5" />{t('retry')}</Button>
            </div>
          )}
        </div>
      )}

      {(registration?.status === 'expired' || registration?.status === 'error' || binding?.connection_status === 'error') && (
        <div className="rounded-xl border border-red-300 bg-red-50 p-4 text-sm text-red-800">
          <div className="font-bold">{registration?.status === 'expired' ? t('larkQRExpired') : t('larkQRFailed')}</div>
          {connectionError && <p className="mt-1 break-all text-xs">{connectionError}</p>}
        </div>
      )}
    </div>
  );
}
