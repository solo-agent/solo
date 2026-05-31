// ============================================================================
// SOLO-29-F: Edit Agent page — brutalist form layout with loading/error states
// ============================================================================

'use client';

import { useEffect, useState, useCallback } from 'react';
import { useRouter, useParams } from 'next/navigation';
import { ArrowLeft, AlertCircle } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { useAgents } from '@/lib/hooks/use-agents';
import { AgentForm, type AgentFormValues } from '@/components/agents/agent-form';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';

export default function EditAgentPage() {
  const router = useRouter();
  const params = useParams();
  const agentId = params.id as string;
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const { getAgent, updateAgent } = useAgents();
  const [agent, setAgent] = useState<AgentFormValues | null>(null);
  const [isLoadingAgent, setIsLoadingAgent] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  // Auth guard
  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.push('/auth/login');
    }
  }, [authLoading, isAuthenticated, router]);

  // Load agent data
  useEffect(() => {
    if (!agentId || !isAuthenticated) return;
    setIsLoadingAgent(true);
    setError(null);

    getAgent(agentId)
      .then((agentData) => {
        if (agentData) {
          setAgent({
            name: agentData.name,
            description: agentData.description || '',
            model_provider: agentData.model_provider,
            model_name: agentData.model_name,
            system_prompt: agentData.system_prompt || '',
          });
        } else {
          setError('Agent 不存在或已被删除');
        }
      })
      .catch(() => {
        setError('加载 Agent 信息失败');
      })
      .finally(() => {
        setIsLoadingAgent(false);
      });
  }, [agentId, isAuthenticated, getAgent]);

  const handleSubmit = useCallback(
    async (values: AgentFormValues) => {
      setIsSubmitting(true);
      try {
        await updateAgent(agentId, {
          name: values.name,
          description: values.description || undefined,
          model_provider: values.model_provider,
          model_name: values.model_name,
          system_prompt: values.system_prompt || undefined,
        });
        router.push('/agents');
      } catch {
        // Error handling will be added with real API
      } finally {
        setIsSubmitting(false);
      }
    },
    [agentId, updateAgent, router],
  );

  if (authLoading || !isAuthenticated) {
    return (
      <div className="flex h-screen items-center justify-center bg-brutal-cream">
        <div className="flex flex-col items-center gap-3">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-brutal-pink border-t-transparent" />
          <p className="font-mono text-sm text-muted-foreground">加载中...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-2xl px-6 py-8">
      {/* Back link */}
      <Button
        variant="ghost"
        className="mb-6 -ml-2 text-muted-foreground"
        onClick={() => router.push('/agents')}
      >
        <ArrowLeft className="mr-2 h-4 w-4" />
        返回 Agent 列表
      </Button>

      <div className="mb-8">
        <h1 className="text-2xl font-heading font-bold text-foreground">
          编辑 Agent
        </h1>
        <p className="mt-1 font-body text-sm text-muted-foreground">
          修改 Agent 配置
        </p>
      </div>

      <div className="card-brutal p-6">
        {/* Loading state */}
        {isLoadingAgent && (
          <div className="space-y-6">
            <div className="space-y-2">
              <Skeleton className="h-4 w-12 rounded-none" />
              <Skeleton className="h-10 w-full rounded-none" />
            </div>
            <div className="space-y-2">
              <Skeleton className="h-4 w-12 rounded-none" />
              <Skeleton className="h-10 w-full rounded-none" />
            </div>
            <div className="space-y-2">
              <Skeleton className="h-4 w-16 rounded-none" />
              <Skeleton className="h-10 w-full rounded-none" />
            </div>
            <div className="space-y-2">
              <Skeleton className="h-4 w-24 rounded-none" />
              <Skeleton className="h-[120px] w-full rounded-none" />
            </div>
          </div>
        )}

        {/* Error state */}
        {!isLoadingAgent && error && (
          <div className="flex flex-col items-center gap-3 py-12 text-center">
            <AlertCircle className="h-8 w-8 text-brutal-red" />
            <p className="font-body text-sm text-brutal-red">{error}</p>
            <button
              type="button"
              onClick={() => router.push('/agents')}
              className="btn-brutal btn-brutal-sm"
            >
              返回列表
            </button>
          </div>
        )}

        {/* Form */}
        {!isLoadingAgent && !error && agent && (
          <AgentForm
            defaultValues={agent}
            onSubmit={handleSubmit}
            isSubmitting={isSubmitting}
            submitLabel="保存修改"
          />
        )}
      </div>
    </div>
  );
}
