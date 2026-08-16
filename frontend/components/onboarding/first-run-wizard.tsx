'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { usePathname, useRouter } from 'next/navigation';
import { Bot, Check, LoaderCircle, MessageSquareText, Monitor, UsersRound } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogCloseButton, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Spinner } from '@/components/ui/spinner';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { Select } from '@/components/ui/select';
import { useAuth } from '@/lib/auth-context';
import { MODEL_PRESETS } from '@/lib/agent-models';
import { useOnboarding } from '@/lib/hooks/use-onboarding';
import { t } from '@/lib/i18n';
import type { OnboardingStatus } from '@/lib/types';
import { cn } from '@/lib/utils';
import { RuntimeLogo } from '@/components/agents/runtime-logo';
import { isSupportedAgentRuntime } from '@/lib/agent-runtimes';

const stepIcons = [Monitor, UsersRound, Bot, MessageSquareText] as const;

type TargetRect = { top: number; right: number; bottom: number; left: number; width: number; height: number };

export function FirstRunWizard() {
  const pathname = usePathname();
  const router = useRouter();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const { createLucy, getStatus, complete, isCreating } = useOnboarding();
  const [status, setStatus] = useState<OnboardingStatus | null>(null);
  const [runtime, setRuntime] = useState('');
  const [model, setModel] = useState('');
  const [lucyOpen, setLucyOpen] = useState(false);
  const [targetRect, setTargetRect] = useState<TargetRect | null>(null);
  const [externalDialogOpen, setExternalDialogOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const targetRef = useRef<HTMLElement | null>(null);
  const completing = useRef(false);

  const load = useCallback(async () => {
    if (!isAuthenticated) return;
    try {
      setStatus(await getStatus());
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('firstRunLoadError'));
    }
  }, [getStatus, isAuthenticated]);

  useEffect(() => {
    if (authLoading || !isAuthenticated) {
      setStatus(null);
      return;
    }
    void load();
  }, [authLoading, isAuthenticated, load]);

  useEffect(() => {
    if (!status?.required || ![1, 2, 4].includes(status.step)) return;
    const timer = window.setInterval(() => void load(), 2500);
    return () => window.clearInterval(timer);
  }, [load, status?.required, status?.step]);

  useEffect(() => {
    if (!status?.required) return;
    if (status.step === 1) {
      if (pathname !== '/home' && pathname !== '/computers') router.replace('/home?onboarding=1');
      return;
    }
    if (status.step === 2) {
      if (pathname !== '/home') router.replace('/home?onboarding=1');
      return;
    }
    const params = new URLSearchParams(window.location.search);
    const currentChannel = params.get('channel');
    if (status.lucy_channel_id && (pathname !== '/dashboard' || currentChannel !== status.lucy_channel_id || !params.has('onboarding'))) {
      router.replace(`/dashboard?channel=${encodeURIComponent(status.lucy_channel_id)}&onboarding=1`);
    }
  }, [pathname, router, status]);

  useEffect(() => {
    const defaultRuntime = status?.runtimes.find((item) => isSupportedAgentRuntime(item.type));
    if (!runtime && defaultRuntime) setRuntime(defaultRuntime.type);
  }, [runtime, status?.runtimes]);

  useEffect(() => {
    if (!status?.required || !status.greeting_ready || completing.current) return;
    completing.current = true;
    void complete().then(({ channel_id }) => {
      setStatus((current) => current ? { ...current, required: false } : current);
      router.replace(`/dashboard?channel=${encodeURIComponent(channel_id)}`);
    }).catch((err) => {
      completing.current = false;
      setError(err instanceof Error ? err.message : t('firstRunCompleteError'));
    });
  }, [complete, router, status]);

  const targetSelector = useMemo(() => {
    if (!status?.required) return '';
    if (status.step === 1) return pathname === '/computers'
      ? '[data-onboarding="connect-computer"]'
      : '[data-onboarding="computers-nav"]';
    if (status.step === 2) return '[data-onboarding="create-workspace"]';
    if (status.step === 3) return '[data-onboarding="create-agent"]';
    return '[data-onboarding="message-composer"]';
  }, [pathname, status?.required, status?.step]);

  useEffect(() => {
    targetRef.current = null;
    setTargetRect(null);
    if (!targetSelector) return;
    let target = document.querySelector<HTMLElement>(targetSelector);
    const update = () => {
      target = document.querySelector<HTMLElement>(targetSelector);
      targetRef.current = target;
      if (!target) {
        setTargetRect(null);
        return;
      }
      const rect = target.getBoundingClientRect();
      setTargetRect({ top: rect.top, right: rect.right, bottom: rect.bottom, left: rect.left, width: rect.width, height: rect.height });
    };
    const openLucy = (event: Event) => {
      event.preventDefault();
      event.stopImmediatePropagation();
      setLucyOpen(true);
    };
    const observer = new MutationObserver(update);
    observer.observe(document.body, { childList: true, subtree: true });
    update();
    window.addEventListener('resize', update);
    window.addEventListener('scroll', update, true);
    if (status?.step === 3) {
      const attach = window.setInterval(() => {
        const next = document.querySelector<HTMLElement>(targetSelector);
        if (next === target) return;
        target?.removeEventListener('click', openLucy, true);
        target = next;
        target?.addEventListener('click', openLucy, true);
        update();
      }, 200);
      target?.addEventListener('click', openLucy, true);
      return () => {
        window.clearInterval(attach);
        target?.removeEventListener('click', openLucy, true);
        observer.disconnect();
        window.removeEventListener('resize', update);
        window.removeEventListener('scroll', update, true);
      };
    }
    return () => {
      observer.disconnect();
      window.removeEventListener('resize', update);
      window.removeEventListener('scroll', update, true);
    };
  }, [status?.step, targetSelector]);

  useEffect(() => {
    const update = () => setExternalDialogOpen(Boolean(document.querySelector('[data-dialog-scroll]')));
    const observer = new MutationObserver(update);
    observer.observe(document.body, { childList: true, subtree: true });
    update();
    return () => observer.disconnect();
  }, []);

  const createFirstLucy = async () => {
    if (!status?.computer_id || !status.lucy_channel_id || !runtime) return;
    setError(null);
    try {
      await createLucy({ computer_id: status.computer_id, channel_id: status.lucy_channel_id, runtime_type: runtime, model_name: model });
      setLucyOpen(false);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : t('firstRunLucyError'));
    }
  };

  if (authLoading || !isAuthenticated || !status?.required) return null;

  const onTarget = () => targetRef.current?.click();
  const onCoachAction = status.step === 3 ? () => setLucyOpen(true) : onTarget;
  const onComputersPage = status.step === 1 && pathname === '/computers';
  const title = status.step === 1
    ? onComputersPage ? t('firstRunComputerTitle') : t('firstRunFindComputerTitle')
    : status.step === 2 ? t('firstRunWorkspaceTitle')
      : status.step === 3 ? t('firstRunLucyTitle') : t('firstRunGreetingTitle');
  const description = status.step === 1
    ? onComputersPage ? t('firstRunComputerDesc') : t('firstRunFindComputerDesc')
    : status.step === 2 ? t('firstRunWorkspaceGuideDesc')
      : status.step === 3 ? t('firstRunLucyGuideDesc') : t('firstRunGreetingDesc');
  const action = status.step === 1
    ? onComputersPage ? t('firstRunConnectComputer') : t('personalOpenComputers')
    : status.step === 2 ? t('firstRunCreateWorkspace')
      : status.step === 3 ? t('firstRunCreateLucy') : '';

  return (
    <>
      {!lucyOpen && !externalDialogOpen && (
        targetRect ? (
          <CoachMark rect={targetRect} step={status.step} title={title} description={description} action={action} onAction={status.step < 4 ? onCoachAction : undefined} />
        ) : (
          <div className="fixed inset-0 z-[90] flex items-center justify-center bg-black/55"><Spinner size="md" /></div>
        )
      )}
      <LucyDialog
        open={lucyOpen && status.step === 3}
        onOpenChange={setLucyOpen}
        status={status}
        runtime={runtime}
        model={model}
        onRuntimeChange={(value) => { setRuntime(value); setModel(''); }}
        onModelChange={setModel}
        onCreate={() => void createFirstLucy()}
        isCreating={isCreating}
        error={error}
      />
    </>
  );
}

