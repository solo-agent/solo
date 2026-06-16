// ============================================================================
// /teams — Teams page (v2)
// Left column: Graph / Agents / Humans sections, each collapsible.
// Right panel: detail view for the selected section or item.
// - No AppFrame: this page owns its layout (no global Inbox/Channels sidebar).
// - Selection: 'graph' | 'agent' | 'human' | null. Defaults to first agent.
// ============================================================================

'use client';

import { useEffect, useState, useCallback, useRef } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { AlertCircle, RefreshCw, Plus, Layers, MessageSquare, User, FolderOpen, Loader2 } from 'lucide-react';
import { useAuth } from '@/lib/auth-context';
import { t } from '@/lib/i18n';
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
import { TeamsAgentProfile } from '@/components/teams/teams-agent-profile';
import { TeamsAgentWorkspace } from '@/components/teams/teams-agent-workspace';
import { TeamsHumanProfile } from '@/components/teams/teams-human-profile';
import { AgentForm, type AgentFormValues } from '@/components/agents/agent-form';
import { Select } from '@/components/ui/select';
import { listTemplates, applyTemplate, type Template } from '@/lib/templates-api';
import { useCliDetection } from '@/lib/hooks/use-cli-detection';
import type { Agent, AgentBackendDetectItem } from '@/lib/types';

type AgentTab = 'profile' | 'workspace';

const AGENT_TABS: TabBarTab[] = [
  { key: 'profile', label: 'Profile', icon: <User className="h-3.5 w-3.5" /> },
  { key: 'workspace', label: 'Workspace', icon: <FolderOpen className="h-3.5 w-3.5" /> },
];

