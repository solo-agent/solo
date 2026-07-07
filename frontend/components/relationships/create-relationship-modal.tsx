// ============================================================================
// CreateRelationshipModal — full modal for creating agent relationships (T5.2.4)
// - From/To agent selectors (searchable dropdown)
// - Relationship type picker with visual preview
// - Channel picker (required for collaborates_with)
// - Cycle check warning display
// ============================================================================

'use client';

import { useState, useEffect, useCallback } from 'react';
import { AlertTriangle, ArrowLeftRight, Loader2 } from 'lucide-react';
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Select, type SelectOption } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { apiClient } from '@/lib/api-client';
import { t } from '@/lib/i18n';
import type { RelationshipType, AgentDetailTarget } from '@/lib/types';

const TYPE_OPTIONS: { type: RelationshipType; labelKey: string; color: string; dash: string }[] = [
  { type: 'assigns_to', labelKey: 'assignsTo', color: '#4A90D9', dash: '' },
  { type: 'collaborates_with', labelKey: 'collaboratesWith', color: '#10B981', dash: '8,4' },
];

// ---- Component ----

interface CreateRelationshipModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
  /** Preselect a source agent (e.g., from the graph editor) */
  preselectedFrom?: string;
  preselectedTo?: string;
  agents: Array<AgentDetailTarget & { is_active?: boolean }>;
}

