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
      .catch(() => setError(t('workspaceInviteUnavailableDescription')))
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
    } catch {
      setError(t('workspaceInviteAcceptFailed'));
      setBusy(false);
    }
  };

  if (loading || authLoading) {
    return (
      <main id="main-content" className="flex min-h-screen items-center justify-center bg-brutal-cream px-4">
        <Spinner size="md" />
      </main>
    );
  }

  return (
    <main id="main-content" className="relative flex min-h-screen items-center justify-center bg-brutal-cream px-4 py-12">
      <div className="pointer-events-none absolute inset-0 bg-noise opacity-25" aria-hidden />
      <section className="relative w-full max-w-[520px] rounded-2xl border border-black bg-white px-6 py-8 shadow-brutal-lg sm:px-12 sm:py-12">
        {error ? (
          <div className="text-center">
            <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl border border-black bg-brutal-danger-light shadow-brutal-sm">
              <Link2 className="h-7 w-7" aria-hidden="true" />
            </div>
            <h1 className="mt-6 font-heading text-3xl font-bold">{t('workspaceInviteUnavailable')}</h1>
            <p className="mt-3 font-body text-sm leading-6 text-muted-foreground">{error}</p>
            <Button className="mt-7 w-full" variant="outline" onClick={() => router.replace('/home')}>
              {t('backToDashboard')}
            </Button>
          </div>
        ) : info ? (
          <div className="text-center">
            <div className="mx-auto flex h-24 w-24 items-center justify-center rounded-[28px] border border-black bg-brutal-success-light font-heading text-4xl font-bold shadow-brutal-sm">
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

            <div className="mt-8 flex items-center justify-center gap-3 border-y border-black py-5 text-black/70">
              <div className="flex -space-x-2" aria-hidden="true">
                <span className="flex h-9 w-9 items-center justify-center rounded-full border-2 border-white bg-brutal-primary font-heading text-sm font-bold">S</span>
                <span className="flex h-9 w-9 items-center justify-center rounded-full border-2 border-white bg-brutal-info-light font-heading text-sm font-bold">+</span>
                <span className="flex h-9 w-9 items-center justify-center rounded-full border-2 border-white bg-brutal-accent-light font-heading text-sm font-bold">•</span>
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
