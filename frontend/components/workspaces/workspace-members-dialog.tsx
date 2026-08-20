'use client';

import { useCallback, useEffect, useState } from 'react';
import { Copy, Eye, Link2, MailPlus, ShieldCheck, Trash2, UsersRound } from 'lucide-react';
import { apiClient } from '@/lib/api-client';
import { useAuth } from '@/lib/auth-context';
import { useWorkspace } from '@/lib/workspace-context';
import { t } from '@/lib/i18n';
import { Button, iconActionClass } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { useToast } from '@/components/ui/toast';
import { UserAvatar } from '@/components/ui/user-avatar';

interface WorkspaceMember {
  user_id: string;
  email: string;
  display_name: string;
  avatar_url?: string;
  role: 'owner' | 'admin' | 'member';
  joined_at: string;
}

interface WorkspaceInvitation {
  id: string;
  email: string;
  role: 'admin' | 'member';
  expires_at: string;
}

interface WorkspaceInviteLink {
  id: string;
  role: 'member';
  expires_at: string;
  use_count: number;
  created_at: string;
}

interface WorkspaceJoinRule {
  id: string;
  rule_type: 'email' | 'domain';
  value: string;
}

interface WorkspaceChannel { id: string; name: string }
interface WorkspaceGuestToken {
  id: string;
  label: string;
  expires_at: string;
  revoked_at?: string;
  token?: string;
  url?: string;
}
interface WorkspaceEmbedSettings {
  enabled: boolean;
  channels: WorkspaceChannel[];
  tokens: WorkspaceGuestToken[];
}

const roleOptions = [
  { value: 'member', label: 'Member' },
  { value: 'admin', label: 'Admin' },
];

