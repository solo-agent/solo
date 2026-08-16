'use client';

import { useCallback, useEffect, useState } from 'react';
import { Gauge, Save } from 'lucide-react';
import { apiClient, ApiError } from '@/lib/api-client';
import { t } from '@/lib/i18n';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { useToast } from '@/components/ui/toast';
import { useWebSocket } from '@/lib/ws-context';

export interface BudgetPeriodSummary {
  limit_tokens: number;
  used_tokens: number;
  reserved_tokens: number;
  remaining_tokens: number;
  blocked: boolean;
}

export interface BudgetView {
  policy: {
    enabled: boolean;
    monthly_limit_tokens: number;
  };
  month: BudgetPeriodSummary;
  unit: 'token';
  period_timezone: string;
  pause_reason?: string;
}

type Draft = {
  enabled: boolean;
  monthlyWan: string;
};

const emptyDraft: Draft = {
  enabled: false,
  monthlyWan: '',
};

const tokensPerWan = 10_000;

export function BudgetSettingsCard({ agentId, compact = false }: { agentId?: string; compact?: boolean }) {
  const { showToast } = useToast();
  const { onEvent } = useWebSocket();
  const [view, setView] = useState<BudgetView | null>(null);
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const endpoint = agentId ? `/api/v1/agents/${agentId}/budget` : '/api/v1/users/me/budget';

  const load = useCallback((silent = false) => {
    if (!silent) setLoading(true);
    return apiClient.get<BudgetView>(endpoint)
      .then((next) => {
        setView(next);
        if (!silent) setDraft(draftFromView(next));
      })
      .catch((error) => {
        if (!silent) showToast(budgetErrorMessage(error, 'budgetLoadError'), 'error');
      })
      .finally(() => {
        if (!silent) setLoading(false);
      });
  }, [endpoint, showToast]);

  useEffect(() => { void load(); }, [load]);

  useEffect(() => onEvent((event) => {
    if (event.type !== 'agent.run.finished') return;
    if (agentId && event.agent_id !== agentId) return;
    void load(true);
  }), [agentId, load, onEvent]);

  async function save() {
    const monthly = parseWanTokenInput(draft.monthlyWan);
    if (monthly === null || (draft.enabled && monthly === 0)) {
      showToast(t('budgetInvalidAmount'), 'error');
      return;
    }
    setSaving(true);
    try {
      const next = await apiClient.put<BudgetView>(endpoint, {
        enabled: draft.enabled,
        monthly_limit_tokens: monthly,
      });
      setView(next);
      setDraft(draftFromView(next));
      showToast(t('budgetSaved'), 'success');
    } catch (error) {
      showToast(budgetErrorMessage(error, 'budgetSaveError'), 'error');
    } finally {
      setSaving(false);
    }
  }

  return (
    <section data-testid={agentId ? 'agent-budget-card' : 'user-budget-card'} className={cn(compact ? 'border-2 border-black bg-brutal-cream' : 'card-brutal-heavy mt-6')}>
      <div data-testid="budget-card-header" className="flex flex-wrap items-center justify-between gap-3 border-b-2 border-black bg-brutal-primary px-4 py-3 text-foreground">
        <div className="flex items-center gap-2">
          <Gauge className="h-5 w-5" />
          <div>
            <h2 className="font-heading text-sm font-black text-foreground">{agentId ? t('agentBudgetTitle') : t('budgetTitle')}</h2>
            <p className="font-mono text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('budgetUtcHint')}</p>
          </div>
        </div>
        <button
          type="button"
          role="switch"
          aria-checked={draft.enabled}
          onClick={() => setDraft((current) => ({ ...current, enabled: !current.enabled }))}
          className={cn('flex items-center gap-2 border-2 border-black px-2 py-1 font-mono text-xs font-bold text-black shadow-brutal-sm', draft.enabled ? 'bg-brutal-success' : 'bg-white')}
        >
          <span className={cn('h-3 w-3 border-2 border-black', draft.enabled ? 'bg-black' : 'bg-white')} />
          {draft.enabled ? t('enabled') : t('disabled')}
        </button>
      </div>

      <div className="space-y-4 p-4">
        {view?.pause_reason && (
          <div role="alert" className="border-2 border-black bg-brutal-warning px-3 py-2 font-mono text-xs font-bold">
            {budgetPauseReason(view.pause_reason)}
          </div>
        )}

        {view && (
          <div>
            <BudgetMeter label={t('budgetMonth')} summary={view.month} enabled={view.policy.enabled} />
          </div>
        )}

        <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
          <TokenInput
            label={t('budgetMonthlyLimit')}
            value={draft.monthlyWan}
            onChange={(monthlyWan) => setDraft((current) => ({ ...current, monthlyWan }))}
          />
          <Button type="button" onClick={save} disabled={loading || saving} className="gap-2">
            <Save className="h-4 w-4" />
            {saving ? t('saving') : t('save')}
          </Button>
        </div>

        <p className="font-mono text-[11px] text-muted-foreground">
          {agentId ? t('budgetLimitNote') : t('budgetUserLimitNote')}
        </p>
      </div>
    </section>
  );
}

