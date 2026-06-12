// ============================================================================
// AgentProfileTab — inline-edit Agent profile fields (v1.5)
// - Each editable field has a pencil icon -> click to edit -> save/cancel
// - Read-only: computer name, created_at, created_by
// - Status toggle: enable/disable switch
// ============================================================================

'use client';

import { useEffect, useState, useCallback } from 'react';
import Link from 'next/link';
import { Pencil, Check, X, AlertCircle, RefreshCw } from 'lucide-react';
import { apiClient, ApiError } from '@/lib/api-client';
import { useToast } from '@/components/ui/toast';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { PixelAvatar } from '@/components/ui/pixel-avatar';
import { Decoration } from '@/components/ui/decoration';
import { BrutalSeparator } from '@/components/ui/brutal-separator';
import { useComputers } from '@/lib/hooks/use-computers';
import { cn } from '@/lib/utils';
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
      <span className="inline-block bg-brutal-primary-light border-2 border-black px-1.5 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider text-black">
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
      {/* v3.3: status pill is now a chunky badge-brutal instead of a
          thin dot + muted text — adds a saturated status color block
          to the panel without changing the toggle's interaction. */}
      <span
        className={cn(
          'inline-flex items-center gap-1.5 border-2 border-black px-2 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider',
          active ? 'bg-brutal-success text-black' : 'bg-brutal-muted text-black',
        )}
      >
        <span
          className={cn(
            'h-2 w-2 border border-black',
            active ? 'bg-white' : 'bg-black',
          )}
          aria-hidden
        />
        {active ? t('agentProfileEnabled') : t('agentProfileDisabled')}
      </span>
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
  // v3.3: reverse-lookup the computer that owns this agent so the META
  // block can show "Connected Computer: <name>".
  const { computers } = useComputers();
  const connectedComputer = computers.find((c) => c.agent_ids?.includes(agentId));

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
        is_active: (res.is_active as boolean) ?? false,
        avatar_url: (res.avatar_url as string) || null,
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
      {/* Avatar + Name — v3.3: framed avatar (2px border + cream
          backdrop + sticker tilt) anchors the header; violet rotating
          star sticker adds a complementary color accent. */}
      <div className="relative">
        <div className="flex items-center gap-3">
          <div
            className="border-2 border-black shadow-brutal-sm bg-brutal-cream"
            style={{ transform: 'rotate(-2deg)' }}
          >
            <PixelAvatar agentId={agent.id} avatarUrl={agent.avatar_url} size="md" />
          </div>
          <div>
            <h3 className="font-heading font-bold text-base text-foreground">{agent.name}</h3>
            <p className="font-mono text-[11px] text-muted-foreground">{agent.model_provider || t('agentProfileNoRuntime')}</p>
          </div>
        </div>
        <Decoration
          shape="star"
          color="violet"
          size="sm"
          animation="spin"
          rotation={14}
          className="absolute -top-3 -right-3"
        />
      </div>

      <BrutalSeparator />

      {/* Status toggle */}
      <StatusToggle
        active={agent.is_active}
        onToggle={(active) => handleUpdate('is_active', active)}
      />

      <BrutalSeparator />

      {/* v3.3: Name field removed (the avatar/header above already
          shows the name). Description + System Prompt are grouped under
          a single `★ INFO` tilted sticker section. */}
      <div className="space-y-2">
        <h4>
          <span
            className="inline-flex items-center gap-1.5 border-2 border-black bg-brutal-primary px-2.5 py-1 font-heading text-[11px] font-black uppercase tracking-widest text-black shadow-brutal-sm"
            style={{ transform: 'rotate(-0.8deg)' }}
          >
            ★ {t('agentProfileInfo')}
          </span>
        </h4>
        <div className="space-y-4">
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
        </div>
      </div>

      <BrutalSeparator />

      {/* Read-only metadata — v3.3: bare layout (no card-brutal wrapper)
          to match the chunky-sticker-title + naked-fields style of the
          Computers detail. Section header is a tilted primary chip. */}
      <div className="space-y-2">
        <h4>
          <span
            className="inline-flex items-center gap-1.5 border-2 border-black bg-brutal-primary px-2.5 py-1 font-heading text-[11px] font-black uppercase tracking-widest text-black shadow-brutal-sm"
            style={{ transform: 'rotate(-0.8deg)' }}
          >
            ★ {t('agentProfileMeta')}
          </span>
        </h4>
        <div className="space-y-1">
          <div className="flex items-center gap-3 py-1.5">
            <span className="inline-block bg-brutal-primary-light border-2 border-black px-1.5 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider text-black flex-shrink-0">
              ID
            </span>
            <span className="font-mono text-xs text-foreground">
              {agent.owner_id?.slice(0, 8) ?? '—'}
            </span>
          </div>
          <div className="flex items-center gap-3 py-1.5">
            <span className="inline-block bg-brutal-primary-light border-2 border-black px-1.5 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider text-black flex-shrink-0">
              {t('agentProfileCreatedAt')}
            </span>
            <span className="font-mono text-xs text-foreground">{formatDate(agent.created_at)}</span>
          </div>
          <div className="flex items-center gap-3 py-1.5">
            <span className="inline-block bg-brutal-primary-light border-2 border-black px-1.5 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider text-black flex-shrink-0">
              {t('agentProfileCreatedBy')}
            </span>
            <span className="font-mono text-xs text-foreground">{agent.owner_id?.slice(0, 8) ?? '—'}</span>
          </div>
          <div className="flex items-center gap-3 py-1.5">
            <span className="inline-block bg-brutal-primary-light border-2 border-black px-1.5 py-0.5 font-heading text-[10px] font-bold uppercase tracking-wider text-black flex-shrink-0">
              Computer
            </span>
            {connectedComputer ? (
              <Link
                href="/computers"
                className="font-mono text-xs text-foreground underline decoration-dotted underline-offset-2 hover:text-brutal-primary transition-colors"
              >
                {connectedComputer.name}
              </Link>
            ) : (
              <span className="font-mono text-xs italic text-muted-foreground">Not connected</span>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
