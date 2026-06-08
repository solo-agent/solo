// ============================================================================
// AgentProfileTab — inline-edit Agent profile fields (v1.5)
// - Each editable field has a pencil icon -> click to edit -> save/cancel
// - Read-only: computer name, created_at, created_by
// - Status toggle: enable/disable switch
// ============================================================================

'use client';

import { useEffect, useState, useCallback } from 'react';
import { Pencil, Check, X, Circle, AlertCircle, RefreshCw, Bot } from 'lucide-react';
import { apiClient, ApiError } from '@/lib/api-client';
import { useToast } from '@/components/ui/toast';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { t } from '@/lib/i18n';
import type { Agent } from '@/lib/types';

interface AgentProfileTabProps {
  agentId: string;
}

// ---- Inline editable field component ----

function InlineTextField({
  label,
  value,
  onSave,
  type = 'text',
  placeholder = '',
  multiline = false,
}: {
  label: string;
  value: string;
  onSave: (val: string) => Promise<void>;
  type?: string;
  placeholder?: string;
  multiline?: boolean;
}) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value);
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    setSaving(true);
    try {
      await onSave(draft);
      setEditing(false);
    } catch {
      // Error handled by parent
    } finally {
      setSaving(false);
    }
  };

  const handleCancel = () => {
    setDraft(value);
    setEditing(false);
  };

  if (editing) {
    return (
      <div className="space-y-1.5">
        <span className="font-heading text-xs font-bold text-muted-foreground uppercase tracking-wider">
          {label}
        </span>
        {multiline ? (
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            className="input-brutal min-h-[80px] w-full resize-y font-body text-sm"
            disabled={saving}
          />
        ) : (
          <input
            type={type}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder={placeholder}
            className="input-brutal h-9 w-full font-body text-sm"
            disabled={saving}
          />
        )}
        <div className="flex items-center gap-1.5">
          <button
            type="button"
            onClick={handleSave}
            disabled={saving || draft === value}
            className="btn-brutal btn-brutal-sm h-7 px-2 text-xs"
          >
            {saving ? '...' : <><Check className="mr-1 h-3 w-3" />{t('save')}</>}
          </button>
          <button
            type="button"
            onClick={handleCancel}
            disabled={saving}
            className="btn-flat h-7 text-xs"
          >
            <X className="mr-1 h-3 w-3" />
            {t('cancel')}
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-1">
      <span className="font-heading text-xs font-bold text-muted-foreground uppercase tracking-wider">
        {label}
      </span>
      <div className="flex items-center gap-2 group">
        <span className="font-body text-sm text-foreground min-w-0 flex-1">
          {value || <span className="italic text-muted-foreground">{placeholder || t('agentProfileNotSet')}</span>}
        </span>
        <button
          type="button"
          onClick={() => {
            setDraft(value);
            setEditing(true);
          }}
          className="flex h-6 w-6 items-center justify-center opacity-0 group-hover:opacity-100 border-2 border-black bg-white shadow-brutal-sm transition-opacity"
          aria-label={t('agentProfileEdit', { label })}
        >
          <Pencil className="h-3 w-3" />
        </button>
      </div>
    </div>
  );
}

// ---- Status toggle ----

function StatusToggle({
  active,
  onToggle,
}: {
  active: boolean;
  onToggle: (active: boolean) => Promise<void>;
}) {
  const [loading, setLoading] = useState(false);

  const handleToggle = async () => {
    setLoading(true);
    try {
      await onToggle(!active);
    } catch {
      // handled
    } finally {
      setLoading(false);
    }
  };

  const statusColor = active
    ? 'bg-brutal-success'
    : 'bg-brutal-muted';

  return (
    <div className="flex items-center justify-between">
      <div className="flex items-center gap-2">
        <Circle className={`h-3 w-3 ${active ? 'fill-brutal-success text-brutal-success' : 'fill-brutal-muted text-brutal-muted'}`} />
        <span className="font-heading text-xs font-bold text-muted-foreground uppercase tracking-wider">
          {active ? t('agentProfileEnabled') : t('agentProfileDisabled')}
        </span>
      </div>
      <button
        type="button"
        onClick={handleToggle}
        disabled={loading}
        className={`relative flex h-7 w-11 flex-shrink-0 items-center border-2 border-black transition-colors ${statusColor}`}
        role="switch"
        aria-checked={active}
        aria-label={active ? t('agentProfileDisable') : t('agentProfileEnable')}
      >
        <span
          className={`absolute h-7 w-[18px] border-r-2 border-l-2 border-black bg-white transition-all ${active ? 'left-[calc(100%-18px)]' : 'left-0'}`}
        />
      </button>
    </div>
  );
}

// ---- Date formatting ----

