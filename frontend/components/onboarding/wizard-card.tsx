'use client';

import { useEffect, useState, useMemo } from 'react';
import Link from 'next/link';
import { Check, Monitor, Cpu, Sparkles, AlertCircle, RefreshCw } from 'lucide-react';
import { useCliDetection } from '@/lib/hooks/use-cli-detection';
import { useComputers } from '@/lib/hooks/use-computers';
import { useOnboarding } from '@/lib/hooks/use-onboarding';
import { Button } from '@/components/ui/button';
import { Spinner } from '@/components/ui/spinner';
import { Select, type SelectOption } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { listTemplates, type Template } from '@/lib/templates-api';
import { recommendTemplate, type TemplateRecommendation } from '@/lib/recommend-template';
import { t } from '@/lib/i18n';

interface WizardCardProps {
  channelId: string;
  onComplete?: () => void;
}

export function WizardCard({ channelId, onComplete }: WizardCardProps) {
  const { computers, isLoading: computersLoading, claimComputer, refetch } = useComputers();
  const isMember = (c: { my_role?: string | null }) => c.my_role === 'owner' || c.my_role === 'member';
  const myComputer = computers.find((c) => c.status === 'online' && isMember(c));
  const {
    results: cliResults,
    isLoaded: cliLoaded,
    error: cliError,
  } = useCliDetection(myComputer?.id, myComputer?.runtime_inventory ?? []);
  const { createLucy, isCreating, error: createError } = useOnboarding();

  const [selectedRuntime, setSelectedRuntime] = useState<string>('');
  const [done, setDone] = useState(false);
  const [claimingId, setClaimingId] = useState<string | null>(null);
  const [goal, setGoal] = useState('');
  const [templates, setTemplates] = useState<Template[]>([]);
  const [recommendation, setRecommendation] = useState<TemplateRecommendation | null>(null);

  useEffect(() => {
    listTemplates().then(setTemplates).catch(() => setTemplates([]));
  }, []);

  const joinableComputers = computers.filter((c) => c.status === 'online' && !isMember(c));

  const runtimeOptions: SelectOption[] = useMemo(() => {
    const options: SelectOption[] = [];
    for (const [type, item] of Object.entries(cliResults)) {
      if (item.available) {
        const label = item.version
          ? `${item.display_name} (${item.version})`
          : item.display_name;
        options.push({ value: type, label });
      }
    }
    return options;
  }, [cliResults]);

  const hasAvailableRuntime = runtimeOptions.length > 0;

  const handleClaim = async (computerId: string) => {
    setClaimingId(computerId);
    try {
      await claimComputer(computerId);
    } catch {
      // error handled inline
    } finally {
      setClaimingId(null);
    }
  };

  const handleCreateLucy = async () => {
    if (!selectedRuntime || !myComputer || isCreating || done) return;
    try {
      await createLucy({
        runtime_type: selectedRuntime,
        channel_id: channelId,
        computer_id: myComputer.id,
      });
      setDone(true);
      onComplete?.();
    } catch {
      // error state handled by hook
    }
  };

  return (
    <div className="card-brutal mb-4">
      {/* Header */}
      <div className="flex items-center gap-2 border-b-2 border-black px-5 py-3">
        <Sparkles className="h-4 w-4 text-brutal-primary" />
        <h3 className="font-heading font-bold text-base text-foreground">
          {t('onboardingWelcomeTitle')}
        </h3>
      </div>

      <div className="space-y-1 px-5 py-4">
        <div className="mb-4 border-2 border-black bg-brutal-accent-light p-3 shadow-brutal-sm">
          <p className="font-heading text-base font-black">{t('onboardingGoalTitle')}</p>
          <p className="mt-1 font-body text-xs text-black/65">{t('onboardingGoalDesc')}</p>
          <Textarea
            value={goal}
            onChange={(event) => {
              setGoal(event.target.value);
              setRecommendation(null);
            }}
            rows={2}
            className="mt-3 min-h-16 resize-none bg-white"
            placeholder={t('onboardingGoalPlaceholder')}
          />
          <Button
            type="button"
            size="sm"
            className="mt-2"
            disabled={!goal.trim() || templates.length === 0}
            onClick={() => setRecommendation(recommendTemplate(goal, templates))}
          >
            <Sparkles className="h-3.5 w-3.5" />
            {t('onboardingGoalRecommend')}
          </Button>
          {recommendation && (
            <div className="mt-3 border-2 border-black bg-white p-3">
              <div className="font-body text-[11px] font-semibold uppercase tracking-wider text-black/50">{t('onboardingGoalResult')}</div>
              <div className="mt-1 font-heading text-base font-black">{recommendation.template.icon} {recommendation.template.name}</div>
              <p className="mt-1 font-body text-xs leading-relaxed text-black/65">{recommendation.reason}</p>
              <p className="mt-2 font-body text-xs font-semibold">{t('onboardingGoalNext')}</p>
            </div>
          )}
        </div>

        <div className="mb-2 border-b-2 border-black pb-2">
          <p className="font-heading text-sm font-black">{t('onboardingSetupTitle')}</p>
          <p className="font-body text-xs text-muted-foreground">{t('onboardingSetupDesc')}</p>
        </div>

        {/* Step 1: Computer */}
        <div className="flex items-start gap-3 py-2">
          <div
            className={`mt-0.5 flex h-5 w-5 flex-shrink-0 items-center justify-center border-2 border-black ${
              computersLoading ? 'bg-brutal-muted' : myComputer ? 'bg-brutal-success' : 'bg-brutal-warning'
            }`}
          >
            {computersLoading ? (
              <Spinner size="sm" />
            ) : myComputer ? (
              <Check className="h-3 w-3 text-black" />
            ) : (
              <Monitor className="h-3 w-3 text-black" />
            )}
          </div>
          <div className="min-w-0">
            <p className="font-heading text-sm font-bold text-foreground">
              {myComputer ? t('onboardingComputerConnected') : t('onboardingConnectComputer')}
            </p>
            <div className="font-sans text-xs text-muted-foreground">
              {computersLoading ? (
                t('onboardingDetecting')
              ) : myComputer ? (
                <span>
                  <Monitor className="mr-1 inline h-3 w-3" />
                  {myComputer.name} — {t('online')}
                </span>
              ) : joinableComputers.length > 0 ? (
                <div className="mt-1 space-y-1">
                  {joinableComputers.map((c) => (
                    <button
                      key={c.id}
                      type="button"
                      disabled={claimingId === c.id}
                      onClick={() => handleClaim(c.id)}
                      className="flex w-full items-center gap-2 border-2 border-black px-2 py-1 text-left text-xs font-medium hover:bg-brutal-primary-light disabled:opacity-50"
                    >
                      <Monitor className="h-3 w-3 flex-shrink-0" />
                      <span className="flex-1 truncate">{c.name || c.hostname || c.id}</span>
                      {claimingId === c.id ? (
                        <Spinner size="sm" />
                      ) : (
                        <span className="text-brutal-primary font-bold">{t('onboardingConnect')}</span>
                      )}
                    </button>
                  ))}
                </div>
              ) : (
                <span className="text-brutal-danger">
                  {t('onboardingNoComputer')}{' '}
                  <button
                    type="button"
                    onClick={() => refetch()}
                    className="inline-flex items-center gap-0.5 font-bold underline hover:text-brutal-primary"
                  >
                    <RefreshCw className="h-3 w-3" />
                    {t('retry')}
                  </button>
                  {' · '}
                  <Link href="/computers" className="font-bold underline hover:text-brutal-primary">
                    {t('onboardingAddComputer')}
                  </Link>
                </span>
              )}
            </div>
          </div>
        </div>

        {/* Step 2: Select Runtime */}
        <div className="flex items-start gap-3 py-2">
          <div className="mt-0.5 flex h-5 w-5 flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-info">
            <Cpu className="h-3 w-3 text-black" />
          </div>
          <div className="min-w-0 flex-1">
            <p className="font-heading text-sm font-bold text-foreground">
              {t('onboardingSelectTool')}
            </p>
            <p className="mb-2 font-sans text-xs text-muted-foreground">
              {t('onboardingSelectToolDesc')}
            </p>
            {!cliLoaded ? (
              <Spinner size="sm" />
            ) : cliError ? (
              <p className="font-body text-xs text-brutal-danger">
                {t('onboardingToolDetectError')}
              </p>
            ) : hasAvailableRuntime ? (
              <Select
                options={runtimeOptions}
                value={selectedRuntime}
                onChange={setSelectedRuntime}
                placeholder={t('onboardingChooseTool')}
              />
            ) : (
              <p className="font-body text-xs text-brutal-danger">
                {t('onboardingNoTool')}
              </p>
            )}
          </div>
        </div>

        {/* Step 3: Create Lucy */}
        <div className="flex items-start gap-3 py-2">
          <div
            className={`mt-0.5 flex h-5 w-5 flex-shrink-0 items-center justify-center border-2 border-black ${
              done ? 'bg-brutal-success' : 'bg-brutal-primary'
            }`}
          >
            {done ? (
              <Check className="h-3 w-3 text-black" />
            ) : isCreating ? (
              <Spinner size="sm" />
            ) : (
              <Sparkles className="h-3 w-3 text-black" />
            )}
          </div>
          <div className="min-w-0">
            <p className="font-heading text-sm font-bold text-foreground">
              {done ? t('onboardingLucyReady') : t('onboardingCreateLucy')}
            </p>
            <p className="font-sans text-xs text-muted-foreground">
              {done
                ? t('onboardingLucyReadyDesc')
                : t('onboardingCreateLucyDesc')}
            </p>

            {!done && (
              <div className="mt-2">
                <Button
                  variant="default"
                  size="sm"
                  disabled={!selectedRuntime || isCreating || !hasAvailableRuntime || !myComputer}
                  onClick={handleCreateLucy}
                  className="gap-1.5"
                >
                  {isCreating ? (
                    <>
                      <Spinner size="sm" />
                      {t('onboardingCreatingLucy')}
                    </>
                  ) : (
                    <>
                      <Sparkles className="h-3.5 w-3.5" />
                      {t('onboardingCreateLucy')}
                    </>
                  )}
                </Button>
              </div>
            )}

            {createError && (
              <div className="mt-2 flex items-center gap-1.5 font-body text-xs text-brutal-danger">
                <AlertCircle className="h-3.5 w-3.5" />
                {createError}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
