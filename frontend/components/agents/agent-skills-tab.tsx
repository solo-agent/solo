// ============================================================================
// AgentSkillsTab — read-only skill catalog discovered from daemon filesystem scan.
// ============================================================================

'use client';

import { useState, useEffect } from 'react';
import { AlertCircle, Puzzle, Globe, Folder } from 'lucide-react';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { apiClient, ApiError } from '@/lib/api-client';
import { t } from '@/lib/i18n';
import type { SkillListItem, SkillListResponse } from '@/lib/types';

interface AgentSkillsTabProps {
  agentId: string;
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

function SkillList({ skills, emptyText }: { skills: SkillListItem[]; emptyText: string }) {
  if (skills.length === 0) {
    return <p className="font-mono text-xs italic text-muted-foreground px-4 py-3">{emptyText}</p>;
  }
  return (
    <div className="divide-y-2 divide-black">
      {skills.map((s) => (
        <div
          key={s.source_path}
          className="flex items-center gap-3 bg-white px-4 py-3"
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
        </div>
      ))}
    </div>
  );
}

export function AgentSkillsTab({ agentId }: AgentSkillsTabProps) {
  const [skills, setSkills] = useState<SkillListItem[]>([]);
  const [globalPaths, setGlobalPaths] = useState<string[]>([]);
  const [workspacePaths, setWorkspacePaths] = useState<string[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<SkillListResponse>(`/api/v1/agents/${agentId}/skills`);
      setSkills(res.skills ?? []);
      setGlobalPaths(res.global_paths ?? []);
      setWorkspacePaths(res.workspace_paths ?? []);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('agentSkillLoadError'));
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => { load(); }, [agentId]);

  const { globalSkills, workspaceSkills } = (() => {
    const g: SkillListItem[] = [];
    const w: SkillListItem[] = [];
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
        <Button type="button" onClick={load} size="sm" className="mt-4">{t('retry')}</Button>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Puzzle className="h-4 w-4" />
        <h3 className="font-heading text-xs font-bold text-muted-foreground uppercase tracking-wider">
          {t('agentSkillCatalog')}
        </h3>
        <span className="font-mono text-[10px] tabular-nums text-muted-foreground/70">
          {skills.length}
        </span>
      </div>

      {skills.length === 0 ? (
        <div className="card-brutal bg-brutal-cream p-6 text-center">
          <p className="font-mono text-sm text-foreground">{t('agentSkillEmpty')}</p>
          <p className="mt-2 font-mono text-[11px] text-muted-foreground">
            {t('agentSkillEmptyHint')}
          </p>
        </div>
      ) : (
        <>
          <div>
            <div className="flex items-center gap-2 mb-1">
              <Globe className="h-3.5 w-3.5 text-muted-foreground" />
              <h4 className="font-heading text-[11px] font-bold text-muted-foreground uppercase tracking-wider">
                {t('agentSkillGlobal')}
              </h4>
              <span className="font-mono text-[10px] text-muted-foreground/70">{globalSkills.length}</span>
            </div>
            <p className="mb-2 font-mono text-[10px] text-muted-foreground/60 truncate" title={globalPaths.join(', ')}>
              {globalPaths.join(', ')}
            </p>
            <div className="border-2 border-black shadow-brutal-sm">
              <SkillList skills={globalSkills} emptyText={t('agentSkillNoGlobal')} />
            </div>
          </div>
          <div>
            <div className="flex items-center gap-2 mb-1">
              <Folder className="h-3.5 w-3.5 text-muted-foreground" />
              <h4 className="font-heading text-[11px] font-bold text-muted-foreground uppercase tracking-wider">
                {t('agentSkillWorkspace')}
              </h4>
              <span className="font-mono text-[10px] text-muted-foreground/70">{workspaceSkills.length}</span>
            </div>
            <p className="mb-2 font-mono text-[10px] text-muted-foreground/60 truncate" title={workspacePaths.join(', ')}>
              {workspacePaths.join(', ')}
            </p>
            <div className="border-2 border-black shadow-brutal-sm">
              <SkillList skills={workspaceSkills} emptyText={t('agentSkillNoWorkspace')} />
            </div>
          </div>
        </>
      )}
    </div>
  );
}
