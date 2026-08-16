'use client';

import Link from 'next/link';
import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { ArrowRight, Monitor, Settings, UsersRound } from 'lucide-react';
import { PersonalFrame } from '@/components/layout/personal-frame';
import { buttonVariants } from '@/components/ui/button';
import { Spinner } from '@/components/ui/spinner';
import { useAuth } from '@/lib/auth-context';
import { useComputers } from '@/lib/hooks/use-computers';
import { t } from '@/lib/i18n';
import { useWorkspace } from '@/lib/workspace-context';
import { cn } from '@/lib/utils';

export default function PersonalHomePage() {
  const router = useRouter();
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
        <header className="border-b-2 border-black bg-brutal-cream px-8 py-7">
          <p className="font-mono text-[10px] font-bold uppercase tracking-[0.18em] text-muted-foreground">Solo</p>
          <h1 className="mt-2 font-heading text-3xl font-black">
            {t('personalWelcome', { name: user.display_name })}
          </h1>
          <p className="mt-2 font-body text-sm text-muted-foreground">{t('personalHomeDesc')}</p>
        </header>

        <div className="flex-1 overflow-y-auto bg-white p-8">
          <div className="mx-auto grid max-w-6xl gap-6 lg:grid-cols-[minmax(0,1.6fr)_minmax(280px,0.8fr)]">
            <section className="card-brutal-heavy overflow-hidden">
              <div className="flex items-start justify-between gap-4 border-b-2 border-black bg-brutal-primary px-5 py-4">
                <div>
                  <h2 className="font-heading text-lg font-black">{t('personalWorkspacesTitle')}</h2>
                  <p className="mt-1 font-body text-sm text-muted-foreground">{t('personalWorkspacesDesc')}</p>
                </div>
                <UsersRound className="h-5 w-5" aria-hidden="true" />
              </div>
              <div>
                {workspacesLoading ? (
                  <p className="p-5 font-mono text-xs text-muted-foreground">{t('loading')}</p>
                ) : workspaces.length === 0 ? (
                  <p className="p-5 font-body text-sm text-muted-foreground">{t('personalNoWorkspaces')}</p>
                ) : (
                  workspaces.map((workspace) => (
                    <button
                      key={workspace.id}
                      type="button"
                      onClick={() => openWorkspace(workspace.id)}
                      aria-label={t('personalOpenWorkspace', { name: workspace.name })}
                      className="personal-workspace-row flex w-full items-center gap-4 border-b-2 border-black px-5 py-4 text-left last:border-b-0 hover:bg-brutal-cream"
                    >
                      <span className="flex h-10 w-10 shrink-0 items-center justify-center border-2 border-black bg-white font-heading text-sm font-black shadow-brutal-sm">
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
                <div className="flex items-center gap-3 border-b-2 border-black bg-brutal-info-light px-5 py-4">
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
                    <p className="mt-4 font-mono text-xs text-muted-foreground">{t('loading')}</p>
                  )}
                  <Link href="/computers" className={buttonVariants({ variant: 'outline', size: 'sm', className: 'mt-5' })}>
                    {t('personalOpenComputers')}
                  </Link>
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
