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
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Bot, Wrench, Terminal } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Skeleton } from '@/components/ui/skeleton';
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
    name: '统筹者',
    desc: '监控进度、分配任务、审批交付',
    prompt:
      '你是团队的统筹者。你的职责是监控整体进度、将大任务拆解为子任务、分派给合适的Agent。你不直接执行编码——你专注于任务分配和决策审批。当任务标记为 in_review 时，审查完成质量后移动到 done。发现阻塞或进度延误时，及时协调解决。',
  },
  {
    key: 'pm',
    name: '项目管理',
    desc: '需求分析、任务规划、优先级管理',
    prompt:
      '你是产品/项目管理者。你的职责是将需求转化为可执行的任务，编写清晰的任务描述和验收标准。设定优先级(P0-P3)确保团队专注高优事项。跟踪任务进度，识别风险并提前沟通。你不直接写代码，但需要确保每个任务都有明确的可交付标准。',
  },
  {
    key: 'rd',
    name: '后端开发',
    desc: '后端编码、架构实现、代码审查',
    prompt:
      '你是后端/架构开发者。你的职责是认领后端编码和架构实现任务，在任务线程中更新工作进度。遇到技术阻塞时及时沟通，完成实现后标记 in_review 并简要说明方案。你专注于后端技术栈（Go、PostgreSQL、分布式系统等），可帮助审查其他agent的代码。',
  },
  {
    key: 'fe',
    name: '前端开发',
    desc: '前端UI实现、响应式设计、交互',
    prompt:
      '你是前端开发者。你的职责是认领前端UI和交互实现任务，关注界面一致性、响应式设计、用户体验。使用 React、Next.js、Tailwind CSS 等技术栈。在任务线程中更新进度，完成实现后标记 in_review 并简要说明方案。可帮助审查前端代码。',
  },
  {
    key: 'qa',
    name: '测试保障',
    desc: '编写测试、发现Bug、质量验证',
    prompt:
      '你是测试/质量保障者。你的职责是认领测试编写和验证任务，为核心功能路径编写测试用例。发现Bug后创建新任务记录问题并@相关人员。验证 in_review 状态的任务，确认功能符合验收标准后可移动到 done。你不直接修代码，但确保交付质量。',
  },
];

const agentFormSchema = z.object({
  name: z
    .string()
    .min(1, '名称不能为空')
    .max(50, '名称不能超过 50 个字符'),
  description: z.string().max(200, '描述不能超过 200 个字符').optional(),
  model_provider: z.string().min(1, '请选择 Runtime'),
  model_name: z.string().min(1, '请选择一个模型'),
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

  const selectedRuntime = watch('model_provider');
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

  // v1.4: runtime change handler — auto-set model_name from backend metadata
  const runtimeReg = register('model_provider');
  const handleRuntimeChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    runtimeReg.onChange(e);
    const type = e.target.value;
    const meta = backendMeta[type];
    const defaultModel = meta?.default_model || meta?.models?.[0]?.id || type;
    setValue('model_name', defaultModel, { shouldValidate: true, shouldDirty: true });
  };

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
          '当前 System Prompt 中有未保存的自定义内容。切换模板将替换现有内容，是否继续？',
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
          名称 <span className="text-brutal-red">*</span>
        </Label>
        <Input
          id="name"
          placeholder="例如：代码审查员"
          autoFocus
          {...register('name')}
          aria-invalid={!!errors.name}
        />
        {errors.name && (
          <p className="font-mono text-[11px] text-brutal-red">
            {errors.name.message}
          </p>
        )}
      </div>

      {/* Description */}
      <div className="space-y-2">
        <Label htmlFor="description">描述</Label>
        <Input
          id="description"
          placeholder="简要描述 Agent 的职责和作用"
          {...register('description')}
          aria-invalid={!!errors.description}
        />
        {errors.description && (
          <p className="font-mono text-[11px] text-brutal-red">
            {errors.description.message}
          </p>
        )}
      </div>

      {/* Runtime Selection (v1.4: dynamic, based on CLI detection) */}
      <div className="space-y-3">
        <Label>
          Runtime <span className="text-brutal-red">*</span>
        </Label>

        {/* Loading state */}
        {detectionLoading && (
          <Skeleton className="h-10 w-full rounded-none" />
        )}

        {/* Runtime dropdown — only show available runtimes */}
        {!detectionLoading && (
          <select
            name={runtimeReg.name}
            ref={runtimeReg.ref}
            onBlur={runtimeReg.onBlur}
            value={selectedRuntime}
            onChange={handleRuntimeChange}
            className="input-brutal h-10 appearance-none bg-white pr-8 font-body text-sm"
            style={{
              backgroundImage:
                "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%23000' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E\")",
              backgroundRepeat: 'no-repeat',
              backgroundPosition: 'right 0.75rem center',
            }}
          >
            <option value="">选择 Runtime...</option>
            {Object.values(detection).map((rt) => (
              <option
                key={rt.type}
                value={rt.type}
                disabled={!rt.available}
              >
                {rt.available ? '●' : '○'} {rt.display_name}
                {rt.version ? ` (v${rt.version})` : ''}
              </option>
            ))}
          </select>
        )}
        {errors.model_provider && (
          <p className="font-mono text-[11px] text-brutal-red">
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
                  {rt.display_name} — 未安装
                  {rt.error ? ` (${rt.error})` : ` (${rt.binary})`}
                </span>
              </div>
            ))}

      </div>

      {/* Role Template Selector (SOLO-210-F) */}
      <div className="space-y-3">
        <Label>角色模板 <span className="font-normal text-muted-foreground">(可选)</span></Label>
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
                    ? 'bg-brutal-pink shadow-brutal-sm translate-x-0.5 translate-y-0.5'
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
        <Label htmlFor="system_prompt">System Prompt</Label>
        <Textarea
          id="system_prompt"
          placeholder="设置 Agent 的行为指令和角色定义..."
          className="min-h-[120px] resize-y"
          aria-label="System Prompt"
          {...register('system_prompt')}
        />
        <p className="font-mono text-[11px] text-muted-foreground">
          定义 Agent 的角色和行为方式。填写后 Agent 将根据此指令回复消息。
        </p>
      </div>

      {/* v1.4: Custom Environment Variables */}
      <div className="space-y-3">
        <div className="flex items-center gap-2">
          <Terminal className="h-4 w-4" />
          <Label>Environment Variables <span className="font-normal text-muted-foreground">(可选)</span></Label>
        </div>
        <p className="font-mono text-[11px] text-muted-foreground">
          为 Agent 运行时注入环境变量，例如 API 密钥、配置参数等。
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
          <Label>Custom Arguments <span className="font-normal text-muted-foreground">(可选)</span></Label>
        </div>
        <p className="font-mono text-[11px] text-muted-foreground">
          传递给 CLI 的额外参数。每个参数作为独立标签添加。
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
          className="btn-brutal btn-brutal-pink"
        >
          {isSubmitting ? (
            <>
              <div className="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent" />
              提交中...
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
