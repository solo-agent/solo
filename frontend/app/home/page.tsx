'use client';

import Link from 'next/link';
import { useEffect } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { ArrowRight, Monitor, Settings, UsersRound } from 'lucide-react';
import { PersonalFrame } from '@/components/layout/personal-frame';
import { buttonVariants } from '@/components/ui/button';
import { EmptyState } from '@/components/ui/empty-state';
import { Spinner } from '@/components/ui/spinner';
import { useAuth } from '@/lib/auth-context';
import { useComputers } from '@/lib/hooks/use-computers';
import { t } from '@/lib/i18n';
import { useWorkspace } from '@/lib/workspace-context';
import { cn } from '@/lib/utils';

export default function PersonalHomePage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const onboardingActive = searchParams.has('onboarding');
  const { user, isAuthenticated, isLoading: authLoading } = useAuth();
  const { workspaces, switchWorkspace, isLoading: workspacesLoading } = useWorkspace();
  const { computers, isLoading: computersLoading } = useComputers();

  useEffect(() => {
    if (!authLoading && !isAuthenticated) router.replace('/auth/login');
  }, [authLoading, isAuthenticated, router]);

  if (authLoading || !isAuthenticated || !user) {
    return (
      <div className="flex h-screen items-center justify-center bg-brutal-cream">
        <Spinner size="md" />
      </div>
    );
  }

  const onlineComputers = computers.filter((computer) => computer.status === 'online').length;

  const openWorkspace = (workspaceId: string) => {
    switchWorkspace(workspaceId);
    router.push('/dashboard');
  };

  return (
    <PersonalFrame>
      <div className="flex h-full flex-col overflow-hidden">
        <header className="relative overflow-hidden border-b border-border bg-skin-canvas px-8 py-9 text-[var(--skin-ink)] lg:px-12 lg:py-12">
          <div className="relative z-10 max-w-[760px]">
            <p className="font-mono text-[10px] font-bold uppercase tracking-[0.18em] text-muted-foreground">Solo</p>
            <h1 className="mt-4 max-w-[760px] font-heading text-4xl font-black leading-[0.98] tracking-[-0.045em] lg:text-5xl">{t('personalHomeTitle')}</h1>
            <p className="mt-4 max-w-[430px] font-body text-base leading-7 text-muted-foreground lg:text-lg">{t('personalHomeDesc')}</p>
          </div>
          <svg className="pointer-events-none absolute right-8 top-1/2 hidden h-[150px] w-[300px] -translate-y-1/2 md:block lg:right-16 lg:h-[180px] lg:w-[360px]" viewBox="0 0 360 180" fill="none" aria-hidden="true">
            <path d="M34 103C83 57 127 61 172 95C214 126 261 123 326 73" stroke="var(--skin-ink)" strokeWidth="6" strokeLinecap="round" strokeLinejoin="round" />
            <circle cx="34" cy="103" r="9" fill="var(--skin-surface)" stroke="var(--skin-ink)" strokeWidth="3" />
            <circle cx="172" cy="95" r="12" fill="var(--skin-accent)" stroke="var(--skin-ink)" strokeWidth="3" />
            <circle cx="326" cy="73" r="9" fill="var(--skin-surface)" stroke="var(--skin-ink)" strokeWidth="3" />
          </svg>
        </header>

        <div className="flex-1 overflow-y-auto bg-skin-canvas p-8">
          <div className="mx-auto grid max-w-[1440px] gap-6 lg:grid-cols-[minmax(0,1.7fr)_minmax(320px,0.7fr)]">
            <section className="card-brutal-heavy overflow-hidden">
              <div className="flex items-start justify-between gap-4 border-b border-border bg-warm-stone px-5 py-5">
                <div>
                  <h2 className="font-heading text-lg font-black">{t('personalWorkspacesTitle')}</h2>
                  <p className="mt-1 font-body text-sm text-muted-foreground">{t('personalWorkspacesDesc')}</p>
                </div>
                <UsersRound className="h-5 w-5" aria-hidden="true" />
              </div>
              <div>
                {workspacesLoading ? (
                  <p className="p-5 font-body text-sm text-muted-foreground">{t('loading')}</p>
                ) : workspaces.length === 0 ? (
                  <div className="p-5">
                    <EmptyState
                      illustration={{ src: '/illustrations/empty-workspaces.png', alt: t('personalNoWorkspaces') }}
                      title={t('personalNoWorkspaces')}
                      className="bg-transparent border-0 shadow-none px-2 py-6"
                    />
                    {onboardingActive && (
                      <div className="mt-2 flex justify-center">
                        <Link href="/home?onboarding=1&guide=1" className={buttonVariants({ variant: 'outline', size: 'sm' })}>
                          {t('firstRunGuideMe')}
                        </Link>
                      </div>
                    )}
                  </div>
                ) : (
                  workspaces.map((workspace) => (
                    <button
                      key={workspace.id}
                      type="button"
                      onClick={() => openWorkspace(workspace.id)}
                      aria-label={t('personalOpenWorkspace', { name: workspace.name })}
                      className="personal-workspace-row flex w-full items-center gap-4 border-b border-border px-5 py-5 text-left last:border-b-0 hover:bg-brutal-cream"
                    >
                      <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-border bg-white font-heading text-sm font-black shadow-none">
                        {workspace.icon?.slice(0, 2) || workspace.name.slice(0, 1).toUpperCase()}
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="block truncate font-heading text-base font-bold">{workspace.name}</span>
                        <span className="mt-0.5 block font-mono text-[10px] text-muted-foreground">
                          {workspace.member_count} {t('workspaceMemberCount')}
                        </span>
                      </span>
                      <ArrowRight className="h-4 w-4 shrink-0" aria-hidden="true" />
                    </button>
                  ))
                )}
              </div>
            </section>

            <div className="space-y-6">
              <section className="card-brutal-heavy overflow-hidden">
                <div className="flex items-center gap-3 border-b border-border bg-warm-stone px-5 py-5">
                  <Monitor className="h-5 w-5" aria-hidden="true" />
                  <h2 className="font-heading text-base font-black">{t('personalComputerTitle')}</h2>
                </div>
                <div className="p-5">
                  <p className="font-body text-sm text-muted-foreground">{t('personalComputerDesc')}</p>
                  {!computersLoading && computers.length > 0 ? (
                    <div className="mt-4 space-y-2">
                      <p className="font-heading text-xl font-black">
                        {t('personalComputerSummary', { online: onlineComputers, total: computers.length })}
                      </p>
                      {computers.slice(0, 3).map((computer) => (
                        <div key={computer.id} className="flex items-center gap-2 font-body text-sm">
                          <span
                            className={cn(
                              'h-2.5 w-2.5 rounded-full border border-black',
                              computer.status === 'online' ? 'bg-brutal-success' : 'bg-brutal-muted',
                            )}
                            aria-hidden="true"
                          />
                          <span className="truncate">{computer.name}</span>
                        </div>
                      ))}
                    </div>
                  ) : !computersLoading ? (
                    <p className="mt-4 font-heading text-base font-bold">{t('personalComputerEmpty')}</p>
                  ) : (
                    <p className="mt-4 font-body text-sm text-muted-foreground">{t('loading')}</p>
                  )}
                  <div className="mt-5 flex flex-wrap gap-2">
                    <Link href="/computers" className={buttonVariants({ variant: 'outline', size: 'sm' })}>
                      {t('personalOpenComputers')}
                    </Link>
                    {onboardingActive && computers.length === 0 && (
                      <Link href="/computers?onboarding=1&guide=1" className={buttonVariants({ variant: 'outline', size: 'sm' })}>
                        {t('firstRunGuideMe')}
                      </Link>
                    )}
                  </div>
                </div>
              </section>

              <section className="card-brutal p-5">
                <div className="flex items-start gap-3">
                  <Settings className="mt-0.5 h-5 w-5" aria-hidden="true" />
                  <div>
                    <h2 className="font-heading text-base font-black">{t('personalSettingsTitle')}</h2>
                    <p className="mt-1 font-body text-sm text-muted-foreground">{t('personalSettingsDesc')}</p>
                  </div>
                </div>
                <Link href="/settings" className={buttonVariants({ variant: 'outline', size: 'sm', className: 'mt-5' })}>
                  {t('personalOpenSettings')}
                </Link>
              </section>
            </div>
          </div>
        </div>
      </div>
    </PersonalFrame>
  );
}
