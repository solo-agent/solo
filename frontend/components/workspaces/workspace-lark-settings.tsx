'use client';

import { useEffect, useMemo, useState } from 'react';
import { Check, Copy, MessageCircleMore, RefreshCw, Unplug } from 'lucide-react';
import { apiClient } from '@/lib/api-client';
import { useAgents } from '@/lib/hooks/use-agents';
import { useChannels } from '@/lib/hooks/use-channels';
import { t } from '@/lib/i18n';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useToast } from '@/components/ui/toast';

interface LarkBinding {
  id: string;
  channel_id: string;
  agent_id: string;
  platform: 'feishu' | 'lark';
  app_id: string;
  external_chat_id?: string;
  external_chat_type?: string;
  last_status?: string;
  last_error?: string;
  callback_url: string;
}

export function WorkspaceLarkSettings({ workspaceId }: { workspaceId: string }) {
  const { showToast } = useToast();
  const { channels, lucyChannel, isLoading: channelsLoading } = useChannels();
  const [binding, setBinding] = useState<LarkBinding | null>(null);
  const [platform, setPlatform] = useState<'feishu' | 'lark'>('feishu');
  const [channelId, setChannelId] = useState('');
  const [agentId, setAgentId] = useState('');
  const [appId, setAppId] = useState('');
  const [appSecret, setAppSecret] = useState('');
  const [verificationToken, setVerificationToken] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const availableChannels = useMemo(
    () => lucyChannel ? [lucyChannel, ...channels] : channels,
    [channels, lucyChannel],
  );
  const { agents, isLoading: agentsLoading } = useAgents(channelId);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    apiClient.get<LarkBinding | null>(`/api/v1/workspaces/${workspaceId}/lark-binding`)
      .then((value) => {
        if (cancelled || !value) return;
        setBinding(value);
        setPlatform(value.platform);
        setChannelId(value.channel_id);
        setAgentId(value.agent_id);
        setAppId(value.app_id);
      })
      .catch((error) => showToast(error instanceof Error ? error.message : t('larkLoadFailed'), 'error'))
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [showToast, workspaceId]);

  useEffect(() => {
    if (!channelId && availableChannels[0]) setChannelId(availableChannels[0].id);
  }, [availableChannels, channelId]);

  useEffect(() => {
    if (agents.length && !agents.some((agent) => agent.id === agentId)) setAgentId(agents[0].id);
  }, [agentId, agents]);

  const save = async () => {
    setBusy(true);
    try {
      const value = await apiClient.put<LarkBinding>(`/api/v1/workspaces/${workspaceId}/lark-binding`, {
        platform, channel_id: channelId, agent_id: agentId, app_id: appId,
        app_secret: appSecret, verification_token: verificationToken,
      });
      setBinding(value);
      setAppSecret('');
      setVerificationToken('');
      showToast(t('larkSaved'), 'success');
    } catch (error) {
      showToast(error instanceof Error ? error.message : t('larkSaveFailed'), 'error');
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
      showToast(t('larkDisconnected'), 'success');
    } catch (error) {
      showToast(error instanceof Error ? error.message : t('larkDisconnectFailed'), 'error');
    } finally {
      setBusy(false);
    }
  };

  const copyCallback = async () => {
    if (!binding?.callback_url) return;
    await navigator.clipboard.writeText(binding.callback_url);
    showToast(t('larkCallbackCopied'), 'success');
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

  return (
    <div className="space-y-5">
      <div>
        <div className="flex items-center gap-2 font-heading text-base font-bold">
          <MessageCircleMore className="h-5 w-5" />
          {t('larkSettingsTitle')}
        </div>
        <p className="mt-1 text-sm text-muted-foreground">{t('larkSettingsDesc')}</p>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <label className="space-y-1">
          <span className="text-xs font-bold">{t('larkPlatform')}</span>
          <select className="input-brutal h-9 w-full px-3 text-sm" value={platform} onChange={(event) => setPlatform(event.target.value as 'feishu' | 'lark')}>
            <option value="feishu">{t('larkPlatformFeishu')}</option>
            <option value="lark">{t('larkPlatformLark')}</option>
          </select>
        </label>
        <label className="space-y-1">
          <span className="text-xs font-bold">{t('larkChannel')}</span>
          <select className="input-brutal h-9 w-full px-3 text-sm" value={channelId} onChange={(event) => { setChannelId(event.target.value); setAgentId(''); }}>
            {availableChannels.map((channel) => <option key={channel.id} value={channel.id}># {channel.name}</option>)}
          </select>
        </label>
        <label className="space-y-1 sm:col-span-2">
          <span className="text-xs font-bold">{t('larkAgent')}</span>
          <select className="input-brutal h-9 w-full px-3 text-sm" value={agentId} onChange={(event) => setAgentId(event.target.value)} disabled={agentsLoading || agents.length === 0}>
            {agents.map((agent) => <option key={agent.id} value={agent.id}>{agent.name}</option>)}
          </select>
          {!agentsLoading && agents.length === 0 && <p className="text-xs text-red-700">{t('larkNoAgent')}</p>}
        </label>
        <label className="space-y-1 sm:col-span-2">
          <span className="text-xs font-bold">{t('larkAppID')}</span>
          <Input value={appId} onChange={(event) => setAppId(event.target.value)} autoComplete="off" />
        </label>
        <label className="space-y-1">
          <span className="text-xs font-bold">{t('larkAppSecret')}</span>
          <Input type="password" value={appSecret} onChange={(event) => setAppSecret(event.target.value)} placeholder={binding ? t('larkSecretKeep') : ''} autoComplete="new-password" />
        </label>
        <label className="space-y-1">
          <span className="text-xs font-bold">{t('larkVerificationToken')}</span>
          <Input type="password" value={verificationToken} onChange={(event) => setVerificationToken(event.target.value)} placeholder={binding ? t('larkSecretKeep') : ''} autoComplete="new-password" />
        </label>
      </div>

      <div className="flex flex-wrap justify-end gap-2">
        {binding && <Button variant="outline" onClick={() => void disconnect()} disabled={busy}><Unplug className="mr-2 h-4 w-4" />{t('larkDisconnect')}</Button>}
        <Button onClick={() => void save()} disabled={busy || !channelId || !agentId || !appId.trim() || (!binding && (!appSecret.trim() || !verificationToken.trim()))}>{binding ? <Check className="mr-2 h-4 w-4" /> : null}{t('larkSave')}</Button>
      </div>

      {binding && (
        <div className="rounded-xl border border-border bg-brutal-cream p-4">
          <div className="text-sm font-bold">{t('larkCallbackTitle')}</div>
          <p className="mt-1 text-xs text-muted-foreground">{t('larkCallbackDesc')}</p>
          <div className="mt-3 flex gap-2">
            <Input readOnly value={binding.callback_url} className="font-mono text-xs" />
            <Button variant="outline" size="icon" onClick={() => void copyCallback()} aria-label={t('larkCopyCallback')}><Copy className="h-4 w-4" /></Button>
          </div>
          <p className="mt-3 text-sm font-medium">{binding.external_chat_id ? t('larkConnectedChat') : t('larkWaitingChat')}</p>
          {binding.last_status === 'failed' && (
            <div className="mt-3 flex items-start justify-between gap-3 rounded-lg border border-red-300 bg-red-50 p-3 text-xs text-red-800">
              <span className="break-all">{binding.last_error || t('larkDeliveryFailed')}</span>
              <Button variant="outline" size="sm" onClick={() => void retry()} disabled={busy}><RefreshCw className="mr-1 h-3.5 w-3.5" />{t('retry')}</Button>
            </div>
          )}
        </div>
      )}

      <ol className="space-y-1 text-sm text-muted-foreground">
        <li>1. {t('larkSetupStep1')}</li>
        <li>2. {t('larkSetupStep2')}</li>
        <li>3. {t('larkSetupStep3')}</li>
      </ol>
    </div>
  );
}
