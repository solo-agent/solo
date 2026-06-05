// ============================================================================
// SOLO-106-F: Agent Detail Page — left info panel + right tabbed panel
// - Route: /agents/[id]
// - Left: Bot icon + name + status + description + metadata
// - Right: Tab switching (Runtime / Tools / History)
// - Full neubrutalism, card-brutal, border-brutal, zero-rounding
// ============================================================================

'use client';

import { useEffect, useState, useCallback } from 'react';
import { useParams, useRouter } from 'next/navigation';
import {
  Bot,
  ArrowLeft,
  Settings,
  History,
  Puzzle,
  Calendar,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { useAuth } from '@/lib/auth-context';
import { useAgents } from '@/lib/hooks/use-agents';
import { apiClient, ApiError } from '@/lib/api-client';
import { StatusIndicator } from '@/components/agents/status-indicator';
import { RuntimeTab } from '@/components/agents/runtime-tab';
import { HistoryTab } from '@/components/agents/history-tab';
import { ToolsTab } from '@/components/agents/tools-tab';
import { InteractionMode } from '@/components/agents/interaction-mode';
import type { Agent, AgentModelProvider, AgentInteractionMode } from '@/lib/types';

// ---- Types ----

type TabKey = 'runtime' | 'tools' | 'history';

interface TabDef {
  key: TabKey;
  label: string;
  icon: typeof Settings;
}

const TABS: TabDef[] = [
  { key: 'runtime', label: '运行时', icon: Settings },
  { key: 'tools', label: '工具', icon: Puzzle },
  { key: 'history', label: '历史', icon: History },
];

// ---- Helpers ----

function formatDate(iso: string): string {
  try {
    const d = new Date(iso);
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  } catch {
    return iso;
  }
}

function mapAgentResponse(resp: Record<string, unknown>): Agent {
  return {
    id: resp.id as string,
    name: resp.name as string,
    description: (resp.description as string) || '',
    owner_id: resp.owner_id as string,
    model_provider: resp.model_provider as AgentModelProvider,
    model_name: resp.model_name as string,
    system_prompt: (resp.system_prompt as string) || '',
    temperature: (resp.temperature as number) ?? 0.7,
    max_tokens: (resp.max_tokens as number) ?? 4096,
    is_active: resp.is_active as boolean,
    auto_join: resp.auto_join as boolean,
    avatar_url: (resp.avatar_url as string) || null,
    enabled_tools: (resp.enabled_tools as string[]) ?? [],
    interaction_mode: (resp.interaction_mode as AgentInteractionMode) ?? 'mention',
    custom_env: (resp.custom_env as Record<string, string>) ?? {},
    custom_args: (resp.custom_args as string[]) ?? [],
    created_at: resp.created_at as string,
    updated_at: resp.updated_at as string,
  };
}

// ============================================================================
// Component
// ============================================================================

export default function AgentDetailPage() {
  const params = useParams();
  const router = useRouter();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const agentId = params.id as string;

  // Agent data state
  const [agent, setAgent] = useState<Agent | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [agentStatus, setAgentStatus] = useState<
    'online' | 'thinking' | 'outputting' | 'offline'
  >('offline');
  const [isSaving, setIsSaving] = useState(false);
  const [isSavingTools, setIsSavingTools] = useState(false);
  const [isSavingInteraction, setIsSavingInteraction] = useState(false);

  // Tab state
  const [activeTab, setActiveTab] = useState<TabKey>('runtime');

  // ---- Fetch agent ----

  const loadAgent = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<Record<string, unknown>>(
        `/api/v1/agents/${agentId}`,
      );
      const data = mapAgentResponse(res);
      setAgent(data);
      setAgentStatus(data.is_active ? 'online' : 'offline');
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.status === 404 ? 'Agent 不存在' : err.message);
      } else {
        setError('加载 Agent 信息失败');
      }
    } finally {
      setIsLoading(false);
    }
  }, [agentId]);

  useEffect(() => {
    if (!authLoading && isAuthenticated) {
      loadAgent();
    }
  }, [authLoading, isAuthenticated, loadAgent]);

  // Auth guard
  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.push('/auth/login');
    }
  }, [authLoading, isAuthenticated, router]);

  // ---- Handlers ----

  const handleRuntimeSave = useCallback(
    async (updates: {
      model_provider: AgentModelProvider;
      model_name: string;
      temperature: number;
      max_tokens: number;
    }) => {
      setIsSaving(true);
      try {
        await apiClient.patch(`/api/v1/agents/${agentId}`, updates);
        // Refresh agent data
        await loadAgent();
      } catch (err) {
        if (err instanceof ApiError) {
          throw err;
        }
        throw new Error('保存失败');
      } finally {
        setIsSaving(false);
      }
    },
    [agentId, loadAgent],
  );

  // ---- Tools save handler ----

  const handleToolsSave = useCallback(
    async (enabledTools: string[]) => {
      setIsSavingTools(true);
      try {
        await apiClient.patch(`/api/v1/agents/${agentId}`, { enabled_tools: enabledTools });
        await loadAgent();
      } catch (err) {
        if (err instanceof ApiError) {
          throw err;
        }
        throw new Error('工具配置保存失败');
      } finally {
        setIsSavingTools(false);
      }
    },
    [agentId, loadAgent],
  );

  // ---- Interaction mode save handler ----

  const handleInteractionModeSave = useCallback(
    async (mode: AgentInteractionMode) => {
      setIsSavingInteraction(true);
      try {
        await apiClient.patch(`/api/v1/agents/${agentId}`, { interaction_mode: mode });
        await loadAgent();
      } catch (err) {
        if (err instanceof ApiError) {
          throw err;
        }
        throw new Error('交互模式保存失败');
      } finally {
        setIsSavingInteraction(false);
      }
    },
    [agentId, loadAgent],
  );

  // Auth loading screen
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

  // Loading state
  if (isLoading) {
    return (
      <div className="mx-auto max-w-6xl px-6 py-8">
        {/* Back link placeholder */}
        <div className="mb-6 h-5 w-32 animate-pulse bg-muted" />

        <div className="flex flex-col gap-6 lg:flex-row">
          {/* Left skeleton */}
          <div className="w-full lg:w-80">
            <div className="card-brutal p-6">
              <div className="flex items-center gap-3">
                <div className="h-14 w-14 animate-pulse border-2 border-black bg-muted" />
                <div className="flex-1 space-y-2">
                  <div className="h-5 w-28 animate-pulse bg-muted" />
                  <div className="h-3 w-20 animate-pulse bg-muted" />
                </div>
              </div>
              <div className="mt-4 space-y-2">
                <div className="h-3 w-full animate-pulse bg-muted" />
                <div className="h-3 w-3/4 animate-pulse bg-muted" />
              </div>
            </div>
          </div>

          {/* Right skeleton */}
          <div className="flex-1">
            <div className="card-brutal p-6">
              <div className="mb-6 flex gap-4">
                <div className="h-8 w-20 animate-pulse bg-muted" />
                <div className="h-8 w-20 animate-pulse bg-muted" />
                <div className="h-8 w-20 animate-pulse bg-muted" />
              </div>
              <div className="space-y-3">
                <div className="h-4 w-24 animate-pulse bg-muted" />
                <div className="h-10 w-full animate-pulse bg-muted" />
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // Error state
  if (error && !agent) {
    return (
      <div className="mx-auto max-w-6xl px-6 py-8">
        <button
          type="button"
          onClick={() => router.push('/agents')}
          className="mb-6 inline-flex items-center gap-2 font-heading text-sm font-bold text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          返回 Agent 列表
        </button>
        <div className="card-brutal p-8">
          <div className="flex flex-col items-center justify-center py-12">
            <div className="mb-4 flex h-14 w-14 items-center justify-center border-2 border-black bg-brutal-red-light shadow-brutal-sm">
              <Bot className="h-7 w-7 text-brutal-red" />
            </div>
            <h2 className="font-heading text-lg font-bold text-foreground">
              {error}
            </h2>
            <p className="mt-1.5 font-body text-sm text-muted-foreground">
              该 Agent 可能已被删除或 ID 不正确
            </p>
            <button
              type="button"
              onClick={loadAgent}
              className="btn-brutal btn-brutal-sm mt-6"
            >
              重试
            </button>
          </div>
        </div>
      </div>
    );
  }

  // Guard: agent should exist by here
  if (!agent) return null;

  return (
    <div className="mx-auto max-w-6xl px-6 py-8">
      {/* Back link */}
      <button
        type="button"
        onClick={() => router.push('/agents')}
        className="mb-6 inline-flex items-center gap-2 font-heading text-sm font-bold text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        返回 Agent 列表
      </button>

      <div className="flex flex-col gap-6 lg:flex-row lg:items-start">
        {/* ================================================================ */}
        {/* Left: Agent Info Panel                                          */}
        {/* ================================================================ */}
        <div className="w-full lg:w-80 lg:flex-shrink-0">
          <div className="card-brutal p-6">
            {/* Avatar + Name + Status */}
            <div className="flex items-center gap-3">
              {/* Bot icon in pink brutalist square */}
              <div className="flex h-14 w-14 flex-shrink-0 items-center justify-center border-2 border-black bg-brutal-pink shadow-brutal-sm">
                <Bot className="h-7 w-7 text-white" />
              </div>
              <div className="min-w-0 flex-1">
                <h2 className="truncate font-heading text-lg font-bold text-foreground">
                  {agent.name}
                </h2>
                <StatusIndicator status={agentStatus} />
              </div>
            </div>

            <hr className="divider-brutal" />

            {/* Description */}
            {agent.description ? (
              <p className="font-body text-sm text-muted-foreground leading-relaxed">
                {agent.description}
              </p>
            ) : (
              <p className="font-body text-sm italic text-muted-foreground">
                暂无描述
              </p>
            )}

            <hr className="divider-brutal" />

            {/* Metadata */}
            <div className="space-y-2">
  
              {/* Created date */}
              <div className="flex items-center gap-2">
                <Calendar className="h-3.5 w-3.5 flex-shrink-0 text-muted-foreground" />
                <span className="font-mono text-[11px] text-muted-foreground">
                  创建于 {formatDate(agent.created_at)}
                </span>
              </div>
            </div>

            <hr className="divider-brutal" />

            {/* Interaction Mode */}
            <InteractionMode
              agent={agent}
              onSave={handleInteractionModeSave}
              isSaving={isSavingInteraction}
            />
          </div>

          {/* Quick actions */}
          <div className="mt-4 flex gap-2">
            <button
              type="button"
              onClick={() => router.push(`/agents/${agentId}/edit`)}
              className="btn-brutal btn-brutal-sm flex-1"
            >
              编辑完整资料
            </button>
          </div>
        </div>

        {/* ================================================================ */}
        {/* Right: Tab Panel                                                */}
        {/* ================================================================ */}
        <div className="min-w-0 flex-1">
          {/* Tab bar */}
          <div className="flex border-b-2 border-black">
            {TABS.map((tab) => {
              const Icon = tab.icon;
              const isActive = activeTab === tab.key;
              return (
                <button
                  key={tab.key}
                  type="button"
                  onClick={() => setActiveTab(tab.key)}
                  className={cn(
                    'flex items-center gap-2 border-2 border-b-0 px-5 py-3 font-heading text-sm font-bold transition-colors',
                    isActive
                      ? 'border-black bg-white text-foreground'
                      : 'border-transparent bg-transparent text-muted-foreground hover:text-foreground',
                  )}
                  role="tab"
                  aria-selected={isActive}
                >
                  <Icon className="h-4 w-4" />
                  {tab.label}
                </button>
              );
            })}
          </div>

          {/* Tab content */}
          <div className="card-brutal border-t-0 p-6">
            {activeTab === 'runtime' && (
              <RuntimeTab
                agent={agent}
                onSave={handleRuntimeSave}
                isSaving={isSaving}
              />
            )}

            {activeTab === 'tools' && (
              <ToolsTab
                agent={agent}
                onSave={handleToolsSave}
                isSaving={isSavingTools}
              />
            )}

            {activeTab === 'history' && <HistoryTab isLoading={false} />}
          </div>
        </div>
      </div>
    </div>
  );
}
