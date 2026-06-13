// ============================================================================
// CreateRelationshipModal — full modal for creating agent relationships (T5.2.4)
// - From/To agent selectors (searchable dropdown)
// - Relationship type picker with visual preview
// - Channel picker (for channel-scoped types: delegates_to, collaborates_with)
// - Cycle check warning display
// ============================================================================

'use client';

import { useState, useEffect, useCallback } from 'react';
import { AlertTriangle, Loader2 } from 'lucide-react';
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog';
import { Select, type SelectOption } from '@/components/ui/select';
import { apiClient } from '@/lib/api-client';
import { t } from '@/lib/i18n';
import type { RelationshipType, Agent, Channel } from '@/lib/types';

// ---- Channel-scoped types ----

const CHANNEL_SCOPED_TYPES: RelationshipType[] = ['delegates_to', 'collaborates_with'];
const GLOBAL_TYPES: RelationshipType[] = ['reports_to', 'escalates_to'];

const TYPE_OPTIONS: { type: RelationshipType; labelKey: string; color: string; dash: string }[] = [
  { type: 'reports_to', labelKey: 'reportsTo', color: '#4A90D9', dash: '' },
  { type: 'delegates_to', labelKey: 'delegatesTo', color: '#7B6CF6', dash: '' },
  { type: 'collaborates_with', labelKey: 'collaboratesWith', color: '#10B981', dash: '8,4' },
  { type: 'escalates_to', labelKey: 'escalatesTo', color: '#EF4444', dash: '' },
];

// ---- Component ----

interface CreateRelationshipModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
  /** Preselect a source agent (e.g., from the graph editor) */
  preselectedFrom?: string;
  preselectedTo?: string;
  agents: Agent[];
  channels: Channel[];
}

