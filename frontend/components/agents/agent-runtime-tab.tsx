// ============================================================================
// AgentRuntimeTab — display Agent runtime configuration (v1.5)
// - Shows: Runtime type, model name
// - Environment variables key-value list
// - Read-only display (editing is handled via Profile tab or future enhancements)
// ============================================================================

'use client';

import { useEffect, useState, useCallback } from 'react';
import { AlertCircle, RefreshCw, Terminal, Layers, Cpu } from 'lucide-react';
import { apiClient, ApiError } from '@/lib/api-client';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { t } from '@/lib/i18n';
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
        is_active: (res.is_active as boolean) ?? false,
        avatar_url: (res.avatar_url as string) || null,
        custom_env: (res.custom_env as Record<string, string>) ?? {},
        custom_args: (res.custom_args as string[]) ?? [],
        created_at: res.created_at as string,
        updated_at: res.updated_at as string,
      } as Agent);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.status === 404 ? t('agentProfileAgentNotFound') : err.message);
      } else {
        setError(t('agentRuntimeError'));
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
        <div className="mb-3 flex h-12 w-12 items-center justify-center border-2 border-black bg-brutal-danger-light shadow-brutal-sm">
          <AlertCircle className="h-6 w-6 text-brutal-danger" />
        </div>
        <p className="font-body text-sm text-brutal-danger">{error}</p>
        <Button type="button" onClick={loadAgent} size="sm" className="mt-4">
          <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
          {t('retry')}
        </Button>
      </div>
    );
  }

  if (!agent) return null;

  const envKeys = Object.keys(agent.custom_env ?? {});

  return (
    <div className="space-y-2">
      <h4>
        <span
          className="inline-flex items-center gap-1.5 border-2 border-black bg-brutal-primary px-2.5 py-1 font-heading text-[11px] font-black uppercase tracking-widest text-black shadow-brutal-sm"
          style={{ transform: 'rotate(-0.8deg)' }}
        >
          ★ {t('agentRuntimeConfig')}
        </span>
      </h4>
      <div className="space-y-1">
        {/* Runtime type */}
        <div className="flex items-center gap-3 py-1.5">
          <span className="inline-block bg-brutal-primary-light border-2 border-black px-1.5 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider text-black flex-shrink-0">
            <Cpu className="inline h-3 w-3 mr-1 -mt-0.5" />
            {t('agentRuntimeType')}
          </span>
          <span className="font-mono text-xs text-foreground">
            {agent.model_provider || t('agentRuntimeNotConfigured')}
          </span>
        </div>

        {/* Model configuration */}
        <div className="flex items-center gap-3 py-1.5">
          <span className="inline-block bg-brutal-primary-light border-2 border-black px-1.5 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider text-black flex-shrink-0">
            <Layers className="inline h-3 w-3 mr-1 -mt-0.5" />
            Model
          </span>
          <span className="font-mono text-xs text-foreground">
            {agent.model_name || t('agentRuntimeDefault')}
          </span>
        </div>

        {/* Environment variables — v3.3: label sits on its own row, and
            the K=V pairs render vertically beneath it so multi-env setups
            don't force the label to wrap or stretch horizontally. */}
        <div className="py-1.5">
          <div className="flex items-center gap-3">
            <span className="inline-block bg-brutal-primary-light border-2 border-black px-1.5 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider text-black flex-shrink-0">
              <Terminal className="inline h-3 w-3 mr-1 -mt-0.5" />
              {t('agentRuntimeEnvVars')}
              <span className="ml-1 inline-block border-2 border-black bg-white px-1 font-mono text-[9px] text-black">
                {envKeys.length}
              </span>
            </span>
          </div>
          {envKeys.length > 0 ? (
            <div className="mt-2 flex flex-col items-start gap-1.5 pl-1">
              {envKeys.map((key) => (
                <div
                  key={key}
                  className="inline-flex max-w-full items-center gap-2 border border-black bg-white px-2 py-0.5 font-mono text-[11px]"
                >
                  <span className="font-bold text-foreground whitespace-nowrap">{key}</span>
                  <span className="text-muted-foreground">=</span>
                  <span className="text-foreground truncate">
                    {agent.custom_env?.[key] || ''}
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <p className="mt-2 pl-1 font-mono text-xs italic text-muted-foreground">
              {t('agentRuntimeNoEnvVars')}
            </p>
          )}
        </div>
      </div>
    </div>
  );
}
