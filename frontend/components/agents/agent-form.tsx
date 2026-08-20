// ============================================================================
// AgentForm — shared daily Agent creation form.
// ============================================================================

'use client';

import { useState, useCallback, useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { ChevronDown, Monitor, Wrench, Terminal } from 'lucide-react';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Button } from '@/components/ui/button';
import { DialogFooter } from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { Select } from '@/components/ui/select';
import { EnvEditor } from '@/components/agents/env-editor';
import { ArgsEditor } from '@/components/agents/args-editor';
import { useCliDetection } from '@/lib/hooks/use-cli-detection';
import { MODEL_PRESETS } from '@/lib/agent-models';
import { useComputers } from '@/lib/hooks/use-computers';
import { RuntimeLogo } from '@/components/agents/runtime-logo';

const agentFormSchema = z.object({
  name: z
    .string()
    .min(1, t('agentFormNameRequired'))
    .max(50, t('agentFormNameMaxLen')),
  description: z.string().max(200, t('agentFormDescMaxLen')).optional(),
  model_provider: z.string().min(1, t('agentFormRuntimeRequired')),
  computer_id: z.string().min(1, t('agentFormComputerRequired')),
  model_name: z.string().optional(),
  system_prompt: z.string().optional(),
  // v1.4: custom_env and custom_args are managed via controlled components,
  // not validated by zod (they use their own UI validation)
  custom_env: z.record(z.string(), z.string()).optional(),
  custom_args: z.array(z.string()).optional(),
});

export type AgentFormValues = z.infer<typeof agentFormSchema>;

interface AgentFormProps {
  defaultValues?: Partial<AgentFormValues>;
  onSubmit: (values: AgentFormValues) => Promise<void>;
  onCancel?: () => void;
  isSubmitting: boolean;
  submitLabel: string;
}

