// ============================================================================
// AgentForm — create/edit Agent form with brutalist styling
// - input-brutal, textarea-brutal
// - radio-brutal for Provider selection
// - CLI detection display next to provider
// - EnvEditor for custom_env key-value pairs (v1.4)
// - ArgsEditor for custom_args tags (v1.4)
// - ROLE_TEMPLATES for role template selector (SOLO-210-F)
// ============================================================================

'use client';

import { useState, useCallback } from 'react';
import { useForm, Controller } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Bot, Wrench, Terminal } from 'lucide-react';
import { cn } from '@/lib/utils';
import { t } from '@/lib/i18n';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Skeleton } from '@/components/ui/skeleton';
import { Select } from '@/components/ui/select';
import { EnvEditor } from '@/components/agents/env-editor';
import { ArgsEditor } from '@/components/agents/args-editor';
import { useCliDetection } from '@/lib/hooks/use-cli-detection';
import { useBackendMeta } from '@/lib/hooks/use-backend-meta';

// ============================================================================
// Role Templates (SOLO-210-F) — frontend-defined preset system prompts
// ============================================================================

interface RoleTemplate {
  key: string;
  name: string;
  desc: string;
  prompt: string;
}

const ROLE_TEMPLATES: RoleTemplate[] = [
  {
    key: 'leader',
    name: 'Orchestrator',
    desc: 'Monitor progress, assign tasks, approve deliverables',
    prompt:
      'You are the team orchestrator. Your responsibility is to monitor overall progress, break down large tasks into sub-tasks, and assign them to the appropriate agents. You do not write code directly — you focus on task assignment and decision approval. When a task is marked in_review, review the quality of completion and move it to done. If you detect blockers or schedule delays, coordinate and resolve them promptly.',
  },
  {
    key: 'pm',
    name: 'Project Manager',
    desc: 'Requirements analysis, task planning, priority management',
    prompt:
      'You are the product/project manager. Your responsibility is to convert requirements into executable tasks, write clear task descriptions and acceptance criteria. Set priorities (P0-P3) to ensure the team focuses on high-priority items. Track task progress, identify risks, and communicate proactively. You do not write code directly, but you ensure every task has clear deliverable standards.',
  },
  {
    key: 'rd',
    name: 'Backend Developer',
    desc: 'Backend coding, architecture implementation, code review',
    prompt:
      'You are a backend/architecture developer. Your responsibility is to take on backend coding and architecture implementation tasks, updating progress in the task thread. Communicate promptly when encountering technical blockers, mark tasks as in_review when implementation is complete, and briefly describe your approach. You focus on the backend tech stack (Go, PostgreSQL, distributed systems, etc.) and can help review other agents\' code.',
  },
  {
    key: 'fe',
    name: 'Frontend Developer',
    desc: 'Frontend UI, responsive design, interactions',
    prompt:
      'You are a frontend developer. Your responsibility is to take on frontend UI and interaction implementation tasks, focusing on interface consistency, responsive design, and user experience. Use React, Next.js, Tailwind CSS, and related tech stacks. Update progress in the task thread, mark tasks as in_review when implementation is complete, and briefly describe your approach. You can help review frontend code.',
  },
  {
    key: 'qa',
    name: 'QA Engineer',
    desc: 'Write tests, find bugs, quality verification',
    prompt:
      'You are a QA/test engineer. Your responsibility is to take on test writing and verification tasks, writing test cases for critical functional paths. When you discover bugs, create new tasks to document the issues and @mention relevant people. Verify tasks in in_review status, and move them to done once you confirm they meet acceptance criteria. You do not fix code directly, but you ensure delivery quality.',
  },
];