export function CreateRelationshipModal({
  open,
  onOpenChange,
  onCreated,
  preselectedFrom,
  preselectedTo,
  agents,
  channels,
}: CreateRelationshipModalProps) {
  const [fromAgentId, setFromAgentId] = useState(preselectedFrom ?? '');
  const [toAgentId, setToAgentId] = useState(preselectedTo ?? '');
  const [relType, setRelType] = useState<RelationshipType>('reports_to');
  const [channelId, setChannelId] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [cycleWarning, setCycleWarning] = useState<string | null>(null);

  // Reset form on open
  useEffect(() => {
    if (open) {
      setFromAgentId(preselectedFrom ?? '');
      setToAgentId(preselectedTo ?? '');
      setRelType('reports_to');
      setChannelId('');
      setError(null);
      setCycleWarning(null);
    }
  }, [open, preselectedFrom, preselectedTo]);

  // Check for cycles when reports_to is selected and both agents are chosen
  const checkCycle = useCallback(async () => {
    if (!fromAgentId || !toAgentId || relType !== 'reports_to') {
      setCycleWarning(null);
      return;
    }
    try {
      // Lightweight cycle detection: if the "to" agent already reports to the "from" agent
      // (or any of its ancestors), that would create a cycle.
      const res = await apiClient.get<{ has_cycle: boolean; path: string[] }>(
        '/api/v1/agent-relationships/check-cycle',
        { from_agent_id: fromAgentId, to_agent_id: toAgentId },
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

  const needsChannel = CHANNEL_SCOPED_TYPES.includes(relType);

  // Build agent select options
  const agentOptions: SelectOption[] = agents
    .filter((a) => a.id !== fromAgentId) // Can't select self as target
    .map((a) => ({
      value: a.id,
      label: `${a.name}${a.is_active ? '' : ' (offline)'}`,
      disabled: !a.is_active && relType === 'delegates_to',
    }));

  const fromAgentOptions: SelectOption[] = agents.map((a) => ({
    value: a.id,
    label: a.name,
  }));

  const channelOptions: SelectOption[] = needsChannel
    ? channels.map((c) => ({ value: c.id, label: `#${c.name}` }))
    : [
        { value: '', label: '-- No channel (global) --' },
        ...channels.map((c) => ({ value: c.id, label: `#${c.name}` })),
      ];

  const canSubmit =
    fromAgentId &&
    toAgentId &&
    fromAgentId !== toAgentId &&
    !isSubmitting &&
    !cycleWarning &&
    (needsChannel ? !!channelId : true);

  const handleSubmit = async () => {
    if (!canSubmit) return;
    setIsSubmitting(true);
    setError(null);
    try {
      await apiClient.post('/api/v1/agent-relationships', {
        from_agent_id: fromAgentId,
        to_agent_id: toAgentId,
        rel_type: relType,
        channel_id: needsChannel && channelId ? channelId : undefined,
      });
      onCreated();
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create relationship');
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
        {/* From Agent */}
        <div>
          <label className="block font-heading text-xs font-bold uppercase tracking-wider mb-1.5">
            {t('relationshipEditorFrom')}
          </label>
          <Select
            options={fromAgentOptions}
            value={fromAgentId}
            onChange={setFromAgentId}
            placeholder={preselectedFrom ? agents.find((a) => a.id === preselectedFrom)?.name : 'Select agent...'}
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
            placeholder={preselectedTo ? agents.find((a) => a.id === preselectedTo)?.name : 'Select agent...'}
            size="md"
            className="w-full"
            disabled={!!preselectedTo}
          />
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
                    strokeWidth={opt.type === 'escalates_to' ? 3 : 2}
                    strokeDasharray={opt.dash || undefined}
                  />
                  {opt.type === 'escalates_to' && (
                    <line x1="0" y1="1" x2="28" y2="1" stroke={opt.color} strokeWidth={2} />
                  )}
                </svg>
                <span className="font-heading text-xs font-bold uppercase tracking-wider text-black">
                  {t(opt.labelKey as Parameters<typeof t>[0])}
                </span>
              </button>
            ))}
          </div>
        </div>

        {/* Channel selector (shown for channel-scoped types) */}
        {needsChannel && (
          <div>
            <label className="block font-heading text-xs font-bold uppercase tracking-wider mb-1.5">
              {t('relationshipEditorChannel')}
            </label>
            <Select
              options={channelOptions}
              value={channelId}
              onChange={setChannelId}
              placeholder={t('relationshipEditorChannelPlaceholder')}
              size="md"
              className="w-full"
            />
            <p className="mt-1 font-mono text-[10px] text-muted-foreground">
              {relType === 'delegates_to'
                ? 'Delegations are channel-scoped. Select a channel to scope this relationship.'
                : 'Collaborations are channel-scoped. Select a channel to scope this relationship.'}
            </p>
          </div>
        )}

        {/* Warning for global types */}
        {GLOBAL_TYPES.includes(relType) && (
          <div className="flex items-start gap-2 px-3 py-2 border-2 border-black bg-brutal-info-light">
            <AlertTriangle className="h-4 w-4 flex-shrink-0 mt-0.5 text-brutal-info" />
            <p className="font-mono text-[10px] text-black">
              {relType === 'reports_to'
                ? 'Reports-to relationships are global and apply across all channels.'
                : 'Escalation relationships are global and apply across all channels.'}
            </p>
          </div>
        )}

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
        <button
          type="button"
          onClick={() => onOpenChange(false)}
          className="btn-brutal-sm px-4 py-1.5"
        >
          {t('cancel')}
        </button>
        <button
          type="button"
          onClick={handleSubmit}
          disabled={!canSubmit || !!cycleWarning}
          className="btn-brutal-sm bg-brutal-success text-black px-4 py-1.5 disabled:opacity-50 disabled:pointer-events-none"
        >
          {isSubmitting ? (
            <span className="flex items-center gap-1.5">
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
              {t('submitting')}
            </span>
          ) : (
            t('create')
          )}
        </button>
      </DialogFooter>
    </Dialog>
  );
}
