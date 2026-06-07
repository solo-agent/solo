// ============================================================================
// AgentRuntimeTab — display Agent runtime configuration (v1.5)
// - Shows: Runtime type, model name, temperature, max_tokens
// - Environment variables key-value list
// - Auto-join channels setting
// - Read-only display (editing is handled via Profile tab or future enhancements)
// ============================================================================

'use client';

import { useEffect, useState, useCallback } from 'react';
import { AlertCircle, RefreshCw, Terminal, Layers, Cpu } from 'lucide-react';
import { apiClient, ApiError } from '@/lib/api-client';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import type { Agent } from '@/lib/types';

interface AgentRuntimeTabProps {
  agentId: string;
}

export function AgentRuntimeTab({ agentId }: AgentRuntimeTabProps) {
  const [agent, setAgent] = useState<Agent | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadAgent = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<Record<string, unknown>>(`/api/v1/agents/${agentId}`);
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
        enabled_tools: (res.enabled_tools as string[]) ?? [],
        interaction_mode: (res.interaction_mode as string) ?? 'mention',
        custom_env: (res.custom_env as Record<string, string>) ?? {},
        custom_args: (res.custom_args as string[]) ?? [],
        created_at: res.created_at as string,
        updated_at: res.updated_at as string,
      } as Agent);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.status === 404 ? 'Agent 不存在' : err.message);
      } else {
        setError('加载 Runtime 配置失败');
      }
    } finally {
      setIsLoading(false);
    }
  }, [agentId]);

  useEffect(() => {
    loadAgent();
  }, [loadAgent]);

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-10 w-full rounded-none" />
        <Skeleton className="h-10 w-full rounded-none" />
        <Skeleton className="h-10 w-full rounded-none" />
        <Skeleton className="h-16 w-full rounded-none" />
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

  if (!agent) return null;

  const envKeys = Object.keys(agent.custom_env ?? {});

  return (
    <div className="space-y-5">
      {/* Runtime type */}
      <div className="space-y-2">
        <div className="flex items-center gap-2">
          <Cpu className="h-4 w-4 flex-shrink-0" />
          <h3 className="font-heading text-xs font-bold text-muted-foreground uppercase tracking-wider">
            Runtime 类型
          </h3>
        </div>
        <div className="card-brutal bg-brutal-cream p-3">
          <p className="font-mono text-sm text-foreground">
            {agent.model_provider || '未配置'}
          </p>
        </div>
      </div>

      {/* Model configuration */}
      <div className="space-y-2">
        <div className="flex items-center gap-2">
          <Layers className="h-4 w-4 flex-shrink-0" />
          <h3 className="font-heading text-xs font-bold text-muted-foreground uppercase tracking-wider">
            模型配置
          </h3>
        </div>
        <div className="card-brutal bg-brutal-cream divide-y-2 divide-black">
          <div className="flex items-center justify-between px-3 py-2">
            <span className="font-mono text-[11px] text-muted-foreground">Model</span>
            <span className="font-mono text-sm text-foreground">
              {agent.model_name || '默认'}
            </span>
          </div>
          <div className="flex items-center justify-between px-3 py-2">
            <span className="font-mono text-[11px] text-muted-foreground">Temperature</span>
            <span className="font-mono text-sm text-foreground">
              {agent.temperature.toFixed(1)}
            </span>
          </div>
          <div className="flex items-center justify-between px-3 py-2">
            <span className="font-mono text-[11px] text-muted-foreground">Max Tokens</span>
            <span className="font-mono text-sm text-foreground">
              {agent.max_tokens.toLocaleString()}
            </span>
          </div>
        </div>
      </div>

      {/* Environment variables */}
      <div className="space-y-2">
        <div className="flex items-center gap-2">
          <Terminal className="h-4 w-4 flex-shrink-0" />
          <h3 className="font-heading text-xs font-bold text-muted-foreground uppercase tracking-wider">
            环境变量
          </h3>
          <span className="badge-brutal text-[10px]">
            {envKeys.length}
          </span>
        </div>
        {envKeys.length > 0 ? (
          <div className="card-brutal bg-brutal-cream divide-y-2 divide-black">
            {envKeys.map((key) => (
              <div key={key} className="flex items-center justify-between px-3 py-2">
                <span className="font-mono text-xs font-bold text-foreground">{key}</span>
                <span className="font-mono text-[11px] text-muted-foreground max-w-[160px] truncate">
                  {agent.custom_env?.[key] || ''}
                </span>
              </div>
            ))}
          </div>
        ) : (
          <div className="card-brutal bg-brutal-cream p-3 text-center">
            <p className="font-mono text-xs italic text-muted-foreground">
              未配置环境变量
            </p>
          </div>
        )}
      </div>

      {/* Auto-join setting */}
      <div className="space-y-2">
        <h3 className="font-heading text-xs font-bold text-muted-foreground uppercase tracking-wider">
          自动加入频道
        </h3>
        <div className={agent.auto_join
          ? 'border-2 border-black bg-brutal-lime-light px-3 py-2'
          : 'border-2 border-black bg-brutal-cream px-3 py-2'
        }>
          <span className={`badge-brutal text-[10px] ${agent.auto_join ? 'bg-brutal-lime text-black' : 'bg-brutal-stone text-black'}`}>
            {agent.auto_join ? '已启用' : '已禁用'}
          </span>
          <p className="mt-1 font-mono text-[11px] text-muted-foreground">
            {agent.auto_join
              ? 'Agent 创建时会自动加入所有频道'
              : 'Agent 需要手动添加到频道'}
          </p>
        </div>
      </div>
    </div>
  );
}