function CoachMark({ rect, step, title, description, action, onAction }: {
  rect: TargetRect;
  step: number;
  title: string;
  description: string;
  action: string;
  onAction?: () => void;
}) {
  const gap = 8;
  const cardBottom = rect.top > window.innerHeight * 0.6
    ? window.innerHeight - rect.top + 24
    : 28;
  const blockers = [
    { left: 0, top: 0, right: 0, height: Math.max(0, rect.top - gap) },
    { left: 0, top: Math.max(0, rect.top - gap), width: Math.max(0, rect.left - gap), height: rect.height + gap * 2 },
    { left: rect.right + gap, top: Math.max(0, rect.top - gap), right: 0, height: rect.height + gap * 2 },
    { left: 0, top: rect.bottom + gap, right: 0, bottom: 0 },
  ];
  return (
    <div role="dialog" aria-modal="true" aria-label={title}>
      {blockers.map((style, index) => <div key={index} className="fixed z-[90] bg-black/60" style={style} />)}
      <div className="pointer-events-none fixed z-[91] border-4 border-brutal-primary shadow-brutal" style={{ left: rect.left - gap, top: rect.top - gap, width: rect.width + gap * 2, height: rect.height + gap * 2 }} />
      <section className="fixed left-1/2 z-[92] w-[min(440px,calc(100vw-32px))] -translate-x-1/2 border-2 border-black bg-brutal-cream p-4 shadow-brutal-xl" style={{ bottom: cardBottom }}>
        <div className="mb-3 flex gap-2" aria-label={t('firstRunProgress')}>
          {stepIcons.map((Icon, index) => {
            const number = index + 1;
            return (
              <span key={number} className={cn('flex h-7 flex-1 items-center justify-center gap-1 border-2 border-black font-mono text-[10px] font-bold', number < step ? 'bg-brutal-success' : number === step ? 'bg-brutal-primary' : 'bg-white text-muted-foreground')}>
                {number < step ? <Check className="h-3.5 w-3.5" /> : <Icon className="h-3.5 w-3.5" />}{number}/4
              </span>
            );
          })}
        </div>
        <h2 className="font-heading text-lg font-black">{title}</h2>
        <p className="mt-1 font-body text-sm text-muted-foreground">{description}</p>
        {onAction ? <Button className="mt-4" onClick={onAction}>{action}</Button> : <div className="mt-4 flex items-center gap-2 font-body text-sm"><LoaderCircle className="h-4 w-4 animate-spin" />{t('firstRunWaitingLucy')}</div>}
      </section>
    </div>
  );
}

