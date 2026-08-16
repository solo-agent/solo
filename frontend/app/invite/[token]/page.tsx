'use client';

import { useEffect, useState } from 'react';
import { ArrowRight, Check, Link2, UsersRound } from 'lucide-react';
import { useParams, useRouter } from 'next/navigation';
import { apiClient, setStoredActiveWorkspaceId } from '@/lib/api-client';
import { useAuth } from '@/lib/auth-context';
import { useWorkspace } from '@/lib/workspace-context';
import { t } from '@/lib/i18n';
import { Button } from '@/components/ui/button';
import { Spinner } from '@/components/ui/spinner';
import { BrutalAlert } from '@/components/ui/brutal-alert';

interface InviteLinkInfo {
  workspace_id: string;
  workspace_name: string;
  workspace_icon: string;
  invited_by: string;
  expires_at: string;
  target_channel_id?: string;
  target_channel_name?: string;
  member_count: number;
}

interface AcceptInviteResult {
  workspace_id: string;
  workspace_name: string;
  channel_id?: string;
  channel_name?: string;
  already_member: boolean;
}

export default function WorkspaceInvitePage() {
  const { token } = useParams<{ token: string }>();
  const router = useRouter();
  const { user, isAuthenticated, isLoading: authLoading } = useAuth();
  const { refetch: refetchWorkspaces, switchWorkspace } = useWorkspace();
  const [info, setInfo] = useState<InviteLinkInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const invitePath = token ? `/invite/${encodeURIComponent(token)}` : '/dashboard';

  // The invitation page is public. Authentication is only needed after the
  // visitor presses Join, so the page can explain what they are joining first.
  useEffect(() => {
    if (!token) return;
    setLoading(true);
    setError(null);
    apiClient.get<InviteLinkInfo>(`/api/v1/workspace-invite-links/${encodeURIComponent(token)}`)
      .then(setInfo)
      .catch((err: unknown) => setError(err instanceof Error ? err.message : t('workspaceInviteUnavailable')))
      .finally(() => setLoading(false));
  }, [token]);

  const join = async () => {
    if (!token) return;
    if (!isAuthenticated) {
      router.push(`/auth/login?return_to=${encodeURIComponent(invitePath)}`);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const result = await apiClient.post<AcceptInviteResult>(
        `/api/v1/workspace-invite-links/${encodeURIComponent(token)}/accept`,
        {},
      );
      setStoredActiveWorkspaceId(result.workspace_id, user?.id);
      await refetchWorkspaces();
      switchWorkspace(result.workspace_id);
      router.replace(result.channel_id
        ? `/dashboard?channel=${encodeURIComponent(result.channel_id)}`
        : '/dashboard');
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t('workspaceInviteAcceptFailed'));
      setBusy(false);
    }
  };

  if (loading || authLoading) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-brutal-cream px-4">
        <Spinner size="md" />
      </main>
    );
  }

  return (
    <main className="relative flex min-h-screen items-center justify-center overflow-hidden bg-brutal-cream px-4 py-12">
      <div className="pointer-events-none absolute inset-0 opacity-30 [background-image:linear-gradient(135deg,transparent_0,transparent_49%,rgba(0,0,0,0.06)_50%,transparent_51%)] [background-size:18px_18px]" />
      <section className="relative w-full max-w-[520px] border-2 border-black bg-white px-6 py-8 shadow-brutal-heavy sm:px-12 sm:py-12">
        {error ? (
          <BrutalAlert variant="error" title={t('workspaceInviteUnavailable')}>
            <p>{error}</p>
            <Button className="mt-4" variant="outline" onClick={() => router.replace('/dashboard')}>
              {t('backToDashboard')}
            </Button>
          </BrutalAlert>
        ) : info ? (
          <div className="text-center">
            <div className="mx-auto flex h-24 w-24 items-center justify-center rounded-[28px] border-2 border-black bg-gradient-to-br from-brutal-primary via-brutal-info to-brutal-accent text-4xl font-black text-black shadow-brutal">
              {info.workspace_icon?.slice(0, 2) || 'S'}
            </div>
            <p className="mt-7 font-mono text-[11px] font-bold uppercase tracking-[0.22em] text-black/55">
              {t('workspaceInviteEyebrow')}
            </p>
            <h1 className="mt-3 font-heading text-3xl font-black leading-tight tracking-tight sm:text-4xl">
              {t('workspaceInviteJoinTitle')}
            </h1>
            <h2 className="mt-3 font-heading text-2xl font-black text-black sm:text-3xl">
              {info.workspace_name}
            </h2>
            <p className="mt-4 font-body text-base text-black/65">
              {t('workspaceInviteFrom', { name: info.invited_by })}
            </p>

            <div className="mt-8 flex items-center justify-center gap-3 border-y-2 border-black/15 py-5 text-black/70">
              <div className="flex -space-x-2" aria-hidden="true">
                <span className="flex h-9 w-9 items-center justify-center rounded-full border-2 border-white bg-brutal-primary font-heading text-sm font-black">S</span>
                <span className="flex h-9 w-9 items-center justify-center rounded-full border-2 border-white bg-brutal-info font-heading text-sm font-black">+</span>
                <span className="flex h-9 w-9 items-center justify-center rounded-full border-2 border-white bg-brutal-accent font-heading text-sm font-black">•</span>
              </div>
              <span className="font-body text-sm">
                {t('workspaceInviteMemberCount', { count: info.member_count })}
              </span>
            </div>

            {info.target_channel_name && (
              <p className="mt-5 flex items-center justify-center gap-2 font-body text-sm text-black/65">
                <Check className="h-4 w-4" />
                {t('workspaceInviteChannelHint', { name: info.target_channel_name })}
              </p>
            )}

            <Button className="mt-7 h-14 w-full text-base" onClick={() => void join()} disabled={busy}>
              {busy ? <Spinner size="sm" /> : isAuthenticated ? <ArrowRight className="h-5 w-5" /> : <Link2 className="h-5 w-5" />}
              {busy ? t('workspaceInviteJoining') : isAuthenticated ? t('workspaceInviteJoinButton') : t('workspaceInviteLoginButton')}
            </Button>
            <p className="mt-5 flex items-center justify-center gap-2 font-body text-sm text-black/55">
              <UsersRound className="h-4 w-4" />
              {t('workspaceInviteFreeToJoin')}
            </p>
          </div>
        ) : null}
      </section>
    </main>
  );
}