function formatDate(iso: string): string {
  try {
    const d = new Date(iso);
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
  } catch {
    return iso;
  }
}

// ---- Component ----

export function AgentProfileTab({ agentId }: AgentProfileTabProps) {
  const [agent, setAgent] = useState<Agent | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const { showToast } = useToast();

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
        temperature: (res.temperature as number) ?? 0.7,
        max_tokens: (res.max_tokens as number) ?? 4096,
        is_active: (res.is_active as boolean) ?? false,
        auto_join: (res.auto_join as boolean) ?? false,
        avatar_url: (res.avatar_url as string) || null,
        enabled_tools: (res.enabled_tools as string[]) ?? [],
        interaction_mode: (res.interaction_mode as string) ?? 'mention',
        custom_env: (res.custom_env as Record<string, string>) ?? {},
        custom_args: (res.custom_args as string[]) ?? [],
        created_at: res.created_at as string,
        updated_at: res.updated_at as string,
      } as Agent);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.status === 404 ? t('agentProfileAgentNotFound') : err.message);
      } else {
        setError(t('agentProfileError'));
      }
    } finally {
      setIsLoading(false);
    }
  }, [agentId]);

  useEffect(() => {
    loadAgent();
  }, [loadAgent]);

  const handleUpdate = useCallback(
    async (field: string, value: string | boolean | number) => {
      const body: Record<string, unknown> = { [field]: value };
      await apiClient.patch(`/api/v1/agents/${agentId}`, body);
      setAgent((prev) => prev ? { ...prev, [field]: value } : null);
      showToast(t('agentProfileUpdateSuccess'), 'success');
    },
    [agentId, showToast],
  );

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-16 w-full rounded-none" />
        <Skeleton className="h-10 w-full rounded-none" />
        <Skeleton className="h-10 w-full rounded-none" />
        <Skeleton className="h-[120px] w-full rounded-none" />
        <Skeleton className="h-10 w-full rounded-none" />
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

  return (
    <div className="space-y-5">
      {/* Avatar + Name */}
      <div className="flex items-center gap-3">
        <PixelAvatar agentId={agent.id} avatarUrl={agent.avatar_url} size="md" />
        <div>
          <h3 className="font-heading font-bold text-base text-foreground">{agent.name}</h3>
          <p className="font-mono text-[11px] text-muted-foreground">{agent.model_provider || t('agentProfileNoRuntime')}</p>
        </div>
      </div>

      <hr className="divider-brutal" />

      {/* Status toggle */}
      <StatusToggle
        active={agent.is_active}
        onToggle={(active) => handleUpdate('is_active', active)}
      />

      <hr className="divider-brutal" />

      {/* Editable fields */}
      <InlineTextField
        label={t('agentFormName')}
        value={agent.name}
        onSave={(val) => handleUpdate('name', val)}
        placeholder={t('agentFormNamePlaceholder')}
      />

      <InlineTextField
        label={t('agentFormDesc')}
        value={agent.description}
        onSave={(val) => handleUpdate('description', val)}
        placeholder={t('agentFormDescPlaceholder')}
        multiline
      />

      <InlineTextField
        label={t('agentFormSystemPrompt')}
        value={agent.system_prompt}
        onSave={(val) => handleUpdate('system_prompt', val)}
        placeholder={t('agentFormSystemPromptPlaceholder')}
        multiline
      />

      <InlineTextField
        label={t('agentFormLabelModel')}
        value={agent.model_name}
        onSave={(val) => handleUpdate('model_name', val)}
        placeholder={t('agentFormModelPlaceholder')}
      />

      <hr className="divider-brutal" />

      {/* Read-only metadata */}
      <div className="space-y-2">
        <h4 className="font-heading text-xs font-bold text-muted-foreground uppercase tracking-wider">
          {t('agentProfileMeta')}
        </h4>
        <div className="space-y-1.5">
          <div className="flex items-center justify-between">
            <span className="font-mono text-[11px] text-muted-foreground">Computer Name</span>
            <span className="font-mono text-[11px] text-foreground">
              {agent.owner_id?.slice(0, 8) ?? '—'}
            </span>
          </div>
          <div className="flex items-center justify-between">
            <span className="font-mono text-[11px] text-muted-foreground">{t('agentProfileCreatedAt')}</span>
            <span className="font-mono text-[11px] text-foreground">{formatDate(agent.created_at)}</span>
          </div>
          <div className="flex items-center justify-between">
            <span className="font-mono text-[11px] text-muted-foreground">{t('agentProfileCreatedBy')}</span>
            <span className="font-mono text-[11px] text-foreground">{agent.owner_id?.slice(0, 8) ?? '—'}</span>
          </div>
        </div>
      </div>
    </div>
  );
}
