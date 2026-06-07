// ============================================================================
// /teams — Teams page (v2)
// Left column: Graph / Agents / Humans sections, each collapsible.
// Right panel: detail view for the selected section or item.
// - No AppFrame: this page owns its layout (no global Inbox/Channels sidebar).
// - Selection: 'graph' | 'agent' | 'human' | null. Defaults to first agent.
// ============================================================================

'use client';

import { useEffect, useState, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { AlertCircle, RefreshCw, Plus, MessageSquare, User, FolderOpen } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { useAgents } from '@/lib/hooks/use-agents';
import { useUser } from '@/lib/hooks/use-user';
import { useDM } from '@/lib/hooks/use-dm';
import { useToast } from '@/components/ui/toast';
import { NavBar } from '@/components/ui/navbar';
import { Spinner } from '@/components/ui/spinner';
import { Button } from '@/components/ui/button';
import { TabBar } from '@/components/ui/tab-bar';
import type { TabBarTab } from '@/components/ui/tab-bar';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogCloseButton,
} from '@/components/ui/dialog';
import { BrutalAlert } from '@/components/ui/brutal-alert';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { TeamsLeftColumn, type TeamsSelection } from '@/components/teams/teams-left-column';
import { TeamsGraphView } from '@/components/teams/teams-graph-view';
import { TeamsAgentProfile } from '@/components/teams/teams-agent-profile';
import { TeamsAgentWorkspace } from '@/components/teams/teams-agent-workspace';
import { TeamsHumanProfile } from '@/components/teams/teams-human-profile';
import { AgentForm, type AgentFormValues } from '@/components/agents/agent-form';
import type { Agent } from '@/lib/types';

type AgentTab = 'profile' | 'workspace';

const AGENT_TABS: TabBarTab[] = [
  { key: 'profile', label: 'Profile', icon: <User className="h-3.5 w-3.5" /> },
  { key: 'workspace', label: 'Workspace', icon: <FolderOpen className="h-3.5 w-3.5" /> },
];

