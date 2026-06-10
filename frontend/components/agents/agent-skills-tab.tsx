// ============================================================================
// AgentSkillsTab — read-only skill catalog, filtered by agent provider
// - Skills are grouped into Global and Workspace sections
// - Filtered to only show skills compatible with the agent's provider type
// - Click a row: opens SkillDetailDrawer
// ============================================================================

'use client';

import { useState, useMemo, useEffect } from 'react';
import { AlertCircle, Puzzle, Globe, Folder } from 'lucide-react';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { useSkills } from '@/lib/hooks/use-skills';
import { apiClient } from '@/lib/api-client';
import { SkillDetailDrawer } from './skill-detail-drawer';
import type { SkillSummary } from '@/lib/types';

interface AgentSkillsTabProps {
  agentId: string;
  agentProvider?: string;
}

// Which skill kinds are visible to each agent provider type.
// opencode also reads claude-ecosystem paths.
const PROVIDER_KINDS: Record<string, string[]> = {
  claude: ['claude', 'ws-claude'],
  local: ['claude', 'ws-claude'],
  codex: ['codex', 'ws-codex'],
  opencode: ['opencode', 'claude', 'ws-opencode', 'ws-claude'],
  copilot: ['copilot', 'ws-copilot'],
  cursor: ['cursor', 'ws-cursor'],
  kiro: ['kiro', 'ws-kiro'],
  openclaw: ['openclaw', 'ws-openclaw'],
  hermes: ['hermes', 'ws-hermes'],
  pi: ['pi', 'ws-pi'],
};

// Scan paths shown in the UI for each provider.
const PROVIDER_PATHS: Record<string, { global: string[]; workspace: string[] }> = {
  claude:    { global: ['~/.claude/skills/'],              workspace: ['.claude/skills/'] },
  local:     { global: ['~/.claude/skills/'],              workspace: ['.claude/skills/'] },
  codex:     { global: ['$CODEX_HOME/skills/'],            workspace: ['.codex/skills/'] },
  opencode:  { global: ['~/.config/opencode/skills/', '~/.claude/skills/'], workspace: ['.opencode/skills/', '.claude/skills/'] },
  copilot:   { global: ['~/.copilot/skills/'],             workspace: ['.github/copilot/skills/'] },
  cursor:    { global: ['~/.cursor/skills/'],              workspace: ['.cursor/skills/'] },
  kiro:      { global: ['~/.kiro/skills/'],                workspace: ['.kiro/skills/'] },
  openclaw:  { global: ['~/.openclaw/skills/'],            workspace: ['skills/'] },
  hermes:    { global: ['~/.hermes/skills/'],              workspace: ['.hermes/skills/'] },
  pi:        { global: ['~/.pi/agent/skills/'],            workspace: ['.pi/skills/'] },
};

function visibleKinds(provider?: string): string[] {
  if (!provider) return [];
  return PROVIDER_KINDS[provider] ?? [];
}

function isWorkspace(kind: string) {
  return kind.startsWith('ws-');
}

const KIND_LABELS: Record<string, string> = {
  claude: 'Claude', codex: 'Codex', opencode: 'OpenCode',
  copilot: 'Copilot', cursor: 'Cursor', kiro: 'Kiro',
  openclaw: 'OpenClaw', hermes: 'Hermes', pi: 'Pi',
};

function kindLabel(kind: string): string {
  const base = kind.replace(/^ws-/, '');
  return KIND_LABELS[base] || base.toUpperCase();
}

function SkillList({ skills, emptyText }: { skills: SkillSummary[]; emptyText: string }) {
  const [drawerId, setDrawerId] = useState<string | null>(null);

  if (skills.length === 0) {
    return <p className="font-mono text-xs italic text-muted-foreground px-4 py-3">{emptyText}</p>;
  }

  return (
    <>
      <div className="divide-y-2 divide-black">
        {skills.map((s) => (
          <button
            key={s.id}
            type="button"
            onClick={() => setDrawerId(s.id)}
            className="flex w-full items-center gap-3 bg-white px-4 py-3 text-left hover:bg-gray-50"
          >
            <Puzzle className="h-4 w-4 flex-shrink-0 text-muted-foreground" />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="font-heading text-sm font-bold text-foreground">{s.name}</span>
                <span className="badge-brutal text-[10px] bg-brutal-cream text-foreground px-1.5">
                  {kindLabel(s.source_kind)}
                </span>
              </div>
              {s.description && (
                <p className="mt-0.5 font-mono text-[11px] text-muted-foreground leading-relaxed line-clamp-2">
                  {s.description}
                </p>
              )}
            </div>
          </button>
        ))}
      </div>
      <SkillDetailDrawer skillId={drawerId} onClose={() => setDrawerId(null)} />
    </>
  );
}