export function CreateRelationshipModal({
  open,
  onOpenChange,
  onCreated,
  preselectedFrom,
  preselectedTo,
  agents,
}: CreateRelationshipModalProps) {
  const [fromAgentId, setFromAgentId] = useState(preselectedFrom ?? '');
  const [toAgentId, setToAgentId] = useState(preselectedTo ?? '');
  const [relType, setRelType] = useState<RelationshipType>('assigns_to');
  const [instruction, setInstruction] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [cycleWarning, setCycleWarning] = useState<string | null>(null);

  // Reset form on open
  useEffect(() => {
    if (open) {
      setFromAgentId(preselectedFrom ?? '');
      setToAgentId(preselectedTo ?? '');
      setRelType('assigns_to');
      setInstruction('');
      setError(null);
      setCycleWarning(null);
    }
  }, [open, preselectedFrom, preselectedTo]);

  // Check for cycles when assigns_to is selected and both agents are chosen
  const checkCycle = useCallback(async () => {
    if (!fromAgentId || !toAgentId || relType !== 'assigns_to') {
      setCycleWarning(null);
      return;
    }
    try {
      // Lightweight cycle detection: if the "to" agent already reports to the "from" agent
      // (or any of its ancestors), that would create a cycle.
      const res = await apiClient.post<{ has_cycle: boolean; path: string[] }>(
        '/api/v1/agent-relationships/check-cycle',
        { from_agent_id: fromAgentId, to_agent_id: toAgentId, rel_type: 'assigns_to' },
      );
      if (res.has_cycle) {
        setCycleWarning(
          `Cycle detected: adding this relationship would create a reporting loop involving ${res.path.join(' -> ')}`,
        );
      } else {
        setCycleWarning(null);
      }
    } catch {
      // Cycle check endpoint may not exist yet — fail silently
      setCycleWarning(null);
    }
  }, [fromAgentId, toAgentId, relType]);

  useEffect(() => {
    checkCycle();
  }, [checkCycle]);

  // Build agent select options
  const agentOptions: SelectOption[] = agents
    .filter((a) => a.id !== fromAgentId) // Can't select self as target
    .map((a) => ({
      value: a.id,
      label: `${a.name}${a.is_active ? '' : ` (${t('offline')})`}`,
      disabled: !a.is_active,
    }));

  const fromAgentOptions: SelectOption[] = agents.map((a) => ({
    value: a.id,
    label: a.name,
  }));

  const canSubmit =
    fromAgentId &&
    toAgentId &&
    fromAgentId !== toAgentId &&
    !isSubmitting &&
    !cycleWarning;

  const handleSwapAgents = () => {
    if (!fromAgentId && !toAgentId) return;
    setFromAgentId(toAgentId);
    setToAgentId(fromAgentId);
  };

  const handleSubmit = async () => {
    if (!canSubmit) return;
    setIsSubmitting(true);
    setError(null);
    try {
      const body: Record<string, unknown> = {
        from_agent_id: fromAgentId,
        to_agent_id: toAgentId,
        rel_type: relType,
      };
      if (instruction.trim()) {
        body.instruction = instruction.trim();
      }
      await apiClient.post('/api/v1/agent-relationships', body);
      onCreated();
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('relationshipCreateError'));
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange} width="lg">
      <DialogHeader>
        <DialogTitle className="font-heading text-base font-black uppercase tracking-wider">
          {t('relationshipEditorCreateRelationship')}
        </DialogTitle>
      </DialogHeader>

      <div className="space-y-4">
        <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] items-end gap-3">
          {/* From Agent */}
          <div>
            <label className="block font-heading text-xs font-bold uppercase tracking-wider mb-1.5">
              {t('relationshipEditorFrom')}
            </label>
            <Select
              options={fromAgentOptions}
              value={fromAgentId}
              onChange={setFromAgentId}
              placeholder={preselectedFrom ? agents.find((a) => a.id === preselectedFrom)?.name : t('relationshipSelectAgent')}
              size="md"
              className="w-full"
              disabled={!!preselectedFrom}
            />
          </div>

          {/* To Agent */}
          <div>
            <label className="block font-heading text-xs font-bold uppercase tracking-wider mb-1.5">
              {t('relationshipEditorTo')}
            </label>
            <Select
              options={agentOptions}
              value={toAgentId}
              onChange={setToAgentId}
              placeholder={preselectedTo ? agents.find((a) => a.id === preselectedTo)?.name : t('relationshipSelectAgent')}
              size="md"
              className="w-full"
              disabled={!!preselectedTo}
            />
          </div>

          <Button
            type="button"
            onClick={handleSwapAgents}
            disabled={!fromAgentId && !toAgentId}
            variant="outline"
            size="icon"
            aria-label={t('relationshipSwapAgents')}
            title={t('relationshipSwapAgents')}
          >
            <ArrowLeftRight className="h-4 w-4" />
          </Button>
        </div>

        {/* Relationship Type */}
        <div>
          <label className="block font-heading text-xs font-bold uppercase tracking-wider mb-1.5">
            {t('relationshipEditorType')}
          </label>
          <div className="grid grid-cols-2 gap-2">
            {TYPE_OPTIONS.map((opt) => (
              <button
                key={opt.type}
                type="button"
                onClick={() => setRelType(opt.type)}
                className={[
                  'flex items-center gap-2 px-3 py-2.5 text-left border-2 transition-colors duration-100',
                  relType === opt.type
                    ? 'border-black bg-brutal-primary-light shadow-brutal-sm'
                    : 'border-transparent hover:border-brutal-muted bg-white',
                ].join(' ')}
              >
                {/* Edge preview */}
                <svg width="28" height="10" className="flex-shrink-0">
                  <line
                    x1="0" y1="5" x2="28" y2="5"
                    stroke={opt.color}
                    strokeWidth={2}
                    strokeDasharray={opt.dash || undefined}
                  />
                </svg>
                <span className="font-heading text-xs font-bold uppercase tracking-wider text-black">
                  {t(opt.labelKey as Parameters<typeof t>[0])}
                </span>
              </button>
            ))}
          </div>
        </div>

        {/* Instruction */}
        <div>
          <label className="block font-heading text-xs font-bold uppercase tracking-wider mb-1.5">
            {relType === 'assigns_to' ? t('relationshipCriteriaDelegation') : t('relationshipCriteriaCollaboration')}
          </label>
          <Textarea
            value={instruction}
            onChange={(e) => setInstruction(e.target.value)}
            placeholder={relType === 'assigns_to'
              ? t('relationshipDelegationPlaceholder')
              : t('relationshipCollaborationPlaceholder')
            }
            className="min-h-[100px] font-mono text-xs resize-y"
            rows={4}
          />
          <p className="mt-1 font-mono text-[10px] text-muted-foreground">
            {t('relationshipCriteriaExportHint', {
              marker: relType === 'assigns_to' ? 'DELEGATE when' : 'COLLABORATES when',
            })}
          </p>
        </div>

        {/* Cycle warning */}
        {cycleWarning && (
          <div className="flex items-start gap-2 px-3 py-2 border-2 border-brutal-danger bg-brutal-danger-light">
            <AlertTriangle className="h-4 w-4 flex-shrink-0 mt-0.5 text-brutal-danger" />
            <p className="font-mono text-[11px] text-brutal-danger font-bold">{cycleWarning}</p>
          </div>
        )}

        {/* Submit error */}
        {error && (
          <p className="font-mono text-xs text-brutal-danger">{error}</p>
        )}
      </div>

      <DialogFooter>
        <Button
          type="button"
          onClick={() => onOpenChange(false)}
          variant="outline"
          size="sm"
          className="px-4"
        >
          {t('cancel')}
        </Button>
        <Button
          type="button"
          onClick={handleSubmit}
          disabled={!canSubmit || !!cycleWarning}
          variant="success"
          size="sm"
          className="px-4"
        >
          {isSubmitting ? (
            <span className="flex items-center gap-1.5">
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
              {t('submitting')}
            </span>
          ) : (
            t('create')
          )}
        </Button>
      </DialogFooter>
    </Dialog>
  );
}