export default function TeamsPage() {
  const router = useRouter();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const { agents, isLoading: agentsLoading, error: agentsError, refetch: refetchAgents, createAgent } = useAgents();
  const { user, isLoading: userLoading, error: userError, refetch: refetchUser } = useUser();
  const { createOrGetDM } = useDM();
  const { showToast } = useToast();

  const [selection, setSelection] = useState<TeamsSelection | null>(null);
  const [agentTab, setAgentTab] = useState<AgentTab>('profile');
  const [isDMLoading, setIsDMLoading] = useState(false);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [isCreating, setIsCreating] = useState(false);

  // Auth guard
  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.push('/auth/login');
    }
  }, [authLoading, isAuthenticated, router]);

  // Auto-select first agent once loaded
  useEffect(() => {
    if (selection === null && agents.length > 0) {
      setSelection({ kind: 'agent', id: agents[0].id });
    }
  }, [agents, selection]);

  // Reset tab when switching agents
  useEffect(() => {
    setAgentTab('profile');
  }, [selection?.kind === 'agent' ? selection.id : null]);

  const humans = user ? [{ id: user.id, display_name: user.display_name }] : [];

  const handleSelectAgent = useCallback((agentId: string) => {
    setSelection({ kind: 'agent', id: agentId });
  }, []);

  const handleSelectHuman = useCallback((userId: string) => {
    setSelection({ kind: 'human', id: userId });
  }, []);

  const handleSelectGraph = useCallback(() => {
    setSelection({ kind: 'graph' });
  }, []);

  const selectedAgent: Agent | undefined =
    selection?.kind === 'agent'
      ? agents.find((a) => a.id === selection.id)
      : undefined;

  const handleMessage = useCallback(async () => {
    if (!selectedAgent || isDMLoading) return;
    setIsDMLoading(true);
    try {
      const dm = await createOrGetDM({ agent_id: selectedAgent.id });
      router.push(`/dashboard?dm=${dm.id}`);
    } catch {
      showToast('无法发起会话，请稍后再试', 'error');
    } finally {
      setIsDMLoading(false);
    }
  }, [selectedAgent, isDMLoading, createOrGetDM, router, showToast]);

  const handleCreateAgent = useCallback(async (values: AgentFormValues) => {
    if (isCreating) return;
    setIsCreating(true);
    try {
      const agent = await createAgent({
        name: values.name,
        description: values.description,
        model_provider: values.model_provider,
        model_name: values.model_name,
        system_prompt: values.system_prompt,
        custom_env: values.custom_env,
        custom_args: values.custom_args,
      });
      showToast('Agent 创建成功', 'success');
      setIsCreateModalOpen(false);
      // Auto-select the new agent to open detail panel
      setSelection({ kind: 'agent', id: agent.id });
      setAgentTab('profile');
    } catch {
      showToast('创建 Agent 失败，请稍后再试', 'error');
    } finally {
      setIsCreating(false);
    }
  }, [isCreating, createAgent, showToast]);

  // Loading shell
  if (authLoading || (agentsLoading && agents.length === 0) || (userLoading && !user)) {
    return (
      <div className="flex h-screen items-center justify-center bg-brutal-cream">
        <div className="flex flex-col items-center gap-3">
          <Spinner size="md" />
          <p className="font-mono text-sm text-muted-foreground">加载中...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-screen min-w-[1024px] overflow-hidden bg-brutal-cream">
      <NavBar />
      <div className="w-[220px] flex-shrink-0">
        <TeamsLeftColumn
          agents={agents}
          humans={humans}
          selection={selection}
          onSelectGraph={handleSelectGraph}
          onSelectAgent={handleSelectAgent}
          onSelectHuman={handleSelectHuman}
          onCreateAgent={() => setIsCreateModalOpen(true)}
        />
      </div>

      <main className="flex flex-1 flex-col overflow-hidden">
        {/* Error banner (agents) */}
        {agentsError && (
          <div className="m-4 space-y-2">
            <BrutalAlert variant="warning">{agentsError}</BrutalAlert>
            <Button
              variant="outline"
              size="sm"
              onClick={() => refetchAgents()}
            >
              <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
              重试
            </Button>
          </div>
        )}
        {userError && !agentsError && (
          <div className="m-4 space-y-2">
            <BrutalAlert variant="warning">{userError}</BrutalAlert>
            <Button
              variant="outline"
              size="sm"
              onClick={() => refetchUser()}
            >
              <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
              重试
            </Button>
          </div>
        )}

        {/* Graph view */}
        {selection?.kind === 'graph' && (
          <TeamsGraphView agents={agents} onSelectAgent={handleSelectAgent} />
        )}

        {/* Human card */}
        {selection?.kind === 'human' && selection.id && (
          <TeamsHumanProfile userId={selection.id} />
        )}

        {/* Agent detail with header + tabs */}
        {selection?.kind === 'agent' && selectedAgent && (
          <>
            <div className="flex items-center gap-3 h-14 border-b-2 border-black bg-brutal-cream px-4">
              <PixelAvatar agentId={selectedAgent.id} size="md" />
              <div className="flex-1 min-w-0">
                <h1 className="truncate font-heading text-base font-bold">
                  {selectedAgent.name}
                </h1>
              </div>
              <Button
                size="sm"
                variant="primary"
                onClick={handleMessage}
                disabled={isDMLoading}
              >
                <MessageSquare className="mr-1.5 h-3.5 w-3.5" />
                {isDMLoading ? '跳转中...' : 'Message'}
              </Button>
            </div>
            <TabBar
              tabs={AGENT_TABS}
              activeKey={agentTab}
              onChange={(key) => setAgentTab(key as AgentTab)}
              variant="pill"
            />
            <div className={agentTab === 'profile' ? 'flex-1 overflow-y-auto p-6' : 'flex-1 overflow-hidden'}>
              {agentTab === 'profile' ? (
                <TeamsAgentProfile agentId={selectedAgent.id} />
              ) : (
                <TeamsAgentWorkspace agentId={selectedAgent.id} />
              )}
            </div>
          </>
        )}

        {/* Empty state: no agents and no selection yet */}
        {selection === null && agents.length === 0 && (
          <div className="flex flex-1 items-center justify-center p-8 text-center">
            <div>
              <div className="mx-auto mb-3 flex h-14 w-14 items-center justify-center border-2 border-black bg-brutal-primary shadow-brutal-sm">
                <Plus className="h-7 w-7 text-white" />
              </div>
              <h2 className="font-heading text-lg font-bold">还没有 Agent</h2>
              <p className="mt-2 font-body text-sm text-muted-foreground">
                请先创建一个 Agent,然后回到 Teams 页面
              </p>
            </div>
          </div>
        )}
      </main>

      {/* Create Agent Modal */}
      <Dialog
        open={isCreateModalOpen}
        onOpenChange={(opened) => {
          if (!opened) setIsCreateModalOpen(false);
        }}
        width="lg"
      >
        <DialogHeader>
          <DialogTitle>创建 Agent</DialogTitle>
          <DialogCloseButton onClick={() => setIsCreateModalOpen(false)} />
        </DialogHeader>
        <AgentForm
          onSubmit={handleCreateAgent}
          isSubmitting={isCreating}
          submitLabel="创建 Agent"
        />
      </Dialog>
    </div>
  );
}
