// ============================================================================
// AgentSkillsTab — read-only skill catalog
// - Data source: useSkills() (daemon-synced, DB-backed)
// - All agents see the same catalog; skills are tagged with provider kind
// - Click a row: opens SkillDetailDrawer
// ============================================================================

'use client';

import { useState } from 'react';
import { AlertCircle, Puzzle } from 'lucide-react';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { useSkills } from '@/lib/hooks/use-skills';
import { SkillDetailDrawer } from './skill-detail-drawer';
import type { SkillSummary } from '@/lib/types';

interface AgentSkillsTabProps {
  agentId: string;
}

const KIND_LABELS: Record<string, string> = {
  claude: 'Claude',
  codex: 'Codex',
  opencode: 'OpenCode',
  copilot: 'Copilot',
  cursor: 'Cursor',
  kiro: 'Kiro',
  openclaw: 'OpenClaw',
  hermes: 'Hermes',
  pi: 'Pi',
  agents: 'Agent',
  'claude-compat': 'Claude',
};

function kindLabel(kind: string): string {
  return KIND_LABELS[kind] || kind.toUpperCase();
}

export function AgentSkillsTab({ agentId: _agentId }: AgentSkillsTabProps) {
  const { skills: catalog, isLoading, error, refetch } = useSkills();
  const [drawerSkillId, setDrawerSkillId] = useState<string | null>(null);

  if (isLoading) {
    return (
      <div className="space-y-3">
        {[1, 2, 3, 4].map((i) => (
          <div key={i} className="flex items-center gap-3 border-2 border-black bg-white p-4 shadow-brutal-sm">
            <Skeleton className="h-4 w-4 rounded-none flex-shrink-0" />
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
        <Button type="button" onClick={refetch} size="sm" className="mt-4">
          重试
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div>
        <div className="flex items-center gap-2">
          <Puzzle className="h-4 w-4" />
          <h3 className="font-heading text-xs font-bold text-muted-foreground uppercase tracking-wider">
            Skill 目录
          </h3>
          <span className="font-mono text-[10px] tabular-nums text-muted-foreground/70">
            {catalog.length}
          </span>
        </div>
        <p className="mt-1 font-mono text-[11px] text-muted-foreground">
          由 Daemon 自动扫描并同步。显示机器上所有已安装的 Skill。
        </p>
      </div>

      {catalog.length === 0 ? (
        <div className="card-brutal bg-brutal-cream p-6 text-center">
          <p className="font-mono text-sm text-foreground">还没有发现任何 Skill</p>
          <p className="mt-2 font-mono text-[11px] text-muted-foreground">
            在 <code className="bg-white px-1.5 py-0.5 border border-black">~/.claude/skills/</code> 或 <code className="bg-white px-1.5 py-0.5 border border-black">~/.codex/skills/</code> 下放置 SKILL.md，Daemon 下次心跳自动同步
          </p>
        </div>
      ) : (
        <div className="divide-y-2 divide-black border-2 border-black shadow-brutal-sm">
          {catalog.map((skill) => (
            <button
              key={skill.id}
              type="button"
              onClick={() => setDrawerSkillId(skill.id)}
              className="flex w-full items-center gap-3 bg-white px-4 py-3 text-left hover:bg-gray-50"
            >
              <Puzzle className="h-4 w-4 flex-shrink-0 text-muted-foreground" />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 flex-wrap">
                  <span className="font-heading text-sm font-bold text-foreground">
                    {skill.name}
                  </span>
                  <span className="badge-brutal text-[10px] bg-brutal-cream text-foreground px-1.5">
                    {kindLabel(skill.source_kind)}
                  </span>
                </div>
                {skill.description && (
                  <p className="mt-0.5 font-mono text-[11px] text-muted-foreground leading-relaxed line-clamp-2">
                    {skill.description}
                  </p>
                )}
              </div>
            </button>
          ))}
        </div>
      )}

      <SkillDetailDrawer
        skillId={drawerSkillId}
        onClose={() => setDrawerSkillId(null)}
      />
    </div>
  );
}