function LucyDialog({ open, onOpenChange, status, runtime, model, onRuntimeChange, onModelChange, onCreate, isCreating, error }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  status: OnboardingStatus;
  runtime: string;
  model: string;
  onRuntimeChange: (runtime: string) => void;
  onModelChange: (model: string) => void;
  onCreate: () => void;
  isCreating: boolean;
  error: string | null;
}) {
  const models = MODEL_PRESETS[runtime] ?? [];
  const runtimes = status.runtimes.filter((item) => isSupportedAgentRuntime(item.type));
  const supportsCustomModel = ['opencode', 'hermes', 'openclaw'].includes(runtime);
  const [customModel, setCustomModel] = useState(false);

  useEffect(() => setCustomModel(false), [runtime]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange} width="lg">
      <DialogHeader className="mb-0">
        <DialogTitle className="text-xl">{t('firstRunLucyTitle')}</DialogTitle>
        <DialogCloseButton onClick={() => onOpenChange(false)} />
      </DialogHeader>
      <div className="-mx-4 my-4 flex items-center gap-4 border-y border-border bg-brutal-cream/40 px-4 py-4 sm:-mx-6 sm:px-6">
        <PixelAvatar agentId="lucy-onboarding" avatarUrl="dicebear:pixel-art:lucy" size="md" className="!h-16 !w-16 rounded-full border border-border shadow-none" />
        <div>
          <p className="font-body text-xl font-semibold">Lucy</p>
          <p className="mt-0.5 max-w-sm font-body text-sm leading-relaxed text-muted-foreground">{t('firstRunLucyDesc')}</p>
        </div>
      </div>
      <div className="space-y-4">
        <div className="flex items-center gap-3 rounded-lg border border-border bg-white px-3 py-2.5">
          <span className="flex h-9 w-9 items-center justify-center rounded-md bg-brutal-cream"><Monitor className="h-4 w-4" /></span>
          <div className="min-w-0 flex-1">
            <p className="truncate font-body text-sm font-semibold">{status.computer_name}</p>
            <p className="mt-0.5 flex items-center gap-1.5 font-body text-xs text-muted-foreground"><span className="h-2 w-2 rounded-full bg-brutal-success" />{t('online')}</p>
          </div>
        </div>
        <fieldset className="space-y-2">
          <legend className="mb-2 font-body text-sm font-medium text-muted-foreground">{t('onboardingSelectTool')}</legend>
          {runtimes.map((item) => (
            <label key={item.type} className={cn('flex min-h-14 cursor-pointer items-center gap-3 rounded-lg border bg-white px-3 py-2 transition-colors hover:bg-brutal-cream/40', runtime === item.type ? 'border-muted-foreground bg-muted ring-1 ring-muted-foreground' : 'border-border')}>
              <input className="h-4 w-4" style={{ accentColor: 'var(--color-muted-foreground)' }} type="radio" name="first-run-runtime" value={item.type} checked={runtime === item.type} onChange={() => onRuntimeChange(item.type)} />
              <span className="flex h-7 w-7 items-center justify-center"><RuntimeLogo runtime={item.type} className="h-5 w-5" /></span>
              <span className="min-w-0 flex-1 truncate font-body text-sm font-semibold">{item.display_name || item.type}</span>
            </label>
          ))}
          {runtimes.length === 0 && <p className="font-body text-sm text-brutal-danger">{t('onboardingNoTool')}</p>}
        </fieldset>
        <div>
          <label htmlFor="first-run-model" className="font-body text-sm font-medium text-muted-foreground">{t('agentFormModel')}</label>
          <Select
            id="first-run-model"
            value={customModel ? '__custom__' : model}
            onChange={(value) => {
              const isCustom = value === '__custom__';
              setCustomModel(isCustom);
              onModelChange(isCustom ? '' : value);
            }}
            options={[
              { value: '', label: t('firstRunDefaultModel') },
              ...models,
              ...(supportsCustomModel ? [{ value: '__custom__', label: t('firstRunCustomModel') }] : []),
            ]}
            size="md"
            className="mt-2 w-full font-body"
            aria-label={t('agentFormModel')}
          />
          {customModel && (
            <input
              autoFocus
              value={model}
              onChange={(event) => onModelChange(event.target.value)}
              placeholder={t('firstRunCustomModelPlaceholder')}
              className="mt-2 h-10 w-full rounded-lg border border-border bg-white px-3 font-mono text-sm outline-none focus:border-brutal-info focus:ring-2 focus:ring-brutal-info-light"
            />
          )}
        </div>
        {error && <p role="alert" className="border-2 border-black bg-brutal-danger-light p-3 font-body text-sm font-bold">{error}</p>}
      </div>
      <DialogFooter className="sticky -bottom-4 z-10 -mx-4 -mb-4 border-t border-border bg-card px-4 py-4 sm:-bottom-6 sm:-mx-6 sm:-mb-6 sm:px-6">
        <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isCreating}>{t('cancel')}</Button>
        <Button onClick={onCreate} disabled={isCreating || !runtime || (customModel && !model.trim())}>{isCreating ? t('onboardingCreatingLucy') : t('firstRunCreateLucy')}</Button>
      </DialogFooter>
    </Dialog>
  );
}