export function WorkspaceSettingsCard({ bare = false, view = 'all' }: { bare?: boolean; view?: 'all' | 'members' | 'invites' } = {}) {
  const { user } = useAuth();
  const { activeWorkspace } = useWorkspace();
  const { showToast } = useToast();
  const [members, setMembers] = useState<WorkspaceMember[]>([]);
  const [invitations, setInvitations] = useState<WorkspaceInvitation[]>([]);
  const [inviteLinks, setInviteLinks] = useState<WorkspaceInviteLink[]>([]);
  const [rules, setRules] = useState<WorkspaceJoinRule[]>([]);
  const [channels, setChannels] = useState<WorkspaceChannel[]>([]);
  const [embed, setEmbed] = useState<WorkspaceEmbedSettings>({ enabled: false, channels: [], tokens: [] });
  const [guestLabel, setGuestLabel] = useState('');
  const [newGuestURL, setNewGuestURL] = useState('');
  const [newInviteURL, setNewInviteURL] = useState('');
  const [ruleType, setRuleType] = useState('domain');
  const [ruleValue, setRuleValue] = useState('');
  const [busy, setBusy] = useState(false);
  const canAdmin = activeWorkspace?.role === 'owner' || activeWorkspace?.role === 'admin';
  const isOwner = activeWorkspace?.role === 'owner';
  const showMembers = view === 'all' || view === 'members';
  const showInvites = view === 'all' || view === 'invites';

  const load = useCallback(async () => {
    if (!activeWorkspace) return;
    if (!canAdmin) {
      setMembers([]);
      setInvitations([]);
      setInviteLinks([]);
      setRules([]);
      return;
    }
    try {
      const nextMembers = await apiClient.get<WorkspaceMember[]>(`/api/v1/workspaces/${activeWorkspace.id}/members`);
      setMembers(nextMembers);
      const [nextInvitations, nextInviteLinks, nextRules, nextChannels, nextEmbed] = await Promise.all([
        apiClient.get<WorkspaceInvitation[]>(`/api/v1/workspaces/${activeWorkspace.id}/invitations`),
        apiClient.get<WorkspaceInviteLink[]>(`/api/v1/workspaces/${activeWorkspace.id}/invite-links`),
        apiClient.get<WorkspaceJoinRule[]>(`/api/v1/workspaces/${activeWorkspace.id}/join-rules`),
        apiClient.get<WorkspaceChannel[]>('/api/v1/channels'),
        apiClient.get<WorkspaceEmbedSettings>(`/api/v1/workspaces/${activeWorkspace.id}/embed`),
      ]);
      setInvitations(nextInvitations);
      setInviteLinks(nextInviteLinks);
      setRules(nextRules);
      setChannels(nextChannels);
      setEmbed(nextEmbed);
    } catch (error) {
      showToast(error instanceof Error ? error.message : t('workspaceMembersLoadFailed'), 'error');
    }
  }, [activeWorkspace, canAdmin, showToast]);

  useEffect(() => {
    void load();
  }, [load]);

  const addRule = async () => {
    if (!activeWorkspace || !ruleValue.trim()) return;
    setBusy(true);
    try {
      await apiClient.post(`/api/v1/workspaces/${activeWorkspace.id}/join-rules`, { rule_type: ruleType, value: ruleValue.trim() });
      setRuleValue('');
      await load();
      showToast(t('workspaceRuleAdded'), 'success');
    } catch (error) {
      showToast(error instanceof Error ? error.message : t('workspaceRuleFailed'), 'error');
    } finally {
      setBusy(false);
    }
  };

  const saveEmbed = async (enabled: boolean, channelIDs: string[]) => {
    if (!activeWorkspace) return;
    setBusy(true);
    try {
      const next = await apiClient.put<WorkspaceEmbedSettings>(`/api/v1/workspaces/${activeWorkspace.id}/embed`, { enabled, channel_ids: channelIDs });
      setEmbed(next);
      if (!enabled) setNewGuestURL('');
      showToast(enabled ? t('workspaceEmbedEnabled') : t('workspaceEmbedDisabled'), 'success');
    } catch (error) {
      showToast(error instanceof Error ? error.message : t('workspaceEmbedFailed'), 'error');
    } finally {
      setBusy(false);
    }
  };

  const createGuestLink = async () => {
    if (!activeWorkspace) return;
    setBusy(true);
    try {
      const token = await apiClient.post<WorkspaceGuestToken>(`/api/v1/workspaces/${activeWorkspace.id}/embed/tokens`, { label: guestLabel.trim(), expires_in_days: 7 });
      const fullURL = new URL(token.url ?? `/guest/${token.token}`, window.location.origin).toString();
      setNewGuestURL(fullURL);
      setGuestLabel('');
      await load();
      showToast(t('workspaceGuestLinkCopied'), 'success');
    } catch (error) {
      showToast(error instanceof Error ? error.message : t('workspaceGuestLinkFailed'), 'error');
    } finally {
      setBusy(false);
    }
  };

  const createInviteLink = async () => {
    if (!activeWorkspace) return;
    setBusy(true);
    try {
      const link = await apiClient.post<WorkspaceInviteLink & { url?: string }>(`/api/v1/workspaces/${activeWorkspace.id}/invite-links`, { expires_in_days: 7 });
      if (!link.url) throw new Error(t('workspaceInviteLinkFailed'));
      const fullURL = new URL(link.url, window.location.origin).toString();
      setNewInviteURL(fullURL);
      try { await navigator.clipboard.writeText(fullURL); } catch { /* copying remains available via the button */ }
      await load();
      showToast(t('workspaceInviteLinkCopied'), 'success');
    } catch (error) {
      showToast(error instanceof Error ? error.message : t('workspaceInviteLinkFailed'), 'error');
    } finally {
      setBusy(false);
    }
  };

  if (!activeWorkspace) return null;

  const sections = (
    <>
      {showMembers && <section>
        <h3 className="mb-2 font-heading text-sm font-black uppercase">{t('workspaceSectionPeople', { count: members.length })}</h3>
        <div className="max-h-52 space-y-2 overflow-y-auto pr-1">
          {members.map((member) => (
            <div key={member.user_id} className="flex items-center gap-3 border-2 border-black bg-white px-3 py-2">
              <UserAvatar
                userId={member.user_id}
                name={member.display_name}
                avatarUrl={member.avatar_url}
                size="md"
              />
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-bold">{member.display_name}{member.user_id === user?.id ? ` ${t('workspaceYouSuffix')}` : ''}</div>
                <div className="truncate font-mono text-[10px] text-black/50">{member.email}</div>
              </div>
              {member.role === 'owner' || !canAdmin || (!isOwner && member.role === 'admin') ? (
                <span className="flex items-center gap-1 font-mono text-[10px] font-bold uppercase"><ShieldCheck className="h-3.5 w-3.5" />{member.role}</span>
              ) : (
                <>
                  {isOwner ? <Select
                    value={member.role}
                    options={roleOptions}
                    onChange={async (nextRole) => {
                      await apiClient.patch(`/api/v1/workspaces/${activeWorkspace.id}/members/${member.user_id}`, { role: nextRole });
                      await load();
                    }}
                    aria-label={t('workspaceRoleAria', { name: member.display_name })}
                  /> : <span className="font-mono text-[10px] font-bold uppercase">{member.role}</span>}
                  {!activeWorkspace.is_default && (isOwner || member.role === 'member') && <button
                    type="button"
                    className={iconActionClass('hover:bg-red-200')}
                    aria-label={t('workspaceRemoveMemberAria', { name: member.display_name })}
                    onClick={async () => {
                      await apiClient.delete(`/api/v1/workspaces/${activeWorkspace.id}/members/${member.user_id}`);
                      await load();
                    }}
                  ><Trash2 className="h-3.5 w-3.5" /></button>}
                </>
              )}
            </div>
          ))}
        </div>
      </section>}

      {showInvites && canAdmin && invitations.length > 0 && (
        <section className={bare ? 'border-t-2 border-black pt-4' : 'mt-5'}>
          <h3 className="mb-2 flex items-center gap-2 font-heading text-sm font-black uppercase"><MailPlus className="h-4 w-4" /> {t('workspaceSectionInvitations')}</h3>
          <div className="space-y-2">
            {invitations.map((invitation) => (
              <div key={invitation.id} className="flex items-center gap-2 border-2 border-dashed border-black px-3 py-2 text-sm">
                <span className="min-w-0 flex-1 truncate font-mono text-xs">{invitation.email}</span>
                <span className="text-xs uppercase">{invitation.role}</span>
                <button type="button" className={iconActionClass()} aria-label={t('workspaceCancelInvitationAria', { email: invitation.email })} onClick={async () => {
                  await apiClient.delete(`/api/v1/workspaces/${activeWorkspace.id}/invitations/${invitation.id}`);
                  await load();
                }}><Trash2 className="h-3.5 w-3.5" /></button>
              </div>
            ))}
          </div>
        </section>
      )}

      {showInvites && canAdmin && (
        <section className={bare ? 'border-t-2 border-black pt-4' : 'mt-5 border-t-2 border-black pt-4'}>
          <h3 className="mb-1 flex items-center gap-2 font-heading text-sm font-black uppercase"><Link2 className="h-4 w-4" /> {t('workspaceSectionInviteLinks')}</h3>
          <p className="mb-3 text-xs text-black/55">{t('workspaceInviteLinkDesc')}</p>
          <Button variant="primary" onClick={() => void createInviteLink()} disabled={busy}>
            <Link2 className="mr-1 h-4 w-4" /> {t('workspaceCreateInviteLink')}
          </Button>
          {newInviteURL && (
            <div className="mt-3 flex items-center gap-2 border-2 border-black bg-[#DBEAFE] p-2">
              <code className="min-w-0 flex-1 truncate text-[10px]">{newInviteURL}</code>
              <button type="button" className={iconActionClass()} aria-label={t('workspaceCopyInviteLinkAria')} onClick={async () => {
                await navigator.clipboard.writeText(newInviteURL);
                showToast(t('workspaceInviteLinkCopied'), 'success');
              }}><Copy className="h-3.5 w-3.5" /></button>
            </div>
          )}
          {inviteLinks.length > 0 && (
            <div className="mt-3 space-y-2">
              {inviteLinks.map((link) => (
                <div key={link.id} className="flex items-center gap-2 border-2 border-dashed border-black px-3 py-2 text-xs">
                  <span className="min-w-0 flex-1 font-bold">{t('workspaceInviteLinkMemberRole')}</span>
                  <span className="font-mono text-[9px]">{t('workspaceInviteLinkUses', { count: link.use_count })} · {t('workspaceGuestLinkExpires', { date: new Date(link.expires_at).toLocaleDateString() })}</span>
                  <button type="button" className={iconActionClass()} aria-label={t('workspaceRevokeInviteLinkAria')} onClick={async () => {
                    await apiClient.delete(`/api/v1/workspaces/${activeWorkspace.id}/invite-links/${link.id}`);
                    await load();
                    showToast(t('workspaceInviteLinkRevoked'), 'success');
                  }}><Trash2 className="h-3.5 w-3.5" /></button>
                </div>
              ))}
            </div>
          )}
        </section>
      )}

      {showInvites && canAdmin && !activeWorkspace.is_default && (
        <section className={bare ? 'border-t-2 border-black pt-4' : 'mt-5 border-t-2 border-black pt-4'}>
          <h3 className="mb-2 font-heading text-sm font-black uppercase">{t('workspaceSectionAutoJoinRules')}</h3>
          <div className="flex gap-2">
            <Select value={ruleType} onChange={setRuleType} options={[{ value: 'domain', label: t('workspaceRuleTypeDomain') }, { value: 'email', label: t('workspaceRuleTypeEmail') }]} size="md" aria-label={t('workspaceRuleTypeAria')} />
            <Input value={ruleValue} onChange={(event) => setRuleValue(event.target.value)} placeholder={ruleType === 'domain' ? t('workspaceRulePlaceholderDomain') : t('workspaceRulePlaceholderEmail')} />
            <Button variant="primary" onClick={() => void addRule()} disabled={busy || !ruleValue.trim()}>{t('workspaceAllowButton')}</Button>
          </div>
          {rules.length > 0 && <div className="mt-3 flex flex-wrap gap-2">{rules.map((rule) => (
            <button key={rule.id} type="button" onClick={async () => {
              await apiClient.delete(`/api/v1/workspaces/${activeWorkspace.id}/join-rules/${rule.id}`);
              await load();
            }} className="border-2 border-black bg-white px-2 py-1 font-mono text-[10px] font-bold hover:bg-red-100" title={t('workspaceRemoveRuleAria', { type: rule.rule_type, value: rule.value })}>
              {rule.rule_type}: {rule.value} ×
            </button>
          ))}</div>}
        </section>
      )}

      {(view === 'all' || view === 'invites') && canAdmin && !activeWorkspace.is_default && (
        <section className={bare ? 'border-t-2 border-black pt-4' : 'mt-5 border-t-2 border-black pt-4'}>
          <h3 className="mb-1 flex items-center gap-2 font-heading text-sm font-black uppercase"><Eye className="h-4 w-4" /> {t('workspaceSectionGuestEmbed')}</h3>
          <p className="mb-3 text-xs text-black/55">{t('workspaceGuestEmbedDesc')}</p>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
            {channels.map((channel) => {
              const selected = embed.channels.some((item) => item.id === channel.id);
              return (
                <label key={channel.id} className="flex cursor-pointer items-center gap-2 border-2 border-black bg-white px-2 py-2 text-xs font-bold">
                  <input type="checkbox" checked={selected} onChange={(event) => {
                    const next = event.target.checked
                      ? [...embed.channels.map((item) => item.id), channel.id]
                      : embed.channels.filter((item) => item.id !== channel.id).map((item) => item.id);
                    void saveEmbed(next.length > 0 && (embed.enabled || event.target.checked), next);
                  }} />
                  <span className="truncate">{t('workspaceChannelHash', { name: channel.name })}</span>
                </label>
              );
            })}
          </div>
          {!embed.enabled && embed.channels.length > 0 && <Button className="mt-3" variant="primary" onClick={() => void saveEmbed(true, embed.channels.map((item) => item.id))} disabled={busy}>{t('workspaceEnableChannelsButton')}</Button>}
          {embed.enabled && (
            <>
              <div className="mt-3 flex gap-2">
                <Input value={guestLabel} onChange={(event) => setGuestLabel(event.target.value)} placeholder={t('workspaceLinkLabelPlaceholder')} />
                <Button variant="primary" onClick={() => void createGuestLink()} disabled={busy || embed.channels.length === 0}><Link2 className="mr-1 h-4 w-4" /> {t('workspaceCreateGuestLink')}</Button>
                <Button variant="danger" onClick={() => void saveEmbed(false, embed.channels.map((item) => item.id))} disabled={busy}>{t('workspaceDisableButton')}</Button>
              </div>
              {newGuestURL && (
                <div className="mt-3 flex items-center gap-2 border-2 border-black bg-[#DBEAFE] p-2">
                  <code className="min-w-0 flex-1 truncate text-[10px]">{newGuestURL}</code>
                  <button type="button" className={iconActionClass()} aria-label={t('workspaceRevokeLinkAria', { label: t('workspaceGuestLinkDefaultName') })} onClick={async () => {
                    await navigator.clipboard.writeText(newGuestURL);
                    showToast(t('workspaceGuestLinkCopied'), 'success');
                  }}><Copy className="h-3.5 w-3.5" /></button>
                </div>
              )}
              {embed.tokens.filter((token) => !token.revoked_at).length > 0 && (
                <div className="mt-3 space-y-2">
                  {embed.tokens.filter((token) => !token.revoked_at).map((token) => (
                    <div key={token.id} className="flex items-center gap-2 border-2 border-dashed border-black px-3 py-2 text-xs">
                      <span className="min-w-0 flex-1 truncate font-bold">{token.label || t('workspaceGuestLinkDefaultName')}</span>
                      <span className="font-mono text-[9px]">{t('workspaceGuestLinkExpires', { date: new Date(token.expires_at).toLocaleDateString() })}</span>
                      <button type="button" className={iconActionClass()} aria-label={t('workspaceRevokeLinkAria', { label: token.label || t('workspaceGuestLinkDefaultName') })} onClick={async () => {
                        await apiClient.delete(`/api/v1/workspaces/${activeWorkspace.id}/embed/tokens/${token.id}`);
                        await load();
                      }}><Trash2 className="h-3.5 w-3.5" /></button>
                    </div>
                  ))}
                </div>
              )}
            </>
          )}
        </section>
      )}
    </>
  );

  if (bare) {
    return (
      <div id="workspace" className="flex flex-col gap-4">
        {sections}
      </div>
    );
  }

  return (
    <section id="workspace" className="card-brutal-heavy mt-6 scroll-mt-6">
      <div data-testid="workspace-card-header" className="flex flex-wrap items-center justify-between gap-3 border-b-2 border-black bg-brutal-primary px-4 py-3 text-foreground">
        <div className="flex items-center gap-2">
          <UsersRound className="h-5 w-5" />
          <div>
            <h2 className="font-heading text-sm font-black text-foreground">Workspace · {activeWorkspace.name}</h2>
            <p className="font-mono text-[10px] font-bold uppercase tracking-wider text-muted-foreground">Advanced settings · {activeWorkspace.role}</p>
          </div>
        </div>
        <span className="flex h-7 min-w-7 items-center justify-center border-2 border-black bg-white px-1 font-heading text-xs font-black shadow-brutal-sm">{activeWorkspace.icon.slice(0, 2)}</span>
      </div>

      <div className="p-4">{sections}</div>
    </section>
  );
}