function TokenInput({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return (
    <div>
      <Label className="font-mono text-[10px] font-bold uppercase tracking-wider">{label}</Label>
      <div className="mt-1 flex items-stretch">
        <Input aria-label={label} type="number" inputMode="decimal" min="0.0001" step="0.0001" value={value} onChange={(event) => onChange(event.target.value)} placeholder="100" className="min-w-0" />
        <span className="flex items-center border-y-2 border-r-2 border-black bg-white px-2 font-mono text-[10px] font-bold">{t('budgetMonthlyUnit')}</span>
      </div>
      <p className="mt-1 font-body text-[11px] text-muted-foreground">{t('budgetMonthlyInputHint')}</p>
    </div>
  );
}

function BudgetMeter({ label, summary, enabled }: { label: string; summary: BudgetPeriodSummary; enabled: boolean }) {
  const total = summary.used_tokens + summary.reserved_tokens;
  const percent = enabled && summary.limit_tokens > 0 ? Math.min(100, (total / summary.limit_tokens) * 100) : 0;
  return (
    <div className="border-2 border-black bg-white p-3">
      <div className="flex items-center justify-between gap-2 font-mono text-xs font-bold">
        <span>{label}</span>
        <span>{enabled ? `${formatTokens(total)} / ${formatTokens(summary.limit_tokens)}` : t('budgetNotLimited')}</span>
      </div>
      <div className="mt-2 h-3 border-2 border-black bg-brutal-cream">
        <div className={cn('h-full', summary.blocked ? 'bg-brutal-danger' : 'bg-brutal-primary')} style={{ width: `${percent}%` }} />
      </div>
      <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 font-mono text-[10px] text-muted-foreground">
        <span>{t('budgetUsed')} {formatTokens(summary.used_tokens)}</span>
        <span>{t('budgetReserved')} {formatTokens(summary.reserved_tokens)}</span>
        {enabled && <span>{t('budgetRemaining')} {formatTokens(summary.remaining_tokens)}</span>}
      </div>
    </div>
  );
}

function draftFromView(view: BudgetView): Draft {
  return {
    enabled: view.policy.enabled,
    monthlyWan: view.policy.monthly_limit_tokens > 0
      ? String(view.policy.monthly_limit_tokens / tokensPerWan)
      : '',
  };
}

function parseWanTokenInput(value: string): number | null {
  const trimmed = value.trim();
  if (trimmed === '') return 0;
  if (!/^\d+(?:\.\d{1,4})?$/.test(trimmed)) return null;
  const [whole, fraction = ''] = trimmed.split('.');
  const tokens = Number(whole) * tokensPerWan + Number(fraction.padEnd(4, '0'));
  if (!Number.isSafeInteger(tokens) || tokens <= 0) return null;
  return tokens;
}

function budgetErrorMessage(error: unknown, fallback: 'budgetLoadError' | 'budgetSaveError'): string {
  if (!(error instanceof ApiError)) return t(fallback);
  if (error.code === 'budget.error.invalid_request') return t('budgetInvalidRequest');
  if (error.code === 'budget.error.monthly_limit_invalid') return t('budgetInvalidAmount');
  if (error.code === 'budget.error.monthly_limit_required') return t('budgetMonthlyLimitRequired');
  return error.message || t(fallback);
}

function budgetPauseReason(reason: string): string {
  if (reason === 'budget.error.monthly_limit_exhausted') return t('budgetMonthlyExhausted');
  return reason;
}

export function formatTokens(tokens: number): string {
  return `${new Intl.NumberFormat().format(tokens)} Token`;
}
