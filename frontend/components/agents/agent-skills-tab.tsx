// ============================================================================
// AgentSkillsTab — list global/workspace skills with toggle switches (v1.5)
// - Shows name, description, enabled/disabled toggle
// - No create/edit/delete — toggles only
// ============================================================================

'use client';

import { useEffect, useState, useCallback } from 'react';
import { AlertCircle, RefreshCw, Puzzle } from 'lucide-react';
import { apiClient, ApiError } from '@/lib/api-client';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { AVAILABLE_TOOLS } from '@/lib/types';
import type { Agent } from '@/lib/types';

interface AgentSkillsTabProps {
  agentId: string;
}

export function AgentSkillsTab({ agentId }: AgentSkillsTabProps) {
  const [agent, setAgent] = useState<Agent | null>(null);
  const [enabledTools, setEnabledTools] = useState<string[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [savingIds, setSavingIds] = useState<Set<string>>(new Set());

  const loadAgent = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<Record<string, unknown>>(`/api/v1/agents/${agentId}`);
      const tools = (res.enabled_tools as string[]) ?? [];
      setAgent({
        id: res.id as string,
        name: res.name as string,
        description: (res.description as string) || '',
        owner_id: res.owner_id as string,
        model_provider: (res.model_provider as string) || '',
        model_name: (res.model_name as string) || '',
        system_prompt: (res.system_prompt as string) || '',
        temperature: (res.temperature as number) ?? 0.7,
        max_tokens: (res.max_tokens as number) ?? 4096,
        is_active: (res.is_active as boolean) ?? false,
        auto_join: (res.auto_join as boolean) ?? false,
        avatar_url: (res.avatar_url as string) || null,
        enabled_tools: tools,
        interaction_mode: (res.interaction_mode as string) ?? 'mention',
        custom_env: (res.custom_env as Record<string, string>) ?? {},
        custom_args: (res.custom_args as string[]) ?? [],
        created_at: res.created_at as string,
        updated_at: res.updated_at as string,
      } as Agent);
      setEnabledTools(tools);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.status === 404 ? 'Agent 不存在' : err.message);
      } else {
        setError('加载 Skills 配置失败');
      }
    } finally {
      setIsLoading(false);
    }
  }, [agentId]);

  useEffect(() => {
    loadAgent();
  }, [loadAgent]);

  const toggleSkill = useCallback(
    async (skillId: string) => {
      const isCurrentlyEnabled = enabledTools.includes(skillId);
      const next = isCurrentlyEnabled
        ? enabledTools.filter((id) => id !== skillId)
        : [...enabledTools, skillId];

      // Optimistic update
      setEnabledTools(next);
      setSavingIds((prev) => new Set(prev).add(skillId));

      try {
        await apiClient.patch(`/api/v1/agents/${agentId}`, { enabled_tools: next });
      } catch {
        // Revert on failure
        setEnabledTools((prev) =>
          isCurrentlyEnabled
            ? [...prev, skillId]
            : prev.filter((id) => id !== skillId),
        );
      } finally {
        setSavingIds((prev) => {
          const next = new Set(prev);
          next.delete(skillId);
          return next;
        });
      }
    },
    [agentId, enabledTools],
  );

  if (isLoading) {
    return (
      <div className="space-y-3">
        {[1, 2, 3, 4].map((i) => (
          <div key={i} className="flex items-center gap-3 border-2 border-black bg-white p-4 shadow-brutal-sm">
            <Skeleton className="h-7 w-11 rounded-none flex-shrink-0" />
            <div className="flex-1 space-y-1.5">
              <Skeleton className="h-4 w-24 rounded-none" />
              <Skeleton className="h-3 w-40 rounded-none" />
            </div>
          </div>
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <div className="mb-3 flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-red-light shadow-brutal-sm">
          <AlertCircle className="h-6 w-6 text-brutal-red" />
        </div>
        <p className="font-body text-sm text-brutal-red">{error}</p>
        <Button type="button" onClick={loadAgent} size="sm" className="mt-4">
          <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
          重试
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Section header */}
      <div>
        <div className="flex items-center gap-2">
          <Puzzle className="h-4 w-4" />
          <h3 className="font-heading text-xs font-bold text-muted-foreground uppercase tracking-wider">
            Global Skills
          </h3>
        </div>
        <p className="mt-1 font-mono text-[11px] text-muted-foreground">
          切换开关来启用/禁用 Agent 拥有的工具。修改即时生效。
        </p>
      </div>

      {/* Skill list */}
      <div className="divide-y-2 divide-black border-2 border-black shadow-brutal-sm">
        {AVAILABLE_TOOLS.map((tool) => {
          const isEnabled = enabledTools.includes(tool.id);
          const isSaving = savingIds.has(tool.id);

          return (
            <div
              key={tool.id}
              className="flex items-center gap-3 bg-white px-4 py-3"
            >
              {/* Toggle switch */}
              <button
                type="button"
                onClick={() => toggleSkill(tool.id)}
                disabled={isSaving}
                className={cn(
                  'relative flex h-7 w-11 flex-shrink-0 items-center border-2 border-black transition-colors',
                  isSaving ? 'opacity-50 cursor-wait' : '',
                  isEnabled ? 'bg-brutal-lime' : 'bg-brutal-stone',
                )}
                role="switch"
                aria-checked={isEnabled}
                aria-label={`${isEnabled ? '禁用' : '启用'} ${tool.name}`}
              >
                <span
                  className={cn(
                    'absolute h-7 w-[18px] border-r-2 border-l-2 border-black bg-white transition-all',
                    isEnabled ? 'left-[calc(100%-18px)]' : 'left-0',
                  )}
                />
              </button>

              {/* Skill info */}
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="font-heading text-sm font-bold text-foreground">
                    {tool.name}
                  </span>
                  <span
                    className={cn(
                      'badge-brutal text-[10px] px-1.5',
                      isSaving
                        ? 'bg-brutal-stone text-white'
                        : isEnabled
                          ? 'bg-brutal-lime text-black'
                          : 'bg-brutal-stone text-white',
                    )}
                  >
                    {isSaving ? '保存中...' : isEnabled ? '已启用' : '未启用'}
                  </span>
                </div>
                <p className="mt-0.5 font-mono text-[11px] text-muted-foreground leading-relaxed">
                  {tool.description}
                </p>
              </div>
            </div>
          );
        })}
      </div>

      {/* Workspace Skills section (placeholder for future) */}
      <div className="mt-6">
        <div className="flex items-center gap-2">
          <Puzzle className="h-4 w-4" />
          <h3 className="font-heading text-xs font-bold text-muted-foreground uppercase tracking-wider">
            Workspace Skills
          </h3>
        </div>
        <div className="mt-2 card-brutal bg-brutal-cream p-4 text-center">
          <p className="font-mono text-xs italic text-muted-foreground">
            Workspace skills 将在后续版本中支持
          </p>
        </div>
      </div>
    </div>
  );
}
