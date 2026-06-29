// ============================================================================
// AddAgentModal — brutalist dialog to add an Agent to the current channel
// - card-brutal dialog wrapper (via Dialog component)
// - input-brutal search bar
// - Agent list with status indicators and add buttons
// ============================================================================

'use client';

import { useState, useEffect, useCallback } from 'react';
import { ArrowLeft, Bot, Circle, AlertCircle, Layers, Loader2, Plus, RefreshCw } from 'lucide-react';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { useAgents } from '@/lib/hooks/use-agents';
import { AgentForm, type AgentFormValues } from '@/components/agents/agent-form';
import { useCliDetection } from '@/lib/hooks/use-cli-detection';
import { listTemplates, applyTemplate, type Template } from '@/lib/templates-api';
import {
  Dialog,
  DialogHeader,
  DialogTitle,
  DialogCloseButton,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { t } from '@/lib/i18n';
import type { Agent } from '@/lib/types';

interface AddAgentModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Already added agent IDs (to filter out) */
  existingAgentIds: string[];
  onAdd: (agentId: string, agentName: string) => Promise<void>;
}

export function AddAgentModal({
  open,
  onOpenChange,
  existingAgentIds,
  onAdd,
}: AddAgentModalProps) {
  const { agents, isLoading, error: agentsError, refetch, createAgent } = useAgents();
  const detection = useCliDetection();
  const [searchQuery, setSearchQuery] = useState('');
  const [addingId, setAddingId] = useState<string | null>(null);
  const [mode, setMode] = useState<'list' | 'create' | 'template'>('list');
  const [isCreating, setIsCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [templates, setTemplates] = useState<Template[]>([]);
  const [templatesLoading, setTemplatesLoading] = useState(false);
  const [templateError, setTemplateError] = useState<string | null>(null);
  const [selectedModelProvider, setSelectedModelProvider] = useState('');
  const [applyingTemplate, setApplyingTemplate] = useState<string | null>(null);

  // Reset search when modal opens
  useEffect(() => {
    if (open) {
      setSearchQuery('');
      setAddingId(null);
      setMode('list');
      setCreateError(null);
      setTemplateError(null);
    }
  }, [open]);

  useEffect(() => {
    if (mode !== 'template' || selectedModelProvider) return;
    const available = Object.values(detection.results).find((rt) => rt.available);
    if (available) setSelectedModelProvider(available.type);
  }, [detection.results, mode, selectedModelProvider]);

  const availableAgents = agents.filter(
    (a) => !existingAgentIds.includes(a.id),
  );

  const filteredAgents = searchQuery
    ? availableAgents.filter(
        (a) =>
          a.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
          (a.description &&
            a.description.toLowerCase().includes(searchQuery.toLowerCase())),
      )
    : availableAgents;

  const handleAdd = useCallback(
    async (agent: Agent) => {
      setAddingId(agent.id);
      try {
        await onAdd(agent.id, agent.name);
        onOpenChange(false);
      } finally {
        setAddingId(null);
      }
    },
    [onAdd, onOpenChange],
  );

  const handleCreate = useCallback(
    async (values: AgentFormValues) => {
      setIsCreating(true);
      setCreateError(null);
      try {
        const agent = await createAgent(values);
        await onAdd(agent.id, agent.name);
        await refetch();
        onOpenChange(false);
      } catch (err) {
        setCreateError(err instanceof Error ? err.message : t('teamsAgentCreateError'));
      } finally {
        setIsCreating(false);
      }
    },
    [createAgent, onAdd, onOpenChange, refetch],
  );

  const handleOpenTemplates = useCallback(async () => {
    setMode('template');
    setTemplatesLoading(true);
    setTemplateError(null);
    try {
      setTemplates(await listTemplates());
    } catch (err) {
      setTemplateError(err instanceof Error ? err.message : t('relationshipTemplateLoadError'));
    } finally {
      setTemplatesLoading(false);
    }
  }, []);

  const handleApplyTemplate = useCallback(
    async (templateId: string) => {
      if (!selectedModelProvider) {
        setTemplateError(t('relationshipRuntimeRequiredError'));
        return;
      }
      setApplyingTemplate(templateId);
      setTemplateError(null);
      try {
        const result = await applyTemplate(templateId, selectedModelProvider);
        await Promise.all(result.created_agent_ids.map((id) => onAdd(id, id)));
        await refetch();
        onOpenChange(false);
      } catch (err) {
        setTemplateError(err instanceof Error ? err.message : t('relationshipTemplateApplyError'));
      } finally {
        setApplyingTemplate(null);
      }
    },
    [onAdd, onOpenChange, refetch, selectedModelProvider],
  );

  const createModeButtons = (
    <div className="mb-4 grid grid-cols-2 gap-2">
      <Button
        type="button"
        onClick={() => setMode('create')}
        variant={mode === 'create' ? 'primary' : 'outline'}
        className="gap-2"
      >
        <Plus className="h-4 w-4" />
        {t('relationshipSingleAgent')}
      </Button>
      <Button
        type="button"
        onClick={handleOpenTemplates}
        variant={mode === 'template' ? 'primary' : 'outline'}
        className="gap-2"
      >
        <Layers className="h-4 w-4" />
        {t('relationshipFromTemplate')}
      </Button>
    </div>
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange} width={mode === 'template' ? 'lg' : 'md'}>
      <DialogHeader>
        <DialogTitle>
          {mode === 'create' ? t('teamsCreateAgent') : mode === 'template' ? t('relationshipCreateFromTemplate') : t('addAgentToChannel')}
        </DialogTitle>
        <DialogCloseButton onClick={() => onOpenChange(false)} />
      </DialogHeader>

      {mode === 'create' ? (
        <div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setMode('list')}
            className="mb-4 gap-1.5"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            {t('back')}
          </Button>
          {createError && (
            <div className="mb-4 flex items-center gap-2 border-2 border-brutal-danger bg-brutal-danger-light/30 px-3 py-2">
              <AlertCircle className="h-4 w-4 flex-shrink-0 text-brutal-danger" />
              <span className="font-mono text-xs flex-1 text-brutal-danger">
                {createError}
              </span>
            </div>
          )}
          <AgentForm
            onSubmit={handleCreate}
            isSubmitting={isCreating}
            submitLabel={t('teamsCreateAgent')}
          />
        </div>
      ) : mode === 'template' ? (
        <div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setMode('list')}
            className="mb-4 gap-1.5"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            {t('back')}
          </Button>
          <div className="space-y-4 max-h-[60vh] overflow-y-auto">
          <div>
            <label className="block font-heading text-xs font-bold uppercase tracking-wider mb-1.5">
              {t('relationshipRuntimeRequired')}
            </label>
            <Select
              value={selectedModelProvider}
              onChange={setSelectedModelProvider}
              options={Object.values(detection.results).map((rt) => ({
                value: rt.type,
                label: `${rt.available ? '●' : '○'} ${rt.display_name}${rt.version ? ` (${rt.version})` : ''}`,
                disabled: !rt.available,
              }))}
              placeholder={t('relationshipSelectRuntime')}
              size="md"
              className="w-full"
            />
          </div>
          {templatesLoading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : templates.length === 0 ? (
            <p className="font-mono text-sm text-muted-foreground text-center py-4">{t('relationshipNoTemplates')}</p>
          ) : (
            [...new Set(templates.map((template) => template.category))].map((category) => (
              <div key={category}>
                <h3 className="font-heading text-xs font-bold uppercase tracking-wider text-muted-foreground mb-2 border-b-2 border-black pb-1">
                  {category}
                </h3>
                <div className="space-y-2">
                  {templates.filter((template) => template.category === category).map((template) => (
                    <div key={template.id} className="flex items-start gap-3 p-3 border-2 border-black bg-white">
                      <span className="text-2xl flex-shrink-0">{template.icon}</span>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-1.5">
                          <span className="font-heading text-sm font-bold text-black">{template.name}</span>
                          <span className="inline-flex items-center justify-center h-5 min-w-[1.25rem] px-1 border-2 border-black bg-brutal-cream font-mono text-[10px] font-bold text-black">
                            {template.member_count}
                          </span>
                        </div>
                        <p className="font-sans text-xs text-muted-foreground mt-0.5">{template.description}</p>
                      </div>
                      <Button
                        type="button"
                        onClick={() => handleApplyTemplate(template.id)}
                        disabled={applyingTemplate === template.id}
                        variant="success"
                        size="sm"
                        className="flex-shrink-0"
                      >
                        {applyingTemplate === template.id ? <Loader2 className="h-3 w-3 animate-spin" /> : t('relationshipApplyTemplate')}
                      </Button>
                    </div>
                  ))}
                </div>
              </div>
            ))
          )}
          {templateError && (
            <p className="font-mono text-xs text-brutal-danger">{templateError}</p>
          )}
          </div>
        </div>
      ) : (
      <>
      {/* Search */}
      <div className="mb-4">
        <Input
          placeholder={t('searchAgent')}
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          autoFocus
        />
      </div>

      {/* Error state */}
      {agentsError && (
        <div className="mb-4 flex items-center gap-2 border-2 border-brutal-danger bg-brutal-danger-light/30 px-3 py-2">
          <AlertCircle className="h-4 w-4 flex-shrink-0 text-brutal-danger" />
          <span className="font-mono text-xs flex-1 text-brutal-danger">
            {agentsError}
          </span>
          <Button
            type="button"
            onClick={refetch}
            size="sm"
            variant="outline"
            className="flex-shrink-0"
          >
            <RefreshCw className="mr-1 h-3 w-3" />
            {t('retry')}
          </Button>
        </div>
      )}

      {createModeButtons}

      {/* Agent list */}
      <div className="max-h-64 overflow-y-auto">
        {isLoading ? (
          <div className="space-y-2">
            {[1, 2, 3].map((i) => (
              <div key={i} className="flex items-center gap-3 border-2 border-black p-2 shadow-brutal-sm">
                <Skeleton className="h-8 w-8 rounded-none" />
                <div className="flex-1 space-y-1">
                  <Skeleton className="h-4 w-24 rounded-none" />
                  <Skeleton className="h-3 w-32 rounded-none" />
                </div>
              </div>
            ))}
          </div>
        ) : filteredAgents.length === 0 ? (
          <div className="py-8 text-center">
            <div className="mx-auto mb-2 flex h-10 w-10 items-center justify-center border-2 border-black bg-white shadow-brutal-sm">
              <Bot className="h-5 w-5 text-muted-foreground" />
            </div>
            <p className="font-body text-sm text-muted-foreground">
              {searchQuery ? t('noMatchingAgents') : t('noAgentsAvailable')}
            </p>
            {!searchQuery && (
              <p className="mt-1 font-mono text-[11px] text-muted-foreground">
                {t('allAgentsInChannel')}
              </p>
            )}
          </div>
        ) : (
          <div className="space-y-1">
            {filteredAgents.map((agent) => (
              <div
                key={agent.id}
                className="flex w-full items-center gap-3 border-2 border-transparent bg-white p-2 text-left transition-all hover:border-black hover:bg-brutal-primary-light hover:shadow-brutal-sm"
              >
                <PixelAvatar agentId={agent.id} size="md" />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate font-heading text-sm font-bold text-foreground">
                      {agent.name}
                    </span>
                    <span className="flex-shrink-0 border-2 border-black bg-brutal-primary px-1.5 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider text-black">
                      {t('agent')}
                    </span>
                  </div>
                  <div className="mt-0.5 flex items-center gap-2 font-mono text-[11px] text-muted-foreground">
                    <span className="flex items-center gap-1">
                      <Circle
                        className={`h-2 w-2 flex-shrink-0 ${
                          agent.is_active
                            ? 'fill-brutal-success text-brutal-success'
                            : 'fill-brutal-muted text-brutal-muted'
                        }`}
                      />
                      {agent.is_active ? t('online') : t('offline')}
                    </span>
                    {agent.description && (
                      <span className="truncate text-muted-foreground/70">
                        {agent.description}
                      </span>
                    )}
                  </div>
                </div>
                <Button
                  type="button"
                  size="sm"
                  variant="success"
                  onClick={() => handleAdd(agent)}
                  disabled={addingId === agent.id}
                  className="flex-shrink-0"
                >
                  {addingId === agent.id ? t('adding') : t('add')}
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>
      </>
      )}
    </Dialog>
  );
}
