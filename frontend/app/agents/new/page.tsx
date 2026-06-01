// ============================================================================
// SOLO-29-F: Create Agent page — brutalist form layout
// ============================================================================

'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { ArrowLeft } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { useAgents } from '@/lib/hooks/use-agents';
import { AgentForm, type AgentFormValues } from '@/components/agents/agent-form';
import { Button } from '@/components/ui/button';

export default function CreateAgentPage() {
  const router = useRouter();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const { createAgent } = useAgents();
  const [isSubmitting, setIsSubmitting] = useState(false);

  // Auth guard
  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.push('/auth/login');
    }
  }, [authLoading, isAuthenticated, router]);

  const handleSubmit = async (values: AgentFormValues) => {
    setIsSubmitting(true);
    try {
      await createAgent({
        name: values.name,
        description: values.description || undefined,
        model_provider: values.model_provider,
        model_name: values.model_name,
        system_prompt: values.system_prompt || undefined,
        custom_env: values.custom_env,
        custom_args: values.custom_args,
      });
      router.push('/agents');
    } catch (err) {
      console.error('创建 Agent 失败:', err);
    } finally {
      setIsSubmitting(false);
    }
  };

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
          创建 Agent
        </h1>
        <p className="mt-1 font-body text-sm text-muted-foreground">
          配置一个新的 AI Agent，设置其角色和行为方式
        </p>
      </div>

      <div className="card-brutal p-6">
        <AgentForm
          onSubmit={handleSubmit}
          isSubmitting={isSubmitting}
          submitLabel="创建 Agent"
        />
      </div>
    </div>
  );
}