export default function TeamsPage() {
  const router = useRouter();
  const { isAuthenticated, isLoading: authLoading, user: authUser } = useAuth();
  const { agents, isLoading: agentsLoading, error: agentsError, refetch: refetchAgents, createAgent } = useAgents();
  const { user, isLoading: userLoading, error: userError, refetch: refetchUser } = useUser();
  const { createOrGetDM } = useDM();
  const { showToast } = useToast();
  const { results: detection, isLoading: detectionLoading } = useCliDetection();

  const [selection, setSelection] = useState<TeamsSelection | null>(null);
  const [agentTab, setAgentTab] = useState<AgentTab>('profile');
  const [isDMLoading, setIsDMLoading] = useState(false);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const [showChoiceDialog, setShowChoiceDialog] = useState(false);
  const [showTemplateModal, setShowTemplateModal] = useState(false);
  const [selectedModelProvider, setSelectedModelProvider] = useState('');
  const [templates, setTemplates] = useState<Template[]>([]);
  const [templatesLoading, setTemplatesLoading] = useState(false);
  const [applyingTemplate, setApplyingTemplate] = useState<string | null>(null);
  const [templateError, setTemplateError] = useState<string | null>(null);

  // ---- URL-based routing ----
  const searchParams = useSearchParams();
  const initializedRef = useRef(false);

  // Initialize selection from URL params on load.
  useEffect(() => {
    if (initializedRef.current || agents.length === 0) return;
    const agentId = searchParams.get('agent');
    const tab = searchParams.get('tab') as AgentTab | null;
    if (agentId && agents.some((a) => a.id === agentId)) {
      setSelection({ kind: 'agent', id: agentId });
      if (tab === 'profile' || tab === 'workspace') setAgentTab(tab);
    } else {
      // Default: first agent
      setSelection({ kind: 'agent', id: agents[0].id });
      router.replace(`/teams?agent=${agents[0].id}&tab=profile`, { scroll: false });
    }
    initializedRef.current = true;
  }, [agents, searchParams, router]);

  // Auth guard
  useEffect(() => {
    if (!authLoading && !isAuthenticated) {
      router.push('/auth/login');
    }
  }, [authLoading, isAuthenticated, router]);

  // Auto-select first agent (fallback when agents load after init or URLs change).
  useEffect(() => {
    if (!initializedRef.current && agents.length > 0) return;
    if (selection === null && agents.length > 0) {
      setSelection({ kind: 'agent', id: agents[0].id });
      router.replace(`/teams?agent=${agents[0].id}&tab=profile`, { scroll: false });
    }
  }, [agents, selection, router]);

  // Reset tab when switching agents
  useEffect(() => {
    setAgentTab('profile');
  }, [selection?.kind === 'agent' ? selection.id : null]);

  const humans = user ? [{ id: user.id, display_name: user.display_name }] : [];

  const handleSelectAgent = useCallback((agentId: string) => {
    setSelection({ kind: 'agent', id: agentId });
    setAgentTab('profile');
    router.replace(`/teams?agent=${agentId}&tab=profile`, { scroll: false });
  }, [router]);

  const handleSelectHuman = useCallback((userId: string) => {
    setSelection({ kind: 'human', id: userId });
  }, []);

  // ---- Template handlers ----

  const loadTemplates = useCallback(async () => {
    setTemplatesLoading(true);
    try {
      setTemplates(await listTemplates());
    } catch { /* noop */ }
    finally { setTemplatesLoading(false); }
  }, []);

  const handleApplyTemplate = useCallback(async (templateId: string) => {
    setApplyingTemplate(templateId);
    setTemplateError(null);
    try {
      if (!authUser?.id) { setTemplateError('Not authenticated'); return; }
      if (!selectedModelProvider) { setTemplateError('Please select a runtime'); return; }
      await applyTemplate(templateId, authUser.id, selectedModelProvider);
      setShowTemplateModal(false);
      await refetchAgents();
      showToast('Team created from template', 'success');
    } catch (err) {
      setTemplateError(err instanceof Error ? err.message : 'Failed to apply template');
    } finally {
      setApplyingTemplate(null);
    }
  }, [authUser, selectedModelProvider, refetchAgents, showToast]);

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
      showToast(t('createDMError'), 'error');
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
      showToast(t('teamsAgentCreated'), 'success');
      setIsCreateModalOpen(false);
      setSelection({ kind: 'agent', id: agent.id });
      setAgentTab('profile');
      router.replace(`/teams?agent=${agent.id}&tab=profile`, { scroll: false });
    } catch {
      showToast(t('teamsAgentCreateError'), 'error');
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
          <p className="font-mono text-sm text-muted-foreground">{t('loading')}</p>
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
          onSelectAgent={handleSelectAgent}
          onSelectHuman={handleSelectHuman}
          onCreateAgent={() => setShowChoiceDialog(true)}
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
              {t('retry')}
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
              {t('retry')}
            </Button>
          </div>
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
                {isDMLoading ? t('teamsJumping') : 'Message'}
              </Button>
            </div>
            <TabBar
              tabs={AGENT_TABS}
              activeKey={agentTab}
              onChange={(key) => {
                setAgentTab(key as AgentTab);
                router.replace(`/teams?agent=${selectedAgent.id}&tab=${key}`, { scroll: false });
              }}
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
              <h2 className="font-heading text-lg font-bold">{t('teamsNoAgents')}</h2>
              <p className="mt-2 font-body text-sm text-muted-foreground">
                {t('teamsNoAgentsDesc')}
              </p>
            </div>
          </div>
        )}
      </main>

      {/* Choice dialog: Create Agent or From Template */}
      <Dialog open={showChoiceDialog} onOpenChange={setShowChoiceDialog} width="sm">
        <DialogHeader>
          <DialogTitle className="font-heading text-base font-black uppercase tracking-wider">
            Create Agent
          </DialogTitle>
          <DialogCloseButton onClick={() => setShowChoiceDialog(false)} />
        </DialogHeader>
        <div className="space-y-3">
          <button
            type="button"
            onClick={() => { setShowChoiceDialog(false); setIsCreateModalOpen(true); }}
            className="w-full flex items-center gap-3 p-4 border-2 border-black bg-white hover:bg-brutal-primary-light text-left"
          >
            <Plus className="h-5 w-5 flex-shrink-0" />
            <div>
              <div className="font-heading text-sm font-bold">Single Agent</div>
              <p className="font-sans text-xs text-muted-foreground mt-0.5">Create one agent with custom name, role, and runtime.</p>
            </div>
          </button>
          <button
            type="button"
            onClick={() => {
              setShowChoiceDialog(false);
              setShowTemplateModal(true);
              loadTemplates();
              if (!selectedModelProvider) {
                const available = (Object.values(detection) as AgentBackendDetectItem[]).find((rt) => rt.available);
                if (available) setSelectedModelProvider(available.type);
              }
            }}
            className="w-full flex items-center gap-3 p-4 border-2 border-black bg-white hover:bg-brutal-accent-light text-left"
          >
            <Layers className="h-5 w-5 flex-shrink-0" />
            <div>
              <div className="font-heading text-sm font-bold">From Template</div>
              <p className="font-sans text-xs text-muted-foreground mt-0.5">Create a team of agents with preset roles and relationships.</p>
            </div>
          </button>
        </div>
      </Dialog>

      {/* Create Agent Modal */}
      <Dialog
        open={isCreateModalOpen}
        onOpenChange={(opened) => {
          if (!opened) setIsCreateModalOpen(false);
        }}
        width="lg"
      >
        <DialogHeader>
          <DialogTitle>{t('teamsCreateAgent')}</DialogTitle>
          <DialogCloseButton onClick={() => setIsCreateModalOpen(false)} />
        </DialogHeader>
        <AgentForm
          onSubmit={handleCreateAgent}
          isSubmitting={isCreating}
          submitLabel={t('teamsCreateAgent')}
        />
      </Dialog>

      {/* Template selection modal */}
      <Dialog open={showTemplateModal} onOpenChange={setShowTemplateModal} width="lg">
        <DialogHeader>
          <DialogTitle className="font-heading text-base font-black uppercase tracking-wider">
            Create from Template
          </DialogTitle>
          <DialogCloseButton onClick={() => setShowTemplateModal(false)} />
        </DialogHeader>
        <div className="space-y-4 max-h-[60vh] overflow-y-auto">
          {/* Model provider select */}
          <div>
            <label className="block font-heading text-xs font-bold uppercase tracking-wider mb-1.5">
              Runtime <span className="text-brutal-danger">*</span>
            </label>
            {detectionLoading ? (
              <p className="font-mono text-xs text-muted-foreground">Detecting runtimes...</p>
            ) : (
              <Select
                value={selectedModelProvider}
                onChange={setSelectedModelProvider}
                options={(Object.values(detection) as AgentBackendDetectItem[]).map((rt) => ({
                  value: rt.type,
                  label: `${rt.available ? '●' : '○'} ${rt.display_name}${rt.version ? ` (${rt.version})` : ''}`,
                  disabled: !rt.available,
                }))}
                placeholder="Select runtime..."
                size="md"
                className="w-full"
              />
            )}
          </div>

          {templatesLoading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : templates.length === 0 ? (
            <p className="font-mono text-sm text-muted-foreground text-center py-4">No templates available.</p>
          ) : (
            <>
              {(() => {
                const cats = [...new Set(templates.map((t) => t.category))];
                return cats.map((cat) => (
                  <div key={cat}>
                    <h3 className="font-heading text-xs font-bold uppercase tracking-wider text-muted-foreground mb-2 border-b-2 border-black pb-1">
                      {cat}
                    </h3>
                    <div className="space-y-2">
                      {templates.filter((t) => t.category === cat).map((tmpl) => (
                        <div
                          key={tmpl.id}
                          className="flex items-start gap-3 p-3 border-2 border-black bg-white"
                        >
                          <span className="text-2xl flex-shrink-0">{tmpl.icon}</span>
                          <div className="flex-1 min-w-0">
                            <div className="font-heading text-sm font-bold text-black">{tmpl.name}</div>
                            <p className="font-sans text-xs text-muted-foreground mt-0.5">{tmpl.description}</p>
                          </div>
                          <button
                            type="button"
                            onClick={() => handleApplyTemplate(tmpl.id)}
                            disabled={applyingTemplate === tmpl.id}
                            className="flex-shrink-0 px-3 py-1.5 border-2 border-black bg-brutal-success text-black font-heading text-[10px] font-bold uppercase tracking-wider hover:bg-brutal-success-light disabled:opacity-50"
                          >
                            {applyingTemplate === tmpl.id ? (
                              <Loader2 className="h-3 w-3 animate-spin" />
                            ) : (
                              'Apply'
                            )}
                          </button>
                        </div>
                      ))}
                    </div>
                  </div>
                ));
              })()}
            </>
          )}
          {templateError && (
            <p className="font-mono text-xs text-brutal-danger">{templateError}</p>
          )}
        </div>
      </Dialog>
    </div>
  );
}