const agentFormSchema = z.object({
  name: z
    .string()
    .min(1, t('agentFormNameRequired'))
    .max(50, t('agentFormNameMaxLen')),
  description: z.string().max(200, t('agentFormDescMaxLen')).optional(),
  model_provider: z.string().min(1, t('agentFormRuntimeRequired')),
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
  isSubmitting: boolean;
  submitLabel: string;
}

export function AgentForm({
  defaultValues,
  onSubmit,
  isSubmitting,
  submitLabel,
}: AgentFormProps) {
  const {
    register,
    handleSubmit,
    control,
    watch,
    setValue,
    formState: { errors },
  } = useForm<AgentFormValues>({
    resolver: zodResolver(agentFormSchema),
    defaultValues: {
      name: '',
      description: '',
      model_provider: '',
      model_name: '',
      system_prompt: '',
      custom_env: {},
      custom_args: [],
      ...defaultValues,
    },
    mode: 'onChange',
  });

  const currentSystemPrompt = watch('system_prompt') || '';

  // v1.4: dynamic CLI detection + backend metadata
  const { results: detection, isLoading: detectionLoading } = useCliDetection();
  const { metas: backendMeta } = useBackendMeta();

  // Role template selection state (SOLO-210-F)
  const [selectedTemplateKey, setSelectedTemplateKey] = useState<string | null>(null);

  // v1.4: separate local state for complex editors, synced to form values
  const [envValues, setEnvValues] = useState<Record<string, string>>(
    defaultValues?.custom_env || {},
  );
  const [argsValues, setArgsValues] = useState<string[]>(
    defaultValues?.custom_args || [],
  );

  const handleTemplateSelect = useCallback(
    (template: RoleTemplate) => {
      if (selectedTemplateKey === template.key) {
        setValue('system_prompt', template.prompt);
        return;
      }

      const isTextareaDirty =
        currentSystemPrompt.trim() !== '' &&
        currentSystemPrompt !==
          ROLE_TEMPLATES.find((t) => t.key === selectedTemplateKey)?.prompt;

      if (isTextareaDirty) {
        const confirmed = window.confirm(
          t('agentFormTemplateWarning'),
        );
        if (!confirmed) return;
      }

      setValue('system_prompt', template.prompt);
      setSelectedTemplateKey(template.key);
    },
    [currentSystemPrompt, selectedTemplateKey, setValue],
  );

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

  return (
    <form onSubmit={handleSubmit(handleFormSubmit)} className="space-y-6">
      {/* Name */}
      <div className="space-y-2">
        <Label htmlFor="name">
          {t('agentFormName')}
        </Label>
        <Input
          id="name"
          placeholder={t('agentFormNamePlaceholder')}
          autoFocus
          {...register('name')}
          aria-invalid={!!errors.name}
        />
        {errors.name && (
          <p className="font-mono text-[11px] text-brutal-danger">
            {errors.name.message}
          </p>
        )}
      </div>

      {/* Description */}
      <div className="space-y-2">
        <Label htmlFor="description">{t('agentFormDesc')}</Label>
        <Input
          id="description"
          placeholder={t('agentFormDescPlaceholder')}
          {...register('description')}
          aria-invalid={!!errors.description}
        />
        {errors.description && (
          <p className="font-mono text-[11px] text-brutal-danger">
            {errors.description.message}
          </p>
        )}
      </div>

      {/* Runtime Selection (v1.4: dynamic, based on CLI detection) */}
      <div className="space-y-3">
        <Label>
          Runtime <span className="text-brutal-danger">*</span>
        </Label>

        {/* Loading state */}
        {detectionLoading && (
          <Skeleton className="h-10 w-full rounded-none" />
        )}

        {/* Runtime dropdown — only show available runtimes */}
        {!detectionLoading && (
          <Controller
            name="model_provider"
            control={control}
            render={({ field }) => (
              <Select
                name={field.name}
                value={field.value ?? ''}
                onChange={field.onChange}
                onBlur={field.onBlur}
                options={Object.values(detection).map((rt) => ({
                  value: rt.type,
                  label: `${rt.available ? '●' : '○'} ${rt.display_name}${rt.version ? ` (v${rt.version})` : ''}`,
                  disabled: !rt.available,
                }))}
                placeholder={t('agentFormSelectRuntime')}
                size="md"
                className="w-full font-body"
              />
            )}
          />
        )}
        {errors.model_provider && (
          <p className="font-mono text-[11px] text-brutal-danger">
            {errors.model_provider.message}
          </p>
        )}

        {/* Unavailable runtimes shown below with install hint */}
        {!detectionLoading &&
          Object.values(detection)
            .filter((rt) => !rt.available)
            .map((rt) => (
              <div
                key={rt.type}
                className="flex items-center gap-1.5 text-xs text-muted-foreground"
              >
                <span className="font-mono text-[11px]">
                  {t('agentFormNotInstalled', { name: rt.display_name })}
                  {rt.error ? ` (${rt.error})` : ` (${rt.binary})`}
                </span>
              </div>
            ))}

      </div>

      {/* Role Template Selector (SOLO-210-F) */}
      <div className="space-y-3">
        <Label>{t('agentFormRoleTemplate')}</Label>
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-5">
          {ROLE_TEMPLATES.map((template) => {
            const isSelected = selectedTemplateKey === template.key;
            return (
              <button
                key={template.key}
                type="button"
                onClick={() => handleTemplateSelect(template)}
                className={cn(
                  'flex flex-col items-start gap-0.5 border-2 border-black px-2.5 py-2 text-left transition-all',
                  isSelected
                    ? 'bg-brutal-primary shadow-brutal-sm translate-x-0.5 translate-y-0.5'
                    : 'bg-white shadow-brutal-sm hover:-translate-x-0.5 hover:-translate-y-0.5 hover:shadow-brutal',
                )}
              >
                <span className="font-heading text-xs font-bold leading-tight">
                  {template.name}
                </span>
                <span className="font-mono text-[9px] leading-tight text-muted-foreground">
                  {template.desc}
                </span>
              </button>
            );
          })}
        </div>
      </div>

      {/* System Prompt */}
      <div className="space-y-2">
        <Label htmlFor="system_prompt">{t('agentFormSystemPrompt')}</Label>
        <Textarea
          id="system_prompt"
          placeholder={t('agentFormSystemPromptPlaceholder')}
          className="min-h-[120px] resize-y"
          aria-label="System Prompt"
          {...register('system_prompt')}
        />
        <p className="font-mono text-[11px] text-muted-foreground">
          {t('agentFormSystemPromptHelp')}
        </p>
      </div>

      {/* v1.4: Custom Environment Variables */}
      <div className="space-y-3">
        <div className="flex items-center gap-2">
          <Terminal className="h-4 w-4" />
          <Label>{t('agentFormEnv')}</Label>
        </div>
        <p className="font-mono text-[11px] text-muted-foreground">
          {t('agentFormEnvHelp')}
        </p>
        <EnvEditor
          value={envValues}
          onChange={handleEnvChange}
          disabled={isSubmitting}
        />
      </div>

      {/* v1.4: Custom CLI Arguments */}
      <div className="space-y-3">
        <div className="flex items-center gap-2">
          <Wrench className="h-4 w-4" />
          <Label>{t('agentFormCustomArgs')}</Label>
        </div>
        <p className="font-mono text-[11px] text-muted-foreground">
          {t('agentFormCustomArgsHelp')}
        </p>
        <ArgsEditor
          value={argsValues}
          onChange={handleArgsChange}
          disabled={isSubmitting}
        />
      </div>

      {/* Submit */}
      <div className="flex items-center gap-3 pt-2">
        <button
          type="submit"
          disabled={isSubmitting}
          className="btn-brutal btn-brutal-primary"
        >
          {isSubmitting ? (
            <>
              <div className="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent" />
              {t('agentFormSubmitting')}
            </>
          ) : (
            <>
              <Bot className="mr-2 h-4 w-4" />
              {submitLabel}
            </>
          )}
        </button>
      </div>
    </form>
  );
}

export { agentFormSchema };
