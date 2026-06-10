// ============================================================================
// AgentSkillsTab — toggle agent's skill bindings against the DB catalog
// - Skills are synced by the daemon on each heartbeat (every 30s)
// - Data source: useAgentSkills(agentId) + useSkills() (DB-backed)
// - Toggle: useSetAgentSkills(agentId) with optimistic update + resync on settle
// - Click a row: opens SkillDetailDrawer
// ============================================================================

'use client';

import { useState, useCallback, useMemo } from 'react';
import { AlertCircle, Puzzle } from 'lucide-react';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import {
  useAgentSkills,
  useSkills,
  useSetAgentSkills,
} from '@/lib/hooks/use-skills';
import { SkillDetailDrawer } from './skill-detail-drawer';
import type { SkillSummary } from '@/lib/types';

interface AgentSkillsTabProps {
  agentId: string;
}

export function AgentSkillsTab({ agentId }: AgentSkillsTabProps) {
  const { skills: catalog, isLoading: catalogLoading, error: catalogError, refetch: refetchCatalog } = useSkills();
  const { skills: bindings, isLoading: bindingsLoading, error: bindingsError, refetch: refetchBindings } = useAgentSkills(agentId);
  const { mutate: setMutate, isPending: setting } = useSetAgentSkills(agentId);

  const [savingIds, setSavingIds] = useState<Set<string>>(new Set());
  const [drawerSkillId, setDrawerSkillId] = useState<string | null>(null);

  const boundIds = useMemo(() => new Set(bindings.map((s) => s.id)), [bindings]);

  const isLoading = catalogLoading || bindingsLoading;
  const error = catalogError || bindingsError;

  const handleToggle = useCallback(
    async (skillId: string) => {
      const isCurrentlyBound = boundIds.has(skillId);
      const next = isCurrentlyBound
        ? bindings.filter((s) => s.id !== skillId).map((s) => s.id)
        : [...bindings.map((s) => s.id), skillId];

      setSavingIds((prev) => new Set(prev).add(skillId));
      try {
        await setMutate(next);
        await refetchBindings();
      } finally {
        setSavingIds((prev) => {
          const nextSet = new Set(prev);
          nextSet.delete(skillId);
          return nextSet;
        });
      }
    },
    [boundIds, bindings, setMutate, refetchBindings],
  );

  if (isLoading) {
    return (
      <div className="space-y-3">
        {[1, 2, 3, 4].map((i) => (
          <div key={i} className="flex items-center gap-3 border-2 border-black bg-white p-4 shadow-brutal-sm">
            <Skeleton className="h-7 w-11 rounded-none flex-shrink-0" />
            <div className="flex-1 space-y-1.5">
              <Skeleton className="h-4 w-24 rounded-none" />
              <Skeleton className="h-3 w-40 rounded-none" />
            </div>
          </div>
        ))}
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
        <Button type="button" onClick={() => { refetchCatalog(); refetchBindings(); }} size="sm" className="mt-4">
          <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
          重试
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Section header */}
      <div>
        <div className="flex items-center gap-2">
          <Puzzle className="h-4 w-4" />
          <h3 className="font-heading text-xs font-bold text-muted-foreground uppercase tracking-wider">
            Agent Skills
          </h3>
          <span className="font-mono text-[10px] tabular-nums text-muted-foreground/70">
            {bindings.length}/{catalog.length}
          </span>
        </div>
        <p className="mt-1 font-mono text-[11px] text-muted-foreground">
          切换开关来启用/禁用 Agent 拥有的 Skill。技能由 Daemon 自动扫描并同步。
        </p>
      </div>

      {/* Empty state */}
      {catalog.length === 0 && (
        <div className="card-brutal bg-brutal-cream p-6 text-center">
          <p className="font-mono text-sm text-foreground">还没有发现任何 Skill</p>
          <p className="mt-2 font-mono text-[11px] text-muted-foreground">
            在 <code className="bg-white px-1.5 py-0.5 border border-black">~/.claude/skills/</code> 或 <code className="bg-white px-1.5 py-0.5 border border-black">~/.codex/skills/</code> 下放置含 <code className="bg-white px-1.5 py-0.5 border border-black">SKILL.md</code> 的目录，Daemon 下次心跳时自动同步（最多 30 秒）
          </p>
        </div>
      )}

      {/* Skill list */}
      {catalog.length > 0 && (
        <div className="divide-y-2 divide-black border-2 border-black shadow-brutal-sm">
          {catalog.map((skill) => (
            <SkillRow
              key={skill.id}
              skill={skill}
              isBound={boundIds.has(skill.id)}
              isSaving={savingIds.has(skill.id) || setting}
              onToggle={() => handleToggle(skill.id)}
              onOpen={() => setDrawerSkillId(skill.id)}
            />
          ))}
        </div>
      )}

      {/* Drawer */}
      <SkillDetailDrawer
        skillId={drawerSkillId}
        onClose={() => setDrawerSkillId(null)}
      />
    </div>
  );
}

function SkillRow({
  skill,
  isBound,
  isSaving,
  onToggle,
  onOpen,
}: {
  skill: SkillSummary;
  isBound: boolean;
  isSaving: boolean;
  onToggle: () => void;
  onOpen: () => void;
}) {
  return (
    <div className="flex items-center gap-3 bg-white px-4 py-3">
      {/* Toggle switch */}
      <button
        type="button"
        onClick={onToggle}
        disabled={isSaving}
        className={cn(
          'relative flex h-7 w-11 flex-shrink-0 items-center border-2 border-black transition-colors',
          isSaving ? 'opacity-50 cursor-wait' : '',
          isBound ? 'bg-brutal-success' : 'bg-brutal-muted',
        )}
        role="switch"
        aria-checked={isBound}
        aria-label={`${isBound ? '禁用' : '启用'} ${skill.name}`}
      >
        <span
          className={cn(
            'absolute h-7 w-[18px] border-r-2 border-l-2 border-black bg-white transition-all',
            isBound ? 'left-[calc(100%-18px)]' : 'left-0',
          )}
        />
      </button>

      {/* Skill info (clickable) */}
      <button
        type="button"
        onClick={onOpen}
        className="min-w-0 flex-1 text-left"
      >
        <div className="flex items-center gap-2 flex-wrap">
          <span className="font-heading text-sm font-bold text-foreground">
            {skill.name}
          </span>
          <span className="badge-brutal text-[10px] bg-brutal-cream text-foreground px-1.5">
            {skill.source_kind}
          </span>
          <span
            className={cn(
              'badge-brutal text-[10px] px-1.5',
              isSaving
                ? 'bg-brutal-muted text-white'
                : isBound
                  ? 'bg-brutal-success text-black'
                  : 'bg-brutal-muted text-white',
            )}
          >
            {isSaving ? '保存中…' : isBound ? '已启用' : '未启用'}
          </span>
        </div>
        {skill.description && (
          <p className="mt-0.5 font-mono text-[11px] text-muted-foreground leading-relaxed line-clamp-2">
            {skill.description}
          </p>
        )}
      </button>
    </div>
  );
}