export function AgentForm({
  defaultValues,
  onSubmit,
  onCancel,
  isSubmitting,
  submitLabel,
}: AgentFormProps) {
  const {
    register,
    handleSubmit,
    watch,
    setValue,
    formState: { errors },
  } = useForm<AgentFormValues>({
    resolver: zodResolver(agentFormSchema),
    defaultValues: {
      name: '',
      description: '',
      model_provider: '',
      computer_id: '',
      model_name: '',
      system_prompt: '',
      custom_env: {},
      custom_args: [],
      ...defaultValues,
    },
    mode: 'onChange',
  });

  const selectedProvider = watch('model_provider') || '';
  const selectedComputerId = watch('computer_id') || '';
  const selectedModel = watch('model_name') || '';

  const { computers, isLoading: computersLoading } = useComputers();
  const selectableComputers = computers.filter(
    (computer) => computer.pairing_status === 'paired' || (computer.status === 'online' && !!computer.daemon_id),
  );
  const selectedComputer = computers.find((computer) => computer.id === selectedComputerId);

  // v1.4: dynamic CLI detection + backend metadata
  const {
    results: detection,
    isLoading: detectionLoading,
    error: detectionError,
  } = useCliDetection(selectedComputerId || undefined, selectedComputer?.runtime_inventory ?? []);

  const [customModel, setCustomModel] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(false);

  // v1.4: separate local state for complex editors, synced to form values
  const [envValues, setEnvValues] = useState<Record<string, string>>(
    defaultValues?.custom_env || {},
  );
  const [argsValues, setArgsValues] = useState<string[]>(
    defaultValues?.custom_args || [],
  );

  useEffect(() => {
    if (!selectedComputerId && selectableComputers.length === 1) {
      setValue('computer_id', selectableComputers[0].id, { shouldValidate: true });
    }
  }, [selectableComputers, selectedComputerId, setValue]);

  const handleEnvChange = useCallback(
    (env: Record<string, string>) => {
      setEnvValues(env);
      setValue('custom_env', env);
    },
    [setValue],
  );

  const handleArgsChange = useCallback(
    (args: string[]) => {
      setArgsValues(args);
      setValue('custom_args', args);
    },
    [setValue],
  );

  const handleFormSubmit = useCallback(
    async (values: AgentFormValues) => {
      await onSubmit({
        ...values,
        custom_env: envValues,
        custom_args: argsValues,
      });
    },
    [onSubmit, envValues, argsValues],
  );

  const runtimes = Object.values(detection);
  const models = MODEL_PRESETS[selectedProvider] ?? [];
  const supportsCustomModel = ['opencode', 'hermes', 'openclaw'].includes(selectedProvider);

  return (
    <form onSubmit={handleSubmit(handleFormSubmit)} className="space-y-5">
      <div className="-mx-4 grid gap-3 border-y border-border bg-brutal-cream/40 px-4 py-4 sm:-mx-6 sm:px-6">
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="name">{t('agentFormName')}</Label>
            <Input id="name" placeholder={t('agentFormNamePlaceholder')} autoFocus {...register('name')} aria-invalid={!!errors.name} />
            {errors.name && <p className="font-body text-[11px] text-brutal-danger">{errors.name.message}</p>}
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="description">{t('agentFormDesc')}</Label>
            <Input id="description" placeholder={t('agentFormDescPlaceholder')} {...register('description')} aria-invalid={!!errors.description} />
            {errors.description && <p className="font-body text-[11px] text-brutal-danger">{errors.description.message}</p>}
          </div>
        </div>
      </div>

      <fieldset className="space-y-2">
        <legend className="mb-2 font-body text-sm font-medium text-muted-foreground">{t('agentFormComputer')}</legend>
        {computersLoading && <Skeleton className="h-16 w-full rounded-lg" />}
        {!computersLoading && selectableComputers.map((computer) => (
          <label key={computer.id} className={cn('flex min-h-16 cursor-pointer items-center gap-3 rounded-lg border bg-white px-3 py-2.5 transition-colors hover:bg-brutal-cream/40', selectedComputerId === computer.id ? 'border-muted-foreground bg-muted ring-1 ring-muted-foreground' : 'border-border')}>
            <input
              type="radio"
              name="computer_id"
              value={computer.id}
              checked={selectedComputerId === computer.id}
              onChange={() => {
                if (selectedComputerId !== computer.id) {
                  setValue('model_provider', '');
                  setValue('model_name', '');
                  setCustomModel(false);
                }
                setValue('computer_id', computer.id, { shouldValidate: true });
              }}
              className="h-4 w-4"
              style={{ accentColor: 'var(--color-muted-foreground)' }}
            />
            <span className="flex h-9 w-9 items-center justify-center rounded-md bg-brutal-cream"><Monitor className="h-4 w-4" /></span>
            <span className="min-w-0 flex-1">
              <span className="block truncate font-body text-sm font-semibold">{computer.name}</span>
              <span className="mt-0.5 flex items-center gap-1.5 font-body text-xs text-muted-foreground">
                <span className={cn('h-2 w-2 rounded-full', computer.status === 'online' ? 'bg-brutal-success' : 'bg-brutal-muted')} />
                {computer.status === 'online' ? t('online') : t('offline')}
              </span>
            </span>
          </label>
        ))}
        {errors.computer_id && <p className="font-body text-[11px] text-brutal-danger">{errors.computer_id.message}</p>}
        {!computersLoading && selectableComputers.length === 0 && <p className="font-body text-sm text-brutal-danger">{t('agentFormNoPairedComputer')}</p>}
      </fieldset>

      <fieldset className="space-y-2">
        <legend className="mb-2 font-body text-sm font-medium text-muted-foreground">{t('agentFormRuntimeLabel')}</legend>
        {selectedComputerId && detectionLoading && <Skeleton className="h-14 w-full rounded-lg" />}
        {selectedComputerId && detectionError && <p className="font-body text-sm text-brutal-danger">{t('cliCheckFailed')}</p>}
        {selectedComputerId && !detectionLoading && !detectionError && runtimes.map((item) => (
          <label key={item.type} className={cn('flex min-h-14 items-center gap-3 rounded-lg border bg-white px-3 py-2 transition-colors', item.available ? 'cursor-pointer hover:bg-brutal-cream/40' : 'cursor-not-allowed opacity-40', selectedProvider === item.type ? 'border-muted-foreground bg-muted ring-1 ring-muted-foreground' : 'border-border')}>
            <input
              type="radio"
              name="model_provider"
              value={item.type}
              checked={selectedProvider === item.type}
              disabled={!item.available}
              onChange={() => {
                setValue('model_provider', item.type, { shouldValidate: true });
                setValue('model_name', '');
                setCustomModel(false);
              }}
              className="h-4 w-4"
              style={{ accentColor: 'var(--color-muted-foreground)' }}
            />
            <span className="flex h-7 w-7 items-center justify-center"><RuntimeLogo runtime={item.type} className="h-5 w-5" /></span>
            <span className="min-w-0 flex-1 truncate font-body text-sm font-semibold">{item.display_name || item.type}</span>
            {!item.available && <span className="font-body text-xs text-muted-foreground">{t('computersRuntimeUnavailable')}</span>}
          </label>
        ))}
        {errors.model_provider && <p className="font-body text-[11px] text-brutal-danger">{errors.model_provider.message}</p>}
      </fieldset>

      {!detectionLoading && selectedProvider && (
        <div>
          <Label htmlFor="model_name" className="font-body text-sm font-medium text-muted-foreground">{t('agentFormModel')}</Label>
          <Select
            id="model_name"
            value={customModel ? '__custom__' : selectedModel}
            onChange={(value) => {
              const nextCustom = value === '__custom__';
              setCustomModel(nextCustom);
              setValue('model_name', nextCustom ? '' : value);
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
              value={selectedModel}
              onChange={(event) => setValue('model_name', event.target.value)}
              placeholder={t('firstRunCustomModelPlaceholder')}
              maxLength={100}
              className="mt-2 h-10 w-full rounded-lg border border-border bg-white px-3 font-mono text-sm outline-none focus:border-muted-foreground focus:ring-2 focus:ring-muted"
            />
          )}
        </div>
      )}

      <div className="space-y-2">
        <Label htmlFor="system_prompt">{t('agentFormSystemPrompt')}</Label>
        <Textarea id="system_prompt" placeholder={t('agentFormSystemPromptPlaceholder')} className="min-h-28 resize-y" aria-label={t('agentFormSystemPrompt')} {...register('system_prompt')} />
      </div>

      <div className="border-t border-border pt-4">
        <button
          type="button"
          onClick={() => setShowAdvanced((current) => !current)}
          className="flex w-full items-center gap-3 text-left"
          aria-expanded={showAdvanced}
        >
          <span className="flex h-8 w-8 items-center justify-center rounded-md bg-brutal-cream">
            <Wrench className="h-3.5 w-3.5" />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block font-heading text-sm font-bold">{t('agentFormAdvancedSettings')}</span>
            <span className="block font-body text-xs text-muted-foreground">{t('agentFormAdvancedSettingsDesc')}</span>
          </span>
          <ChevronDown className={cn('h-4 w-4 transition-transform', showAdvanced && 'rotate-180')} />
        </button>

        {showAdvanced && (
          <div className="mt-5 space-y-6 rounded-lg border border-border bg-brutal-cream/40 p-4">
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <Terminal className="h-4 w-4" />
                <Label>{t('agentFormEnv')}</Label>
              </div>
              <EnvEditor
                value={envValues}
                onChange={handleEnvChange}
                disabled={isSubmitting}
              />
            </div>
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <Wrench className="h-4 w-4" />
                <Label>{t('agentFormCustomArgs')}</Label>
              </div>
              <ArgsEditor
                value={argsValues}
                onChange={handleArgsChange}
                disabled={isSubmitting}
              />
            </div>
          </div>
        )}
      </div>

      <DialogFooter className="sticky -bottom-4 z-10 -mx-4 -mb-4 border-t border-border bg-card px-4 py-4 sm:-bottom-6 sm:-mx-6 sm:-mb-6 sm:px-6">
        {onCancel && <Button type="button" variant="outline" onClick={onCancel} disabled={isSubmitting}>{t('cancel')}</Button>}
        <Button
          type="submit"
          variant="primary"
          disabled={isSubmitting || (customModel && !selectedModel.trim())}
        >
          {isSubmitting ? (
            <>
              <div className="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent" />
              {t('agentFormSubmitting')}
            </>
          ) : (
            <>
              {submitLabel}
            </>
          )}
        </Button>
      </DialogFooter>
    </form>
  );
}

export { agentFormSchema };
