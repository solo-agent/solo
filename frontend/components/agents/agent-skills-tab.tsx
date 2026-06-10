// ============================================================================
// AgentSkillsTab — skill catalog filtered by agent's provider type.
// Reads skills from the agent detail endpoint (server-side filtering).
// ============================================================================

'use client';

import { useState, useEffect } from 'react';
import { AlertCircle, Puzzle, Globe, Folder } from 'lucide-react';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { apiClient, ApiError } from '@/lib/api-client';
import { SkillDetailDrawer } from './skill-detail-drawer';
import type { SkillSummary } from '@/lib/types';

interface AgentSkillsTabProps {
  agentId: string;
}

interface AgentSkillEmbed {
  id: string;
  name: string;
  description: string;
  source_kind: string;
}

function isWorkspace(kind: string) {
  return kind.startsWith('ws-');
}

const KIND_LABELS: Record<string, string> = {
  claude: 'Claude', codex: 'Codex', opencode: 'OpenCode',
  copilot: 'Copilot', cursor: 'Cursor', kiro: 'Kiro',
  openclaw: 'OpenClaw', hermes: 'Hermes', pi: 'Pi',
  agents: 'Agent',
};

function kindLabel(kind: string): string {
  const base = kind.replace(/^ws-/, '');
  return KIND_LABELS[base] || base.toUpperCase();
}

function SkillList({ skills, emptyText }: { skills: AgentSkillEmbed[]; emptyText: string }) {
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

export function AgentSkillsTab({ agentId }: AgentSkillsTabProps) {
  const [skills, setSkills] = useState<SkillSummary[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = async () => {
    setIsLoading(true);
    setError(null);
    try {
      // API returns Skills field embedded, plus source_kind for grouping
      const res = await apiClient.get<{ skills: SkillSummary[] }>(`/api/v1/agents/${agentId}`);
      setSkills(res.skills ?? []);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '加载 Skills 失败');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => { load(); }, [agentId]);

  const { globalSkills, workspaceSkills } = (() => {
    const g: SkillSummary[] = [];
    const w: SkillSummary[] = [];
    for (const s of skills) {
      if (isWorkspace(s.source_kind)) w.push(s); else g.push(s);
    }
    return { globalSkills: g, workspaceSkills: w };
  })();

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
        <Button type="button" onClick={load} size="sm" className="mt-4">重试</Button>
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
            {skills.length}
          </span>
        </div>
        <p className="mt-1 font-mono text-[11px] text-muted-foreground">
          仅显示当前 Agent 可用的 Skill（按 Provider 过滤）。
        </p>
      </div>

      {skills.length === 0 ? (
        <div className="card-brutal bg-brutal-cream p-6 text-center">
          <p className="font-mono text-sm text-foreground">还没有发现任何 Skill</p>
          <p className="mt-2 font-mono text-[11px] text-muted-foreground">
            Daemon 启动后会在心跳时自动扫描并同步。
          </p>
        </div>
      ) : (
        <>
          <div>
            <div className="flex items-center gap-2 mb-2">
              <Globe className="h-3.5 w-3.5 text-muted-foreground" />
              <h4 className="font-heading text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Global</h4>
              <span className="font-mono text-[10px] text-muted-foreground/70">{globalSkills.length}</span>
            </div>
            <div className="border-2 border-black shadow-brutal-sm">
              <SkillList skills={globalSkills} emptyText="无全局 Skill" />
            </div>
          </div>
          <div>
            <div className="flex items-center gap-2 mb-2">
              <Folder className="h-3.5 w-3.5 text-muted-foreground" />
              <h4 className="font-heading text-[11px] font-bold text-muted-foreground uppercase tracking-wider">Workspace</h4>
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