export function AgentSkillsTab({ agentId, agentProvider }: AgentSkillsTabProps) {
  const { skills: catalog, isLoading, error, refetch } = useSkills();
  const [resolvedProvider, setResolvedProvider] = useState(agentProvider);

  useEffect(() => {
    if (agentProvider) return;
    let cancelled = false;
    apiClient.get<{ model_provider?: string }>(`/api/v1/agents/${agentId}`).then((res) => {
      if (!cancelled && res.model_provider) {
        setResolvedProvider(res.model_provider);
      }
    }).catch(() => {});
    return () => { cancelled = true; };
  }, [agentId, agentProvider]);

  const kinds = useMemo(() => visibleKinds(resolvedProvider), [resolvedProvider]);

  const { globalSkills, workspaceSkills } = useMemo(() => {
    if (kinds.length === 0) {
      return { globalSkills: catalog, workspaceSkills: [] as SkillSummary[] };
    }
    const kindSet = new Set(kinds);
    const global: SkillSummary[] = [];
    const workspace: SkillSummary[] = [];
    for (const s of catalog) {
      if (!kindSet.has(s.source_kind)) continue;
      if (isWorkspace(s.source_kind)) {
        workspace.push(s);
      } else {
        global.push(s);
      }
    }
    return { globalSkills: global, workspaceSkills: workspace };
  }, [catalog, kinds]);

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
        <Button type="button" onClick={refetch} size="sm" className="mt-4">重试</Button>
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
            {globalSkills.length + workspaceSkills.length}
          </span>
          {resolvedProvider && (
            <span className="badge-brutal text-[10px] bg-brutal-primary text-white px-1.5">
              {resolvedProvider}
            </span>
          )}
        </div>
        <p className="mt-1 font-mono text-[11px] text-muted-foreground">
          由 Daemon 自动扫描，仅显示当前 Agent 可用的 Skill。
        </p>
        {resolvedProvider && PROVIDER_PATHS[resolvedProvider] && (
          <div className="mt-2 space-y-1 font-mono text-[10px] text-muted-foreground/80">
            <div>
              <span className="font-bold">全局:</span>{' '}
              {PROVIDER_PATHS[resolvedProvider].global.map((p, i) => (
                <span key={p}>
                  {i > 0 && ', '}
                  <code className="bg-brutal-cream px-1 border border-black/20">{p}</code>
                </span>
              ))}
            </div>
            <div>
              <span className="font-bold">Workspace:</span>{' '}
              {PROVIDER_PATHS[resolvedProvider].workspace.map((p, i) => (
                <span key={p}>
                  {i > 0 && ', '}
                  <code className="bg-brutal-cream px-1 border border-black/20">{p}</code>
                </span>
              ))}
            </div>
          </div>
        )}
      </div>

      {catalog.length === 0 ? (
        <div className="card-brutal bg-brutal-cream p-6 text-center">
          <p className="font-mono text-sm text-foreground">还没有发现任何 Skill</p>
          <p className="mt-2 font-mono text-[11px] text-muted-foreground">
            Daemon 启动后会在心跳时自动扫描并同步。
            请确保 Daemon 已重启并运行。
          </p>
          {resolvedProvider && PROVIDER_PATHS[resolvedProvider] && (
            <p className="mt-2 font-mono text-[10px] text-muted-foreground/70">
              扫描路径: {PROVIDER_PATHS[resolvedProvider].global.join(', ')}
            </p>
          )}
        </div>
      ) : (
        <>
          <div>
            <div className="flex items-center gap-2 mb-2">
              <Globe className="h-3.5 w-3.5 text-muted-foreground" />
              <h4 className="font-heading text-[11px] font-bold text-muted-foreground uppercase tracking-wider">
                Global
              </h4>
              <span className="font-mono text-[10px] text-muted-foreground/70">{globalSkills.length}</span>
            </div>
            <div className="border-2 border-black shadow-brutal-sm">
              <SkillList skills={globalSkills} emptyText="无全局 Skill" />
            </div>
          </div>

          <div>
            <div className="flex items-center gap-2 mb-2">
              <Folder className="h-3.5 w-3.5 text-muted-foreground" />
              <h4 className="font-heading text-[11px] font-bold text-muted-foreground uppercase tracking-wider">
                Workspace
              </h4>
              <span className="font-mono text-[10px] text-muted-foreground/70">{workspaceSkills.length}</span>
            </div>
            <div className="border-2 border-black shadow-brutal-sm">
              <SkillList skills={workspaceSkills} emptyText="无 Workspace Skill" />
            </div>
          </div>
        </>
      )}
    </div>
  );
}
